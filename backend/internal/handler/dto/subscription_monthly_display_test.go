package dto

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestAdminSubscriptionLinkedMonthlyUsageMatchesCurrentWindow(t *testing.T) {
	now := time.Now()
	start := now.Add(-31 * 24 * time.Hour)
	limit := 100.0
	sub := &service.UserSubscription{ID: 1, StartsAt: start, Status: "active", ExpiresAt: now.Add(24 * time.Hour),
		EntitlementLink: &service.UserSubscriptionEntitlementLink{EntitlementID: 2, Status: "active", ExpiresAt: now.Add(24 * time.Hour), MonthlyWindowStart: &start, MonthlyUsageUSD: 100, MonthlyLimitUSD: &limit}}
	require.Zero(t, UserSubscriptionFromServiceAdmin(sub).MonthlyUsageUSD)
	require.Equal(t, 100.0, sub.EntitlementLink.MonthlyUsageUSD)
	sub.EntitlementLink.ExpiresAt = now.Add(-time.Hour)
	require.Equal(t, 100.0, UserSubscriptionFromServiceAdmin(sub).MonthlyUsageUSD)
}
