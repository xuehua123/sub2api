//go:build unit

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type middlewareEntitlementRuntimeProvider struct {
	enabled bool
}

func (p middlewareEntitlementRuntimeProvider) GetSubscriptionEntitlementsRuntime(context.Context) service.SubscriptionEntitlementsRuntime {
	return service.SubscriptionEntitlementsRuntime{Enabled: p.enabled}
}

type middlewareEntitlementRepo struct {
	entitlements map[int64]*service.SubscriptionEntitlement
}

func newMiddlewareEntitlementRepo(entitlements ...*service.SubscriptionEntitlement) *middlewareEntitlementRepo {
	repo := &middlewareEntitlementRepo{entitlements: make(map[int64]*service.SubscriptionEntitlement)}
	for _, ent := range entitlements {
		if ent == nil {
			continue
		}
		repo.entitlements[ent.ID] = cloneMiddlewareEntitlement(ent)
	}
	return repo
}

func (r *middlewareEntitlementRepo) Create(context.Context, *service.SubscriptionEntitlement, []int64) error {
	return errors.New("not implemented")
}
func (r *middlewareEntitlementRepo) CreateTx(context.Context, *service.SubscriptionEntitlement, []int64) error {
	return errors.New("not implemented")
}
func (r *middlewareEntitlementRepo) CreateWithFulfillment(context.Context, *service.SubscriptionEntitlement, []int64, *service.SubscriptionEntitlementFulfillment) error {
	return errors.New("not implemented")
}
func (r *middlewareEntitlementRepo) GetByID(_ context.Context, id int64) (*service.SubscriptionEntitlement, error) {
	ent, ok := r.entitlements[id]
	if !ok {
		return nil, service.ErrSubscriptionEntitlementNotFound
	}
	return cloneMiddlewareEntitlement(ent), nil
}
func (r *middlewareEntitlementRepo) GetBySourceID(context.Context, string, int64) (*service.SubscriptionEntitlement, error) {
	return nil, service.ErrSubscriptionEntitlementNotFound
}
func (r *middlewareEntitlementRepo) GetBySourceExternalID(context.Context, string, string) (*service.SubscriptionEntitlement, error) {
	return nil, service.ErrSubscriptionEntitlementNotFound
}
func (r *middlewareEntitlementRepo) GetBySourceRedeemCodeID(context.Context, int64) (*service.SubscriptionEntitlement, error) {
	return nil, service.ErrSubscriptionEntitlementNotFound
}
func (r *middlewareEntitlementRepo) GetFulfillmentBySourceID(context.Context, string, int64) (*service.SubscriptionEntitlementFulfillment, error) {
	return nil, service.ErrSubscriptionEntitlementNotFound
}
func (r *middlewareEntitlementRepo) GetFulfillmentBySourceExternalID(context.Context, string, string) (*service.SubscriptionEntitlementFulfillment, error) {
	return nil, service.ErrSubscriptionEntitlementNotFound
}
func (r *middlewareEntitlementRepo) GetFulfillmentBySourceRedeemCodeID(context.Context, int64) (*service.SubscriptionEntitlementFulfillment, error) {
	return nil, service.ErrSubscriptionEntitlementNotFound
}
func (r *middlewareEntitlementRepo) GetActiveCoveringGroup(ctx context.Context, userID, groupID int64) ([]service.SubscriptionEntitlement, error) {
	return r.ListActiveCoveringGroupForUser(ctx, userID, groupID)
}
func (r *middlewareEntitlementRepo) ListByUserID(_ context.Context, userID int64) ([]service.SubscriptionEntitlement, error) {
	out := make([]service.SubscriptionEntitlement, 0)
	for _, ent := range r.entitlements {
		if ent.UserID == userID {
			out = append(out, *cloneMiddlewareEntitlement(ent))
		}
	}
	return out, nil
}
func (r *middlewareEntitlementRepo) ListByUserPlanID(context.Context, int64, int64) ([]service.SubscriptionEntitlement, error) {
	return nil, nil
}
func (r *middlewareEntitlementRepo) ListActiveByUserID(_ context.Context, userID int64) ([]service.SubscriptionEntitlement, error) {
	out := make([]service.SubscriptionEntitlement, 0)
	for _, ent := range r.entitlements {
		if ent.UserID == userID && ent.Status == service.SubscriptionStatusActive {
			out = append(out, *cloneMiddlewareEntitlement(ent))
		}
	}
	return out, nil
}
func (r *middlewareEntitlementRepo) ListActiveCoveringGroupForUser(_ context.Context, userID, groupID int64) ([]service.SubscriptionEntitlement, error) {
	out := make([]service.SubscriptionEntitlement, 0)
	for _, ent := range r.entitlements {
		if ent.UserID != userID || ent.Status != service.SubscriptionStatusActive || !middlewareEntitlementCoversGroup(ent, groupID) {
			continue
		}
		out = append(out, *cloneMiddlewareEntitlement(ent))
	}
	return out, nil
}
func (r *middlewareEntitlementRepo) UpdateTerm(context.Context, int64, time.Time, time.Time, string, string) error {
	return nil
}
func (r *middlewareEntitlementRepo) UpdateTermAndSource(context.Context, int64, time.Time, time.Time, string, string, service.SubscriptionEntitlementSourceRef) error {
	return nil
}
func (r *middlewareEntitlementRepo) ExtendWithFulfillment(context.Context, int64, time.Time, time.Time, string, string, service.SubscriptionEntitlementSourceRef, *service.SubscriptionEntitlementFulfillment, bool, time.Time) error {
	return nil
}
func (r *middlewareEntitlementRepo) ResetUsage(context.Context, int64, bool, bool, bool, time.Time) error {
	return nil
}
func (r *middlewareEntitlementRepo) ApplyEntitlementUsage(context.Context, int64, float64, time.Time) (*service.EntitlementUsageApplyResult, error) {
	return nil, errors.New("unexpected ApplyEntitlementUsage call")
}
func (r *middlewareEntitlementRepo) ReplaceGroups(context.Context, int64, []int64) error {
	return nil
}

func TestAPIKeyAuthEntitlementV2StandardGroupUsesBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	apiKey := middlewareAPIKey("standard-v2-key", middlewareStandardGroup(10), nil, 10)
	apiKey.User.Balance = 1
	apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, nil, nil)
	router := newEntitlementAuthRouter(apiKeyService, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuthEntitlementV2CapabilityGroupUsesEntitlement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	group := service.Group{
		ID:                    20,
		Name:                  "capability-subscription",
		Platform:              service.PlatformOpenAI,
		Status:                service.StatusActive,
		SubscriptionType:      service.SubscriptionTypeStandard,
		SubscriptionEnabled:   true,
		AllowMessagesDispatch: true,
	}
	entitlementID := int64(100)
	apiKey := middlewareAPIKey("capability-entitlement-key", group, &entitlementID, 0)
	apiKey.AccessSource = service.APIKeyAccessSourceEntitlement
	ent := middlewareEntitlement(entitlementID, 1, now, []service.Group{group})
	apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, nil, ent)
	router := newEntitlementAuthRouter(apiKeyService, func(c *gin.Context) {
		resolved, ok := GetSubscriptionEntitlementFromContext(c)
		require.True(t, ok)
		require.Equal(t, entitlementID, resolved.ID)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuthEntitlementV2RejectsEntitlementSourceOnBalanceOnlyGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	group := middlewareStandardGroup(10)
	entitlementID := int64(100)
	apiKey := middlewareAPIKey("stale-entitlement-group-key", group, &entitlementID, 100)
	apiKey.AccessSource = service.APIKeyAccessSourceEntitlement
	ent := middlewareEntitlement(entitlementID, 1, now, []service.Group{group})
	apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, nil, ent)
	router := newEntitlementAuthRouter(apiKeyService, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestAPIKeyAuthEntitlementV2BalanceSourceOnDualModeGroupSkipsEntitlement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	group := middlewareSubscriptionGroup(20)
	group.BalanceEnabled = true
	group.SubscriptionEnabled = true
	apiKey := middlewareAPIKey("balance-dual-mode-key", group, nil, 1)
	apiKey.AccessSource = service.APIKeyAccessSourceBalance
	ent := middlewareEntitlement(100, 1, now, []service.Group{group})
	apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, nil, ent)
	router := newEntitlementAuthRouter(apiKeyService, func(c *gin.Context) {
		_, entitlementOK := GetSubscriptionEntitlementFromContext(c)
		require.False(t, entitlementOK)
		_, subscriptionOK := GetSubscriptionFromContext(c)
		require.False(t, subscriptionOK)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuthEntitlementV2EntitlementSourceRequiresEntitlementID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := middlewareSubscriptionGroup(20)
	apiKey := middlewareAPIKey("missing-entitlement-id-key", group, nil, 1)
	apiKey.AccessSource = service.APIKeyAccessSourceEntitlement
	apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, nil, nil)
	router := newEntitlementAuthRouter(apiKeyService, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestAPIKeyAuthEntitlementV2BalanceSourceIgnoresStaleEntitlementID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	group := middlewareSubscriptionGroup(20)
	group.BalanceEnabled = true
	group.SubscriptionEnabled = true
	entID := int64(100)
	apiKey := middlewareAPIKey("balance-stale-entitlement-key", group, &entID, 1)
	apiKey.AccessSource = service.APIKeyAccessSourceBalance
	ent := middlewareEntitlement(100, 1, now, []service.Group{group})
	apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, nil, ent)
	router := newEntitlementAuthRouter(apiKeyService, func(c *gin.Context) {
		_, entitlementOK := GetSubscriptionEntitlementFromContext(c)
		require.False(t, entitlementOK)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuthEntitlementV2ExplicitSetsEntitlementContext(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	entID := int64(100)
	apiKey := middlewareAPIKey("explicit-entitlement-key", middlewareSubscriptionGroup(20), &entID, 0)
	ent := middlewareEntitlement(100, 1, now, []service.Group{middlewareSubscriptionGroup(20)})
	apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, nil, ent)
	router := newEntitlementAuthRouter(apiKeyService, func(c *gin.Context) {
		userID, ok := c.Request.Context().Value(ctxkey.UserID).(int64)
		require.True(t, ok)
		require.Equal(t, int64(1), userID)
		entitlement, ok := GetSubscriptionEntitlementFromContext(c)
		require.True(t, ok)
		require.Equal(t, int64(100), entitlement.ID)
		_, legacyOK := GetSubscriptionFromContext(c)
		require.False(t, legacyOK, "entitlement id must not be forged as legacy subscription id")
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuthEntitlementV2DoesNotDefaultResolveWithoutBinding(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	apiKey := middlewareAPIKey("default-entitlement-key", middlewareSubscriptionGroup(20), nil, 0)
	entA := middlewareEntitlement(100, 1, now, []service.Group{middlewareSubscriptionGroup(20)})
	entA.ExpiresAt = now.Add(48 * time.Hour)
	entB := middlewareEntitlement(101, 1, now, []service.Group{middlewareSubscriptionGroup(20)})
	entB.ExpiresAt = now.Add(24 * time.Hour)
	apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, nil, entA, entB)
	router := newEntitlementAuthRouter(apiKeyService, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestAPIKeyAuthEntitlementV2RejectsInvalidExplicitEntitlement(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		ent  *service.SubscriptionEntitlement
	}{
		{name: "not-owned", ent: middlewareEntitlement(100, 2, now, []service.Group{middlewareSubscriptionGroup(20)})},
		{name: "future", ent: middlewareEntitlementWithWindow(100, 1, now.Add(time.Hour), now.Add(24*time.Hour), service.SubscriptionStatusActive, []service.Group{middlewareSubscriptionGroup(20)})},
		{name: "expired", ent: middlewareEntitlementWithWindow(100, 1, now.Add(-48*time.Hour), now.Add(-time.Hour), service.SubscriptionStatusActive, []service.Group{middlewareSubscriptionGroup(20)})},
		{name: "revoked", ent: middlewareEntitlementWithWindow(100, 1, now.Add(-time.Hour), now.Add(24*time.Hour), service.SubscriptionStatusSuspended, []service.Group{middlewareSubscriptionGroup(20)})},
		{name: "not-covering", ent: middlewareEntitlement(100, 1, now, []service.Group{middlewareSubscriptionGroup(30)})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entID := int64(100)
			apiKey := middlewareAPIKey("invalid-entitlement-"+tt.name, middlewareSubscriptionGroup(20), &entID, 0)
			apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, nil, tt.ent)
			router := newEntitlementAuthRouter(apiKeyService, nil)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
			req.Header.Set("x-api-key", apiKey.Key)
			router.ServeHTTP(w, req)

			require.NotEqual(t, http.StatusOK, w.Code)
		})
	}
}

func TestAPIKeyAuthEntitlementV2AutoSwitchesWithinSameEntitlement(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	current := middlewareSubscriptionGroup(20)
	current.Platform = service.PlatformOpenAI
	current.ClaudeCodeOnly = true
	current.AllowMessagesDispatch = false
	target := middlewareSubscriptionGroup(30)
	target.Platform = service.PlatformOpenAI
	entID := int64(100)
	apiKey := middlewareAPIKey("switch-entitlement-key", current, &entID, 0)
	apiKey.AutoSwitchGroupEnabled = true
	ent := middlewareEntitlement(100, 1, now, []service.Group{current, target})
	compareCalls := 0
	apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, func(ctx context.Context, id int64, oldGroupID, newGroupID int64, expectedEntitlementID, newEntitlementID *int64) (bool, error) {
		compareCalls++
		require.Equal(t, int64(20), oldGroupID)
		require.Equal(t, int64(30), newGroupID)
		require.NotNil(t, expectedEntitlementID)
		require.Equal(t, entID, *expectedEntitlementID)
		require.NotNil(t, newEntitlementID)
		require.Equal(t, entID, *newEntitlementID)
		return true, nil
	}, ent)
	router := newEntitlementAuthRouter(apiKeyService, func(c *gin.Context) {
		contextKey, ok := GetAPIKeyFromContext(c)
		require.True(t, ok)
		require.NotNil(t, contextKey.GroupID)
		require.Equal(t, int64(30), *contextKey.GroupID)
		entitlement, ok := GetSubscriptionEntitlementFromContext(c)
		require.True(t, ok)
		require.Equal(t, int64(100), entitlement.ID)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, compareCalls)
}

func TestAPIKeyAuthEntitlementV2QuotaExceededDoesNotAutoSwitch(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	limit := 5.0
	current := middlewareSubscriptionGroup(20)
	target := middlewareSubscriptionGroup(30)
	entID := int64(100)
	apiKey := middlewareAPIKey("quota-entitlement-key", current, &entID, 0)
	apiKey.AutoSwitchGroupEnabled = true
	ent := middlewareEntitlement(100, 1, now, []service.Group{current, target})
	ent.MonthlyLimitUSD = &limit
	ent.MonthlyUsageUSD = 6
	compareCalls := 0
	apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, func(context.Context, int64, int64, int64, *int64, *int64) (bool, error) {
		compareCalls++
		return true, nil
	}, ent)
	router := newEntitlementAuthRouter(apiKeyService, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, 0, compareCalls)
}

func TestAPIKeyAuthEntitlementV2AutoSwitchCASConflict(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	current := middlewareSubscriptionGroup(20)
	current.Platform = service.PlatformOpenAI
	current.ClaudeCodeOnly = true
	current.AllowMessagesDispatch = false
	target := middlewareSubscriptionGroup(30)
	target.Platform = service.PlatformOpenAI
	entID := int64(100)
	apiKey := middlewareAPIKey("switch-entitlement-conflict-key", current, &entID, 0)
	apiKey.AutoSwitchGroupEnabled = true
	ent := middlewareEntitlement(100, 1, now, []service.Group{current, target})
	apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, func(context.Context, int64, int64, int64, *int64, *int64) (bool, error) {
		return false, nil
	}, ent)
	router := newEntitlementAuthRouter(apiKeyService, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
}

func TestAPIKeyAuthWithSubscriptionGoogleEntitlementV2(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	entID := int64(100)
	apiKey := middlewareAPIKey("google-explicit-entitlement-key", middlewareSubscriptionGroup(20), &entID, 0)
	ent := middlewareEntitlement(100, 1, now, []service.Group{middlewareSubscriptionGroup(20)})
	apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, nil, ent)

	router := gin.New()
	router.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{RunMode: config.RunModeStandard}))
	router.GET("/v1beta/models", func(c *gin.Context) {
		entitlement, ok := GetSubscriptionEntitlementFromContext(c)
		require.True(t, ok)
		require.Equal(t, int64(100), entitlement.ID)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	req.Header.Set("x-goog-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuthWithSubscriptionGoogleBalanceSourceSkipsEntitlement(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	group := middlewareSubscriptionGroup(20)
	group.BalanceEnabled = true
	group.SubscriptionEnabled = true
	apiKey := middlewareAPIKey("google-balance-dual-mode-key", group, nil, 1)
	apiKey.AccessSource = service.APIKeyAccessSourceBalance
	ent := middlewareEntitlement(100, 1, now, []service.Group{group})
	apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, nil, ent)

	router := gin.New()
	router.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{RunMode: config.RunModeStandard}))
	router.GET("/v1beta/models", func(c *gin.Context) {
		_, ok := GetSubscriptionEntitlementFromContext(c)
		require.False(t, ok)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	req.Header.Set("x-goog-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuthWithSubscriptionGoogleEntitlementSourceRequiresID(t *testing.T) {
	group := middlewareSubscriptionGroup(20)
	apiKey := middlewareAPIKey("google-missing-entitlement-id-key", group, nil, 1)
	apiKey.AccessSource = service.APIKeyAccessSourceEntitlement
	apiKeyService := newMiddlewareEntitlementAPIKeyService(t, apiKey, true, nil, nil)

	router := gin.New()
	router.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, &config.Config{RunMode: config.RunModeStandard}))
	router.GET("/v1beta/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	req.Header.Set("x-goog-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func newEntitlementAuthRouter(apiKeyService *service.APIKeyService, handler gin.HandlerFunc) *gin.Engine {
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, &config.Config{RunMode: config.RunModeStandard})))
	if handler == nil {
		handler = func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }
	}
	router.GET("/v1/messages", handler)
	return router
}

func newMiddlewareEntitlementAPIKeyService(t *testing.T, apiKey *service.APIKey, v2Enabled bool, compare func(context.Context, int64, int64, int64, *int64, *int64) (bool, error), entitlements ...*service.SubscriptionEntitlement) *service.APIKeyService {
	t.Helper()
	repo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			return cloneMiddlewareAPIKey(apiKey), nil
		},
		compareAndSwapGroupIDEntitlement: compare,
	}
	entRepo := newMiddlewareEntitlementRepo(entitlements...)
	entSvc := service.NewSubscriptionEntitlementService(entRepo, nil)
	entSvc.SetNowFunc(func() time.Time { return time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC) })
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})
	svc.SetSubscriptionEntitlementDependencies(middlewareEntitlementRuntimeProvider{enabled: v2Enabled}, entSvc)
	return svc
}

func middlewareAPIKey(key string, group service.Group, entitlementID *int64, balance float64) *service.APIKey {
	groupID := group.ID
	user := &service.User{ID: 1, Status: service.StatusActive, Role: service.RoleUser, Balance: balance, Concurrency: 1}
	return &service.APIKey{
		ID:                        10,
		UserID:                    user.ID,
		Key:                       key,
		Status:                    service.StatusAPIKeyActive,
		User:                      user,
		GroupID:                   &groupID,
		Group:                     &group,
		SubscriptionEntitlementID: cloneMiddlewareInt64(entitlementID),
		AutoSwitchGroupEnabled:    false,
	}
}

func middlewareStandardGroup(id int64) service.Group {
	return service.Group{ID: id, Name: "standard", Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, BalanceEnabled: true}
}

func middlewareSubscriptionGroup(id int64) service.Group {
	return service.Group{ID: id, Name: "subscription", Platform: service.PlatformOpenAI, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeSubscription, SubscriptionEnabled: true, AllowMessagesDispatch: true}
}

func middlewareEntitlement(id, userID int64, now time.Time, groups []service.Group) *service.SubscriptionEntitlement {
	return middlewareEntitlementWithWindow(id, userID, now.Add(-time.Hour), now.Add(24*time.Hour), service.SubscriptionStatusActive, groups)
}

func middlewareEntitlementWithWindow(id, userID int64, startsAt, expiresAt time.Time, status string, groups []service.Group) *service.SubscriptionEntitlement {
	grants := make([]service.SubscriptionEntitlementGroupGrant, 0, len(groups))
	groupCopies := make([]service.Group, 0, len(groups))
	for i := range groups {
		group := groups[i]
		groupCopies = append(groupCopies, group)
		grants = append(grants, service.SubscriptionEntitlementGroupGrant{
			GroupID:   group.ID,
			SortOrder: i,
			Enabled:   true,
			Group:     &group,
		})
	}
	return &service.SubscriptionEntitlement{
		ID:                 id,
		UserID:             userID,
		Name:               "entitlement",
		Status:             status,
		StartsAt:           startsAt,
		ExpiresAt:          expiresAt,
		DailyWindowStart:   &startsAt,
		WeeklyWindowStart:  &startsAt,
		MonthlyWindowStart: &startsAt,
		OveragePolicy:      service.SubscriptionEntitlementOverageBlock,
		GroupGrants:        grants,
		Groups:             groupCopies,
	}
}

func middlewareEntitlementCoversGroup(ent *service.SubscriptionEntitlement, groupID int64) bool {
	if ent == nil {
		return false
	}
	for _, grant := range ent.GroupGrants {
		if grant.Enabled && grant.GroupID == groupID {
			return true
		}
	}
	return false
}

func cloneMiddlewareAPIKey(apiKey *service.APIKey) *service.APIKey {
	if apiKey == nil {
		return nil
	}
	cp := *apiKey
	cp.GroupID = cloneMiddlewareInt64(apiKey.GroupID)
	cp.SubscriptionEntitlementID = cloneMiddlewareInt64(apiKey.SubscriptionEntitlementID)
	if apiKey.User != nil {
		user := *apiKey.User
		cp.User = &user
	}
	if apiKey.Group != nil {
		group := *apiKey.Group
		cp.Group = &group
	}
	return &cp
}

func cloneMiddlewareEntitlement(ent *service.SubscriptionEntitlement) *service.SubscriptionEntitlement {
	if ent == nil {
		return nil
	}
	cp := *ent
	cp.PlanID = cloneMiddlewareInt64(ent.PlanID)
	cp.LegacySubscriptionID = cloneMiddlewareInt64(ent.LegacySubscriptionID)
	cp.PrimaryGroupID = cloneMiddlewareInt64(ent.PrimaryGroupID)
	cp.DailyWindowStart = cloneMiddlewareTime(ent.DailyWindowStart)
	cp.WeeklyWindowStart = cloneMiddlewareTime(ent.WeeklyWindowStart)
	cp.MonthlyWindowStart = cloneMiddlewareTime(ent.MonthlyWindowStart)
	cp.DailyLimitUSD = cloneMiddlewareFloat64(ent.DailyLimitUSD)
	cp.WeeklyLimitUSD = cloneMiddlewareFloat64(ent.WeeklyLimitUSD)
	cp.MonthlyLimitUSD = cloneMiddlewareFloat64(ent.MonthlyLimitUSD)
	cp.GroupGrants = make([]service.SubscriptionEntitlementGroupGrant, len(ent.GroupGrants))
	for i := range ent.GroupGrants {
		cp.GroupGrants[i] = ent.GroupGrants[i]
		if ent.GroupGrants[i].Group != nil {
			group := *ent.GroupGrants[i].Group
			cp.GroupGrants[i].Group = &group
		}
	}
	cp.Groups = append([]service.Group(nil), ent.Groups...)
	return &cp
}

func cloneMiddlewareInt64(v *int64) *int64 {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneMiddlewareFloat64(v *float64) *float64 {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneMiddlewareTime(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}
