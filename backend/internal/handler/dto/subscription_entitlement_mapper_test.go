package dto

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
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

func TestUserEntitlementFromServiceDailyResetUsesNextCalendarMidnight(t *testing.T) {
	base := time.Date(2026, 6, 14, 0, 0, 0, 0, timezone.Location())
	windowStart := base.Add(16*time.Hour + 45*time.Minute)
	dailyLimit := 10.0
	now := base.Add(23 * time.Hour)

	out := UserEntitlementFromService(&service.SubscriptionEntitlement{
		ID:               22,
		Status:           service.SubscriptionStatusActive,
		StartsAt:         base.Add(9 * time.Hour),
		ExpiresAt:        base.AddDate(0, 0, 10),
		DailyLimitUSD:    &dailyLimit,
		DailyWindowStart: &windowStart,
		DailyUsageUSD:    3,
	}, now)

	require.NotNil(t, out.DailyWindowStart)
	require.Equal(t, base, *out.DailyWindowStart)
	require.NotNil(t, out.DailyResetsAt)
	require.Equal(t, base.AddDate(0, 0, 1), *out.DailyResetsAt)
	require.EqualValues(t, 3600, *out.DailyResetsInSeconds)
	require.Equal(t, 3.0, out.DailyUsageUSD)
}

func TestUserEntitlementFromServiceAdvancesStaleDailyWindowForDisplay(t *testing.T) {
	base := time.Date(2026, 6, 14, 0, 0, 0, 0, timezone.Location())
	staleWindowStart := base.AddDate(0, 0, -1).Add(16*time.Hour + 45*time.Minute)
	dailyLimit := 10.0
	now := base.Add(10 * time.Hour)

	out := UserEntitlementFromService(&service.SubscriptionEntitlement{
		ID:               24,
		Status:           service.SubscriptionStatusActive,
		StartsAt:         base.AddDate(0, 0, -5).Add(9 * time.Hour),
		ExpiresAt:        base.AddDate(0, 0, 10),
		DailyLimitUSD:    &dailyLimit,
		DailyWindowStart: &staleWindowStart,
		DailyUsageUSD:    7,
	}, now)

	require.NotNil(t, out.DailyWindowStart)
	require.Equal(t, base, *out.DailyWindowStart)
	require.NotNil(t, out.DailyResetsAt)
	require.Equal(t, base.AddDate(0, 0, 1), *out.DailyResetsAt)
	require.EqualValues(t, 14*3600, *out.DailyResetsInSeconds)
	require.Zero(t, out.DailyUsageUSD)
}

func TestUserEntitlementFromServiceDoesNotProjectInactiveDailyWindows(t *testing.T) {
	base := time.Date(2026, 6, 14, 0, 0, 0, 0, timezone.Location())
	dailyLimit := 10.0

	tests := []struct {
		name            string
		entitlement     service.SubscriptionEntitlement
		expectedWindow  time.Time
		expectedResetAt time.Time
	}{
		{
			name: "future",
			entitlement: service.SubscriptionEntitlement{
				ID:            26,
				Status:        service.SubscriptionStatusActive,
				StartsAt:      base.AddDate(0, 0, 2).Add(9 * time.Hour),
				ExpiresAt:     base.AddDate(0, 0, 12),
				DailyLimitUSD: &dailyLimit,
				DailyUsageUSD: 5,
			},
			expectedWindow:  base.AddDate(0, 0, 2),
			expectedResetAt: base.AddDate(0, 0, 3),
		},
		{
			name: "expired",
			entitlement: service.SubscriptionEntitlement{
				ID:               27,
				Status:           service.SubscriptionStatusExpired,
				StartsAt:         base.AddDate(0, 0, -5).Add(9 * time.Hour),
				ExpiresAt:        base.AddDate(0, 0, -1),
				DailyLimitUSD:    &dailyLimit,
				DailyWindowStart: entitlementMapperTimePtr(base.AddDate(0, 0, -3)),
				DailyUsageUSD:    7,
			},
			expectedWindow:  base.AddDate(0, 0, -3),
			expectedResetAt: base.AddDate(0, 0, -2),
		},
		{
			name: "revoked",
			entitlement: service.SubscriptionEntitlement{
				ID:               28,
				Status:           service.SubscriptionStatusRevoked,
				StartsAt:         base.AddDate(0, 0, -5).Add(9 * time.Hour),
				ExpiresAt:        base.AddDate(0, 0, 5),
				DailyLimitUSD:    &dailyLimit,
				DailyWindowStart: entitlementMapperTimePtr(base.AddDate(0, 0, -2)),
				DailyUsageUSD:    9,
			},
			expectedWindow:  base.AddDate(0, 0, -2),
			expectedResetAt: base.AddDate(0, 0, -1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := UserEntitlementFromService(&tt.entitlement, base.Add(10*time.Hour))

			require.NotNil(t, out.DailyWindowStart)
			require.Equal(t, tt.expectedWindow, *out.DailyWindowStart)
			require.Equal(t, tt.entitlement.DailyUsageUSD, out.DailyUsageUSD)
			require.NotNil(t, out.DailyResetsAt)
			require.Equal(t, tt.expectedResetAt, *out.DailyResetsAt)
		})
	}
}

func entitlementMapperTimePtr(value time.Time) *time.Time {
	return &value
}

func TestUserEntitlementFromServiceOneDayCardDailyResetUsesExpiry(t *testing.T) {
	base := time.Date(2026, 6, 14, 0, 0, 0, 0, timezone.Location())
	startsAt := base.Add(17 * time.Hour)
	expiresAt := startsAt.AddDate(0, 0, 1)
	dailyWindowStart := base
	dailyLimit := 10.0
	now := base.AddDate(0, 0, 1).Add(time.Hour)

	out := UserEntitlementFromService(&service.SubscriptionEntitlement{
		ID:               23,
		Status:           service.SubscriptionStatusActive,
		StartsAt:         startsAt,
		ExpiresAt:        expiresAt,
		DailyLimitUSD:    &dailyLimit,
		DailyWindowStart: &dailyWindowStart,
		DailyUsageUSD:    7,
	}, now)

	require.NotNil(t, out.DailyResetsAt)
	require.Equal(t, expiresAt, *out.DailyResetsAt)
	require.EqualValues(t, 16*3600, *out.DailyResetsInSeconds)
	require.Equal(t, 7.0, out.DailyUsageUSD)
}

func TestUserEntitlementFromServiceDailyResetIsCappedAtExpiry(t *testing.T) {
	base := time.Date(2026, 6, 14, 0, 0, 0, 0, timezone.Location())
	dailyWindowStart := base
	expiresAt := base.Add(20 * time.Hour)
	dailyLimit := 10.0
	now := base.Add(10 * time.Hour)

	out := UserEntitlementFromService(&service.SubscriptionEntitlement{
		ID:               25,
		Status:           service.SubscriptionStatusActive,
		StartsAt:         base.AddDate(0, 0, -5).Add(9 * time.Hour),
		ExpiresAt:        expiresAt,
		DailyLimitUSD:    &dailyLimit,
		DailyWindowStart: &dailyWindowStart,
		DailyUsageUSD:    4,
	}, now)

	require.NotNil(t, out.DailyResetsAt)
	require.Equal(t, expiresAt, *out.DailyResetsAt)
	require.EqualValues(t, 10*3600, *out.DailyResetsInSeconds)
	require.Equal(t, 4.0, out.DailyUsageUSD)
}

func TestUserSubscriptionFromServiceAdminUsesLinkedEntitlementExpiryAndStatus(t *testing.T) {
	legacyExpiresAt := time.Date(2026, 7, 17, 11, 48, 47, 0, time.UTC)
	entitlementExpiresAt := time.Date(2026, 7, 27, 11, 48, 47, 0, time.UTC)

	out := UserSubscriptionFromServiceAdmin(&service.UserSubscription{
		ID:        912,
		UserID:    989,
		GroupID:   12,
		StartsAt:  time.Date(2026, 5, 16, 19, 21, 36, 0, time.UTC),
		ExpiresAt: legacyExpiresAt,
		Status:    service.SubscriptionStatusExpired,
		EntitlementLink: &service.UserSubscriptionEntitlementLink{
			EntitlementID: 1103,
			Status:        service.SubscriptionStatusActive,
			ExpiresAt:     entitlementExpiresAt,
		},
	})

	require.NotNil(t, out)
	require.Equal(t, entitlementExpiresAt, out.ExpiresAt)
	require.Equal(t, service.SubscriptionStatusActive, out.Status)
	require.NotNil(t, out.EntitlementExpiresAt)
	require.Equal(t, entitlementExpiresAt, *out.EntitlementExpiresAt)
}
