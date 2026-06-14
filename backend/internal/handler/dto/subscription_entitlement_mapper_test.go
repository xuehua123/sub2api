package dto

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserEntitlementFromServiceUsesStartsAtWhenWindowStartMissing(t *testing.T) {
	startsAt := time.Date(2026, 6, 14, 0, 58, 11, 0, time.UTC)
	now := startsAt.Add(12 * time.Hour)
	monthlyLimit := 1800.0

	out := UserEntitlementFromService(&service.SubscriptionEntitlement{
		ID:              21,
		UserID:          7,
		Name:            "standard month",
		Status:          service.SubscriptionStatusActive,
		StartsAt:        startsAt,
		ExpiresAt:       startsAt.Add(30 * 24 * time.Hour),
		MonthlyLimitUSD: &monthlyLimit,
		MonthlyUsageUSD: 0,
	}, now)

	require.NotNil(t, out)
	require.NotNil(t, out.MonthlyWindowStart)
	require.Equal(t, startsAt, *out.MonthlyWindowStart)
	require.NotNil(t, out.MonthlyResetsAt)
	require.Equal(t, startsAt.Add(30*24*time.Hour), *out.MonthlyResetsAt)
	require.NotNil(t, out.MonthlyResetsInSeconds)
	require.EqualValues(t, int64((29*24+12)*3600), *out.MonthlyResetsInSeconds)
}
