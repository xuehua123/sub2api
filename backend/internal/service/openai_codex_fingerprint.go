package service

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// codexFingerprintMode 控制 OAuth 账号出站请求的设备指纹收敛强度。
// 多人共享同一 OAuth 账号时，每个用户的 Codex 客户端会携带各自不同的
// installation_id / session_id / thread_id，上游据此判定设备数和会话数。
// 收敛模式将这些标识改写为账号级恒定值，减少上游可见的设备/会话指纹。
type codexFingerprintMode string

const (
	// codexFingerprintOff 不做任何收敛，原样透传客户端标识（默认行为）。
	codexFingerprintOff codexFingerprintMode = "off"
	// codexFingerprintDevice 仅收敛 installation_id 为账号级恒定值。
	// 上游看到 1 台设备 + 多会话（每用户各自的 session）。
	codexFingerprintDevice codexFingerprintMode = "device"
	// codexFingerprintSession 收敛 installation_id + session_id，
	// thread_id 按客户端原始 session-id 确定性派生（每个真实 Codex 会话一个独立线程）。
	// session_id 在账号级种子上再按 API Key 隔离：同一 Key 是稳定会话，
	// 共账号的不同用户不会撞上同一条上游 session。
	codexFingerprintSession codexFingerprintMode = "session"
	// codexFingerprintFull 收敛所有标识：installation_id + session_id + thread_id。
	// 上游看到 1 台设备 + 1 会话 + 1 线程，最激进。
	codexFingerprintFull codexFingerprintMode = "full"
)

const codexFingerprintModeExtraKey = "codex_fingerprint_mode"

// codexFingerprintIDsContextKey stores the current turn fingerprint set on
// the Gin context while building an OAuth request. The context is reused by
// the failover loop, so callers must clear and revalidate the value per
// account attempt.
const codexFingerprintIDsContextKey = "codex_fingerprint_ids"

// codexFingerprintConnectionIDsContextKey stores the connection-stable
// portion of a WebSocket fingerprint. It is separate from the current turn
// value so a long-lived connection can rotate turn_id without changing the
// installation/session/thread/window identity.
const codexFingerprintConnectionIDsContextKey = "codex_fingerprint_connection_ids"

// GetCodexFingerprintMode 从账号 extra JSON 读取指纹收敛模式。
// 未设置时默认 session（设备+会话收敛），显式设为 "off" 才关闭。
func (a *Account) GetCodexFingerprintMode() codexFingerprintMode {
	if a == nil || !a.IsOpenAIOAuth() {
		return codexFingerprintOff
	}
	raw := strings.TrimSpace(a.GetExtraString(codexFingerprintModeExtraKey))
	switch codexFingerprintMode(raw) {
	case codexFingerprintOff, codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull:
		return codexFingerprintMode(raw)
	default:
		return codexFingerprintSession
	}
}

// deriveStableUUIDv4 从种子确定性派生一个 UUIDv4 格式的字符串。
// 同一种子永远返回同一值。
func deriveStableUUIDv4(seed string) string {
	h := sha256.Sum256([]byte(seed))
	b := h[:16]
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16])
}

// resolveConvergedInstallationID 返回账号级恒定的 installation_id。
// 优先使用管理员配置的真实 device_id，无则从 accountID 确定性派生。
func resolveConvergedInstallationID(account *Account) string {
	if account == nil {
		return ""
	}
	if deviceID := account.GetOpenAIDeviceID(); deviceID != "" {
		return deviceID
	}
	return deriveStableUUIDv4(fmt.Sprintf("sub2api:codex-install-id:v1:%d", account.ID))
}

// resolveConvergedSessionID 返回账号级恒定的 session_id。
func resolveConvergedSessionID(account *Account) string {
	if account == nil {
		return ""
	}
	return deriveStableUUIDv4(fmt.Sprintf("sub2api:codex-session-id:v1:%d", account.ID))
}

// resolveSessionFingerprintSessionID 是 session 模式的出站 session_id。
// 上游默认把 session 收到账号级恒定值，fork 则用 isolateOpenAISessionID 按 API
// Key 隔离共账号用户。两者叠在一起时指纹后写会盖掉隔离；这里先收敛再隔离，
// 同一 Key 仍是稳定会话，不同 Key 不会撞上同一条上游 session。
func resolveSessionFingerprintSessionID(account *Account, apiKeyID int64) string {
	base := resolveConvergedSessionID(account)
	if base == "" {
		return ""
	}
	return isolateOpenAISessionID(apiKeyID, base)
}

// resolveConvergedThreadID 按客户端原始 session-id 确定性派生 thread_id。
// 每个真实 Codex 会话（不同客户端启动实例）获得一个独立线程，
// 模拟正常用户 spawn 子代理或开多窗口的模式。
func resolveConvergedThreadID(account *Account, clientSessionID string) string {
	if account == nil || clientSessionID == "" {
		return ""
	}
	return deriveStableUUIDv4(fmt.Sprintf("sub2api:codex-thread-id:v1:%d:%s", account.ID, clientSessionID))
}

// codexFingerprintIDs 收敛后的完整 ID 集合。
// 由 resolveCodexFingerprintIDs 一次性生成，同一个实例在头改写和体改写之间共享，
// 确保所有载体中的 turn_id 等随机字段一致。
type codexFingerprintIDs struct {
	accountID      int64
	apiKeyID       int64
	mode           codexFingerprintMode
	installationID string
	sessionID      string
	threadID       string
	turnID         string
	windowID       string
}

// codexFingerprintIDsBelongToAccount prevents a fingerprint set generated for
// a previous failover attempt from being applied to another OAuth account.
func codexFingerprintIDsBelongToAccount(ids *codexFingerprintIDs, account *Account) bool {
	return ids != nil && account != nil && ids.accountID == account.ID
}

// codexFingerprintIDsBelongToAttempt also requires the same API key. Session
// mode derives session_id from account+key isolation; reusing another key's
// set would reintroduce the cross-user collision the fork guard prevents.
func codexFingerprintIDsBelongToAttempt(ids *codexFingerprintIDs, account *Account, apiKeyID int64) bool {
	return codexFingerprintIDsBelongToAccount(ids, account) && ids.apiKeyID == apiKeyID
}

// newCodexFingerprintTurnIDs clones the connection identity and creates a new
// turn identity for the next Responses request. Device mode intentionally has
// no turn metadata, so its clone remains equivalent to the base value.
func newCodexFingerprintTurnIDs(base *codexFingerprintIDs) *codexFingerprintIDs {
	if base == nil {
		return nil
	}
	next := *base
	if base.mode != codexFingerprintDevice && base.mode != codexFingerprintOff {
		next.turnID = uuid.Must(uuid.NewV7()).String()
	}
	return &next
}

// ensureCodexFingerprintAttemptIDs returns the current account-scoped set,
// deriving it from the inbound headers only when the request has not already
// initialized one. This keeps HTTP headers and body, as well as WS handshake
// and first frame, on the same turn identity.
func ensureCodexFingerprintAttemptIDs(c *gin.Context, account *Account) *codexFingerprintIDs {
	if c == nil || account == nil || !account.IsOpenAIOAuth() {
		if c != nil {
			c.Set(codexFingerprintIDsContextKey, nil)
			c.Set(codexFingerprintConnectionIDsContextKey, nil)
		}
		return nil
	}
	apiKeyID := getAPIKeyIDFromContext(c)
	if currentValue, ok := c.Get(codexFingerprintIDsContextKey); ok {
		if current, ok := currentValue.(*codexFingerprintIDs); ok && codexFingerprintIDsBelongToAttempt(current, account, apiKeyID) {
			return current
		}
	}
	if baseValue, ok := c.Get(codexFingerprintConnectionIDsContextKey); ok {
		if base, ok := baseValue.(*codexFingerprintIDs); ok && codexFingerprintIDsBelongToAttempt(base, account, apiKeyID) {
			current := newCodexFingerprintTurnIDs(base)
			c.Set(codexFingerprintIDsContextKey, current)
			return current
		}
	}
	var clientHeaders http.Header
	if c.Request != nil {
		clientHeaders = c.Request.Header
	}
	base := resolveCodexFingerprintIDsFromRequestWithAPIKey(account, clientHeaders, apiKeyID)
	c.Set(codexFingerprintConnectionIDsContextKey, base)
	c.Set(codexFingerprintIDsContextKey, base)
	return base
}

// advanceCodexFingerprintTurn moves a WebSocket request to the next turn while
// preserving the connection-level identity. The account ownership check also
// prevents a failover attempt from inheriting the prior account's IDs.
func advanceCodexFingerprintTurn(c *gin.Context, account *Account) *codexFingerprintIDs {
	if c == nil || account == nil || !account.IsOpenAIOAuth() {
		return ensureCodexFingerprintAttemptIDs(c, account)
	}
	baseValue, _ := c.Get(codexFingerprintConnectionIDsContextKey)
	base, _ := baseValue.(*codexFingerprintIDs)
	if !codexFingerprintIDsBelongToAttempt(base, account, getAPIKeyIDFromContext(c)) {
		base = ensureCodexFingerprintAttemptIDs(c, account)
		if base == nil {
			return nil
		}
		// ensure may have installed a current value; use its stable fields as the
		// base and rotate only for this explicitly requested next turn.
		base = cloneCodexFingerprintConnectionIDs(base)
		c.Set(codexFingerprintConnectionIDsContextKey, base)
	}
	next := newCodexFingerprintTurnIDs(base)
	c.Set(codexFingerprintIDsContextKey, next)
	return next
}

func cloneCodexFingerprintConnectionIDs(ids *codexFingerprintIDs) *codexFingerprintIDs {
	if ids == nil {
		return nil
	}
	clone := *ids
	return &clone
}

// applyCodexFingerprintJSONBody applies an attempt-local fingerprint set to a
// Responses request body. It deliberately leaves the original bytes untouched
// when fingerprinting is disabled so off-mode requests remain byte-for-byte
// compatible with the previous forwarding path.
func applyCodexFingerprintJSONBody(body []byte, ids *codexFingerprintIDs) ([]byte, bool, error) {
	if ids == nil || len(body) == 0 {
		return body, false, nil
	}
	var requestBody map[string]any
	if err := json.Unmarshal(body, &requestBody); err != nil {
		return nil, false, err
	}
	if !applyCodexFingerprintClientMetadata(requestBody, ids) {
		return body, false, nil
	}
	rebuilt, err := json.Marshal(requestBody)
	if err != nil {
		return nil, false, err
	}
	return rebuilt, true, nil
}

// resolveCodexFingerprintIDs 按收敛模式计算出站 ID 集合。
// clientSessionID 是客户端原始的 session-id 头值（连字符形式），用于 session 模式下
// 的 thread_id 派生——每个真实 Codex 会话得到一个独立线程。
// 返回 nil 表示 off 模式，不需要改写。
// 注意：包含随机生成的 turn_id，调用方必须只调用一次并共享结果给头改写和体改写。
func resolveCodexFingerprintIDs(account *Account, clientSessionID string, mode codexFingerprintMode, apiKeyID int64) *codexFingerprintIDs {
	if account == nil || mode == codexFingerprintOff {
		return nil
	}

	ids := &codexFingerprintIDs{accountID: account.ID, apiKeyID: apiKeyID, mode: mode}

	ids.installationID = resolveConvergedInstallationID(account)
	if ids.installationID == "" {
		return nil
	}

	switch mode {
	case codexFingerprintDevice:
		return ids

	case codexFingerprintSession:
		ids.sessionID = resolveSessionFingerprintSessionID(account, apiKeyID)
		ids.threadID = resolveConvergedThreadID(account, clientSessionID)
		if ids.threadID == "" {
			ids.threadID = ids.sessionID
		}
		ids.turnID = uuid.Must(uuid.NewV7()).String()
		ids.windowID = ids.threadID + ":0"
		return ids

	case codexFingerprintFull:
		// full 是管理员显式选择的最激进收敛：1 设备 + 1 会话 + 1 线程，
		// 不再按 API Key 隔离。
		ids.sessionID = resolveConvergedSessionID(account)
		ids.threadID = ids.sessionID
		ids.turnID = uuid.Must(uuid.NewV7()).String()
		ids.windowID = ids.threadID + ":0"
		return ids
	}

	return nil
}

// extractClientSessionID 从请求头中提取客户端原始的会话标识。
// 优先取 session-id（连字符形式，Codex CLI 标准），回退到 session_id（下划线形式）。
// 返回的值尚未被 isolateOpenAISessionID 改写，是客户端的真实标识。
func extractClientSessionID(h http.Header) string {
	if v := strings.TrimSpace(h.Get("session-id")); v != "" {
		return v
	}
	return strings.TrimSpace(h.Get("session_id"))
}

// resolveCodexFingerprintIDsFromRequest 从客户端原始请求头中提取 session-id，
// 结合账号配置一次性解析收敛 ID 集合。调用方应将返回的 ids 同时传给
// applyCodexFingerprintHeaders 和 applyCodexFingerprintClientMetadata。
func resolveCodexFingerprintIDsFromRequest(account *Account, clientHeaders http.Header) *codexFingerprintIDs {
	return resolveCodexFingerprintIDsFromRequestWithAPIKey(account, clientHeaders, 0)
}

func resolveCodexFingerprintIDsFromRequestWithAPIKey(account *Account, clientHeaders http.Header, apiKeyID int64) *codexFingerprintIDs {
	if account == nil {
		return nil
	}
	mode := account.GetCodexFingerprintMode()
	if mode == codexFingerprintOff {
		return nil
	}
	clientSessionID := ""
	if clientHeaders != nil {
		clientSessionID = extractClientSessionID(clientHeaders)
	}
	return resolveCodexFingerprintIDs(account, clientSessionID, mode, apiKeyID)
}

// applyCodexFingerprintHeaders 按预计算的收敛 ID 改写出站 HTTP 头中的设备指纹。
// 在 buildUpstreamRequest 的白名单透传之后、enforceCodexIdentityHeaders 之前调用。
func applyCodexFingerprintHeaders(h http.Header, ids *codexFingerprintIDs) {
	if h == nil || ids == nil {
		return
	}

	// 所有非 off 模式都收敛 installation_id
	h.Set("x-codex-installation-id", ids.installationID)

	if ids.mode == codexFingerprintDevice {
		rewriteCodexTurnMetadataFields(h, map[string]any{
			"installation_id": ids.installationID,
		})
		return
	}

	// session / full 模式：改写所有相关头
	h.Set("x-codex-window-id", ids.windowID)
	h.Set("x-client-request-id", ids.threadID)
	// 连字符形式和下划线形式都改写，保证一致
	h.Set("session-id", ids.sessionID)
	h.Set("session_id", ids.sessionID)
	h.Set("thread-id", ids.threadID)

	rewriteCodexTurnMetadataFields(h, map[string]any{
		"installation_id":         ids.installationID,
		"session_id":              ids.sessionID,
		"thread_id":               ids.threadID,
		"turn_id":                 ids.turnID,
		"window_id":               ids.windowID,
		"turn_started_at_unix_ms": time.Now().UnixMilli(),
	})
}

// rewriteCodexTurnMetadataFields 解析 x-codex-turn-metadata 头中的 JSON，
// 替换指定字段后回写。保留未指定字段原样（如 sandbox、thread_source 等）。
func rewriteCodexTurnMetadataFields(h http.Header, fields map[string]any) {
	raw := strings.TrimSpace(h.Get("x-codex-turn-metadata"))
	if raw == "" {
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return
	}
	if metadata == nil {
		metadata = make(map[string]any)
	}
	for k, v := range fields {
		metadata[k] = v
	}
	rebuilt, err := json.Marshal(metadata)
	if err != nil {
		return
	}
	h.Set("x-codex-turn-metadata", string(rebuilt))
}

// applyCodexFingerprintClientMetadata 按预计算的收敛 ID 改写请求体中的 client_metadata。
// 使用与头改写相同的 ids 实例，确保 turn_id 等随机字段一致。
func applyCodexFingerprintClientMetadata(reqBody map[string]any, ids *codexFingerprintIDs) bool {
	if reqBody == nil || ids == nil {
		return false
	}

	var existing map[string]any
	switch raw := reqBody["client_metadata"].(type) {
	case map[string]any:
		existing = raw
	case map[string]string:
		existing = make(map[string]any, len(raw))
		for key, value := range raw {
			existing[key] = value
		}
	default:
		existing = make(map[string]any)
	}

	modified := false

	if ids.installationID != "" {
		existing["x-codex-installation-id"] = ids.installationID
		modified = true
	}

	if ids.mode == codexFingerprintDevice {
		rewriteClientMetadataEmbeddedTurnMetadata(existing, map[string]any{
			"installation_id": ids.installationID,
		})
		if modified {
			reqBody["client_metadata"] = existing
		}
		return modified
	}

	// session / full 模式
	existing["session_id"] = ids.sessionID
	existing["thread_id"] = ids.threadID
	existing["turn_id"] = ids.turnID
	existing["x-codex-window-id"] = ids.windowID

	rewriteClientMetadataEmbeddedTurnMetadata(existing, map[string]any{
		"installation_id":         ids.installationID,
		"session_id":              ids.sessionID,
		"thread_id":               ids.threadID,
		"turn_id":                 ids.turnID,
		"window_id":               ids.windowID,
		"turn_started_at_unix_ms": time.Now().UnixMilli(),
	})

	reqBody["client_metadata"] = existing
	return true
}

// rewriteClientMetadataEmbeddedTurnMetadata 改写 client_metadata 中内嵌的
// x-codex-turn-metadata JSON 字符串里的指定字段。
func rewriteClientMetadataEmbeddedTurnMetadata(clientMetadata map[string]any, fields map[string]any) {
	raw, ok := clientMetadata["x-codex-turn-metadata"].(string)
	if !ok || raw == "" {
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return
	}
	if metadata == nil {
		metadata = make(map[string]any)
	}
	for k, v := range fields {
		metadata[k] = v
	}
	if rebuilt, err := json.Marshal(metadata); err == nil {
		clientMetadata["x-codex-turn-metadata"] = string(rebuilt)
	}
}
