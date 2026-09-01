package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// openAIWSIngressCapacityShedRepo 补齐 SetError，避免非容量类错误（如
// workspace_suspended）走到账号状态副作用时打空指针。
type openAIWSIngressCapacityShedRepo struct {
	stubOpenAIAccountRepo
}

func (r *openAIWSIngressCapacityShedRepo) SetError(context.Context, int64, string) error { return nil }

func (r *openAIWSIngressCapacityShedRepo) SetRateLimited(context.Context, int64, time.Time) error {
	return nil
}

func (r *openAIWSIngressCapacityShedRepo) UpdateExtra(context.Context, int64, map[string]any) error {
	return nil
}

type openAIWSIngressCapacityHarness struct {
	clientConn  *coderws.Conn
	server      *httptest.Server
	serverErrCh chan error
	pool        *openAIWSConnPool
	captureConn *openAIWSCaptureConn
}

type openAIWSNoReaderPingCaptureConn struct {
	*openAIWSCaptureConn
}

func (c *openAIWSNoReaderPingCaptureConn) SupportsIdlePingWithoutReader() bool {
	return false
}

func newOpenAIWSIngressCapacityHarness(t *testing.T, upstreamEvents [][]byte) *openAIWSIngressCapacityHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	events := make([][]byte, 0, len(upstreamEvents))
	for _, event := range upstreamEvents {
		events = append(events, append([]byte(nil), event...))
	}
	captureConn := &openAIWSCaptureConn{events: events}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: captureConn})

	account := &Account{
		ID:          5402,
		Name:        "openai-ingress-capacity-failover",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}
	repo := &openAIWSIngressCapacityShedRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{*account}}}
	svc := &OpenAIGatewayService{
		accountRepo:      repo,
		rateLimitService: &RateLimitService{accountRepo: repo},
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		cfg:              cfg,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}

	serverErrCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			serverErrCh <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		req.Header.Set("User-Agent", "unit-test-agent/1.0")
		ginCtx.Request = req

		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		msgType, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			serverErrCh <- readErr
			return
		}
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			serverErrCh <- errors.New("unsupported websocket client message type")
			return
		}
		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "sk-test", firstMessage, nil)
	}))

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)

	harness := &openAIWSIngressCapacityHarness{
		clientConn:  clientConn,
		server:      server,
		serverErrCh: serverErrCh,
		pool:        pool,
		captureConn: captureConn,
	}
	t.Cleanup(func() {
		_ = harness.clientConn.CloseNow()
		harness.server.Close()
		harness.pool.Close()
	})
	return harness
}

func (h *openAIWSIngressCapacityHarness) write(t *testing.T, payload string) {
	t.Helper()
	writeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, h.clientConn.Write(writeCtx, coderws.MessageText, []byte(payload)))
}

func (h *openAIWSIngressCapacityHarness) read(t *testing.T) ([]byte, error) {
	t.Helper()
	readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, message, err := h.clientConn.Read(readCtx)
	return message, err
}

func (h *openAIWSIngressCapacityHarness) serverErr(t *testing.T) error {
	t.Helper()
	select {
	case err := <-h.serverErrCh:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("等待 ingress websocket 结束超时")
		return nil
	}
}

// 已经有语义输出后不能再切账号；此时 error 与 response.failed 必须精确下发，
// 但容量码要改成 Codex 可重试的 server_error。非容量错误仍保留原码。
func TestProxyResponsesWebSocketFromClient_RewritesCapacityShedCodeAfterSemanticOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name               string
		upstreamEvents     [][]byte
		wantTypes          []string
		wantCodes          []string
		wantAbsent         []string
		capacityFrameStart int
	}{
		{
			name: "server_is_overloaded_error_and_failed_are_rewritten",
			upstreamEvents: [][]byte{
				[]byte(`{"type":"response.output_text.delta","delta":"partial"}`),
				[]byte(`{"type":"error","error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`),
				[]byte(`{"type":"response.failed","response":{"id":"resp_shed","status":"failed","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}`),
			},
			wantTypes:          []string{"response.output_text.delta", "error", "response.failed"},
			wantCodes:          []string{"", "server_error", "server_error"},
			wantAbsent:         []string{"server_is_overloaded"},
			capacityFrameStart: 1,
		},
		{
			name: "slow_down_error_and_failed_are_rewritten",
			upstreamEvents: [][]byte{
				[]byte(`{"type":"response.output_text.delta","delta":"partial"}`),
				[]byte(`{"type":"error","error":{"type":"service_unavailable_error","code":"slow_down","message":"Please slow down and retry."}}`),
				[]byte(`{"type":"response.failed","response":{"id":"resp_slow","status":"failed","error":{"code":"slow_down","message":"Please slow down and retry."}}}`),
			},
			wantTypes:          []string{"response.output_text.delta", "error", "response.failed"},
			wantCodes:          []string{"", "server_error", "server_error"},
			wantAbsent:         []string{"slow_down"},
			capacityFrameStart: 1,
		},
		{
			name: "non_capacity_error_code_is_passed_through",
			upstreamEvents: [][]byte{
				[]byte(`{"type":"error","error":{"type":"invalid_request_error","code":"workspace_suspended","message":"workspace is suspended"}}`),
				[]byte(`{"type":"response.failed","response":{"id":"resp_suspended","status":"failed","error":{"code":"workspace_suspended","message":"workspace is suspended"}}}`),
			},
			wantTypes:          []string{"error", "response.failed"},
			wantCodes:          []string{"workspace_suspended", "workspace_suspended"},
			wantAbsent:         []string{"server_error"},
			capacityFrameStart: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newOpenAIWSV2TestConfig()
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
			cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

			events := make([][]byte, 0, len(tt.upstreamEvents))
			for _, event := range tt.upstreamEvents {
				events = append(events, append([]byte(nil), event...))
			}
			captureConn := &openAIWSCaptureConn{events: events}
			pool := newOpenAIWSConnPool(cfg)
			defer pool.Close()
			pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: captureConn})

			account := Account{
				ID:          5401,
				Name:        "openai-ingress-capacity-shed",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Credentials: map[string]any{"api_key": "sk-test"},
				Extra:       map[string]any{"responses_websockets_v2_enabled": true},
			}
			repo := &openAIWSIngressCapacityShedRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}}
			svc := &OpenAIGatewayService{
				accountRepo:      repo,
				rateLimitService: &RateLimitService{accountRepo: repo},
				httpUpstream:     &httpUpstreamRecorder{},
				cache:            &stubGatewayCache{},
				cfg:              cfg,
				openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
				toolCorrector:    NewCodexToolCorrector(),
				openaiWSPool:     pool,
			}

			serverDone := make(chan struct{})
			wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(serverDone)
				conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
				if err != nil {
					return
				}
				defer func() { _ = conn.CloseNow() }()

				rec := httptest.NewRecorder()
				ginCtx, _ := gin.CreateTestContext(rec)
				req := r.Clone(r.Context())
				req.Header = req.Header.Clone()
				req.Header.Set("User-Agent", "unit-test-agent/1.0")
				ginCtx.Request = req

				readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				msgType, firstMessage, readErr := conn.Read(readCtx)
				cancel()
				if readErr != nil || (msgType != coderws.MessageText && msgType != coderws.MessageBinary) {
					return
				}
				_ = svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, &account, "sk-test", firstMessage, nil)
			}))
			defer wsServer.Close()

			dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
			clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
			cancelDial()
			require.NoError(t, err)
			defer func() { _ = clientConn.CloseNow() }()

			writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
			err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":false}`))
			cancelWrite()
			require.NoError(t, err)

			var frames []string
			for len(frames) < len(tt.upstreamEvents) {
				readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_, message, readErr := clientConn.Read(readCtx)
				cancel()
				if readErr != nil {
					break
				}
				frames = append(frames, string(message))
			}
			// 本轮已终止，主动断开客户端让 ingress 退出 turn 循环。
			_ = clientConn.CloseNow()

			require.Len(t, frames, len(tt.upstreamEvents), "每个上游事件都应精确下发一次")
			for i, frame := range frames {
				require.Equal(t, tt.wantTypes[i], gjson.Get(frame, "type").String(), "frame[%d]=%s", i, frame)
				code := gjson.Get(frame, "error.code").String()
				if tt.wantTypes[i] == "response.failed" {
					code = gjson.Get(frame, "response.error.code").String()
				}
				require.Equal(t, tt.wantCodes[i], code, "frame[%d]=%s", i, frame)
			}
			if tt.capacityFrameStart >= 0 {
				require.Equal(t, "error", gjson.Get(frames[tt.capacityFrameStart], "type").String())
				require.Equal(t, "response.failed", gjson.Get(frames[tt.capacityFrameStart+1], "type").String())
			}
			joined := strings.Join(frames, "\n")
			for _, absent := range tt.wantAbsent {
				require.NotContains(t, joined, absent, "客户端收到的事件:\n%s", joined)
			}

			select {
			case <-serverDone:
			case <-time.After(5 * time.Second):
				t.Fatal("等待 ingress websocket 结束超时")
			}
		})
	}
}

func TestProxyResponsesWebSocketFromClient_CapacityShedBeforeSemanticOutputFailsOverWithoutClientFrames(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		eventType string
	}{
		{name: "error_server_is_overloaded", code: "server_is_overloaded", eventType: "error"},
		{name: "failed_server_is_overloaded", code: "server_is_overloaded", eventType: "response.failed"},
		{name: "error_slow_down", code: "slow_down", eventType: "error"},
		{name: "failed_slow_down", code: "slow_down", eventType: "response.failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capacityEvent := `{"type":"error","error":{"type":"service_unavailable_error","code":"` + tt.code + `","message":"capacity unavailable"}}`
			if tt.eventType == "response.failed" {
				capacityEvent = `{"type":"response.failed","response":{"id":"resp_capacity","status":"failed","error":{"code":"` + tt.code + `","message":"capacity unavailable"}}}`
			}
			harness := newOpenAIWSIngressCapacityHarness(t, [][]byte{
				[]byte(`{"type":"response.created","response":{"id":"resp_pending"}}`),
				[]byte(capacityEvent),
			})

			harness.write(t, `{"type":"response.create","model":"gpt-5.1","stream":false,"input":[{"role":"user","content":"hello"}]}`)
			message, readErr := harness.read(t)
			require.Error(t, readErr)
			require.Empty(t, message, "容量 failover 前不得泄漏 response.created 或失败帧")

			serverErr := harness.serverErr(t)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, serverErr, &failoverErr)
			require.True(t, failoverErr.RequestScopedTransient)
			require.True(t, failoverErr.RetryableOnSameAccount)
			require.Equal(t, tt.code, func() string {
				if tt.eventType == "error" {
					return gjson.GetBytes(failoverErr.ResponseBody, "error.code").String()
				}
				return gjson.GetBytes(failoverErr.ResponseBody, "response.error.code").String()
			}())
		})
	}
}

func TestProxyResponsesWebSocketFromClient_CurrentTurnCapacityFailoverBuildsReplayPayload(t *testing.T) {
	harness := newOpenAIWSIngressCapacityHarness(t, [][]byte{
		[]byte(`{"type":"response.completed","response":{"id":"resp_turn_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
		[]byte(`{"type":"response.created","response":{"id":"resp_turn_2_pending"}}`),
		[]byte(`{"type":"error","error":{"type":"service_unavailable_error","code":"slow_down","message":"Please slow down and retry."}}`),
		[]byte(`{"type":"response.failed","response":{"id":"resp_turn_2_pending","status":"failed","error":{"code":"slow_down","message":"Please slow down and retry."}}}`),
	})

	harness.write(t, `{"type":"response.create","model":"gpt-5.1","stream":false,"input":[{"role":"user","content":"first"}]}`)
	firstFrame, readErr := harness.read(t)
	require.NoError(t, readErr)
	require.Equal(t, "response.completed", gjson.GetBytes(firstFrame, "type").String())
	require.Equal(t, "resp_turn_1", gjson.GetBytes(firstFrame, "response.id").String())

	harness.write(t, `{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"resp_turn_1","input":[{"role":"user","content":"second"}]}`)
	secondFrame, readErr := harness.read(t)
	require.Error(t, readErr)
	require.Empty(t, secondFrame, "current-turn failover 前不得泄漏新 response.created 或容量帧")

	serverErr := harness.serverErr(t)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, serverErr, &failoverErr)
	require.True(t, failoverErr.RequestScopedTransient)
	require.Contains(t, string(failoverErr.ResponseBody), "slow_down")

	retryPayload, ok := OpenAIWSCurrentTurnRetryPayload(serverErr)
	require.True(t, ok)
	require.NotEmpty(t, retryPayload)
	require.False(t, gjson.GetBytes(retryPayload, "previous_response_id").Exists())
	require.Equal(t, "gpt-5.1", gjson.GetBytes(retryPayload, "model").String())
	input := gjson.GetBytes(retryPayload, "input").Array()
	require.Len(t, input, 2)
	require.Equal(t, "first", input[0].Get("content").String())
	require.Equal(t, "second", input[1].Get("content").String())
}

func TestProxyResponsesWebSocketFromClient_RecyclesLeasedIdleConnWithoutReaderPing(t *testing.T) {
	tests := []struct {
		name              string
		storeDisabled     bool
		wantReconnect     bool
		wantPolicyFailure bool
	}{
		{name: "stored_continuation_reconnects", wantReconnect: true},
		{name: "store_disabled_strict_continuation_fails_closed", storeDisabled: true, wantPolicyFailure: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harness := newOpenAIWSIngressCapacityHarness(t, nil)
			firstConn := &openAIWSNoReaderPingCaptureConn{openAIWSCaptureConn: &openAIWSCaptureConn{events: [][]byte{
				[]byte(`{"type":"response.completed","response":{"id":"resp_idle_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
			}}}
			secondConn := &openAIWSCaptureConn{events: [][]byte{
				[]byte(`{"type":"response.completed","response":{"id":"resp_idle_2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
			}}
			dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{firstConn, secondConn}}
			harness.pool.setClientDialerForTest(dialer)

			storeField := ""
			if tt.storeDisabled {
				storeField = `,"store":false`
			}
			harness.write(t, `{"type":"response.create","model":"gpt-5.1","stream":false`+storeField+`,"input":[{"role":"user","content":"first"}]}`)
			firstFrame, readErr := harness.read(t)
			require.NoError(t, readErr)
			require.Equal(t, "resp_idle_1", gjson.GetBytes(firstFrame, "response.id").String())

			ap, ok := harness.pool.getAccountPool(5402)
			require.True(t, ok)
			ap.mu.Lock()
			require.Len(t, ap.conns, 1)
			for _, conn := range ap.conns {
				conn.lastUsedNano.Store(time.Now().Add(-openAIWSConnIdleRecycleAfter - time.Second).UnixNano())
			}
			ap.mu.Unlock()

			harness.write(t, `{"type":"response.create","model":"gpt-5.1","stream":false`+storeField+`,"previous_response_id":"resp_idle_1","input":[{"role":"user","content":"second"}]}`)
			secondFrame, readErr := harness.read(t)
			if tt.wantReconnect {
				require.NoError(t, readErr)
				require.Equal(t, "resp_idle_2", gjson.GetBytes(secondFrame, "response.id").String())
				require.NoError(t, harness.clientConn.Close(coderws.StatusNormalClosure, "done"))
				require.NoError(t, harness.serverErr(t))
				require.Equal(t, 2, dialer.DialCount())
				firstConn.mu.Lock()
				firstWriteCount := len(firstConn.writes)
				firstConn.mu.Unlock()
				require.Equal(t, 1, firstWriteCount, "过期连接不得收到下一轮请求")
				secondConn.mu.Lock()
				secondWrites := append([]map[string]any(nil), secondConn.writes...)
				secondConn.mu.Unlock()
				require.Len(t, secondWrites, 1)
				require.Equal(t, "resp_idle_1", gjson.Get(requestToJSONString(secondWrites[0]), "previous_response_id").String())
				return
			}

			require.True(t, tt.wantPolicyFailure)
			require.Error(t, readErr)
			require.Empty(t, secondFrame)
			serverErr := harness.serverErr(t)
			var closeErr *OpenAIWSClientCloseError
			require.ErrorAs(t, serverErr, &closeErr)
			require.Equal(t, coderws.StatusPolicyViolation, closeErr.StatusCode())
			require.Contains(t, closeErr.Reason(), "expired while idle")
			require.Equal(t, 1, dialer.DialCount(), "严格续链不能静默漂移到新连接")
		})
	}
}
