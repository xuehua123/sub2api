//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const grokModelUnavailableBody = `{"error":{"code":"model_not_found","message":"unknown provider for model grok-4.6"}}`

func clearGrokModelUnavailableTestBlock(t *testing.T, accountID int64, model string) {
	t.Helper()
	key := grokModelQuotaBlockKey(accountID, model)
	clear := func() {
		globalGrokModelQuotaBlocks.mu.Lock()
		delete(globalGrokModelQuotaBlocks.items, key)
		globalGrokModelQuotaBlocks.mu.Unlock()
	}
	clear()
	t.Cleanup(clear)
}

func requireGrokModelUnavailableFailover(t *testing.T, err error, accountID int64, model string) {
	t.Helper()
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadRequest, failoverErr.StatusCode)
	require.False(t, failoverErr.RetryableOnSameAccount)
	require.True(t, isGrokModelQuotaBlocked(accountID, model, time.Now()))
}

func grokModelUnavailableResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestGrokModelProviderUnavailableClassifierIsExactAndGrokOnly(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "structured model not found", body: grokModelUnavailableBody, want: true},
		{name: "nested response error", body: `{"type":"response.failed","response":{"error":{"code":"unknown_provider","message":"unknown provider for model grok-4.6"}}}`, want: true},
		{name: "exact phrase", body: `{"error":{"message":"no available provider for model grok-4.6"}}`, want: true},
		{name: "ordinary validation", body: `{"error":{"message":"input must be a string"}}`},
		{name: "incidental unknown word", body: `{"error":{"message":"unknown request parameter provider"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := classifyGrokUpstreamFailure(http.StatusBadRequest, []byte(tt.body), "grok-4.6")
			require.Equal(t, tt.want, decision.Class == GrokFailureModelUnavailable)
		})
	}

	svc := &OpenAIGatewayService{}
	require.Equal(t, http.StatusBadGateway, openAIWSErrorHTTPStatusFromRaw("model_not_found", ""),
		"the shared OpenAI WS status mapper must not gain Grok-specific semantics")
	require.False(t, svc.shouldFailoverOpenAIUpstreamResponse(
		http.StatusBadRequest,
		"unknown provider for model gpt-5.5",
		[]byte(`{"error":{"code":"model_not_found","message":"unknown provider for model gpt-5.5"}}`),
	), "official OpenAI 400 model_not_found must retain its existing non-failover semantics")
	require.NotEqual(t, GrokFailureModelUnavailable,
		classifyGrokUpstreamFailure(http.StatusInternalServerError, []byte(grokModelUnavailableBody), "grok-4.6").Class,
		"the signature is deliberately limited to upstream 400 responses")
}

func TestForwardGrokResponsesModelUnavailableBlocksMappedModelAndFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const accountID int64 = 98101
	clearGrokModelUnavailableTestBlock(t, accountID, "grok-4.6")
	body := []byte(`{"model":"grok","input":"hi","stream":false}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	account := &Account{
		ID: accountID, Platform: PlatformGrok, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "xai-test", "base_url": "https://api.x.ai/v1",
			"model_mapping": map[string]any{"grok": "grok-4.6"},
		},
	}
	svc := &OpenAIGatewayService{httpUpstream: &httpUpstreamRecorder{resp: grokModelUnavailableResponse(grokModelUnavailableBody)}}

	result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok", false, time.Now())

	require.Nil(t, result)
	requireGrokModelUnavailableFailover(t, err, accountID, "grok-4.6")
	require.False(t, isGrokModelQuotaBlocked(accountID, "grok-4.5", time.Now()))
}

func TestForwardGrokRawChatModelUnavailableFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const accountID int64 = 98102
	clearGrokModelUnavailableTestBlock(t, accountID, "grok-4.6")
	body := []byte(`{"model":"grok","messages":[{"role":"user","content":"hi"}],"stream":false}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	account := &Account{
		ID: accountID, Platform: PlatformGrok, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "xai-test", "base_url": "https://api.x.ai/v1",
			"model_mapping": map[string]any{"grok": "grok-4.6"},
		},
	}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: &httpUpstreamRecorder{resp: grokModelUnavailableResponse(grokModelUnavailableBody)},
	}

	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")

	require.Nil(t, result)
	requireGrokModelUnavailableFailover(t, err, accountID, "grok-4.6")
}

func TestForwardGrokOAuthChatBridgeModelUnavailableFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const accountID int64 = 98103
	clearGrokModelUnavailableTestBlock(t, accountID, "grok-4.5")
	body := []byte(`{"model":"grok","messages":[{"role":"user","content":"hi"}],"stream":false}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: accountID})
	account := grokChatBridgeTestAccount(accountID)
	account.Credentials["model_mapping"] = map[string]any{"grok": "grok-4.5"}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{accountID: account},
	}}
	svc := &OpenAIGatewayService{
		httpUpstream: &httpUpstreamRecorder{resp: grokModelUnavailableResponse(
			`{"error":{"code":"model_not_found","message":"unknown provider for model grok-4.5"}}`,
		)},
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")

	require.Nil(t, result)
	requireGrokModelUnavailableFailover(t, err, accountID, "grok-4.5")
	require.Equal(t, grokChatResponsesEndpoint, GetActualOpenAIUpstreamEndpoint(c))
}

func TestForwardGrokMediaModelUnavailableBlocksMappedUpstreamModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const accountID int64 = 98104
	const upstreamModel = "vendor-image-model"
	clearGrokModelUnavailableTestBlock(t, accountID, upstreamModel)
	body := []byte(`{"model":"grok-imagine","prompt":"draw a cat"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	account := &Account{
		ID: accountID, Platform: PlatformGrok, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "xai-test", "base_url": "https://api.x.ai/v1",
			"model_mapping": map[string]any{"grok-imagine-image-quality": upstreamModel},
		},
	}
	responseBody := `{"error":{"code":"unknown_provider","message":"unknown provider for model vendor-image-model"}}`
	svc := &OpenAIGatewayService{httpUpstream: &httpUpstreamRecorder{resp: grokModelUnavailableResponse(responseBody)}}

	result, err := svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointImagesGenerations, "", body, "application/json")

	require.Nil(t, result)
	requireGrokModelUnavailableFailover(t, err, accountID, upstreamModel)
	require.False(t, isGrokModelQuotaBlocked(accountID, "grok-imagine-image-quality", time.Now()))
}

func TestOpenAIWSHTTPBridgeResponseFailedReconcilesGrokBeforeFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now()
	limitedAt := now.Add(-10 * time.Minute)
	resetAt := now.Add(-time.Minute)
	repo := &grokQuotaAccountRepo{recoveryClearResult: true}
	payload := []byte(`{"type":"response.create","model":"grok-4.6","input":"hi"}`)
	upstreamBody := "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed\",\"status\":\"failed\",\"error\":{\"type\":\"rate_limit_error\",\"code\":\"rate_limit_exceeded\",\"message\":\"rate limited\"}}}\n\n"
	svc := &OpenAIGatewayService{
		accountRepo: repo,
		httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(upstreamBody)),
		}},
	}
	account := &Account{
		ID: 98105, Platform: PlatformGrok, Type: AccountTypeOAuth, Concurrency: 1,
		RateLimitedAt: &limitedAt, RateLimitResetAt: &resetAt,
	}
	writes := 0

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), nil, account, "token", payload, len(payload),
		"grok-4.6", "", "", "", "cache-id", 1,
		func([]byte) error { writes++; return nil },
	)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Zero(t, writes)
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Zero(t, repo.recoveryClearCalls, "HTTP 200 must not clear cooldown before response.failed is parsed")
}

func TestOpenAIWSHTTPBridgeModelUnavailableIsGrokOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload := []byte(`{"type":"response.create","model":"grok-4.6","input":"hi"}`)
	failedBody := "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed\",\"status\":\"failed\",\"error\":{\"code\":\"model_not_found\",\"message\":\"unknown provider for model grok-4.6\"}}}\n\n"

	t.Run("Grok fails over and blocks the model", func(t *testing.T) {
		const accountID int64 = 98106
		clearGrokModelUnavailableTestBlock(t, accountID, "grok-4.6")
		svc := &OpenAIGatewayService{httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(failedBody)),
		}}}
		account := &Account{ID: accountID, Platform: PlatformGrok, Type: AccountTypeAPIKey, Concurrency: 1}
		result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
			context.Background(), nil, account, "token", payload, len(payload), "grok-4.6", "", "", "", "", 1,
			func([]byte) error { return nil },
		)
		require.Nil(t, result)
		requireGrokModelUnavailableFailover(t, err, accountID, "grok-4.6")
	})

	t.Run("official OpenAI retains terminal event semantics", func(t *testing.T) {
		svc := &OpenAIGatewayService{httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(failedBody)),
		}}}
		account := &Account{ID: 98107, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1}
		writes := 0
		result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
			context.Background(), nil, account, "token", payload, len(payload), "grok-4.6", "", "", "", "", 1,
			func([]byte) error { writes++; return nil },
		)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, 1, writes)
		var failoverErr *UpstreamFailoverError
		require.False(t, errors.As(err, &failoverErr))
	})
}

func TestOpenAIWSHTTPBridgeErrorEventModelUnavailableUsesGrokScopedClassifier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const accountID int64 = 98108
	clearGrokModelUnavailableTestBlock(t, accountID, "grok-4.6")
	payload := []byte(`{"type":"response.create","model":"grok-4.6","input":"hi"}`)
	upstreamBody := "data: {\"type\":\"error\",\"error\":{\"code\":\"unknown_provider\",\"message\":\"unknown provider for model grok-4.6\"}}\n\n"
	svc := &OpenAIGatewayService{httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(upstreamBody)),
	}}}
	account := &Account{ID: accountID, Platform: PlatformGrok, Type: AccountTypeAPIKey, Concurrency: 1}

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), nil, account, "token", payload, len(payload), "grok-4.6", "", "", "", "", 1,
		func([]byte) error { return nil },
	)

	require.Nil(t, result)
	requireGrokModelUnavailableFailover(t, err, accountID, "grok-4.6")
}

func TestDoGrokNativeResponsesJSONAppliesAccountTransportPolicy(t *testing.T) {
	account := grokNativeSearchPoolAccount(false)
	account.Extra = map[string]any{
		AccountExtraOpenAIHTTPProtocol:  OpenAIHTTPProtocolOverrideH1,
		AccountExtraUpstreamGzipEnabled: false,
	}
	upstream := &grokNativeSearchUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"output":[]}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	_, err := svc.DoGrokNativeResponsesJSON(context.Background(), account, []byte(`{"model":"grok-4.5","input":"query"}`))

	require.NoError(t, err)
	requireOpenAIUpstreamPolicyContext(t, upstream.request, OpenAIHTTPProtocolOverrideH1, false)
}
