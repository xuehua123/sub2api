package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIRequestedEffortUsageLogRepoStub struct {
	service.UsageLogRepository
	created chan *service.UsageLog
}

func (s *openAIRequestedEffortUsageLogRepoStub) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	if s.created != nil {
		s.created <- log
	}
	return true, nil
}

func TestOpenAISuccessUsagePreservesRequestedReasoningBeforeGroupPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "chat completions",
			path: "/v1/chat/completions",
			body: `{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"max","stream":false}`,
		},
		{
			name: "messages",
			path: "/v1/messages",
			body: `{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"output_config":{"effort":"max"},"stream":false}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groupID := int64(4601)
			account := service.Account{
				ID:          9901,
				Name:        "openai-requested-effort-success",
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
			usageRepo := &openAIRequestedEffortUsageLogRepoStub{created: make(chan *service.UsageLog, 1)}
			upstream := &openAIChatCompletionsHTTPUpstreamStub{
				header: http.Header{
					"Content-Type": []string{"application/json"},
					"X-Request-Id": []string{"upstream-requested-effort"},
				},
				body: `{"id":"chatcmpl_success","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`,
			}

			cfg := &config.Config{RunMode: config.RunModeSimple}
			cfg.Default.RateMultiplier = 1
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billingCacheSvc.Stop)
			gatewaySvc := service.NewOpenAIGatewayService(
				accountRepo,
				usageRepo,
				nil,
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
				acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
				acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
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
				ID:      406,
				UserID:  14,
				GroupID: &groupID,
				Status:  service.StatusActive,
				User:    &service.User{ID: 14, Status: service.StatusActive, Balance: 1, Concurrency: 1},
				Group: &service.Group{
					ID:                    groupID,
					Platform:              service.PlatformOpenAI,
					Status:                service.StatusActive,
					RateMultiplier:        1,
					MaxReasoningEffort:    "low",
					AllowMessagesDispatch: true,
				},
			}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyAPIKey), apiKey)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1})
				c.Next()
			})
			router.POST("/v1/chat/completions", h.ChatCompletions)
			router.POST("/v1/messages", h.Messages)

			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Equal(t, 1, upstream.calls)
			select {
			case usageLog := <-usageRepo.created:
				require.NotNil(t, usageLog.RequestedReasoningEffort)
				require.Equal(t, "max", *usageLog.RequestedReasoningEffort)
				require.NotNil(t, usageLog.ReasoningEffort)
				require.Equal(t, "low", *usageLog.ReasoningEffort)
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for successful usage log")
			}
		})
	}
}
