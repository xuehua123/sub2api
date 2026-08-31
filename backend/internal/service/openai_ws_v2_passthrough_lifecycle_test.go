package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type stagedPassthroughFrame struct {
	messageType coderws.MessageType
	payload     []byte
	err         error
}

type stagedPassthroughConn struct {
	frames    chan stagedPassthroughFrame
	writes    chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

func newStagedPassthroughConn() *stagedPassthroughConn {
	return &stagedPassthroughConn{
		frames: make(chan stagedPassthroughFrame, 4),
		writes: make(chan []byte, 4),
		closed: make(chan struct{}),
	}
}

func (c *stagedPassthroughConn) Send(payload string) {
	c.frames <- stagedPassthroughFrame{messageType: coderws.MessageText, payload: []byte(payload)}
}

func (c *stagedPassthroughConn) Fail(err error) {
	c.frames <- stagedPassthroughFrame{err: err}
}

func (c *stagedPassthroughConn) WriteJSON(context.Context, any) error { return nil }

func (c *stagedPassthroughConn) ReadMessage(ctx context.Context) ([]byte, error) {
	_, payload, err := c.ReadFrame(ctx)
	return payload, err
}

func (c *stagedPassthroughConn) Ping(context.Context) error { return nil }

func (c *stagedPassthroughConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return coderws.MessageText, nil, ctx.Err()
	case <-c.closed:
		return coderws.MessageText, nil, errOpenAIWSConnClosed
	case frame := <-c.frames:
		return frame.messageType, append([]byte(nil), frame.payload...), frame.err
	}
}

func (c *stagedPassthroughConn) WriteFrame(ctx context.Context, _ coderws.MessageType, payload []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		return errOpenAIWSConnClosed
	default:
	}
	var parsed any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return err
	}
	select {
	case c.writes <- append([]byte(nil), payload...):
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		return errOpenAIWSConnClosed
	}
	return nil
}

func (c *stagedPassthroughConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

type stagedPassthroughDialer struct {
	conn openAIWSClientConn
}

func (d *stagedPassthroughDialer) Dial(context.Context, string, http.Header, string) (openAIWSClientConn, int, http.Header, error) {
	return d.conn, http.StatusSwitchingProtocols, http.Header{}, nil
}

type stagedPassthroughSequenceDialer struct {
	mu               sync.Mutex
	conns            []openAIWSClientConn
	handshakeHeaders []http.Header
	requestHeaders   []http.Header
}

func (d *stagedPassthroughSequenceDialer) Dial(_ context.Context, _ string, headers http.Header, _ string) (openAIWSClientConn, int, http.Header, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	index := len(d.requestHeaders)
	d.requestHeaders = append(d.requestHeaders, cloneHeader(headers))
	if index >= len(d.conns) {
		return nil, 0, nil, errors.New("unexpected passthrough websocket dial")
	}
	handshakeHeaders := http.Header{}
	if index < len(d.handshakeHeaders) {
		handshakeHeaders = cloneHeader(d.handshakeHeaders[index])
	}
	return d.conns[index], http.StatusSwitchingProtocols, handshakeHeaders, nil
}

func (d *stagedPassthroughSequenceDialer) requestHeader(index int) http.Header {
	d.mu.Lock()
	defer d.mu.Unlock()
	if index < 0 || index >= len(d.requestHeaders) {
		return nil
	}
	return cloneHeader(d.requestHeaders[index])
}

func newPassthroughLifecycleService(cfg *config.Config, upstream *stagedPassthroughConn) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg:                       cfg,
		httpUpstream:              &httpUpstreamRecorder{},
		cache:                     &stubGatewayCache{},
		openaiWSResolver:          NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:             NewCodexToolCorrector(),
		openaiWSPassthroughDialer: &stagedPassthroughDialer{conn: upstream},
	}
}

func passthroughLifecycleConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 1
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 1
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 1
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	return cfg
}

func passthroughLifecycleAccount() *Account {
	return &Account{
		ID:          901,
		Name:        "passthrough-lifecycle",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
		},
	}
}

func startPassthroughLifecycleServer(
	t *testing.T,
	controlCtx context.Context,
	svc *OpenAIGatewayService,
	account *Account,
) (*httptest.Server, <-chan error) {
	return startPassthroughLifecycleServerWithAPIKeyAndHooks(t, controlCtx, svc, account, nil, nil)
}

func startPassthroughLifecycleServerWithAPIKey(
	t *testing.T,
	controlCtx context.Context,
	svc *OpenAIGatewayService,
	account *Account,
	apiKey *APIKey,
) (*httptest.Server, <-chan error) {
	return startPassthroughLifecycleServerWithAPIKeyAndHooks(t, controlCtx, svc, account, apiKey, nil)
}

func startPassthroughLifecycleServerWithHooks(
	t *testing.T,
	controlCtx context.Context,
	svc *OpenAIGatewayService,
	account *Account,
	hooksFactory func(*gin.Context) *OpenAIWSIngressHooks,
) (*httptest.Server, <-chan error) {
	return startPassthroughLifecycleServerWithAPIKeyAndHooks(t, controlCtx, svc, account, nil, hooksFactory)
}

func startPassthroughLifecycleServerWithAPIKeyAndHooks(
	t *testing.T,
	controlCtx context.Context,
	svc *OpenAIGatewayService,
	account *Account,
	apiKey *APIKey,
	hooksFactory func(*gin.Context) *OpenAIWSIngressHooks,
) (*httptest.Server, <-chan error) {
	t.Helper()
	serverErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			serverErr <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()

		msgType, firstMessage, err := ReadOpenAIWSClientMessage(
			controlCtx,
			conn,
			3*time.Second,
			coderws.StatusPolicyViolation,
			"missing first response.create message",
		)
		if err != nil {
			serverErr <- err
			return
		}
		if msgType != coderws.MessageText {
			serverErr <- errors.New("first message was not text")
			return
		}

		recorder := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(recorder)
		req := r.Clone(controlCtx)
		req.Header = req.Header.Clone()
		ginCtx.Request = req
		if apiKey != nil {
			ginCtx.Set("api_key", apiKey)
		}
		var hooks *OpenAIWSIngressHooks
		if hooksFactory != nil {
			hooks = hooksFactory(ginCtx)
		}
		serverErr <- svc.ProxyResponsesWebSocketFromClient(controlCtx, ginCtx, conn, account, "sk-test", firstMessage, hooks)
	}))
	return server, serverErr
}

func TestPassthroughLifecycle_CyberTerminalEventsMarkBeforeAfterTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		events      []string
		wantBody    string
		wantMessage string
		wantInput   int
		wantOutput  int
	}{
		{
			name: "error",
			events: []string{
				`{"type":"error","error":{"code":"cyber_policy","message":"blocked by error event"},"usage":{"input_tokens":5,"output_tokens":1}}`,
				`{"type":"response.failed","response":{"id":"resp_error","error":{"code":"cyber_policy","message":"blocked by paired failed event"},"usage":{"input_tokens":9,"output_tokens":2}}}`,
			},
			wantBody:    `"type":"error"`,
			wantMessage: "blocked by error event",
			wantInput:   5,
			wantOutput:  1,
		},
		{
			name: "response_failed",
			events: []string{
				`{"type":"response.failed","response":{"id":"resp_failed","error":{"code":"cyber_policy","message":"blocked by failed event"},"usage":{"input_tokens":9,"output_tokens":2}}}`,
			},
			wantBody:    `"type":"response.failed"`,
			wantMessage: "blocked by failed event",
			wantInput:   9,
			wantOutput:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controlCtx, cancelControl := context.WithCancelCause(context.Background())
			defer cancelControl(context.Canceled)
			upstream := newStagedPassthroughConn()
			for _, event := range tt.events {
				upstream.Send(event)
			}

			markSeen := make(chan CyberPolicyMark, 1)
			afterTurnCalls := atomic.Int32{}
			server, serverErr := startPassthroughLifecycleServerWithHooks(
				t,
				controlCtx,
				newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream),
				passthroughLifecycleAccount(),
				func(c *gin.Context) *OpenAIWSIngressHooks {
					return &OpenAIWSIngressHooks{AfterTurn: func(_ int, _ *OpenAIForwardResult, _ error) {
						afterTurnCalls.Add(1)
						if mark := GetOpsCyberPolicy(c); mark != nil {
							select {
							case markSeen <- *mark:
							default:
							}
						}
					}}
				},
			)
			defer server.Close()
			clientConn := dialPassthroughLifecycleClient(t, server)
			defer func() { _ = clientConn.CloseNow() }()

			for range tt.events {
				_, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
				require.NoError(t, err)
			}

			select {
			case mark := <-markSeen:
				require.Equal(t, "cyber_policy", mark.Code)
				require.Equal(t, tt.wantMessage, mark.Message)
				require.Contains(t, mark.Body, tt.wantBody)
				require.Equal(t, http.StatusOK, mark.UpstreamStatus)
				require.Equal(t, tt.wantInput, mark.UpstreamInTok)
				require.Equal(t, tt.wantOutput, mark.UpstreamOutTok)
			case <-time.After(3 * time.Second):
				t.Fatal("cyber mark was not visible to AfterTurn")
			}
			require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
			select {
			case <-serverErr:
			case <-time.After(3 * time.Second):
				t.Fatal("cyber passthrough test did not exit")
			}
			require.Equal(t, int32(1), afterTurnCalls.Load(), "error/response.failed pair must complete and record once")
		})
	}
}

func TestPassthroughLifecycle_NonCyberFailureKeepsAccountSideEffects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.failed","response":{"id":"resp_non_cyber","error":{"type":"authentication_error","code":"invalid_api_key","status_code":401,"message":"credential rejected"},"usage":{"input_tokens":3,"output_tokens":1}}}`)
	repo := &openAIStream403AccountRepo{}
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)
	svc.rateLimitService = NewRateLimitService(repo, nil, svc.cfg, nil, nil)
	account := passthroughLifecycleAccount()

	markSeen := make(chan *CyberPolicyMark, 1)
	server, serverErr := startPassthroughLifecycleServerWithHooks(
		t,
		controlCtx,
		svc,
		account,
		func(c *gin.Context) *OpenAIWSIngressHooks {
			return &OpenAIWSIngressHooks{AfterTurn: func(_ int, _ *OpenAIForwardResult, _ error) {
				markSeen <- GetOpsCyberPolicy(c)
			}}
		},
	)
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.failed", gjson.GetBytes(event, "type").String())
	select {
	case mark := <-markSeen:
		require.Nil(t, mark)
	case <-time.After(3 * time.Second):
		t.Fatal("non-cyber terminal event did not complete its turn")
	}
	require.Equal(t, 1, repo.setErrorCalls, "non-cyber credential failure must retain account failure side effects")
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("non-cyber passthrough test did not exit")
	}
}

func TestPassthroughLifecycle_CyberSkipsFailureAccountSideEffects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.failed","response":{"id":"resp_cyber_auth","error":{"type":"authentication_error","code":"cyber_policy","status_code":401,"message":"request blocked"}}}`)
	repo := &openAIStream403AccountRepo{}
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream)
	svc.rateLimitService = NewRateLimitService(repo, nil, svc.cfg, nil, nil)
	account := passthroughLifecycleAccount()

	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, svc, account)
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.failed", gjson.GetBytes(event, "type").String())
	require.Zero(t, repo.setErrorCalls, "cyber_policy is request-scoped and must not cool down the account")
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("cyber side-effect test did not exit")
	}
}

func TestPassthroughLifecycle_CloseReasonTruncationPreservesUTF8(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	originalReason := strings.Repeat("a", 119) + "界"
	upstream.Fail(NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, originalReason, errors.New("policy rejected")))

	server, serverErr := startPassthroughLifecycleServer(
		t,
		controlCtx,
		newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream),
		passthroughLifecycleAccount(),
	)
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	_, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
	require.True(t, utf8.ValidString(closeErr.Reason))
	require.LessOrEqual(t, len(closeErr.Reason), 120)
	require.Equal(t, strings.Repeat("a", 119), closeErr.Reason)

	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough close reason test did not exit")
	}
}

func dialPassthroughLifecycleClient(t *testing.T, server *httptest.Server) *coderws.Conn {
	return dialPassthroughLifecycleClientWithHeaders(t, server, nil)
}

func dialPassthroughLifecycleClientWithHeaders(t *testing.T, server *httptest.Server, headers http.Header) *coderws.Conn {
	t.Helper()
	return dialPassthroughLifecycleClientWithHeadersAndPayload(t, server, headers, `{"type":"response.create","model":"gpt-5.1","stream":false}`)
}

func dialPassthroughLifecycleClientWithPayload(t *testing.T, server *httptest.Server, payload string) *coderws.Conn {
	t.Helper()
	return dialPassthroughLifecycleClientWithHeadersAndPayload(t, server, nil, payload)
}

func dialPassthroughLifecycleClientWithHeadersAndPayload(t *testing.T, server *httptest.Server, headers http.Header, payload string) *coderws.Conn {
	t.Helper()
	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	var dialOptions *coderws.DialOptions
	if headers != nil {
		dialOptions = &coderws.DialOptions{HTTPHeader: cloneHeader(headers)}
	}
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), dialOptions)
	cancelDial()
	require.NoError(t, err)
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(payload))
	cancelWrite()
	require.NoError(t, err)
	return clientConn
}

func readPassthroughLifecycleFrame(t *testing.T, clientConn *coderws.Conn, timeout time.Duration) ([]byte, error) {
	t.Helper()
	readCtx, cancelRead := context.WithTimeout(context.Background(), timeout)
	_, payload, err := clientConn.Read(readCtx)
	cancelRead()
	return payload, err
}

func requirePassthroughUpstreamWrite(t *testing.T, upstream *stagedPassthroughConn, timeout time.Duration) []byte {
	t.Helper()
	select {
	case payload := <-upstream.writes:
		return payload
	case <-time.After(timeout):
		t.Fatal("passthrough request was not forwarded upstream")
		return nil
	}
}

func TestPassthroughLifecycle_CompatibleEndpointFiltersNoneAndTracksCurrentFrameEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)

	upstream := newStagedPassthroughConn()
	account := passthroughLifecycleAccount()
	account.Credentials["base_url"] = "https://compat.example/v1"
	afterTurns := make(chan *OpenAIForwardResult, 3)
	server, serverErr := startPassthroughLifecycleServerWithHooks(
		t,
		controlCtx,
		newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream),
		account,
		func(*gin.Context) *OpenAIWSIngressHooks {
			return &OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, err error) {
				require.NoError(t, err)
				afterTurns <- result
			}}
		},
	)
	defer server.Close()

	clientConn := dialPassthroughLifecycleClientWithPayload(
		t,
		server,
		`{"type":"response.create","model":"company-model-low","reasoning":{"effort":"none","summary":"auto"},"stream":false}`,
	)
	defer func() { _ = clientConn.CloseNow() }()

	assertForwarded := func(wantModel string, wantSummary bool) {
		t.Helper()
		forwarded := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
		require.Equal(t, wantModel, gjson.GetBytes(forwarded, "model").String())
		require.False(t, gjson.GetBytes(forwarded, "reasoning.effort").Exists())
		require.False(t, gjson.GetBytes(forwarded, "reasoning_effort").Exists())
		require.Equal(t, wantSummary, gjson.GetBytes(forwarded, "reasoning.summary").Exists())
	}
	completeTurn := func(id, model string) *OpenAIForwardResult {
		t.Helper()
		upstream.Send(fmt.Sprintf(`{"type":"response.completed","response":{"id":%q,"model":%q,"usage":{"input_tokens":1,"output_tokens":1}}}`, id, model))
		payload, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
		require.NoError(t, err)
		require.Equal(t, id, gjson.GetBytes(payload, "response.id").String())
		select {
		case result := <-afterTurns:
			require.NotNil(t, result)
			return result
		case <-time.After(3 * time.Second):
			t.Fatal("passthrough turn result was not reported")
			return nil
		}
	}
	writeTurn := func(payload string) {
		t.Helper()
		writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
		err := clientConn.Write(writeCtx, coderws.MessageText, []byte(payload))
		cancelWrite()
		require.NoError(t, err)
	}

	assertForwarded("company-model-low", true)
	first := completeTurn("resp_none_first", "company-model-low")
	require.NotNil(t, first.RequestedReasoningEffort)
	require.Equal(t, "none", *first.RequestedReasoningEffort)

	writeTurn(`{"type":"response.create","model":"company-model-high","reasoning_effort":"none","stream":false}`)
	assertForwarded("company-model-high", false)
	second := completeTurn("resp_none_second", "company-model-high")
	require.NotNil(t, second.RequestedReasoningEffort)
	require.Equal(t, "none", *second.RequestedReasoningEffort)

	writeTurn(`{"type":"response.create","model":"company-model-xhigh","stream":false}`)
	assertForwarded("company-model-xhigh", false)
	third := completeTurn("resp_suffix_third", "company-model-xhigh")
	require.NotNil(t, third.RequestedReasoningEffort)
	require.Equal(t, "xhigh", *third.RequestedReasoningEffort, "current frame suffix must override the first turn's low suffix")

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case err := <-serverErr:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough reasoning test did not exit")
	}
}

func TestPassthroughLifecycle_MultiTurnFailureTargetsCurrentModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), newStagedPassthroughConn())
	svc.rateLimitService = NewRateLimitService(transientCooldownAccountRepo{}, nil, svc.cfg, nil, nil)
	account := passthroughLifecycleAccount()

	runFailureSession := func(responseID string) {
		t.Helper()
		controlCtx, cancelControl := context.WithCancelCause(context.Background())
		defer cancelControl(context.Canceled)
		upstream := newStagedPassthroughConn()
		svc.openaiWSPassthroughDialer = &stagedPassthroughDialer{conn: upstream}
		server, serverErr := startPassthroughLifecycleServer(t, controlCtx, svc, account)
		defer server.Close()
		clientConn := dialPassthroughLifecycleClientWithPayload(t, server, `{"type":"response.create","model":"model-a-low","stream":false}`)
		defer func() { _ = clientConn.CloseNow() }()

		firstForwarded := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
		require.Equal(t, "model-a-low", gjson.GetBytes(firstForwarded, "model").String())
		upstream.Send(fmt.Sprintf(`{"type":"response.completed","response":{"id":%q,"model":"model-a-low","usage":{"input_tokens":1,"output_tokens":1}}}`, responseID+"_ok"))
		_, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
		require.NoError(t, err)

		writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
		err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"model-b-high","stream":false}`))
		cancelWrite()
		require.NoError(t, err)
		secondForwarded := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
		require.Equal(t, "model-b-high", gjson.GetBytes(secondForwarded, "model").String())

		upstream.Send(fmt.Sprintf(`{"type":"response.failed","response":{"id":%q,"model":"model-b-high","error":{"status_code":500,"code":"server_error","type":"server_error","message":"temporary failure"},"usage":{"input_tokens":1,"output_tokens":0}}}`, responseID+"_failed"))
		failedPayload, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
		require.NoError(t, err)
		require.Equal(t, "response.failed", gjson.GetBytes(failedPayload, "type").String())

		require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
		select {
		case <-serverErr:
		case <-time.After(3 * time.Second):
			t.Fatal("passthrough failure attribution test did not exit")
		}
	}

	runFailureSession("resp_failure_1")
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "model-a-low"))
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "model-b-high"))
	runFailureSession("resp_failure_2")
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "model-a-low"), "stale session model must not receive the current turn failure")
	require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "model-b-high"), "current failed model must receive the transient cooldown")
}

func TestOpenAIWSPassthroughHandshakeTurnState_RequiresSuccessfulTerminalClientWrite(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg:   passthroughLifecycleConfig(),
		cache: &stubGatewayCache{},
	}
	account := passthroughLifecycleAccount()
	const apiKeyID int64 = 981
	c, _ := newTurnStateTestContext(t, apiKeyID, "passthrough-terminal-write")
	store := svc.getOpenAIWSStateStore()
	sessionHash := svc.GenerateSessionHash(c, nil)
	require.NotEmpty(t, sessionHash)

	var commitAttempted atomic.Bool
	require.False(t, svc.commitOpenAIWSPassthroughHandshakeTurnStateAfterClientWrite(
		c,
		account,
		store,
		0,
		sessionHash,
		"state-not-delivered",
		errors.New("downstream websocket write failed"),
		&commitAttempted,
	))
	_, found := store.GetSessionTurnState(0, apiKeyID, account.ID, sessionHash)
	require.False(t, found, "a failed terminal client write must not populate the replay cache")
	key, ok := openAICodexTurnStateProvenanceKeyFor(c, "state-not-delivered")
	require.True(t, ok)
	_, found = svc.openaiCodexTurnStateOrigins.Load(key)
	require.False(t, found, "a failed terminal client write must not record blob provenance")

	require.True(t, svc.commitOpenAIWSPassthroughHandshakeTurnStateAfterClientWrite(
		c,
		account,
		store,
		0,
		sessionHash,
		"state-delivered",
		nil,
		&commitAttempted,
	))
	state, found := store.GetSessionTurnState(0, apiKeyID, account.ID, sessionHash)
	require.True(t, found)
	require.Equal(t, "state-delivered", state)

	require.False(t, svc.commitOpenAIWSPassthroughHandshakeTurnStateAfterClientWrite(
		c,
		account,
		store,
		0,
		sessionHash,
		"state-from-a-later-turn",
		nil,
		&commitAttempted,
	))
	state, found = store.GetSessionTurnState(0, apiKeyID, account.ID, sessionHash)
	require.True(t, found)
	require.Equal(t, "state-delivered", state, "one upstream handshake state may commit only once")
}

func TestPassthroughTurnState_ReconnectLoadsCommittedHandshakeState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)

	firstUpstream := newStagedPassthroughConn()
	firstUpstream.Send(`{"type":"response.completed","response":{"id":"resp_turn_state_first","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	secondUpstream := newStagedPassthroughConn()
	secondUpstream.Send(`{"type":"response.completed","response":{"id":"resp_turn_state_second","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	firstHandshake := http.Header{}
	firstHandshake.Set(openAIWSTurnStateHeader, "passthrough-handshake-state")
	dialer := &stagedPassthroughSequenceDialer{
		conns:            []openAIWSClientConn{firstUpstream, secondUpstream},
		handshakeHeaders: []http.Header{firstHandshake, {}},
	}
	svc := newPassthroughLifecycleService(passthroughLifecycleConfig(), firstUpstream)
	svc.openaiWSPassthroughDialer = dialer
	account := passthroughLifecycleAccount()
	apiKey := &APIKey{ID: 982}
	clientHeaders := http.Header{}
	clientHeaders.Set("session_id", "passthrough-reconnect-session")

	firstServer, firstServerErr := startPassthroughLifecycleServerWithAPIKey(t, controlCtx, svc, account, apiKey)
	firstClient := dialPassthroughLifecycleClientWithHeaders(t, firstServer, clientHeaders)
	firstEvent, err := readPassthroughLifecycleFrame(t, firstClient, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(firstEvent, "type").String())
	require.NoError(t, firstClient.CloseNow())
	select {
	case <-firstServerErr:
	case <-time.After(3 * time.Second):
		t.Fatal("first passthrough turn did not exit after downstream close")
	}
	firstServer.Close()
	require.Empty(t, dialer.requestHeader(0).Get(openAIWSTurnStateHeader))

	secondServer, secondServerErr := startPassthroughLifecycleServerWithAPIKey(t, controlCtx, svc, account, apiKey)
	defer secondServer.Close()
	secondClient := dialPassthroughLifecycleClientWithHeaders(t, secondServer, clientHeaders)
	secondEvent, err := readPassthroughLifecycleFrame(t, secondClient, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(secondEvent, "type").String())
	require.NoError(t, secondClient.CloseNow())
	select {
	case <-secondServerErr:
	case <-time.After(3 * time.Second):
		t.Fatal("reconnected passthrough turn did not exit after downstream close")
	}

	require.Equal(t, "passthrough-handshake-state", dialer.requestHeader(1).Get(openAIWSTurnStateHeader))
}

func TestPassthroughLifecycle_ResponsesLiteFirstFramePinsParallelToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_lite","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClientWithPayload(t, server, `{
		"type":"response.create","model":"gpt-5.1","stream":false,
		"parallel_tool_calls":true,
		"client_metadata":{"ws_request_header_x_openai_internal_codex_responses_lite":"true"}
	}`)
	defer func() { _ = clientConn.CloseNow() }()

	upstreamBody := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Equal(t, gjson.False, gjson.GetBytes(upstreamBody, "parallel_tool_calls").Type, string(upstreamBody))

	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("Lite 首帧测试等待 passthrough 退出超时")
	}
}

func TestOpenAIWSPassthroughTurnLifecycle_SerializesTerminalCommitAndNextTurn(t *testing.T) {
	clientFrameConn := &openAIWSClientFrameConn{interTurnStarted: make(chan struct{}, 1)}
	clientFrameConn.markTurnCompleted()
	lifecycle := newOpenAIWSPassthroughTurnLifecycle(true)
	lifecycle.beginTerminalWrite()

	admitted := make(chan bool, 1)
	go func() {
		admitted <- lifecycle.beginResponseCreate(clientFrameConn.markTurnStarted)
	}()
	select {
	case <-admitted:
		t.Fatal("next response.create was admitted before terminal commit completed")
	case <-time.After(50 * time.Millisecond):
	}

	lifecycle.finishTerminalWrite(true, clientFrameConn.markTurnCompleted)
	select {
	case ok := <-admitted:
		require.True(t, ok)
	case <-time.After(time.Second):
		t.Fatal("next response.create remained blocked after terminal commit")
	}
	require.False(t, clientFrameConn.waitingForNextTurn.Load(), "accepted next turn must win over terminal idle state")

	lifecycle = newOpenAIWSPassthroughTurnLifecycle(true)
	lifecycle.beginTerminalWrite()
	admitted = make(chan bool, 1)
	go func() {
		admitted <- lifecycle.beginResponseCreate(nil)
	}()
	lifecycle.finishTerminalWrite(false, func() {
		t.Error("failed terminal write must not commit idle state")
	})
	require.False(t, <-admitted, "failed terminal write must keep the current turn in flight")
}

func TestPassthroughLifecycle_LeaseLossSendsRetryClose(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.created","response":{"id":"resp_lease","model":"gpt-5.1"}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.created", gjson.GetBytes(event, "type").String())
	cancelControl(ErrOpenAIWSIngressLeaseLost)

	_, err = readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusTryAgainLater, closeErr.Code)
	require.Equal(t, "websocket ingress capacity lease lost; please reconnect", closeErr.Reason)
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough lease-loss reader did not exit")
	}
}

func TestPassthroughLifecycle_CompletedTurnStartsInterTurnIdle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_idle","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
	_, err = readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusNormalClosure, closeErr.Code)
	require.Equal(t, "websocket idle timeout", closeErr.Reason)
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough idle reader did not exit")
	}
}

func TestPassthroughLifecycle_ActiveTurnInactivityUsesReadTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.output_text.delta","response_id":"resp_active","delta":"hello"}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	delta, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.output_text.delta", gjson.GetBytes(delta, "type").String())
	_, err = readPassthroughLifecycleFrame(t, clientConn, 2500*time.Millisecond)
	var websocketCloseErr coderws.CloseError
	require.ErrorAs(t, err, &websocketCloseErr)
	require.Equal(t, coderws.StatusGoingAway, websocketCloseErr.Code)
	require.Equal(t, "upstream websocket read timeout; please reconnect", websocketCloseErr.Reason)
	select {
	case err := <-serverErr:
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, coderws.StatusGoingAway, closeErr.StatusCode())
		require.Equal(t, "upstream websocket read timeout; please reconnect", closeErr.Reason())
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("passthrough active turn remained unbounded after upstream activity stopped")
	}
}

func TestPassthroughLifecycle_PreambleAllowsPromptClientCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 3
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.created","response":{"id":"resp_cancel","model":"gpt-5.1"}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(cfg, upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()
	require.Equal(t, "response.create", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, time.Second), "type").String())

	created, err := readPassthroughLifecycleFrame(t, clientConn, time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.cancel","response_id":"resp_cancel"}`))
	cancelWrite()
	require.NoError(t, err)
	cancelFrame := requirePassthroughUpstreamWrite(t, upstream, 500*time.Millisecond)
	require.Equal(t, "response.cancel", gjson.GetBytes(cancelFrame, "type").String())

	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough cancel test did not exit")
	}
}

func TestPassthroughLifecycle_RejectsOverlappingResponseCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 3
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.created","response":{"id":"resp_overlap_first","model":"gpt-5.1"}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(cfg, upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()
	require.Equal(t, "response.create", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, time.Second), "type").String())

	created, err := readPassthroughLifecycleFrame(t, clientConn, time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1"}`))
	cancelWrite()
	require.NoError(t, err)

	_, err = readPassthroughLifecycleFrame(t, clientConn, time.Second)
	var websocketCloseErr coderws.CloseError
	require.ErrorAs(t, err, &websocketCloseErr)
	require.Equal(t, coderws.StatusPolicyViolation, websocketCloseErr.Code)
	require.Equal(t, "overlapping response.create is not supported", websocketCloseErr.Reason)
	select {
	case err := <-serverErr:
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, coderws.StatusPolicyViolation, closeErr.StatusCode())
		require.Equal(t, "overlapping response.create is not supported", closeErr.Reason())
	case <-time.After(3 * time.Second):
		t.Fatal("overlapping response.create did not terminate passthrough")
	}
}

func TestPassthroughLifecycle_ActiveTurnActivityRefreshesReadTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.output_text.delta","response_id":"resp_active_refresh","delta":"one"}`)
	go func() {
		for _, event := range []string{
			`{"type":"response.output_text.delta","response_id":"resp_active_refresh","delta":"two"}`,
			`{"type":"response.output_text.delta","response_id":"resp_active_refresh","delta":"three"}`,
			`{"type":"response.completed","response":{"id":"resp_active_refresh","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":3}}}`,
		} {
			timer := time.NewTimer(600 * time.Millisecond)
			<-timer.C
			timer.Stop()
			upstream.Send(event)
		}
	}()
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	for _, wantType := range []string{
		"response.output_text.delta",
		"response.output_text.delta",
		"response.output_text.delta",
		"response.completed",
	} {
		frame, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
		require.NoError(t, err)
		require.Equal(t, wantType, gjson.GetBytes(frame, "type").String())
	}
	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough active-turn refresh test did not exit")
	}
}

func TestPassthroughLifecycle_TerminalSwitchesToInterTurnIdleTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 1
	cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 2
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_idle_first","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(cfg, upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()
	require.Equal(t, "response.create", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, 3*time.Second), "type").String())

	completed, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "resp_idle_first", gjson.GetBytes(completed, "response.id").String())
	time.Sleep(1300 * time.Millisecond)
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_idle_first"}`))
	cancelWrite()
	require.NoError(t, err)
	require.Equal(t, "response.create", gjson.GetBytes(requirePassthroughUpstreamWrite(t, upstream, 3*time.Second), "type").String())
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_idle_second","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	completed, err = readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "resp_idle_second", gjson.GetBytes(completed, "response.id").String())
	_, err = readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	var websocketCloseErr coderws.CloseError
	require.ErrorAs(t, err, &websocketCloseErr)
	require.Equal(t, coderws.StatusNormalClosure, websocketCloseErr.Code)
	require.Equal(t, "websocket idle timeout", websocketCloseErr.Reason)

	select {
	case err := <-serverErr:
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, coderws.StatusNormalClosure, closeErr.StatusCode())
		require.Equal(t, "websocket idle timeout", closeErr.Reason())
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough terminal turn did not use inter-turn idle timeout")
	}
}

func TestPassthroughLifecycle_FirstOutputTimeoutRemainsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	select {
	case err := <-serverErr:
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, http.StatusGatewayTimeout, failoverErr.StatusCode)
		require.Contains(t, string(failoverErr.ResponseBody), "first_output_timeout")
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("passthrough first output was left unbounded")
	}
}

func TestPassthroughLifecycle_ResponseCreatedTimeoutClosesWithoutFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.created","response":{"id":"resp_preamble","model":"gpt-5.1"}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	created, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	_, err = readPassthroughLifecycleFrame(t, clientConn, 2500*time.Millisecond)
	var websocketCloseErr coderws.CloseError
	require.ErrorAs(t, err, &websocketCloseErr)
	require.Equal(t, coderws.StatusGoingAway, websocketCloseErr.Code)
	require.Equal(t, "upstream produced no semantic output; please reconnect", websocketCloseErr.Reason)
	select {
	case err := <-serverErr:
		var failoverErr *UpstreamFailoverError
		require.NotErrorAs(t, err, &failoverErr)
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, coderws.StatusGoingAway, closeErr.StatusCode())
		require.Equal(t, "upstream produced no semantic output; please reconnect", closeErr.Reason())
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("response.created timeout did not close the passthrough connection")
	}
}

func TestPassthroughLifecycle_SecondTurnTimeoutIsNotFailoverSafe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_first","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	completed, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_first"}`))
	cancelWrite()
	require.NoError(t, err)
	upstream.Send(`{"type":"response.created","response":{"id":"resp_second","model":"gpt-5.1"}}`)

	created, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
	_, err = readPassthroughLifecycleFrame(t, clientConn, 2500*time.Millisecond)
	var websocketCloseErr coderws.CloseError
	require.ErrorAs(t, err, &websocketCloseErr)
	require.Equal(t, coderws.StatusGoingAway, websocketCloseErr.Code)
	require.Equal(t, "upstream produced no semantic output; please reconnect", websocketCloseErr.Reason)
	select {
	case err := <-serverErr:
		var failoverErr *UpstreamFailoverError
		require.NotErrorAs(t, err, &failoverErr, "handler must not replay the initial request on another account for a later-turn timeout")
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, coderws.StatusGoingAway, closeErr.StatusCode())
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("second turn first semantic output was left unbounded")
	}
}
