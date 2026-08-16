package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
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
// 溯源键包含下游 group/API Key、客户端会话和 blob 的 SHA-256。map 中绝不
// 保存原始 blob：一是它本身是不透明的上游凭据，二是必须逐 blob 校验，不能让
// 账号 B 最近铸造的新 blob 覆盖账号 A 的旧 blob 的归属记录。
type openAICodexTurnStateProvenanceKey struct {
	groupID     int64
	apiKeyID    int64
	sessionHash [sha256.Size]byte
	stateHash   [sha256.Size]byte
}

func (k openAICodexTurnStateProvenanceKey) String() string {
	return fmt.Sprintf("%d:%d:%x:%x", k.groupID, k.apiKeyID, k.sessionHash, k.stateHash)
}

type openAICodexTurnStateOrigin struct {
	accountID int64
	expiresAt time.Time
}

const (
	// Shared state is written/read on bounded terminal and recovery paths.
	// A complete path (raw state plus provenance) must share this single budget,
	// rather than giving each Redis operation a fresh timeout.
	openAICodexTurnStateStoreTimeout         = 500 * time.Millisecond
	openAICodexTurnStateStoreFailureLogEvery = 30 * time.Second
)

func newOpenAICodexTurnStateStoreContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, openAICodexTurnStateStoreTimeout)
}

func openAICodexTurnStateRequestContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil && c.Request.Context() != nil {
		return c.Request.Context()
	}
	return context.Background()
}

// CodexTurnStateOriginStore provides shared L2 verification across slots and
// server instances for x-codex-turn-state origins. It never exposes an opaque
// turn-state blob itself.
type CodexTurnStateOriginStore interface {
	SetTurnStateOrigin(ctx context.Context, key string, accountID int64, ttl time.Duration) error
	GetTurnStateOrigin(ctx context.Context, key string) (accountID int64, ttl time.Duration, found bool, err error)
}

// CodexTurnStateSessionStore is deliberately separate from origin lookup: it
// contains an encrypted raw handshake state for WS clients that never receive
// the upstream handshake header. Implementations must enforce a TTL and bind
// the ciphertext to the supplied scope.
type CodexTurnStateSessionStore interface {
	SetSessionTurnState(ctx context.Context, key string, state string, ttl time.Duration) error
	GetSessionTurnState(ctx context.Context, key string) (state string, ttl time.Duration, found bool, err error)
	DeleteSessionTurnState(ctx context.Context, key string) error
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

func (s *OpenAIGatewayService) reportOpenAICodexTurnStateStoreFailure(operation string, groupID, apiKeyID, accountID int64, err error) {
	if s == nil || err == nil {
		return
	}
	failureCount := s.openaiCodexTurnStateStoreFailures.Add(1)
	now := time.Now().UnixNano()
	last := s.openaiCodexTurnStateStoreLastFailureLog.Load()
	if last != 0 && now-last < openAICodexTurnStateStoreFailureLogEvery.Nanoseconds() {
		return
	}
	if !s.openaiCodexTurnStateStoreLastFailureLog.CompareAndSwap(last, now) {
		return
	}
	// Do not include the scope key, encrypted value, or raw upstream state in
	// logs. The operation and concrete error type retain enough signal for
	// alerting without turning a credential cache into a logging side-channel.
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[Codex turn-state] shared_store_failed operation=%s group_id=%d api_key_id=%d account_id=%d failure_count=%d error_type=%T",
		strings.TrimSpace(operation),
		groupID,
		apiKeyID,
		accountID,
		failureCount,
		err,
	)
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
		groupID:     getOpenAIGroupIDFromContext(c),
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
	return s.noteOpenAICodexTurnStateOriginWithContext(context.Background(), c, account, state)
}

func (s *OpenAIGatewayService) noteOpenAICodexTurnStateOriginWithContext(storeParentCtx context.Context, c *gin.Context, account *Account, state string) bool {
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
		writeCtx, cancel := newOpenAICodexTurnStateStoreContext(storeParentCtx)
		defer cancel()
		if err := store.SetTurnStateOrigin(writeCtx, key.String(), account.ID, ttl); err != nil {
			s.reportOpenAICodexTurnStateStoreFailure("origin_write", key.groupID, key.apiKeyID, account.ID, err)
		}
	}
	return true
}

// guardedOpenAIWSTurnState applies the same fail-closed provenance gate to
// WebSocket handshake state as HTTP requests. The state-store is only a
// performance cache; it is never an authority for a blob's account owner.
func (s *OpenAIGatewayService) guardedOpenAIWSTurnState(c *gin.Context, account *Account, state string) string {
	return s.guardedOpenAIWSTurnStateWithContext(openAICodexTurnStateRequestContext(c), c, account, state)
}

func (s *OpenAIGatewayService) guardedOpenAIWSTurnStateWithContext(storeParentCtx context.Context, c *gin.Context, account *Account, state string) string {
	state = strings.TrimSpace(state)
	if state == "" {
		return ""
	}
	headers := http.Header{}
	headers.Set(openAICodexTurnStateHeader, state)
	s.guardOpenAICodexTurnStateEchoWithContext(storeParentCtx, c, account, headers)
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

// loadOpenAIWSSessionTurnState reads a group/API-key/account/session-scoped
// cache entry and then independently verifies the exact blob against L1 or its
// short-lived shared provenance record. A cache hit without either provenance
// source remains unknown and is deleted fail-closed.
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
	storeCtx, cancel := newOpenAICodexTurnStateStoreContext(openAICodexTurnStateRequestContext(c))
	defer cancel()

	stateScopeHash := strings.TrimSpace(sessionHash)
	state, found := getOpenAIWSSessionTurnStateWithContext(storeCtx, stateStore, groupID, apiKeyID, accountID, stateScopeHash)
	if !found && s.openAISessionHashReadOldFallbackEnabled() {
		// A v0.1.177 slot keyed raw WS state by the old scheduler hash because
		// it did not honor the hyphenated Codex session-id header.  Probe only
		// the one deterministic old hash captured from this same request.  The
		// lookup retains group/API-key/account scope and never scans Redis keys.
		preCanonicalHash := openAIPreCanonicalSessionHashFromContext(storeCtx, stateScopeHash)
		if preCanonicalHash == "" {
			return ""
		}
		state, found = getOpenAIWSSessionTurnStateWithContext(storeCtx, stateStore, groupID, apiKeyID, accountID, preCanonicalHash)
		if !found {
			return ""
		}
		stateScopeHash = preCanonicalHash
	}
	guarded := s.guardedOpenAIWSTurnStateWithContext(storeCtx, c, account, state)
	if guarded == "" {
		// The cache is not trusted authority. Drop values whose provenance is
		// absent, expired, or no longer matches this exact owner.
		deleteOpenAIWSSessionTurnStateWithContext(storeCtx, stateStore, groupID, apiKeyID, accountID, stateScopeHash)
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
	// Provenance and raw handshake state are written after delivery. They must
	// share one deadline so a degraded Redis cannot add one timeout per write.
	storeCtx, cancel := newOpenAICodexTurnStateStoreContext(context.Background())
	defer cancel()
	if !s.noteOpenAICodexTurnStateOriginWithContext(storeCtx, c, account, state) {
		return false
	}
	if stateStore == nil || strings.TrimSpace(sessionHash) == "" {
		return true
	}
	apiKeyID, accountID, ok := openAIWSTurnStateScopeIDs(c, account)
	if !ok {
		return true
	}
	bindOpenAIWSSessionTurnStateWithContext(storeCtx, stateStore, groupID, apiKeyID, accountID, sessionHash, state, s.openAIWSSessionStickyTTL())
	return true
}

// guardOpenAICodexTurnStateEcho 出站守卫：只允许已知、未过期且归属于本次
// API Key/session/账号的 blob 出站。新槽、重启或缓存淘汰后没有可信溯源时
// 必须 fail-closed；否则攻击者可把其他账号铸造的 blob 注入任意账号。
// 只剥离、不注入——/responses 路径的客户端会按自身回合语义自行回带；服务端
// 注入是 Claude 兼容桥（无法回带的客户端）的专属行为。
func (s *OpenAIGatewayService) guardOpenAICodexTurnStateEcho(c *gin.Context, account *Account, h http.Header) {
	s.guardOpenAICodexTurnStateEchoWithContext(openAICodexTurnStateRequestContext(c), c, account, h)
}

func (s *OpenAIGatewayService) guardOpenAICodexTurnStateEchoWithContext(storeParentCtx context.Context, c *gin.Context, account *Account, h http.Header) {
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
		if storeParentCtx == nil {
			storeParentCtx = openAICodexTurnStateRequestContext(c)
		}
		checkCtx, cancel := newOpenAICodexTurnStateStoreContext(storeParentCtx)
		defer cancel()
		accountID, ttl, found, err := store.GetTurnStateOrigin(checkCtx, key.String())
		if err != nil {
			s.reportOpenAICodexTurnStateStoreFailure("origin_read", key.groupID, key.apiKeyID, account.ID, err)
		}
		if err == nil && found && accountID > 0 && ttl > 0 {
			if accountID == account.ID {
				// Warm up L1 memory cache for subsequent turns
				s.openaiCodexTurnStateOrigins.Store(key, openAICodexTurnStateOrigin{
					accountID: account.ID,
					expiresAt: time.Now().Add(ttl),
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
