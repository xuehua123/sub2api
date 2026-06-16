//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type adminSubscriptionListUserRepo struct {
	service.UserSubscriptionRepository
	subs      []service.UserSubscription
	gotUserID int64
}

func (r *adminSubscriptionListUserRepo) ListByUserID(_ context.Context, userID int64) ([]service.UserSubscription, error) {
	r.gotUserID = userID
	out := make([]service.UserSubscription, len(r.subs))
	copy(out, r.subs)
	return out, nil
}

func TestSubscriptionHandlerListByUserReturnsEntitlementLinkFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	planID := int64(7001)
	planName := "Backfilled Pro"
	primaryGroupID := int64(901)
	dailyLimit := 5.0
	monthlyLimit := 5000.0
	dailyWindow := now.Add(-30 * time.Minute)
	monthlyWindow := now.Add(-2 * time.Hour)
	repo := &adminSubscriptionListUserRepo{
		subs: []service.UserSubscription{
			{
				ID:        100,
				UserID:    33,
				GroupID:   9,
				StartsAt:  now.Add(-time.Hour),
				ExpiresAt: now.Add(24 * time.Hour),
				Status:    service.SubscriptionStatusActive,
				Notes:     "internal legacy note",
			},
			{
				ID:        101,
				UserID:    33,
				GroupID:   10,
				StartsAt:  now.Add(-time.Hour),
				ExpiresAt: now.Add(24 * time.Hour),
				Status:    service.SubscriptionStatusActive,
				Notes:     "internal linked note",
				EntitlementLink: &service.UserSubscriptionEntitlementLink{
					EntitlementID:      5001,
					PlanID:             &planID,
					PlanName:           &planName,
					Status:             service.SubscriptionStatusActive,
					ExpiresAt:          now.Add(24 * time.Hour),
					DailyWindowStart:   &dailyWindow,
					MonthlyWindowStart: &monthlyWindow,
					DailyLimitUSD:      &dailyLimit,
					MonthlyLimitUSD:    &monthlyLimit,
					DailyUsageUSD:      1.25,
					WeeklyUsageUSD:     12.5,
					MonthlyUsageUSD:    125,
					PrimaryGroupID:     &primaryGroupID,
					OveragePolicy:      service.SubscriptionEntitlementOverageBalanceFallback,
				},
			},
			{
				ID:              -5002,
				UserID:          33,
				GroupID:         10,
				StartsAt:        now.Add(-time.Hour),
				ExpiresAt:       now.Add(48 * time.Hour),
				Status:          service.SubscriptionStatusActive,
				Notes:           "internal entitlement-only note",
				EntitlementOnly: true,
				EntitlementLink: &service.UserSubscriptionEntitlementLink{
					EntitlementID:  5002,
					PlanID:         &planID,
					PlanName:       &planName,
					Status:         service.SubscriptionStatusActive,
					ExpiresAt:      now.Add(48 * time.Hour),
					PrimaryGroupID: &primaryGroupID,
					OveragePolicy:  service.SubscriptionEntitlementOverageBlock,
				},
			},
		},
	}
	svc := service.NewSubscriptionService(nil, repo, nil, nil, &config.Config{})
	handler := NewSubscriptionHandler(svc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/33/subscriptions", nil)
	c.Params = gin.Params{{Key: "id", Value: "33"}}

	handler.ListByUser(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(33), repo.gotUserID)
	body := w.Body.String()
	require.Contains(t, body, `"id":100`)
	require.Contains(t, body, `"entitlement_id":null`)
	require.Contains(t, body, `"id":101`)
	require.Contains(t, body, `"entitlement_id":5001`)
	require.Contains(t, body, `"plan_id":7001`)
	require.Contains(t, body, `"plan_name":"Backfilled Pro"`)
	require.Contains(t, body, `"entitlement_status":"active"`)
	require.Contains(t, body, `"entitlement_primary_group_id":901`)
	require.Contains(t, body, `"entitlement_overage_policy":"balance_fallback"`)
	require.Contains(t, body, `"daily_usage_usd":1.25`)
	require.Contains(t, body, `"weekly_usage_usd":12.5`)
	require.Contains(t, body, `"monthly_usage_usd":125`)
	require.Contains(t, body, `"daily_limit_usd":5`)
	require.Contains(t, body, `"monthly_limit_usd":5000`)
	require.Contains(t, body, `"id":-5002`)
	require.Contains(t, body, `"entitlement_only":true`)
	require.Contains(t, body, `"entitlement_id":5002`)
	require.Contains(t, body, `"entitlement_overage_policy":"block"`)
	for _, forbidden := range []string{
		"notes",
		"internal legacy note",
		"internal linked note",
		"internal entitlement-only note",
		"source_external_id",
		"source_id",
		"source_type",
		"source_redeem_code_id",
		"plan_snapshot",
		"fulfillment",
	} {
		require.NotContains(t, strings.ToLower(body), forbidden)
	}
}

func TestSubscriptionHandlerListByUserRejectsInvalidUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &adminSubscriptionListUserRepo{}
	svc := service.NewSubscriptionService(nil, repo, nil, nil, &config.Config{})
	handler := NewSubscriptionHandler(svc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/bad/subscriptions", nil)
	c.Params = gin.Params{{Key: "id", Value: "bad"}}

	handler.ListByUser(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

var _ service.UserSubscriptionRepository = (*adminSubscriptionListUserRepo)(nil)
