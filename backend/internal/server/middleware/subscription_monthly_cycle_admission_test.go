//go:build unit

package middleware

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type reviewCycleRepo struct {
	*middlewareEntitlementRepo
	resets int
}

func (r *reviewCycleRepo) LockEntitlementMonthlyCycle(ctx context.Context, userID, id int64) (*service.SubscriptionEntitlementMonthlyCycleSnapshot, error) {
	e, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &service.SubscriptionEntitlementMonthlyCycleSnapshot{ID: e.ID, UserID: e.UserID, Status: e.Status, StartsAt: e.StartsAt, ExpiresAt: e.ExpiresAt, MonthlyLimitUSD: e.MonthlyLimitUSD, MonthlyUsageUSD: e.MonthlyUsageUSD, MonthlyWindowStart: e.MonthlyWindowStart}, nil
}
func (r *reviewCycleRepo) UpdateEntitlementMonthlyCycle(_ context.Context, u service.SubscriptionEntitlementMonthlyCycleUpdate) error {
	e := r.entitlements[u.EntitlementID]
	e.ExpiresAt = u.NewExpiresAt
	e.MonthlyWindowStart = &u.NewMonthlyWindowStart
	e.MonthlyUsageUSD = u.NewMonthlyUsageUSD
	r.resets++
	return nil
}
func (r *reviewCycleRepo) InsertEntitlementCycleResetLog(context.Context, service.SubscriptionEntitlementCycleResetLog) error {
	return nil
}

func TestRejectedModelMustNotConsumeFutureCycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC().Truncate(time.Second)
	group := middlewareSubscriptionGroup(20)
	group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{"allowed-model"}}
	ent := middlewareEntitlementWithWindow(100, 1, now.Add(-10*24*time.Hour), now.Add(80*24*time.Hour), service.SubscriptionStatusActive, []service.Group{group})
	limit := 100.0
	ent.MonthlyLimitUSD = &limit
	ent.MonthlyUsageUSD = 101
	ent.AutoAdvanceMonthly = true
	key := middlewareAPIKey("review-key", group, &ent.ID, 0)
	repo := &reviewCycleRepo{middlewareEntitlementRepo: newMiddlewareEntitlementRepo(ent)}
	entSvc := service.NewSubscriptionEntitlementService(repo, nil)
	entSvc.SetNowFunc(func() time.Time { return now })
	apiRepo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { return cloneMiddlewareAPIKey(key), nil }}
	apiSvc := service.NewAPIKeyService(apiRepo, nil, nil, nil, nil, nil, &config.Config{})
	apiSvc.SetSubscriptionEntitlementDependencies(middlewareEntitlementRuntimeProvider{enabled: true}, entSvc)
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiSvc, nil, &config.Config{RunMode: config.RunModeStandard})))
	router.Use(GroupModelAllowlist())
	router.POST("/v1/messages", func(c *gin.Context) {
		current, _ := GetSubscriptionEntitlementFromContext(c)
		require.True(t, service.HasEntitlementGenerationAdmission(c.Request.Context()))
		require.NoError(t, service.PrepareEntitlementGenerationAdmission(c.Request.Context(), current))
		if c.Query("no_account") == "1" {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		require.NoError(t, service.CompleteEntitlementGenerationAdmission(c.Request.Context(), current, 0))
		require.Zero(t, current.MonthlyUsageUSD)
		require.NoError(t, service.CompleteEntitlementGenerationAdmission(c.Request.Context(), current, 0))
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{\"model\":\"blocked-model\"}"))
	req.Header.Set("x-api-key", key.Key)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
	t.Logf("rejected response=%d, cycles consumed=%d, expiry shift=%s", recorder.Code, repo.resets, ent.ExpiresAt.Sub(repo.entitlements[100].ExpiresAt))
	require.Zero(t, repo.resets, "a denied model must not consume a purchased future cycle")
	req = httptest.NewRequest(http.MethodPost, "/v1/messages?no_account=1", strings.NewReader("{\"model\":\"allowed-model\"}"))
	req.Header.Set("x-api-key", key.Key)
	req.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Zero(t, repo.resets, "account selection failure must not consume a cycle")
	req = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{\"model\":\"allowed-model\"}"))
	req.Header.Set("x-api-key", key.Key)
	req.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, repo.resets)
}

func TestBillableSearchCoverage(t *testing.T) {
	for _, path := range []string{"/v1/alpha/search", "/v1/web_search", "/v1/x_search", "/v1/live", "/backend-api/codex/realtime/calls", "/v1/contents/generations/tasks"} {
		t.Run(path, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, path, nil)
			require.True(t, isMonthlyCycleBillableRequest(c))
		})
	}
}
