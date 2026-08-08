//go:build unit

package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestUsageBillingEntitlementWindowUsageActivatesDailyAtMidnightAndPeriodicAtTermStart(t *testing.T) {
	base := time.Date(2026, 6, 11, 0, 0, 0, 0, timezone.Location())
	startsAt := base.Add(9 * time.Hour)
	now := base.AddDate(0, 0, 2).Add(18 * time.Hour)
	usage := &usageBillingEntitlementWindowUsage{}

	usage.activateAndReset(startsAt, startsAt.AddDate(0, 0, 45), now)

	require.Equal(t, timezone.StartOfDay(now), *usage.dailyWindowStart)
	require.Equal(t, startsAt, *usage.weeklyWindowStart)
	require.Equal(t, startsAt, *usage.monthlyWindowStart)
}

func TestUsageBillingEntitlementWindowUsageResetsDailyAtCalendarMidnight(t *testing.T) {
	base := time.Date(2026, 6, 11, 0, 0, 0, 0, timezone.Location())
	startsAt := base.Add(9 * time.Hour)
	dailyStart := base.Add(16*time.Hour + 45*time.Minute)
	weeklyStart := startsAt
	monthlyStart := startsAt
	usage := &usageBillingEntitlementWindowUsage{
		dailyWindowStart:   &dailyStart,
		weeklyWindowStart:  &weeklyStart,
		monthlyWindowStart: &monthlyStart,
		dailyUsageUSD:      7,
		weeklyUsageUSD:     8,
		monthlyUsageUSD:    9,
	}

	usage.activateAndReset(startsAt, startsAt.AddDate(0, 0, 45), base.Add(23*time.Hour+59*time.Minute))
	require.Equal(t, 7.0, usage.dailyUsageUSD)

	nextDay := base.AddDate(0, 0, 1).Add(time.Minute)
	usage.activateAndReset(startsAt, startsAt.AddDate(0, 0, 45), nextDay)
	require.Equal(t, timezone.StartOfDay(nextDay), *usage.dailyWindowStart)
	require.Zero(t, usage.dailyUsageUSD)
	require.Equal(t, 8.0, usage.weeklyUsageUSD)
	require.Equal(t, 9.0, usage.monthlyUsageUSD)
}

func TestUsageBillingEntitlementWindowUsageDoesNotResetOneDayCardAtMidnight(t *testing.T) {
	base := time.Date(2026, 6, 11, 0, 0, 0, 0, timezone.Location())
	startsAt := base.Add(17 * time.Hour)
	dailyStart := base
	usage := &usageBillingEntitlementWindowUsage{
		dailyWindowStart: &dailyStart,
		dailyUsageUSD:    7,
	}

	usage.activateAndReset(startsAt, startsAt.AddDate(0, 0, 1), base.AddDate(0, 0, 1).Add(time.Hour))

	require.Equal(t, dailyStart, *usage.dailyWindowStart)
	require.Equal(t, 7.0, usage.dailyUsageUSD)
}
