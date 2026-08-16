package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newTurnStateTestContext(t *testing.T, apiKeyID int64, sessionID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if sessionID != "" {
		c.Request.Header.Set("session_id", sessionID)
	}
	if apiKeyID > 0 {
		c.Set("api_key", &APIKey{ID: apiKeyID})
	}
	return c, rec
}

func TestOpenAICodexTurnStateProvenanceKey_HashesBlobAndScopesOwnership(t *testing.T) {
	c, _ := newTurnStateTestContext(t, 7, "sess-1")
	key, ok := openAICodexTurnStateProvenanceKeyFor(c, "blob-A")
	require.True(t, ok)
	require.Equal(t, int64(7), key.apiKeyID)

	// 同一 blob 不能跨 API Key 或 session 复用；key 中只保留两个 SHA-256，
	// 不会把原始 state 放进 sync.Map。
	otherSession, _ := newTurnStateTestContext(t, 7, "sess-2")
	otherSessionKey, ok := openAICodexTurnStateProvenanceKeyFor(otherSession, "blob-A")
	require.True(t, ok)
	require.NotEqual(t, key.sessionHash, otherSessionKey.sessionHash)

	otherKey, _ := newTurnStateTestContext(t, 8, "sess-1")
	otherKeyProvenance, ok := openAICodexTurnStateProvenanceKeyFor(otherKey, "blob-A")
	require.True(t, ok)
	require.NotEqual(t, key.apiKeyID, otherKeyProvenance.apiKeyID)

	otherBlob, ok := openAICodexTurnStateProvenanceKeyFor(c, "blob-B")
	require.True(t, ok)
	require.NotEqual(t, key.stateHash, otherBlob.stateHash)

	// 连字符形式优先（Codex CLI 标准头）。
	c.Request.Header.Set("session-id", "sess-hyphen")
	hyphenKey, ok := openAICodexTurnStateProvenanceKeyFor(c, "blob-A")
	require.True(t, ok)
	require.NotEqual(t, key.sessionHash, hyphenKey.sessionHash)

	cNoSession, _ := newTurnStateTestContext(t, 7, "")
	_, ok = openAICodexTurnStateProvenanceKeyFor(cNoSession, "blob-A")
	require.False(t, ok)
	cNoKey, _ := newTurnStateTestContext(t, 0, "sess-1")
	_, ok = openAICodexTurnStateProvenanceKeyFor(cNoKey, "blob-A")
	require.False(t, ok)
	_, ok = openAICodexTurnStateProvenanceKeyFor(nil, "blob-A")
	require.False(t, ok)
}

func TestRelayOpenAICodexTurnState_RecordsOnlyAfterResponseCommit(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 42}
	c, _ := newTurnStateTestContext(t, 7, "sess-relay")

	upstream := http.Header{}
	upstream.Set("x-codex-turn-state", "blob-A")
	state := svc.relayOpenAICodexTurnState(c, upstream)

	require.Equal(t, "blob-A", c.Writer.Header().Get("X-Codex-Turn-State"))
	require.Equal(t, "blob-A", state)
	key, ok := openAICodexTurnStateProvenanceKeyFor(c, state)
	require.True(t, ok)
	_, found := svc.openaiCodexTurnStateOrigins.Load(key)
	require.False(t, found, "仅设置 response header 不得提前污染溯源")
	require.False(t, svc.noteOpenAICodexTurnStateCommitted(c, account, state), "未真正写出时不得记录")

	_, err := c.Writer.Write([]byte("ok"))
	require.NoError(t, err)
	require.True(t, svc.noteOpenAICodexTurnStateCommitted(c, account, state))

	raw, found := svc.openaiCodexTurnStateOrigins.Load(key)
	require.True(t, found)
	origin, ok := raw.(openAICodexTurnStateOrigin)
	require.True(t, ok)
	require.Equal(t, int64(42), origin.accountID)
	require.True(t, origin.expiresAt.After(time.Now()))
}

func TestRelayOpenAICodexTurnState_ClearsStaleValueWhenUpstreamAbsent(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 7, "sess-stale")
	// 模拟上一 failover attempt 残留的值
	c.Writer.Header().Set("X-Codex-Turn-State", "blob-old")

	state := svc.relayOpenAICodexTurnState(c, http.Header{})

	require.Empty(t, state)
	require.Empty(t, c.Writer.Header().Get("X-Codex-Turn-State"))
}

func TestStageOpenAICodexTurnState_StagedHeaders(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 9, "sess-staged")

	// nil 集合 + 上游有值 → 创建集合并写入，但此刻还不记录溯源
	var staged http.Header
	upstream := http.Header{}
	upstream.Set("x-codex-turn-state", "blob-B")
	stageOpenAICodexTurnState(&staged, upstream)
	require.NotNil(t, staged)
	require.Equal(t, "blob-B", staged.Get("X-Codex-Turn-State"))
	key, ok := openAICodexTurnStateProvenanceKeyFor(c, "blob-B")
	require.True(t, ok)
	_, noted := svc.openaiCodexTurnStateOrigins.Load(key)
	require.False(t, noted, "暂存阶段不得记录溯源：该 attempt 仍可能 failover 丢弃")

	// 写入 staged header 后，只有响应实际提交才记录。
	c.Writer.Header().Set("X-Codex-Turn-State", staged.Get("X-Codex-Turn-State"))
	_, err := c.Writer.Write([]byte("data: committed\n\n"))
	require.NoError(t, err)
	svc.noteStagedOpenAICodexTurnStateCommitted(c, &Account{ID: 44}, staged)
	raw, noted := svc.openaiCodexTurnStateOrigins.Load(key)
	require.True(t, noted)
	origin, ok := raw.(openAICodexTurnStateOrigin)
	require.True(t, ok)
	require.Equal(t, int64(44), origin.accountID)

	// 上游无值 → 清除已暂存的值；nil 集合保持 nil
	stageOpenAICodexTurnState(&staged, http.Header{})
	require.Empty(t, staged.Get("X-Codex-Turn-State"))
	var nilStaged http.Header
	stageOpenAICodexTurnState(&nilStaged, http.Header{})
	require.Nil(t, nilStaged)
}

// 首输出 failover 丢弃 A 的 attempt 时，A 的 blob 不得进入溯源；B 真正
// 提交自己的 blob 后，客户端若仍回带 A 的旧 blob，B 出站必须剥离它。
func TestStagedTurnState_AbandonedAttemptDoesNotPoisonProvenance(t *testing.T) {
	svc := &OpenAIGatewayService{}
	cA, _ := newTurnStateTestContext(t, 11, "sess-abandoned")

	// 账号 A 的 attempt 暂存了 blob，但从未提交（首输出超时 → failover）
	var staged http.Header
	upstreamA := http.Header{}
	upstreamA.Set("x-codex-turn-state", "blob-A")
	stageOpenAICodexTurnState(&staged, upstreamA)
	keyA, ok := openAICodexTurnStateProvenanceKeyFor(cA, "blob-A")
	require.True(t, ok)
	_, found := svc.openaiCodexTurnStateOrigins.Load(keyA)
	require.False(t, found)

	// 账号 B 接手并真正提交另一个 blob。
	cB, _ := newTurnStateTestContext(t, 11, "sess-abandoned")
	stateB := svc.relayOpenAICodexTurnState(cB, http.Header{"X-Codex-Turn-State": []string{"blob-B"}})
	_, err := cB.Writer.Write([]byte("data: committed\n\n"))
	require.NoError(t, err)
	require.True(t, svc.noteOpenAICodexTurnStateCommitted(cB, &Account{ID: 52}, stateB))

	// 客户端回带被放弃的 A blob 时，B 出站一律 fail-closed。
	old := http.Header{"X-Codex-Turn-State": []string{"blob-A"}}
	svc.guardOpenAICodexTurnStateEcho(cB, &Account{ID: 52}, old)
	require.Empty(t, old.Get("x-codex-turn-state"))

	// B 自己真正提交的 blob 仍可在 B 账号上继续回放。
	current := http.Header{"X-Codex-Turn-State": []string{"blob-B"}}
	svc.guardOpenAICodexTurnStateEcho(cB, &Account{ID: 52}, current)
	require.Equal(t, "blob-B", current.Get("x-codex-turn-state"))
}

func TestNoteStagedOpenAICodexTurnStateCommitted_NoopWithoutState(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 12, "sess-nostate")

	svc.noteStagedOpenAICodexTurnStateCommitted(c, &Account{ID: 60}, nil)
	svc.noteStagedOpenAICodexTurnStateCommitted(c, &Account{ID: 60}, http.Header{"X-Request-Id": []string{"rid"}})

	key, ok := openAICodexTurnStateProvenanceKeyFor(c, "blob-never-recorded")
	require.True(t, ok)
	_, found := svc.openaiCodexTurnStateOrigins.Load(key)
	require.False(t, found)
}

func TestGuardOpenAICodexTurnStateEcho(t *testing.T) {
	commit := func(t *testing.T, svc *OpenAIGatewayService, c *gin.Context, account *Account, state string) {
		t.Helper()
		require.Equal(t, state, svc.relayOpenAICodexTurnState(c, http.Header{"X-Codex-Turn-State": []string{state}}))
		_, err := c.Writer.Write([]byte("ok"))
		require.NoError(t, err)
		require.True(t, svc.noteOpenAICodexTurnStateCommitted(c, account, state))
	}
	newOutbound := func(state string) http.Header {
		h := http.Header{}
		if state != "" {
			h.Set("x-codex-turn-state", state)
		}
		return h
	}

	t.Run("same_account_keeps_exact_committed_blob", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		c, _ := newTurnStateTestContext(t, 7, "sess-g1")
		commit(t, svc, c, &Account{ID: 42}, "blob-A")

		h := newOutbound("blob-A")
		svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 42}, h)
		require.Equal(t, "blob-A", h.Get("x-codex-turn-state"))
	})

	t.Run("a_to_b_then_old_blob_is_stripped", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		cA, _ := newTurnStateTestContext(t, 7, "sess-g2")
		commit(t, svc, cA, &Account{ID: 42}, "blob-A")
		cB, _ := newTurnStateTestContext(t, 7, "sess-g2")
		commit(t, svc, cB, &Account{ID: 43}, "blob-B")

		old := newOutbound("blob-A")
		svc.guardOpenAICodexTurnStateEcho(cB, &Account{ID: 43}, old)
		require.Empty(t, old.Get("x-codex-turn-state"), "B 不能回带 A 铸造的旧 blob")

		current := newOutbound("blob-B")
		svc.guardOpenAICodexTurnStateEcho(cB, &Account{ID: 43}, current)
		require.Equal(t, "blob-B", current.Get("x-codex-turn-state"))
	})

	t.Run("unknown_cross_slot_and_restart_fail_closed", func(t *testing.T) {
		slotA := &OpenAIGatewayService{}
		cA, _ := newTurnStateTestContext(t, 7, "sess-g3")
		commit(t, slotA, cA, &Account{ID: 42}, "blob-A")

		// 新 slot 没有内存 provenance，不能猜测 blob 的账号归属。
		slotB := &OpenAIGatewayService{}
		cB, _ := newTurnStateTestContext(t, 7, "sess-g3")
		h := newOutbound("blob-A")
		slotB.guardOpenAICodexTurnStateEcho(cB, &Account{ID: 42}, h)
		require.Empty(t, h.Get("x-codex-turn-state"))
	})

	t.Run("unknown_expired_or_unscoped_blob_fail_closed", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		c, _ := newTurnStateTestContext(t, 7, "sess-g4")
		unknown := newOutbound("blob-unknown")
		svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 42}, unknown)
		require.Empty(t, unknown.Get("x-codex-turn-state"))

		key, ok := openAICodexTurnStateProvenanceKeyFor(c, "blob-expired")
		require.True(t, ok)
		svc.openaiCodexTurnStateOrigins.Store(key, openAICodexTurnStateOrigin{
			accountID: 42,
			expiresAt: time.Now().Add(-time.Minute),
		})
		expired := newOutbound("blob-expired")
		svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 42}, expired)
		require.Empty(t, expired.Get("x-codex-turn-state"))
		_, found := svc.openaiCodexTurnStateOrigins.Load(key)
		require.False(t, found)

		noSession, _ := newTurnStateTestContext(t, 7, "")
		unscoped := newOutbound("blob-A")
		svc.guardOpenAICodexTurnStateEcho(noSession, &Account{ID: 42}, unscoped)
		require.Empty(t, unscoped.Get("x-codex-turn-state"))
	})

	t.Run("empty_header_is_noop", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		c, _ := newTurnStateTestContext(t, 7, "sess-g5")
		h := newOutbound("")
		svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 43}, h)
		require.Empty(t, h.Get("x-codex-turn-state"))
	})
}

func TestSweepOpenAICodexTurnStateOrigins_PrunesExpiredEntries(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 7, "sess-sweep")
	expiredKey, ok := openAICodexTurnStateProvenanceKeyFor(c, "blob-expired")
	require.True(t, ok)
	aliveKey, ok := openAICodexTurnStateProvenanceKeyFor(c, "blob-alive")
	require.True(t, ok)
	svc.openaiCodexTurnStateOrigins.Store(expiredKey, openAICodexTurnStateOrigin{
		accountID: 1,
		expiresAt: time.Now().Add(-time.Minute),
	})
	svc.openaiCodexTurnStateOrigins.Store(aliveKey, openAICodexTurnStateOrigin{
		accountID: 2,
		expiresAt: time.Now().Add(time.Hour),
	})

	// 计数器推进到触发清扫的边界
	svc.openaiCodexTurnStateWrites.Store(255)
	svc.sweepOpenAICodexTurnStateOrigins()

	_, expiredOK := svc.openaiCodexTurnStateOrigins.Load(expiredKey)
	require.False(t, expiredOK)
	_, aliveOK := svc.openaiCodexTurnStateOrigins.Load(aliveKey)
	require.True(t, aliveOK)
}

func newTurnStateResponseTestService(firstOutputTimeoutSeconds int) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			OpenAIFirstOutputTimeoutSeconds: firstOutputTimeoutSeconds,
			MaxLineSize:                     defaultMaxLineSize,
		}},
		toolCorrector: NewCodexToolCorrector(),
	}
}

func requireTurnStateOrigin(t *testing.T, svc *OpenAIGatewayService, c *gin.Context, state string, accountID int64) {
	t.Helper()
	key, ok := openAICodexTurnStateProvenanceKeyFor(c, state)
	require.True(t, ok)
	raw, found := svc.openaiCodexTurnStateOrigins.Load(key)
	require.True(t, found)
	origin, ok := raw.(openAICodexTurnStateOrigin)
	require.True(t, ok)
	require.Equal(t, accountID, origin.accountID)
}

// 三条响应链都必须在实际写入下游之后才记录：普通 JSON、原生 SSE 和
// OAuth passthrough。若以后有人把记录移动回收到上游 header 的时点，本测试
// 会在 failover/跨账号防护失效前失败。
func TestTurnStateProvenance_RecordsOnNormalJSONSSEAndPassthroughCommit(t *testing.T) {
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	t.Run("normal_json", func(t *testing.T) {
		svc := newTurnStateResponseTestService(0)
		c, rec := newTurnStateTestContext(t, 7, "sess-json")
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":       []string{"application/json"},
				"X-Codex-Turn-State": []string{"blob-json"},
			},
			Body: io.NopCloser(strings.NewReader(`{"id":"resp-json","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)),
		}

		_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, account, "gpt-5.5", "gpt-5.5")
		require.NoError(t, err)
		require.True(t, c.Writer.Written())
		require.Equal(t, "blob-json", rec.Header().Get("X-Codex-Turn-State"))
		requireTurnStateOrigin(t, svc, c, "blob-json", account.ID)
	})

	t.Run("native_sse", func(t *testing.T) {
		svc := newTurnStateResponseTestService(0)
		c, rec := newTurnStateTestContext(t, 7, "sess-sse")
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":       []string{"text/event-stream"},
				"X-Codex-Turn-State": []string{"blob-sse"},
			},
			Body: io.NopCloser(strings.NewReader(strings.Join([]string{
				`data: {"type":"response.created","response":{"id":"resp-sse"}}`,
				"",
				`data: {"type":"response.completed","response":{"id":"resp-sse","usage":{"input_tokens":1,"output_tokens":1}}}`,
				"",
			}, "\n"))),
		}

		_, err := svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "gpt-5.5", "gpt-5.5")
		require.NoError(t, err)
		require.True(t, c.Writer.Written())
		require.Equal(t, "blob-sse", rec.Header().Get("X-Codex-Turn-State"))
		requireTurnStateOrigin(t, svc, c, "blob-sse", account.ID)
	})

	t.Run("oauth_passthrough", func(t *testing.T) {
		svc := newTurnStateResponseTestService(0)
		c, rec := newTurnStateTestContext(t, 7, "sess-passthrough")
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":       []string{"application/json"},
				"X-Codex-Turn-State": []string{"blob-passthrough"},
			},
			Body: io.NopCloser(strings.NewReader(`{"id":"resp-pass","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)),
		}

		_, err := svc.handleNonStreamingResponsePassthrough(context.Background(), resp, c, account, "gpt-5.5", "gpt-5.5")
		require.NoError(t, err)
		require.True(t, c.Writer.Written())
		require.Equal(t, "blob-passthrough", rec.Header().Get("X-Codex-Turn-State"))
		requireTurnStateOrigin(t, svc, c, "blob-passthrough", account.ID)
	})
}

func TestTurnStateProvenance_GuardedSSEFailoverDoesNotRecordUncommittedAttempt(t *testing.T) {
	svc := newTurnStateResponseTestService(1)
	c, rec := newTurnStateTestContext(t, 7, "sess-failover")
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":       []string{"text/event-stream"},
			"X-Codex-Turn-State": []string{"blob-abandoned"},
		},
		// 只有 preamble，流在首个可提交输出前结束，必须允许 failover。
		Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-abandoned\"}}\n\n")),
	}

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "gpt-5.5", "gpt-5.5")
	require.Error(t, err)
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
	key, ok := openAICodexTurnStateProvenanceKeyFor(c, "blob-abandoned")
	require.True(t, ok)
	_, found := svc.openaiCodexTurnStateOrigins.Load(key)
	require.False(t, found)
}

func TestPassthroughTurnState_FailoverDropsProvisionalHeaderAndCommitsReplacement(t *testing.T) {
	svc := newTurnStateResponseTestService(0)
	c, rec := newTurnStateTestContext(t, 7, "sess-passthrough-failover")
	accountA := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	accountB := &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	// A gives us a state in its headers but fails before the first client byte.
	// The same gin writer is then reused by the real failover loop for B.
	respA := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":       []string{"text/event-stream"},
			"X-Codex-Turn-State": []string{"blob-attempt-a"},
		},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp-attempt-a"}}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","error":{"code":"server_error","message":"upstream processing failed"}}`,
			"",
		}, "\n"))),
	}
	_, err := svc.handleStreamingResponsePassthrough(context.Background(), respA, c, accountA, time.Now(), "gpt-5.5", "gpt-5.5")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Header().Get(openAICodexTurnStateHeader), "abandoned attempt must not poison B response headers")
	keyA, ok := openAICodexTurnStateProvenanceKeyFor(c, "blob-attempt-a")
	require.True(t, ok)
	_, found := svc.openaiCodexTurnStateOrigins.Load(keyA)
	require.False(t, found)

	respB := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":       []string{"text/event-stream"},
			"X-Codex-Turn-State": []string{"blob-attempt-b"},
		},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"ok"}`,
			"",
			`data: {"type":"response.completed","response":{"id":"resp-attempt-b","usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
		}, "\n"))),
	}
	_, err = svc.handleStreamingResponsePassthrough(context.Background(), respB, c, accountB, time.Now(), "gpt-5.5", "gpt-5.5")
	require.NoError(t, err)
	require.True(t, c.Writer.Written())
	require.Equal(t, "blob-attempt-b", rec.Header().Get(openAICodexTurnStateHeader))
	requireTurnStateOrigin(t, svc, c, "blob-attempt-b", accountB.ID)
}

func TestOpenAIWSTurnState_RequiresExactProvenanceAndScopedCache(t *testing.T) {
	svc := &OpenAIGatewayService{}
	store := NewOpenAIWSStateStore(nil)
	accountA := &Account{ID: 42}
	accountB := &Account{ID: 43}
	cA, _ := newTurnStateTestContext(t, 7, "sess-ws-provenance")
	const sessionHash = "ws-session-hash"
	const state = "ws-state-a"

	// A completed WS turn may persist its upstream handshake state, but only
	// under this exact API key + account + session scope.
	require.True(t, svc.commitOpenAIWSSessionTurnState(cA, accountA, store, 9, sessionHash, state))
	require.Equal(t, state, svc.loadOpenAIWSSessionTurnState(cA, accountA, store, 9, sessionHash))

	cOtherKey, _ := newTurnStateTestContext(t, 8, "sess-ws-provenance")
	require.Empty(t, svc.loadOpenAIWSSessionTurnState(cOtherKey, accountA, store, 9, sessionHash))
	require.Empty(t, svc.guardedOpenAIWSTurnState(cOtherKey, accountA, state), "another API key cannot inject A's blob")

	cOtherAccount, _ := newTurnStateTestContext(t, 7, "sess-ws-provenance")
	require.Empty(t, svc.loadOpenAIWSSessionTurnState(cOtherAccount, accountB, store, 9, sessionHash))
	require.Empty(t, svc.guardedOpenAIWSTurnState(cOtherAccount, accountB, state), "a failover account cannot replay A's blob")

	// A cache record without this slot's blob provenance is never enough to
	// authorize a replay after a blue/green switch or process restart.
	newSlot := &OpenAIGatewayService{}
	require.Empty(t, newSlot.loadOpenAIWSSessionTurnState(cA, accountA, store, 9, sessionHash))
	_, stillCached := store.GetSessionTurnState(9, 7, accountA.ID, sessionHash)
	require.False(t, stillCached, "unverifiable cache state is deleted fail-closed")
}

func TestWriteOpenAIPassthroughResponseHeaders_RelaysAndClearsTurnState(t *testing.T) {
	// filter=nil 走 content-type 兜底分支；turn-state 强制放行不依赖 filter。
	dst := http.Header{}
	src := http.Header{}
	src.Set("X-Codex-Turn-State", "blob-P")
	writeOpenAIPassthroughResponseHeaders(dst, src, nil)
	require.Equal(t, "blob-P", dst.Get("X-Codex-Turn-State"))

	// 上游缺失时清除残留（failover 换号防串扰）
	writeOpenAIPassthroughResponseHeaders(dst, http.Header{"Content-Type": []string{"application/json"}}, nil)
	require.Empty(t, dst.Get("X-Codex-Turn-State"))
}

func TestApplyOpenAIStagedResponseHeaders_ReplacesOldTurnState(t *testing.T) {
	dst := http.Header{
		"X-Codex-Turn-State": []string{"blob-attempt-a"},
		"X-Existing":         []string{"preserved"},
	}
	staged := http.Header{
		"X-Codex-Turn-State": []string{"blob-attempt-b"},
		"X-Request-Id":       []string{"request-b"},
	}

	applyOpenAIStagedResponseHeaders(dst, staged)

	// State has single-value ownership semantics: B replaces, never appends to,
	// a provisional A value. Other staged headers intentionally retain Add.
	require.Equal(t, []string{"blob-attempt-b"}, dst.Values(openAICodexTurnStateHeader))
	require.Equal(t, "preserved", dst.Get("X-Existing"))
	require.Equal(t, "request-b", dst.Get("X-Request-Id"))

	applyOpenAIStagedResponseHeaders(dst, nil)
	require.Empty(t, dst.Get(openAICodexTurnStateHeader), "a successful attempt without state must clear stale A/B state")
	require.Equal(t, []string{"request-b"}, dst.Values("X-Request-Id"))
}

func TestEnsureOpenAIRemoteCompactionV2BetaFeature(t *testing.T) {
	t.Run("absent_sets_feature", func(t *testing.T) {
		h := http.Header{}
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		require.Equal(t, "remote_compaction_v2", h.Get("x-codex-beta-features"))
	})

	t.Run("present_unchanged", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-beta-features", "responses_websockets_v2, remote_compaction_v2")
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		require.Equal(t, "responses_websockets_v2, remote_compaction_v2", h.Get("x-codex-beta-features"))
	})

	t.Run("other_tokens_merged", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-beta-features", "responses_websockets_v2")
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		require.Equal(t, "responses_websockets_v2,remote_compaction_v2", h.Get("x-codex-beta-features"))
	})

	t.Run("multi_line_values_merged_single_line", func(t *testing.T) {
		h := http.Header{}
		h.Add("x-codex-beta-features", "feature_a")
		h.Add("x-codex-beta-features", "feature_b")
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		require.Equal(t, []string{"feature_a,feature_b,remote_compaction_v2"}, h.Values("x-codex-beta-features"))
	})
}

// 对齐真实 Codex：该头是会话级常量，挂在 OAuth 的每个请求上，而不是只在
// 压缩回合出现（codex-rs build_model_client_beta_features_header）。
func TestApplyOpenAICodexBetaFeatures(t *testing.T) {
	oauthAccount := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKeyAccount := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	t.Run("oauth_plain_request_gets_default_codex_shape", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		h := http.Header{}
		applyOpenAICodexBetaFeatures(c, oauthAccount, h)
		require.Equal(t, "remote_compaction_v2", h.Get("x-codex-beta-features"),
			"OAuth 的普通请求也必须带会话级 beta 头")
	})

	t.Run("client_declared_header_preserved", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		h := http.Header{}
		h.Set("x-codex-beta-features", "some_other_feature")
		applyOpenAICodexBetaFeatures(c, oauthAccount, h)
		require.Equal(t, "some_other_feature", h.Get("x-codex-beta-features"),
			"客户端显式声明的能力集不得被网关改写（非空即视为用户已关闭 v2）")
	})

	t.Run("native_v2_forces_feature_even_when_client_trimmed_it", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		MarkOpenAINativeCompactionV2(c)
		h := http.Header{}
		h.Set("x-codex-beta-features", "some_other_feature")
		applyOpenAICodexBetaFeatures(c, oauthAccount, h)
		require.Contains(t, h.Get("x-codex-beta-features"), "remote_compaction_v2",
			"body 带 compaction_trigger 是实锤，必须确保 v2 在列")
		require.Contains(t, h.Get("x-codex-beta-features"), "some_other_feature")
	})

	t.Run("native_v2_applies_to_non_oauth_too", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		MarkOpenAINativeCompactionV2(c)
		h := http.Header{}
		applyOpenAICodexBetaFeatures(c, apiKeyAccount, h)
		require.Equal(t, "remote_compaction_v2", h.Get("x-codex-beta-features"))
	})

	t.Run("non_oauth_plain_request_untouched", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		h := http.Header{}
		applyOpenAICodexBetaFeatures(c, apiKeyAccount, h)
		require.Empty(t, h.Get("x-codex-beta-features"),
			"非 Codex 后端不做会话级注入")
	})

	t.Run("nil_account_plain_request_untouched", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		h := http.Header{}
		applyOpenAICodexBetaFeatures(c, nil, h)
		require.Empty(t, h.Get("x-codex-beta-features"))
	})
}

// WS 握手与 HTTP 出站必须给出同一份会话级 beta 头：真实 Codex 的
// build_websocket_headers 复用 build_responses_headers（client.rs），
// 两侧不一致还会让预热连接与实际请求落进不同的连接池兼容分桶。
func TestBuildOpenAIWSHeaders_CarriesSessionBetaFeatures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	decision := OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}

	build := func(t *testing.T, account *Account, clientBeta string) http.Header {
		t.Helper()
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
		if clientBeta != "" {
			c.Request.Header.Set("x-codex-beta-features", clientBeta)
		}
		headers, _, err := svc.buildOpenAIWSHeaders(
			context.Background(), c, account, "test-token", decision,
			true, "", "", "", "gpt-5.6-codex", "",
		)
		require.NoError(t, err)
		return headers
	}

	oauthAccount := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "test-account"},
	}

	headers := build(t, oauthAccount, "")
	require.Equal(t, "remote_compaction_v2", headers.Get("x-codex-beta-features"),
		"WS 握手也必须带会话级 beta 头")

	declared := build(t, oauthAccount, "some_other_feature")
	require.Equal(t, []string{"some_other_feature"}, declared.Values("x-codex-beta-features"),
		"客户端已声明时原样保留")

	apiKeyHeaders := build(t, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "")
	require.Empty(t, apiKeyHeaders.Get("x-codex-beta-features"),
		"非 Codex 后端不注入")
}

type stubCodexTurnStateOriginStore struct {
	data map[string]int64
}

func (s *stubCodexTurnStateOriginStore) SetTurnStateOrigin(_ context.Context, key string, accountID int64, _ time.Duration) error {
	if s.data == nil {
		s.data = make(map[string]int64)
	}
	s.data[key] = accountID
	return nil
}

func (s *stubCodexTurnStateOriginStore) GetTurnStateOrigin(_ context.Context, key string) (int64, error) {
	if s.data == nil {
		return 0, nil
	}
	val, ok := s.data[key]
	if !ok {
		return 0, nil
	}
	return val, nil
}

type stubGatewayCacheWithOriginStore struct {
	stubGatewayCache
	stubCodexTurnStateOriginStore
}

func TestGuardOpenAICodexTurnStateEcho_RecoversFromRedisL2OnBlueGreenSwitch(t *testing.T) {
	sharedStore := &stubGatewayCacheWithOriginStore{
		stubCodexTurnStateOriginStore: stubCodexTurnStateOriginStore{data: make(map[string]int64)},
	}

	// 模拟 Blue 槽：处理第一轮请求并记录 turn-state 溯源
	svcA := &OpenAIGatewayService{cache: sharedStore}
	accountA := &Account{ID: 100}
	cA, _ := newTurnStateTestContext(t, 1, "session-blue-green")

	state := "valid-turn-state-blob"
	key, ok := openAICodexTurnStateProvenanceKeyFor(cA, state)
	require.True(t, ok)

	require.True(t, svcA.noteOpenAICodexTurnStateOrigin(cA, accountA, state))
	// 确保写入了 sharedStore
	require.NoError(t, sharedStore.SetTurnStateOrigin(context.Background(), key.String(), accountA.ID, time.Hour))

	// 模拟 Green 槽（全新实例/切换）：本地 L1 内存为空，但共享 L2 存储
	svcB := &OpenAIGatewayService{cache: sharedStore}
	cB, _ := newTurnStateTestContext(t, 1, "session-blue-green")
	hB := http.Header{}
	hB.Set("x-codex-turn-state", state)

	// Green 槽校验：L1 未命中，但从 Redis L2 成功命中并放行
	svcB.guardOpenAICodexTurnStateEcho(cB, accountA, hB)
	require.Equal(t, state, hB.Get("x-codex-turn-state"), "蓝绿切换后应从 L2 Redis 恢复溯源放行")

	// 验证已回填至 Green 槽的 L1 内存
	raw, found := svcB.openaiCodexTurnStateOrigins.Load(key)
	require.True(t, found, "应已热加载至本地 L1")
	origin, ok := raw.(openAICodexTurnStateOrigin)
	require.True(t, ok)
	require.Equal(t, accountA.ID, origin.accountID)

	// 验证不同账号请求（盗用该 blob）：即使 Redis 存在，账号不匹配仍被拦截
	mismatchedAccount := &Account{ID: 200}
	hMismatched := http.Header{}
	hMismatched.Set("x-codex-turn-state", state)
	svcB.guardOpenAICodexTurnStateEcho(cB, mismatchedAccount, hMismatched)
	require.Empty(t, hMismatched.Get("x-codex-turn-state"), "账号不匹配必须 fail-closed 剥除")
}
