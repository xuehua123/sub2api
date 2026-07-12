package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIChatCompletionsHTTPUpstreamStub struct {
	statusCode int
	body       string
	header     http.Header
	calls      int
}

func (s *openAIChatCompletionsHTTPUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	s.calls++
	statusCode := s.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	header := s.header.Clone()
	if header == nil {
		header = make(http.Header)
	}
	return &http.Response{
		StatusCode: statusCode,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Request:    req,
	}, nil
}

func (s *openAIChatCompletionsHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

type openAIChatCompletionsUsageBillingRepoStub struct {
	service.UsageBillingRepository
	err   error
	calls int
}

func (s *openAIChatCompletionsUsageBillingRepoStub) Apply(ctx context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return &service.UsageBillingApplyResult{Applied: true}, nil
}

type openAIChatCompletionsUsageLogRepoStub struct {
	service.UsageLogRepository
	calls int
}

func (s *openAIChatCompletionsUsageLogRepoStub) Create(ctx context.Context, log *service.UsageLog) (bool, error) {
	s.calls++
	return true, nil
}

func TestOpenAIChatCompletionsNonStreamBillingFailureReplacesBufferedSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(2)
	entitlementID := int64(4)
	account := service.Account{
		ID:          9901,
		Name:        "openai-chat-completions-billing-failure",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://upstream.test",
		},
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
		},
	}
	accountRepo := &openAIWSUsageHandlerAccountRepoStub{account: account}
	usageRepo := &openAIChatCompletionsUsageLogRepoStub{}
	billingRepo := &openAIChatCompletionsUsageBillingRepoStub{err: service.ErrInsufficientBalance}
	upstream := &openAIChatCompletionsHTTPUpstreamStub{
		header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"upstream-success-before-billing"},
		},
		body: `{"id":"chatcmpl_success","object":"chat.completion","model":"gpt-5.1","choices":[{"index":0,"message":{"role":"assistant","content":"upstream ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`,
	}

	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	gatewaySvc := service.NewOpenAIGatewayService(
		accountRepo,
		usageRepo,
		billingRepo,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
		nil,
		service.NewBillingService(cfg, nil),
		nil,
		billingCacheSvc,
		upstream,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
			return true, nil
		},
		acquireAccountSlotFn: func(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
			return true, nil
		},
	}
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewaySvc,
		billingCacheService: billingCacheSvc,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
		maxAccountSwitches:  1,
		cfg:                 cfg,
	}

	apiKey := &service.APIKey{
		ID:                        406,
		UserID:                    14,
		GroupID:                   &groupID,
		SubscriptionEntitlementID: &entitlementID,
		AccessSource:              service.APIKeyAccessSourceEntitlement,
		Status:                    service.StatusActive,
		User:                      &service.User{ID: 14, Status: service.StatusActive, Balance: 0, Concurrency: 1},
		Group: &service.Group{
			ID:                  groupID,
			Platform:            service.PlatformOpenAI,
			Status:              service.StatusActive,
			RateMultiplier:      1,
			SubscriptionEnabled: true,
		},
	}
	entitlement := &service.SubscriptionEntitlement{
		ID:            entitlementID,
		UserID:        apiKey.UserID,
		OveragePolicy: service.SubscriptionEntitlementOverageBalanceFallback,
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1})
		c.Set(string(middleware.ContextKeySubscriptionEntitlement), entitlement)
		c.Set(string(middleware.ContextKeySubscriptionEntitlementBalanceFallback), true)
		c.Next()
	})
	router.POST("/v1/chat/completions", h.ChatCompletions)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.1","messages":[{"role":"user","content":"hello"}],"stream":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Contains(t, rec.Body.String(), "USAGE_LIMIT_EXCEEDED")
	require.Contains(t, rec.Body.String(), service.QuotaInsufficientMessage)
	require.NotContains(t, rec.Body.String(), "chatcmpl_success")
	require.NotContains(t, rec.Body.String(), "upstream ok")
	require.Equal(t, 1, upstream.calls)
	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, 0, usageRepo.calls)
}
