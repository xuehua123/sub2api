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
					EntitlementID:  5001,
					PlanID:         &planID,
					PlanName:       &planName,
					Status:         service.SubscriptionStatusActive,
					ExpiresAt:      now.Add(24 * time.Hour),
					PrimaryGroupID: &primaryGroupID,
					OveragePolicy:  service.SubscriptionEntitlementOverageBalanceFallback,
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
	for _, forbidden := range []string{
		"notes",
		"internal legacy note",
		"internal linked note",
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
