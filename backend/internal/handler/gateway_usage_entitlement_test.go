package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayUsageUnrestrictedUsesEntitlementLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)

	dailyLimit := 1.0
	monthlyLimit := 6000.0
	entitlement := &service.SubscriptionEntitlement{
		ID:              77,
		Name:            "OpenAI Main",
		ExpiresAt:       time.Now().Add(24 * time.Hour),
		DailyLimitUSD:   &dailyLimit,
		WeeklyLimitUSD:  nil,
		MonthlyLimitUSD: &monthlyLimit,
		DailyUsageUSD:   0,
		WeeklyUsageUSD:  667.56,
		MonthlyUsageUSD: 2828.64,
	}
	c.Set(string(middleware2.ContextKeySubscriptionEntitlement), entitlement)

	groupWeeklyLimit := 300.0
	apiKey := &service.APIKey{
		Status:       service.StatusAPIKeyActive,
		AccessSource: service.APIKeyAccessSourceEntitlement,
		Group: &service.Group{
			ID:               10,
			Name:             "OpenAI Main",
			SubscriptionType: service.SubscriptionTypeSubscription,
			WeeklyLimitUSD:   &groupWeeklyLimit,
		},
	}

	(&GatewayHandler{}).usageUnrestricted(
		c,
		context.Background(),
		apiKey,
		middleware2.AuthSubject{UserID: 1},
		nil,
		nil,
		nil,
	)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	subscription, ok := body["subscription"].(map[string]any)
	require.True(t, ok)
	require.Nil(t, subscription["weekly_limit_usd"])
	require.Equal(t, 667.56, subscription["weekly_usage_usd"])
	require.Equal(t, 1.0, subscription["daily_limit_usd"])
	require.Equal(t, 6000.0, subscription["monthly_limit_usd"])
	require.Equal(t, float64(77), subscription["entitlement_id"])
}

func TestCalculateEntitlementRemainingIgnoresNilWeeklyLimit(t *testing.T) {
	dailyLimit := 1.0
	monthlyLimit := 6000.0
	entitlement := &service.SubscriptionEntitlement{
		DailyLimitUSD:   &dailyLimit,
		WeeklyLimitUSD:  nil,
		MonthlyLimitUSD: &monthlyLimit,
		DailyUsageUSD:   0,
		WeeklyUsageUSD:  667.56,
		MonthlyUsageUSD: 2828.64,
	}

	require.Equal(t, 1.0, calculateEntitlementRemaining(entitlement))
}
