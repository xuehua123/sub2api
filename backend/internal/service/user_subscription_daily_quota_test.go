package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type dailyResetTrackingUserSubRepo struct {
	userSubRepoNoop

	resetDailyCalled bool
	resetDailyStart  time.Time
}

func (r *dailyResetTrackingUserSubRepo) ResetDailyUsage(_ context.Context, _ int64, _ *time.Time, windowStart time.Time) error {
	r.resetDailyCalled = true
	r.resetDailyStart = windowStart
	return nil
}

type windowActivationTrackingUserSubRepo struct {
	userSubRepoNoop

	activated   bool
	windowStart time.Time
}

func (r *windowActivationTrackingUserSubRepo) ActivateWindows(_ context.Context, _ int64, windowStart time.Time) error {
	r.activated = true
	r.windowStart = windowStart
	return nil
}

func TestCreateSubscription_InitializesAlignedUsageWindows(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	created, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       300,
		GroupID:      1,
		ValidityDays: 30,
	})

	require.NoError(t, err)
	require.NotNil(t, created.DailyWindowStart)
	require.NotNil(t, created.WeeklyWindowStart)
	require.NotNil(t, created.MonthlyWindowStart)
	require.Equal(t, created.StartsAt, *created.DailyWindowStart)
	require.Equal(t, created.StartsAt, *created.WeeklyWindowStart)
	require.Equal(t, created.StartsAt, *created.MonthlyWindowStart)
}

func TestAssignOrExtendSubscription_ExpiredDailyCardStartsNewOneTimeQuota(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	oldStart := time.Now().AddDate(0, 0, -3)
	oldWindowStart := oldStart
	subRepo.seed(&UserSubscription{
		ID:                 100,
		UserID:             200,
		GroupID:            1,
		StartsAt:           oldStart,
		ExpiresAt:          oldStart.AddDate(0, 0, 1),
		Status:             SubscriptionStatusExpired,
		DailyWindowStart:   &oldWindowStart,
		WeeklyWindowStart:  &oldWindowStart,
		MonthlyWindowStart: &oldWindowStart,
		DailyUsageUSD:      10,
		WeeklyUsageUSD:     20,
		MonthlyUsageUSD:    30,
		Notes:              "old",
	})
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	renewed, reused, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       200,
		GroupID:      1,
		ValidityDays: 1,
		Notes:        "new",
	})

	require.NoError(t, err)
	require.True(t, reused)
	require.True(t, renewed.HasOneTimeDailyQuota(), "过期后重新购买 1 日卡仍应被识别为一次性日额度")
	require.Equal(t, SubscriptionStatusActive, renewed.Status)
	require.True(t, renewed.StartsAt.After(oldStart), "重新购买过期订阅时应重置当前周期 StartsAt")
	require.False(t, renewed.ExpiresAt.After(renewed.StartsAt.AddDate(0, 0, 1)))
	require.NotNil(t, renewed.DailyWindowStart)
	require.Equal(t, renewed.StartsAt, *renewed.DailyWindowStart)
	require.Equal(t, 0.0, renewed.DailyUsageUSD)
	require.Equal(t, 0.0, renewed.WeeklyUsageUSD)
	require.Equal(t, 0.0, renewed.MonthlyUsageUSD)
	require.Equal(t, "old\nnew", renewed.Notes)
}

func TestCheckAndActivateWindow_UsesSubscriptionStartTime(t *testing.T) {
	repo := &windowActivationTrackingUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	startsAt := time.Date(2026, 5, 20, 15, 30, 0, 0, time.UTC)
	sub := &UserSubscription{
		ID:        1,
		UserID:    10,
		GroupID:   20,
		StartsAt:  startsAt,
		ExpiresAt: startsAt.AddDate(0, 0, 30),
	}

	err := svc.CheckAndActivateWindow(context.Background(), sub)

	require.NoError(t, err)
	require.True(t, repo.activated)
	require.Equal(t, startsAt, repo.windowStart)
	require.Equal(t, startsAt, *sub.DailyWindowStart)
	require.Equal(t, startsAt, *sub.WeeklyWindowStart)
	require.Equal(t, startsAt, *sub.MonthlyWindowStart)
}

func TestUserSubscriptionNeedsDailyReset_DailyCardKeepsOneTimeQuota(t *testing.T) {
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	dailyWindowStart := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	sub := &UserSubscription{
		StartsAt:         start,
		ExpiresAt:        start.Add(24 * time.Hour),
		DailyWindowStart: &dailyWindowStart,
		DailyUsageUSD:    10,
	}

	require.True(t, sub.HasOneTimeDailyQuota())
	require.False(t, sub.NeedsDailyResetAt(dailyWindowStart.Add(25*time.Hour)), "日卡应作为一次性配额，跨 0 点后不再刷新日额度")
}

func TestUserSubscriptionNeedsDailyReset_MultiDaySubscriptionStillRefreshes(t *testing.T) {
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	dailyWindowStart := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	sub := &UserSubscription{
		StartsAt:         start,
		ExpiresAt:        start.AddDate(0, 0, 2),
		DailyWindowStart: &dailyWindowStart,
	}

	require.False(t, sub.HasOneTimeDailyQuota())
	require.False(t, sub.NeedsDailyResetAt(dailyWindowStart.Add(24*time.Hour)), "旧 0 点窗口不能在购买时刻满 24 小时前提前刷新")
	require.True(t, sub.NeedsDailyResetAt(start.Add(24*time.Hour)), "多日订阅仍应按购买时刻对齐的 24 小时窗口刷新")
}

func TestUserSubscriptionDailyResetTime_DailyCardReturnsExpiry(t *testing.T) {
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	dailyWindowStart := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	expiresAt := start.Add(24 * time.Hour)
	sub := &UserSubscription{
		StartsAt:         start,
		ExpiresAt:        expiresAt,
		DailyWindowStart: &dailyWindowStart,
	}

	resetAt := sub.DailyResetTime()
	require.NotNil(t, resetAt)
	require.Equal(t, expiresAt, *resetAt, "日卡展示的日额度结束时间应为订阅过期时间")
}

func TestCheckAndResetWindows_DailyCardDoesNotResetDailyUsage(t *testing.T) {
	now := time.Now()
	startsAt := now.Add(-23 * time.Hour)
	dailyWindowStart := now.Add(-25 * time.Hour)
	repo := &dailyResetTrackingUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:               1,
		UserID:           10,
		GroupID:          20,
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.Add(24 * time.Hour),
		DailyUsageUSD:    10,
		DailyWindowStart: &dailyWindowStart,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub)

	require.NoError(t, err)
	require.False(t, repo.resetDailyCalled, "日卡作为一次性配额，过了 24 小时日窗口也不应重置 daily usage")
	require.Equal(t, 10.0, sub.DailyUsageUSD)
}

func TestCheckAndResetWindows_MultiDaySubscriptionStillResetsDailyUsage(t *testing.T) {
	now := time.Now()
	startsAt := now.Add(-48 * time.Hour)
	dailyWindowStart := startOfDay(startsAt)
	repo := &dailyResetTrackingUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:               1,
		UserID:           10,
		GroupID:          20,
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.AddDate(0, 0, 2),
		DailyUsageUSD:    10,
		DailyWindowStart: &dailyWindowStart,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub)

	require.NoError(t, err)
	require.True(t, repo.resetDailyCalled, "多日订阅仍应重置过期 daily window")
	require.Equal(t, 0.0, sub.DailyUsageUSD)
	require.Equal(t, startsAt.Add(48*time.Hour), repo.resetDailyStart)
}

func TestAlignedCycleStart_PreservesOriginalPurchaseTime(t *testing.T) {
	startsAt := time.Date(2026, 5, 20, 15, 30, 0, 0, time.UTC)
	now := startsAt.Add(35 * time.Hour)

	aligned, ok := alignedCycleStart(startsAt, 24*time.Hour, now)

	require.True(t, ok)
	require.Equal(t, startsAt.Add(24*time.Hour), aligned)
}

func TestEffectiveWindowStartAt_PrefersManualResetAnchor(t *testing.T) {
	startsAt := time.Date(2026, 5, 1, 15, 30, 0, 0, time.UTC)
	manualResetAt := time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)

	start := effectiveWindowStartAt(&manualResetAt, startsAt, 30*24*time.Hour, now)

	require.NotNil(t, start)
	require.Equal(t, manualResetAt, *start)
	require.False(t, needsWindowResetAt(&manualResetAt, startsAt, 30*24*time.Hour, manualResetAt.Add(29*24*time.Hour)))
	require.True(t, needsWindowResetAt(&manualResetAt, startsAt, 30*24*time.Hour, manualResetAt.Add(30*24*time.Hour)))
}

func TestEffectiveWindowStartAt_PrefersFutureAdvanceAnchor(t *testing.T) {
	startsAt := time.Date(2026, 4, 22, 8, 40, 26, 0, time.UTC)
	advanceAnchor := time.Date(2026, 5, 22, 8, 40, 26, 0, time.UTC)
	now := time.Date(2026, 5, 21, 8, 31, 0, 0, time.UTC)

	start := effectiveWindowStartAt(&advanceAnchor, startsAt, 30*24*time.Hour, now)

	require.NotNil(t, start)
	require.Equal(t, advanceAnchor, *start)
	require.False(t, needsWindowResetAt(&advanceAnchor, startsAt, 30*24*time.Hour, now))
	require.True(t, needsWindowResetAt(&advanceAnchor, startsAt, 30*24*time.Hour, advanceAnchor.Add(30*24*time.Hour)))
}

func TestNeedsWindowResetAt_LegacyMonthlyAnchorDoesNotResetBeforeAlignedCycleEnds(t *testing.T) {
	startsAt := time.Date(2026, 5, 7, 15, 31, 55, 0, time.UTC)
	legacyWindowStart := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 20, 14, 32, 0, 0, time.UTC)

	require.False(t, needsWindowResetAt(&legacyWindowStart, startsAt, 30*24*time.Hour, now))
}

func TestNeedsWindowResetAt_LegacyMonthlyAnchorResetsAfterFullAlignedCycle(t *testing.T) {
	startsAt := time.Date(2026, 5, 7, 15, 31, 55, 0, time.UTC)
	legacyWindowStart := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	now := startsAt.Add(30 * 24 * time.Hour)

	require.True(t, needsWindowResetAt(&legacyWindowStart, startsAt, 30*24*time.Hour, now))
	require.Equal(t, startsAt.Add(30*24*time.Hour), resolvedWindowResetStart(&legacyWindowStart, startsAt, 30*24*time.Hour, now))
}

func TestValidateAndCheckLimits_DailyCardDoesNotAllowSecondQuotaAfterMidnight(t *testing.T) {
	start := time.Now().Add(-23 * time.Hour)
	dailyWindowStart := time.Now().Add(-25 * time.Hour)
	dailyLimit := 10.0
	sub := &UserSubscription{
		Status:           SubscriptionStatusActive,
		StartsAt:         start,
		ExpiresAt:        start.Add(24 * time.Hour),
		DailyWindowStart: &dailyWindowStart,
		DailyUsageUSD:    dailyLimit + 0.01,
	}
	group := &Group{
		SubscriptionType: SubscriptionTypeSubscription,
		DailyLimitUSD:    &dailyLimit,
	}
	svc := NewSubscriptionService(groupRepoNoop{}, userSubRepoNoop{}, nil, nil, nil)

	needsMaintenance, err := svc.ValidateAndCheckLimits(sub, group)

	require.False(t, needsMaintenance, "日卡跨过日窗口后不应触发 daily reset 维护")
	require.True(t, errors.Is(err, ErrDailyLimitExceeded))
	require.Equal(t, dailyLimit+0.01, sub.DailyUsageUSD, "热路径不应清零日卡已用额度")
}
