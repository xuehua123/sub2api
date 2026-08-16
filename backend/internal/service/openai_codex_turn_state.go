package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// openAICodexTurnStateHeader 是 Codex 的回合状态头。上游在响应头中铸造该
// 不透明 blob，客户端在同一回合的后续请求中原样回带（codex-rs 侧从
// /responses SSE、/responses/compact JSON 与 WS 握手三种响应中捕获，见
// codex-api/src/sse/responses.rs 与 endpoint/compact.rs）。
const openAICodexTurnStateHeader = "x-codex-turn-state"

// turn-state blob 是上游在"出站身份"（含 #5553 指纹收敛改写后的
// installation/session/thread 标识）下铸造的，同账号回放自洽；跨账号回放
// （failover 换号后客户端仍回带旧账号的 blob）是代理链独有、真实 Codex
// 永远不会产生的矛盾信号。
//
// 溯源键包含下游 API Key、客户端会话和 blob 的 SHA-256。map 中绝不保存
// 原始 blob：一是它本身是不透明的上游凭据，二是必须逐 blob 校验，不能让
// 账号 B 最近铸造的新 blob 覆盖账号 A 的旧 blob 的归属记录。
type openAICodexTurnStateProvenanceKey struct {
	apiKeyID    int64
	sessionHash [sha256.Size]byte
	stateHash   [sha256.Size]byte
}

func (k openAICodexTurnStateProvenanceKey) String() string {
	return fmt.Sprintf("%d:%x:%x", k.apiKeyID, k.sessionHash, k.stateHash)
}

type openAICodexTurnStateOrigin struct {
	accountID int64
	expiresAt time.Time
}

// CodexTurnStateOriginStore provides shared L2 verification across slots and
// server instances for x-codex-turn-state origins.
type CodexTurnStateOriginStore interface {
	SetTurnStateOrigin(ctx context.Context, key string, accountID int64, ttl time.Duration) error
	GetTurnStateOrigin(ctx context.Context, key string) (int64, error)
}

func (s *OpenAIGatewayService) getCodexTurnStateStore() CodexTurnStateOriginStore {
	if s == nil {
		return nil
	}
	if store, ok := s.cache.(CodexTurnStateOriginStore); ok {
		return store
	}
	return nil
}

// openAICodexTurnStateProvenanceKeyFor 生成不含原文的归属键。客户端会话
// 标识取自请求头（与指纹收敛的 thread 派生同源，见 extractClientSessionID）。
// 缺少 API Key、会话或 blob 时无法可靠归属；出站守卫会对此 fail-closed。
func openAICodexTurnStateProvenanceKeyFor(c *gin.Context, state string) (openAICodexTurnStateProvenanceKey, bool) {
	if c == nil || c.Request == nil {
		return openAICodexTurnStateProvenanceKey{}, false
	}
	apiKeyID := getAPIKeyIDFromContext(c)
	if apiKeyID <= 0 {
		return openAICodexTurnStateProvenanceKey{}, false
	}
	sessionID := extractClientSessionID(c.Request.Header)
	if sessionID == "" {
		return openAICodexTurnStateProvenanceKey{}, false
	}
	state = strings.TrimSpace(state)
	if state == "" {
		return openAICodexTurnStateProvenanceKey{}, false
	}
	return openAICodexTurnStateProvenanceKey{
		apiKeyID:    apiKeyID,
		sessionHash: sha256.Sum256([]byte(sessionID)),
		stateHash:   sha256.Sum256([]byte(state)),
	}, true
}

// relayOpenAICodexTurnState 将上游响应中的 turn-state 显式写入下游响应头，
// 返回实际待提交的值；调用方必须在后续成功提交响应后才记录其归属。若响应头
// 已经提交（例如 compact keepalive），此时再设置 header 不会到达客户端，因而
// 返回空值并绝不创建溯源记录。
func (s *OpenAIGatewayService) relayOpenAICodexTurnState(c *gin.Context, upstream http.Header) string {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return ""
	}
	canonical := http.CanonicalHeaderKey(openAICodexTurnStateHeader)
	state := extractOpenAICodexTurnState(upstream)
	if state == "" {
		c.Writer.Header().Del(canonical)
		return ""
	}
	c.Writer.Header().Set(canonical, state)
	return state
}

// stageOpenAICodexTurnState 将上游 turn-state 暂存到延迟提交的响应头集合
// （首输出守卫路径先缓存头、见到首个输出事件才提交）。此处**不**记录铸造
// 账号：该 attempt 仍可能在首输出超时后 failover，暂存头会被整体丢弃，
// 客户端从未收到该 blob。溯源必须在真正提交时记录，见
// noteStagedOpenAICodexTurnStateCommitted。
func stageOpenAICodexTurnState(dst *http.Header, upstream http.Header) {
	if dst == nil {
		return
	}
	canonical := http.CanonicalHeaderKey(openAICodexTurnStateHeader)
	state := extractOpenAICodexTurnState(upstream)
	if state == "" {
		if *dst != nil {
			dst.Del(canonical)
		}
		return
	}
	if *dst == nil {
		*dst = http.Header{}
	}
	dst.Set(canonical, state)
}

// noteStagedOpenAICodexTurnStateCommitted 在暂存响应头已经随下游输出真正
// 提交时记录铸造账号。调用方须在 Write/Flush 成功之后调用；本函数额外检查
// Writer.Written，避免被 failover 丢弃的 attempt 污染溯源。
func (s *OpenAIGatewayService) noteStagedOpenAICodexTurnStateCommitted(c *gin.Context, account *Account, staged http.Header) {
	if staged == nil {
		return
	}
	s.noteOpenAICodexTurnStateCommitted(c, account, staged.Get(openAICodexTurnStateHeader))
}

func extractOpenAICodexTurnState(upstream http.Header) string {
	if upstream == nil {
		return ""
	}
	return strings.TrimSpace(upstream.Get(openAICodexTurnStateHeader))
}

// noteOpenAICodexTurnStateCommitted 仅在响应已经提交到下游时记录
// （API Key, session, SHA-256(blob) → account）的归属。它不保存 blob 原文。
// 返回值可让流式写入路径避免每次 Flush 重复写入同一条记录。
func (s *OpenAIGatewayService) noteOpenAICodexTurnStateCommitted(c *gin.Context, account *Account, state string) bool {
	if s == nil || c == nil || c.Writer == nil || !c.Writer.Written() || account == nil || account.ID <= 0 {
		return false
	}
	state = strings.TrimSpace(state)
	if state == "" || strings.TrimSpace(c.Writer.Header().Get(openAICodexTurnStateHeader)) != state {
		return false
	}
	return s.noteOpenAICodexTurnStateOrigin(c, account, state)
}

// noteOpenAICodexTurnStateOrigin records a state that has become usable by a
// WebSocket session. HTTP callers must use noteOpenAICodexTurnStateCommitted,
// which additionally proves the response header was actually written. A WS
// upstream handshake has no later HTTP response header to observe, so its
// caller invokes this only after the corresponding downstream WS turn has
// completed successfully.
func (s *OpenAIGatewayService) noteOpenAICodexTurnStateOrigin(c *gin.Context, account *Account, state string) bool {
	if s == nil || account == nil || account.ID <= 0 {
		return false
	}
	state = strings.TrimSpace(state)
	if state == "" {
		return false
	}
	key, ok := openAICodexTurnStateProvenanceKeyFor(c, state)
	if !ok {
		return false
	}
	ttl := s.openAIWSSessionStickyTTL()
	s.openaiCodexTurnStateOrigins.Store(key, openAICodexTurnStateOrigin{
		accountID: account.ID,
		expiresAt: time.Now().Add(ttl),
	})
	s.sweepOpenAICodexTurnStateOrigins()

	if store := s.getCodexTurnStateStore(); store != nil {
		go func(st CodexTurnStateOriginStore, keyStr string, accID int64, t time.Duration) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = st.SetTurnStateOrigin(ctx, keyStr, accID, t)
		}(store, key.String(), account.ID, ttl)
	}
	return true
}

// guardedOpenAIWSTurnState applies the same fail-closed provenance gate to
// WebSocket handshake state as HTTP requests. The state-store is only a
// performance cache; it is never an authority for a blob's account owner.
func (s *OpenAIGatewayService) guardedOpenAIWSTurnState(c *gin.Context, account *Account, state string) string {
	state = strings.TrimSpace(state)
	if state == "" {
		return ""
	}
	headers := http.Header{}
	headers.Set(openAICodexTurnStateHeader, state)
	s.guardOpenAICodexTurnStateEcho(c, account, headers)
	return extractOpenAICodexTurnState(headers)
}

func openAIWSTurnStateScopeIDs(c *gin.Context, account *Account) (int64, int64, bool) {
	if account == nil || account.ID <= 0 {
		return 0, 0, false
	}
	apiKeyID := getAPIKeyIDFromContext(c)
	if apiKeyID <= 0 {
		return 0, 0, false
	}
	return apiKeyID, account.ID, true
}

// loadOpenAIWSSessionTurnState reads an account/API-key-scoped cache entry and
// then independently verifies the exact blob against in-memory provenance.
// A cache hit from another slot/restart is deliberately treated as unknown.
func (s *OpenAIGatewayService) loadOpenAIWSSessionTurnState(
	c *gin.Context,
	account *Account,
	stateStore OpenAIWSStateStore,
	groupID int64,
	sessionHash string,
) string {
	if stateStore == nil || strings.TrimSpace(sessionHash) == "" {
		return ""
	}
	apiKeyID, accountID, ok := openAIWSTurnStateScopeIDs(c, account)
	if !ok {
		return ""
	}
	state, found := stateStore.GetSessionTurnState(groupID, apiKeyID, accountID, sessionHash)
	if !found {
		return ""
	}
	guarded := s.guardedOpenAIWSTurnState(c, account, state)
	if guarded == "" {
		// The cache is not trusted authority. Drop values whose provenance is
		// absent, expired, or no longer matches this exact owner.
		stateStore.DeleteSessionTurnState(groupID, apiKeyID, accountID, sessionHash)
	}
	return guarded
}

// commitOpenAIWSSessionTurnState records a WS handshake/bridge state only
// after the corresponding downstream turn has completed, then stores it in a
// cache scoped to both the API key and account. The cache never replaces the
// exact-blob provenance check performed on read.
func (s *OpenAIGatewayService) commitOpenAIWSSessionTurnState(
	c *gin.Context,
	account *Account,
	stateStore OpenAIWSStateStore,
	groupID int64,
	sessionHash string,
	state string,
) bool {
	if !s.noteOpenAICodexTurnStateOrigin(c, account, state) {
		return false
	}
	if stateStore == nil || strings.TrimSpace(sessionHash) == "" {
		return true
	}
	apiKeyID, accountID, ok := openAIWSTurnStateScopeIDs(c, account)
	if !ok {
		return true
	}
	stateStore.BindSessionTurnState(groupID, apiKeyID, accountID, sessionHash, state, s.openAIWSSessionStickyTTL())
	return true
}

// guardOpenAICodexTurnStateEcho 出站守卫：只允许已知、未过期且归属于本次
// API Key/session/账号的 blob 出站。新槽、重启或缓存淘汰后没有可信溯源时
// 必须 fail-closed；否则攻击者可把其他账号铸造的 blob 注入任意账号。
// 只剥离、不注入——/responses 路径的客户端会按自身回合语义自行回带；服务端
// 注入是 Claude 兼容桥（无法回带的客户端）的专属行为。
func (s *OpenAIGatewayService) guardOpenAICodexTurnStateEcho(c *gin.Context, account *Account, h http.Header) {
	if h == nil {
		return
	}
	state := strings.TrimSpace(h.Get(openAICodexTurnStateHeader))
	if state == "" {
		return
	}
	if s == nil || account == nil || account.ID <= 0 {
		h.Del(openAICodexTurnStateHeader)
		return
	}
	key, ok := openAICodexTurnStateProvenanceKeyFor(c, state)
	if !ok {
		h.Del(openAICodexTurnStateHeader)
		return
	}

	// 1. Check L1 in-memory map first (zero latency)
	if raw, ok := s.openaiCodexTurnStateOrigins.Load(key); ok {
		origin, ok := raw.(openAICodexTurnStateOrigin)
		if ok && (origin.expiresAt.IsZero() || time.Now().Before(origin.expiresAt)) {
			if origin.accountID != account.ID {
				h.Del(openAICodexTurnStateHeader)
			}
			return
		}
		s.openaiCodexTurnStateOrigins.Delete(key)
	}

	// 2. Check L2 Redis origin store (e.g. across blue-green slot switch / restart)
	if store := s.getCodexTurnStateStore(); store != nil {
		reqCtx := context.Background()
		if c != nil && c.Request != nil && c.Request.Context() != nil {
			reqCtx = c.Request.Context()
		}
		checkCtx, cancel := context.WithTimeout(reqCtx, 1500*time.Millisecond)
		defer cancel()
		accountID, err := store.GetTurnStateOrigin(checkCtx, key.String())
		if err == nil && accountID > 0 {
			if accountID == account.ID {
				// Warm up L1 memory cache for subsequent turns
				s.openaiCodexTurnStateOrigins.Store(key, openAICodexTurnStateOrigin{
					accountID: account.ID,
					expiresAt: time.Now().Add(s.openAIWSSessionStickyTTL()),
				})
				return
			}
			// Account mismatch in shared storage: strip header
			h.Del(openAICodexTurnStateHeader)
			return
		}
	}

	// 3. Fail-closed if origin cannot be verified
	h.Del(openAICodexTurnStateHeader)
}

// sweepOpenAICodexTurnStateOrigins 机会式清扫过期溯源记录：每 256 次写入
// 全量遍历一轮，防止仅靠读侧惰性删除导致的慢泄漏（会话键无上界）。
func (s *OpenAIGatewayService) sweepOpenAICodexTurnStateOrigins() {
	if s.openaiCodexTurnStateWrites.Add(1)%256 != 0 {
		return
	}
	now := time.Now()
	s.openaiCodexTurnStateOrigins.Range(func(key, value any) bool {
		origin, ok := value.(openAICodexTurnStateOrigin)
		if !ok || (!origin.expiresAt.IsZero() && now.After(origin.expiresAt)) {
			s.openaiCodexTurnStateOrigins.Delete(key)
		}
		return true
	})
}
