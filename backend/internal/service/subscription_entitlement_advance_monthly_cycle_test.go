package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type advanceEntitlementMonthlyCycleRepo struct {
	*fakeSubscriptionEntitlementRepo

	txCalls     int
	updateCalls []SubscriptionEntitlementMonthlyCycleUpdate
	resetLogs   []SubscriptionEntitlementCycleResetLog
}

func newAdvanceEntitlementMonthlyCycleRepo(now time.Time) *advanceEntitlementMonthlyCycleRepo {
	return &advanceEntitlementMonthlyCycleRepo{
		fakeSubscriptionEntitlementRepo: newFakeSubscriptionEntitlementRepo(now),
	}
}

func (r *advanceEntitlementMonthlyCycleRepo) WithUserEntitlementMutationTx(ctx context.Context, _ int64, fn func(context.Context) error) error {
	r.txCalls++
	return fn(ctx)
}

func (r *advanceEntitlementMonthlyCycleRepo) LockEntitlementMonthlyCycle(_ context.Context, userID, entitlementID int64) (*SubscriptionEntitlementMonthlyCycleSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ent, ok := r.entitlements[entitlementID]
	if !ok || ent.UserID != userID {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	return &SubscriptionEntitlementMonthlyCycleSnapshot{
		ID:                 ent.ID,
		UserID:             ent.UserID,
		PlanID:             cloneInt64Ptr(ent.PlanID),
		Status:             ent.Status,
		StartsAt:           ent.StartsAt,
		ExpiresAt:          ent.ExpiresAt,
		MonthlyLimitUSD:    cloneFloat64Ptr(ent.MonthlyLimitUSD),
		MonthlyUsageUSD:    ent.MonthlyUsageUSD,
		MonthlyWindowStart: cloneTimePtr(ent.MonthlyWindowStart),
	}, nil
}

func (r *advanceEntitlementMonthlyCycleRepo) UpdateEntitlementMonthlyCycle(_ context.Context, update SubscriptionEntitlementMonthlyCycleUpdate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ent, ok := r.entitlements[update.EntitlementID]
	if !ok || ent.UserID != update.UserID {
		return ErrSubscriptionEntitlementNotFound
	}
	ent.MonthlyUsageUSD = update.NewMonthlyUsageUSD
	ent.MonthlyWindowStart = cloneTimeValue(update.NewMonthlyWindowStart)
	ent.ExpiresAt = update.NewExpiresAt
	ent.UpdatedAt = update.UpdatedAt
	r.updateCalls = append(r.updateCalls, update)
	return nil
}

func (r *advanceEntitlementMonthlyCycleRepo) InsertEntitlementCycleResetLog(_ context.Context, log SubscriptionEntitlementCycleResetLog) error {
	r.resetLogs = append(r.resetLogs, log)
	return nil
}

func TestAdvanceEntitlementMonthlyCycleSucceedsAndOnlyResetsMonthlyUsage(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	userID := int64(42)
	entitlementID := int64(3001)
	planID := int64(7001)
	monthlyLimit := 100.0
	weeklyLimit := 50.0
	dailyLimit := 10.0
	startsAt := now.Add(-10 * 24 * time.Hour)
	monthlyWindowStart := startsAt
	dailyWindowStart := now.Add(-6 * time.Hour)
	weeklyWindowStart := now.Add(-2 * 24 * time.Hour)
	expiresAt := startsAt.Add(120 * 24 * time.Hour)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	require.NoError(t, repo.Create(ctx, &SubscriptionEntitlement{
		ID:                 entitlementID,
		UserID:             userID,
		PlanID:             &planID,
		Name:               "V2 monthly card",
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          expiresAt,
		DailyLimitUSD:      &dailyLimit,
		WeeklyLimitUSD:     &weeklyLimit,
		MonthlyLimitUSD:    &monthlyLimit,
		DailyUsageUSD:      3.25,
		WeeklyUsageUSD:     22.5,
		MonthlyUsageUSD:    92.25,
		DailyWindowStart:   &dailyWindowStart,
		WeeklyWindowStart:  &weeklyWindowStart,
		MonthlyWindowStart: &monthlyWindowStart,
	}, []int64{101}))
	svc := NewSubscriptionEntitlementService(repo, nil)
	svc.SetNowFunc(func() time.Time { return now })

	result, err := svc.AdvanceMonthlyCycle(ctx, userID, entitlementID)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Entitlement)
	expectedResetAt := monthlyWindowStart.Add(monthlyCycleDuration)
	expectedDeductedSeconds := int64(expectedResetAt.Sub(now).Seconds())
	require.Equal(t, 92.25, result.PreviousMonthlyUsage)
	require.Equal(t, now, result.NewMonthlyWindowStart)
	require.Equal(t, expectedDeductedSeconds, result.DeductedSeconds)
	require.Equal(t, expiresAt.Add(-time.Duration(expectedDeductedSeconds)*time.Second), result.NewExpiresAt)
	require.Equal(t, result.NewExpiresAt, result.Entitlement.ExpiresAt)
	require.Zero(t, result.Entitlement.MonthlyUsageUSD)
	require.Equal(t, now, *result.Entitlement.MonthlyWindowStart)
	require.Equal(t, 3.25, result.Entitlement.DailyUsageUSD)
	require.Equal(t, 22.5, result.Entitlement.WeeklyUsageUSD)
	require.Equal(t, dailyWindowStart, *result.Entitlement.DailyWindowStart)
	require.Equal(t, weeklyWindowStart, *result.Entitlement.WeeklyWindowStart)
	require.Equal(t, 1, repo.txCalls)
	require.Len(t, repo.updateCalls, 1)
	require.Len(t, repo.resetLogs, 1)
	log := repo.resetLogs[0]
	require.Equal(t, userID, log.UserID)
	require.Equal(t, entitlementID, log.EntitlementID)
	require.NotNil(t, log.PlanID)
	require.Equal(t, planID, *log.PlanID)
	require.Equal(t, expiresAt, log.PreviousExpiresAt)
	require.Equal(t, result.NewExpiresAt, log.NewExpiresAt)
	require.Equal(t, 92.25, log.PreviousMonthlyUsageUSD)
	require.NotNil(t, log.PreviousMonthlyWindowStart)
	require.Equal(t, monthlyWindowStart, *log.PreviousMonthlyWindowStart)
	require.Equal(t, now, log.NewMonthlyWindowStart)
	require.Equal(t, result.DeductedDays, log.DeductedDays)
	require.Equal(t, result.DeductedSeconds, log.DeductedSeconds)
	require.True(t, log.ResetMonthlyUsage)
}

func TestAdvanceEntitlementMonthlyCycleSyncsLinkedLegacyLifecycle(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	userID := int64(42)
	entitlementID := int64(3002)
	legacySubscriptionID := int64(4002)
	planID := int64(7002)
	groupID := int64(101)
	monthlyLimit := 100.0
	startsAt := now.Add(-10 * 24 * time.Hour)
	monthlyWindowStart := startsAt
	dailyWindowStart := now.Add(-6 * time.Hour)
	weeklyWindowStart := now.Add(-2 * 24 * time.Hour)
	expiresAt := startsAt.Add(120 * 24 * time.Hour)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	require.NoError(t, repo.Create(ctx, &SubscriptionEntitlement{
		ID:                   entitlementID,
		UserID:               userID,
		LegacySubscriptionID: &legacySubscriptionID,
		PlanID:               &planID,
		PrimaryGroupID:       &groupID,
		Name:                 "Linked V2 monthly card",
		Status:               SubscriptionStatusActive,
		StartsAt:             startsAt,
		ExpiresAt:            expiresAt,
		MonthlyLimitUSD:      &monthlyLimit,
		DailyUsageUSD:        3.25,
		WeeklyUsageUSD:       22.5,
		MonthlyUsageUSD:      92.25,
		DailyWindowStart:     &dailyWindowStart,
		WeeklyWindowStart:    &weeklyWindowStart,
		MonthlyWindowStart:   &monthlyWindowStart,
	}, []int64{groupID}))
	legacyRepo := &linkedEntitlementUserSubRepoStub{sub: &UserSubscription{
		ID:        legacySubscriptionID,
		UserID:    userID,
		GroupID:   groupID,
		StartsAt:  startsAt.Add(-24 * time.Hour),
		ExpiresAt: expiresAt.Add(24 * time.Hour),
		Status:    SubscriptionStatusExpired,
	}}
	svc := NewSubscriptionEntitlementService(repo, nil)
	svc.SetLegacySubscriptionRepository(legacyRepo)
	svc.SetNowFunc(func() time.Time { return now })

	result, err := svc.AdvanceMonthlyCycle(ctx, userID, entitlementID)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, startsAt, legacyRepo.sub.StartsAt)
	require.Equal(t, result.NewExpiresAt, legacyRepo.sub.ExpiresAt)
	require.Equal(t, SubscriptionStatusActive, legacyRepo.sub.Status)
	require.Equal(t, dailyWindowStart, *legacyRepo.sub.DailyWindowStart)
	require.Equal(t, weeklyWindowStart, *legacyRepo.sub.WeeklyWindowStart)
	require.Equal(t, now, *legacyRepo.sub.MonthlyWindowStart)
	require.Equal(t, 3.25, legacyRepo.sub.DailyUsageUSD)
	require.Equal(t, 22.5, legacyRepo.sub.WeeklyUsageUSD)
	require.Zero(t, legacyRepo.sub.MonthlyUsageUSD)
}

func TestAdvanceEntitlementMonthlyCycleRejectsBelowThreshold(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	monthlyLimit := 100.0
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	require.NoError(t, repo.Create(context.Background(), &SubscriptionEntitlement{
		ID:                 1,
		UserID:             7,
		Status:             SubscriptionStatusActive,
		StartsAt:           now.Add(-10 * 24 * time.Hour),
		ExpiresAt:          now.Add(90 * 24 * time.Hour),
		MonthlyLimitUSD:    &monthlyLimit,
		MonthlyUsageUSD:    89.99,
		MonthlyWindowStart: cloneTimeValue(now.Add(-10 * 24 * time.Hour)),
	}, []int64{101}))
	svc := NewSubscriptionEntitlementService(repo, nil)
	svc.SetNowFunc(func() time.Time { return now })

	result, err := svc.AdvanceMonthlyCycle(context.Background(), 7, 1)

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrMonthlyCycleNotExhausted)
	require.Empty(t, repo.updateCalls)
	require.Empty(t, repo.resetLogs)
}

func TestAdvanceEntitlementMonthlyCycleRejectsWithoutMonthlyLimit(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	require.NoError(t, repo.Create(context.Background(), &SubscriptionEntitlement{
		ID:                 1,
		UserID:             7,
		Status:             SubscriptionStatusActive,
		StartsAt:           now.Add(-10 * 24 * time.Hour),
		ExpiresAt:          now.Add(90 * 24 * time.Hour),
		MonthlyUsageUSD:    100,
		MonthlyWindowStart: cloneTimeValue(now.Add(-10 * 24 * time.Hour)),
	}, []int64{101}))
	svc := NewSubscriptionEntitlementService(repo, nil)
	svc.SetNowFunc(func() time.Time { return now })

	result, err := svc.AdvanceMonthlyCycle(context.Background(), 7, 1)

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrMonthlyCycleNotExhausted)
	require.Empty(t, repo.updateCalls)
	require.Empty(t, repo.resetLogs)
}

func TestAdvanceEntitlementMonthlyCycleRejectsInactiveOrInvalidWindow(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	monthlyLimit := 100.0
	tests := []struct {
		name      string
		status    string
		startsAt  time.Time
		expiresAt time.Time
		wantErr   error
	}{
		{
			name:      "suspended",
			status:    SubscriptionStatusSuspended,
			startsAt:  now.Add(-10 * 24 * time.Hour),
			expiresAt: now.Add(90 * 24 * time.Hour),
			wantErr:   ErrSubscriptionEntitlementInactive,
		},
		{
			name:      "expired status",
			status:    SubscriptionStatusExpired,
			startsAt:  now.Add(-10 * 24 * time.Hour),
			expiresAt: now.Add(90 * 24 * time.Hour),
			wantErr:   ErrSubscriptionEntitlementExpired,
		},
		{
			name:      "future entitlement",
			status:    SubscriptionStatusActive,
			startsAt:  now.Add(24 * time.Hour),
			expiresAt: now.Add(90 * 24 * time.Hour),
			wantErr:   ErrSubscriptionEntitlementExpired,
		},
		{
			name:      "expired window",
			status:    SubscriptionStatusActive,
			startsAt:  now.Add(-90 * 24 * time.Hour),
			expiresAt: now.Add(-time.Second),
			wantErr:   ErrSubscriptionEntitlementExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newAdvanceEntitlementMonthlyCycleRepo(now)
			require.NoError(t, repo.Create(context.Background(), &SubscriptionEntitlement{
				ID:                 1,
				UserID:             7,
				Status:             tt.status,
				StartsAt:           tt.startsAt,
				ExpiresAt:          tt.expiresAt,
				MonthlyLimitUSD:    &monthlyLimit,
				MonthlyUsageUSD:    100,
				MonthlyWindowStart: cloneTimeValue(now.Add(-10 * 24 * time.Hour)),
			}, []int64{101}))
			svc := NewSubscriptionEntitlementService(repo, nil)
			svc.SetNowFunc(func() time.Time { return now })

			result, err := svc.AdvanceMonthlyCycle(context.Background(), 7, 1)

			require.Nil(t, result)
			require.ErrorIs(t, err, tt.wantErr)
			require.Empty(t, repo.updateCalls)
			require.Empty(t, repo.resetLogs)
		})
	}
}

func TestAdvanceEntitlementMonthlyCycleRejectsWhenValidityCannotCoverNextCycle(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	monthlyLimit := 100.0
	startsAt := now.Add(-10 * 24 * time.Hour)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	require.NoError(t, repo.Create(context.Background(), &SubscriptionEntitlement{
		ID:                 1,
		UserID:             7,
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.Add(30 * 24 * time.Hour),
		MonthlyLimitUSD:    &monthlyLimit,
		MonthlyUsageUSD:    100,
		MonthlyWindowStart: &startsAt,
	}, []int64{101}))
	svc := NewSubscriptionEntitlementService(repo, nil)
	svc.SetNowFunc(func() time.Time { return now })

	result, err := svc.AdvanceMonthlyCycle(context.Background(), 7, 1)

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrMonthlyCycleNoFutureTime)
	require.Empty(t, repo.updateCalls)
	require.Empty(t, repo.resetLogs)
}

func TestAdvanceEntitlementMonthlyCycleCrossUserReturnsNotFound(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	monthlyLimit := 100.0
	startsAt := now.Add(-10 * 24 * time.Hour)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	require.NoError(t, repo.Create(context.Background(), &SubscriptionEntitlement{
		ID:                 1,
		UserID:             7,
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.Add(120 * 24 * time.Hour),
		MonthlyLimitUSD:    &monthlyLimit,
		MonthlyUsageUSD:    100,
		MonthlyWindowStart: &startsAt,
	}, []int64{101}))
	svc := NewSubscriptionEntitlementService(repo, nil)
	svc.SetNowFunc(func() time.Time { return now })

	result, err := svc.AdvanceMonthlyCycle(context.Background(), 8, 1)

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrSubscriptionEntitlementNotFound)
	require.Empty(t, repo.updateCalls)
	require.Empty(t, repo.resetLogs)
}
