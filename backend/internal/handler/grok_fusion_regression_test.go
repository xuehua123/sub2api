//go:build unit

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
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokFusionRetryUpstreamStub struct {
	service.HTTPUpstream
	calls       int
	accountIDs  []int64
	successBody string
	contentType string
}

func (s *grokFusionRetryUpstreamStub) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	s.calls++
	s.accountIDs = append(s.accountIDs, accountID)
	if s.calls == 1 {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"temporary"}}`)),
			Request:    req,
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{s.contentType}},
		Body:       io.NopCloser(strings.NewReader(s.successBody)),
		Request:    req,
	}, nil
}

func grokFusionGroup(groupID int64) *service.Group {
	searchPrice := 1.0
	audioPrice := 15.0
	return &service.Group{
		ID:                           groupID,
		Platform:                     service.PlatformGrok,
		Status:                       service.StatusActive,
		Hydrated:                     true,
		RateMultiplier:               0.1,
		SubscriptionType:             service.SubscriptionTypeStandard,
		SubscriptionEnabled:          true,
		ProfitControlEnabled:         true,
		ProfitMinMargin:              0,
		ProfitSafetyBuffer:           0,
		SearchPricePer1k:             &searchPrice,
		AudioTTSPricePerMillionChars: &audioPrice,
	}
}

func grokFusionAccount(accountID, groupID int64) service.Account {
	rate := 10.0
	return service.Account{
		ID:             accountID,
		Name:           "grok-fusion-pool",
		Platform:       service.PlatformGrok,
		Type:           service.AccountTypeAPIKey,
		Status:         service.StatusActive,
		Schedulable:    true,
		Concurrency:    1,
		RateMultiplier: &rate,
		AccountGroups:  []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
		GroupIDs:       []int64{groupID},
		Credentials: map[string]any{
			"api_key":                      "xai-test",
			"base_url":                     "https://api.x.ai/v1",
			"pool_mode":                    true,
			"pool_mode_retry_count":        1,
			"pool_mode_retry_status_codes": []any{float64(http.StatusBadGateway)},
		},
	}
}

func grokFusionIdentity(group *service.Group, entitlementID int64) (*service.User, *service.APIKey, *service.SubscriptionEntitlement) {
	user := &service.User{ID: 41, Status: service.StatusActive, Balance: 0, Concurrency: 1}
	groupID := group.ID
	apiKey := &service.APIKey{
		ID:                        61,
		UserID:                    user.ID,
		GroupID:                   &groupID,
		SubscriptionEntitlementID: &entitlementID,
		AccessSource:              service.APIKeyAccessSourceEntitlement,
		Status:                    service.StatusActive,
		User:                      user,
		Group:                     group,
	}
	entitlement := &service.SubscriptionEntitlement{
		ID:            entitlementID,
		UserID:        user.ID,
		Status:        service.SubscriptionStatusActive,
		ExpiresAt:     time.Now().Add(time.Hour),
		OveragePolicy: service.SubscriptionEntitlementOverageBalanceFallback,
	}
	return user, apiKey, entitlement
}

func addGrokFusionIdentityMiddleware(router *gin.Engine, group *service.Group, apiKey *service.APIKey, entitlement *service.SubscriptionEntitlement, gatewayTokenPricing bool) {
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), ctxkey.Group, group)
		if gatewayTokenPricing {
			ctx, _ = service.WithGatewayTokenRequestPricing(ctx)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: apiKey.UserID, Concurrency: 1})
		c.Set(string(middleware2.ContextKeySubscriptionEntitlement), entitlement)
		c.Set(string(middleware2.ContextKeySubscriptionEntitlementBalanceFallback), true)
		c.Next()
	})
}

func TestGrokVoiceFusionUsesEntitlementSuppressesProfitGateAndRetriesSameAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := grokFusionGroup(72)
	account := grokFusionAccount(501, group.ID)
	user, apiKey, entitlement := grokFusionIdentity(group, 91)
	accountRepo := &grokMediaHandlerAccountRepoStub{account: account}
	userRepo := &grokMediaHandlerUserRepoStub{user: user}
	usageBillingRepo := &grokMediaHandlerUsageBillingRepoStub{}
	usageLogRepo := &grokMediaHandlerUsageLogRepoStub{}
	upstream := &grokFusionRetryUpstreamStub{successBody: "audio", contentType: "audio/mpeg"}
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	cfg.Gateway.MaxAccountSwitches = 1
	billingCacheSvc := service.NewBillingCacheService(nil, userRepo, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheSvc.Stop)
	gatewaySvc := service.NewOpenAIGatewayService(
		accountRepo, usageLogRepo, usageBillingRepo, userRepo, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCacheSvc, upstream, &service.DeferredService{},
		nil, nil, nil, nil, nil, nil, nil,
	)
	concurrencyCache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewaySvc,
		billingCacheService: billingCacheSvc,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(concurrencyCache), SSEPingFormatNone, time.Second),
		maxAccountSwitches:  1,
		cfg:                 cfg,
	}

	router := gin.New()
	addGrokFusionIdentityMiddleware(router, group, apiKey, entitlement, false)
	router.POST("/v1/audio/speech", func(c *gin.Context) { h.GrokVoice(c, "tts") })
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []int64{account.ID, account.ID}, upstream.accountIDs)
	require.Zero(t, userRepo.getCalls, "entitlement eligibility must not query the zero balance")
	require.NotNil(t, usageBillingRepo.lastCommand)
	require.Equal(t, &entitlement.ID, usageBillingRepo.lastCommand.EntitlementID)
	require.True(t, usageBillingRepo.lastCommand.EntitlementBalanceFallback)
	require.True(t, usageBillingRepo.lastCommand.AllowEntitlementOverage)
}

func TestGrokWebSearchFusionUsesEntitlementSuppressesProfitGateAndRetriesSameAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := grokFusionGroup(82)
	account := grokFusionAccount(601, group.ID)
	user, apiKey, entitlement := grokFusionIdentity(group, 92)
	accountRepo := &grokMediaHandlerAccountRepoStub{account: account}
	userRepo := &grokMediaHandlerUserRepoStub{user: user}
	usageBillingRepo := &grokMediaHandlerUsageBillingRepoStub{}
	usageLogRepo := &grokMediaHandlerUsageLogRepoStub{}
	upstream := &grokFusionRetryUpstreamStub{successBody: `{"output":[]}`, contentType: "application/json"}
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	cfg.Gateway.MaxAccountSwitches = 1
	billingCacheSvc := service.NewBillingCacheService(nil, userRepo, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheSvc.Stop)
	concurrencySvc := service.NewConcurrencyService(&fakeConcurrencyCache{})
	schedulerCache := &fakeSchedulerCache{accounts: []*service.Account{&account}}
	groupRepo := &fakeGroupRepo{group: group}
	schedulerSnapshot := service.NewSchedulerSnapshotService(schedulerCache, nil, nil, groupRepo, nil)
	gatewaySvc := service.NewGatewayService(
		accountRepo, groupRepo, usageLogRepo, usageBillingRepo, userRepo, nil, nil, nil, cfg,
		schedulerSnapshot, concurrencySvc, service.NewBillingService(cfg, nil), nil, billingCacheSvc,
		nil, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	h := &GatewayHandler{
		gatewayService:      gatewaySvc,
		billingCacheService: billingCacheSvc,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(concurrencySvc, SSEPingFormatNone, time.Second),
		maxAccountSwitches:  1,
		cfg:                 cfg,
	}

	router := gin.New()
	addGrokFusionIdentityMiddleware(router, group, apiKey, entitlement, true)
	router.POST("/v1/web_search", h.WebSearch)
	req := httptest.NewRequest(http.MethodPost, "/v1/web_search", strings.NewReader(`{"query":"release notes"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []int64{account.ID, account.ID}, upstream.accountIDs)
	require.Zero(t, userRepo.getCalls, "entitlement eligibility must not query the zero balance")
	require.NotNil(t, usageBillingRepo.lastCommand)
	require.Equal(t, &entitlement.ID, usageBillingRepo.lastCommand.EntitlementID)
	require.True(t, usageBillingRepo.lastCommand.EntitlementBalanceFallback)
	require.True(t, usageBillingRepo.lastCommand.AllowEntitlementOverage)
}
