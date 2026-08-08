//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

// resetQuotaUserSubRepoStub 支持 GetByID、ResetUsageWindows，
// 其余方法继承 userSubRepoNoop（panic）。
type resetQuotaUserSubRepoStub struct {
	userSubRepoNoop

	sub *UserSubscription

	resetDailyCalled   bool
	resetWeeklyCalled  bool
	resetMonthlyCalled bool
	resetDailyErr      error
	resetWeeklyErr     error
	resetMonthlyErr    error
	dailyStart         time.Time
	periodicStart      time.Time
	lockCalls          int
	updateCalls        int
}

func (r *resetQuotaUserSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *resetQuotaUserSubRepoStub) GetByIDIncludeDeletedForUpdate(_ context.Context, id int64) (*UserSubscription, error) {
	r.lockCalls++
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *resetQuotaUserSubRepoStub) Update(_ context.Context, sub *UserSubscription) error {
	if r.sub == nil || sub == nil || r.sub.ID != sub.ID {
		return ErrSubscriptionNotFound
	}
	r.updateCalls++
	cp := *sub
	r.sub = &cp
	return nil
}

func (r *resetQuotaUserSubRepoStub) ResetUsageWindows(_ context.Context, _ int64, resetDaily, resetWeekly, resetMonthly bool, dailyStart, periodicStart time.Time) error {
	r.resetDailyCalled = resetDaily
	r.resetWeeklyCalled = resetWeekly
	r.resetMonthlyCalled = resetMonthly
	r.dailyStart = dailyStart
	r.periodicStart = periodicStart
	if resetDaily && r.resetDailyErr != nil {
		return r.resetDailyErr
	}
	if resetWeekly && r.resetWeeklyErr != nil {
		return r.resetWeeklyErr
	}
	if resetMonthly && r.resetMonthlyErr != nil {
		return r.resetMonthlyErr
	}
	if r.sub == nil {
		return nil
	}
	if resetDaily {
		r.sub.DailyUsageUSD = 0
		r.sub.DailyWindowStart = &dailyStart
	}
	if resetWeekly {
		r.sub.WeeklyUsageUSD = 0
		r.sub.WeeklyWindowStart = &periodicStart
	}
	if resetMonthly {
		r.sub.MonthlyUsageUSD = 0
		r.sub.MonthlyWindowStart = &periodicStart
	}
	return nil
}

func (r *resetQuotaUserSubRepoStub) ResetDailyUsage(_ context.Context, _ int64, _ *time.Time, windowStart time.Time) error {
	r.resetDailyCalled = true
	if r.resetDailyErr == nil && r.sub != nil {
		r.sub.DailyUsageUSD = 0
		r.sub.DailyWindowStart = &windowStart
	}
	return r.resetDailyErr
}

func (r *resetQuotaUserSubRepoStub) ResetWeeklyUsage(_ context.Context, _ int64, _ *time.Time, windowStart time.Time) error {
	r.resetWeeklyCalled = true
	if r.resetWeeklyErr == nil && r.sub != nil {
		r.sub.WeeklyUsageUSD = 0
		r.sub.WeeklyWindowStart = &windowStart
	}
	return r.resetWeeklyErr
}

func (r *resetQuotaUserSubRepoStub) ResetMonthlyUsage(_ context.Context, _ int64, _ *time.Time, windowStart time.Time) error {
	r.resetMonthlyCalled = true
	if r.resetMonthlyErr == nil && r.sub != nil {
		r.sub.MonthlyUsageUSD = 0
		r.sub.MonthlyWindowStart = &windowStart
	}
	return r.resetMonthlyErr
}

func newResetQuotaSvc(stub *resetQuotaUserSubRepoStub) *SubscriptionService {
	return NewSubscriptionService(groupRepoNoop{}, stub, nil, nil, nil)
}

func TestAdminResetQuota_ResetBoth(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 1, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)
	resetAt := time.Date(2026, 7, 1, 10, 37, 42, 123, time.UTC)
	svc.now = func() time.Time { return resetAt }

	result, err := svc.AdminResetQuota(context.Background(), 1, true, true, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, stub.resetDailyCalled, "应调用 ResetDailyUsage")
	require.True(t, stub.resetWeeklyCalled, "应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
	// 手动重置后日窗口锚定当天 0 点（保持 0 点刷新节奏），周窗口锚定重置时刻。
	require.Equal(t, timezone.StartOfDay(resetAt), stub.dailyStart)
	require.Equal(t, resetAt, stub.periodicStart)
	require.Equal(t, timezone.StartOfDay(resetAt), *result.DailyWindowStart)
	require.Equal(t, resetAt, *result.WeeklyWindowStart)
}

func TestAdminResetQuota_LinkedEntitlementResetsEntitlementUsage(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 37, 42, 0, time.UTC)
	entitlementID := int64(92)
	legacySubscriptionID := int64(11)
	groupID := int64(28)
	dailyStart := timezone.StartOfDay(now)
	weeklyStart := now.Add(-3 * 24 * time.Hour)
	monthlyStart := now.Add(-15 * 24 * time.Hour)
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{
			ID:                 legacySubscriptionID,
			UserID:             10,
			GroupID:            20,
			DailyUsageUSD:      4.4,
			WeeklyUsageUSD:     5.5,
			MonthlyUsageUSD:    6.6,
			DailyWindowStart:   &dailyStart,
			WeeklyWindowStart:  &weeklyStart,
			MonthlyWindowStart: &monthlyStart,
			EntitlementLink: &UserSubscriptionEntitlementLink{
				EntitlementID: entitlementID,
			},
		},
	}
	entitlementRepo := newFakeSubscriptionEntitlementRepo(now)
	entitlementRepo.entitlements[entitlementID] = &SubscriptionEntitlement{
		ID:                   entitlementID,
		UserID:               10,
		LegacySubscriptionID: &legacySubscriptionID,
		PrimaryGroupID:       &groupID,
		Name:                 "Linked Plan",
		Status:               SubscriptionStatusActive,
		StartsAt:             now.Add(-time.Hour),
		ExpiresAt:            now.Add(24 * time.Hour),
		DailyUsageUSD:        1.1,
		WeeklyUsageUSD:       2.2,
		MonthlyUsageUSD:      3.3,
		GroupGrants:          testGroupGrants([]int64{groupID}),
	}
	svc := newResetQuotaSvc(stub)
	svc.entitlementSvc = NewSubscriptionEntitlementService(entitlementRepo, &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{}})
	svc.entitlementSvc.SetLegacySubscriptionRepository(stub)
	svc.entitlementSvc.SetNowFunc(func() time.Time { return now })

	result, err := svc.AdminResetQuota(context.Background(), legacySubscriptionID, true, true, true)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, legacySubscriptionID, result.ID)
	require.Equal(t, 2, stub.lockCalls, "linked alias must be validated before mutation and then synced from the refreshed entitlement")
	require.Equal(t, 1, stub.updateCalls)
	require.False(t, stub.resetDailyCalled, "linked alias must receive the entitlement's absolute lifecycle snapshot")
	require.False(t, stub.resetWeeklyCalled, "linked alias must receive the entitlement's absolute lifecycle snapshot")
	require.False(t, stub.resetMonthlyCalled, "linked alias must receive the entitlement's absolute lifecycle snapshot")
	require.Zero(t, stub.sub.DailyUsageUSD)
	require.Zero(t, stub.sub.WeeklyUsageUSD)
	require.Zero(t, stub.sub.MonthlyUsageUSD)
	require.Equal(t, dailyStart, *stub.sub.DailyWindowStart)
	require.Equal(t, now, *stub.sub.WeeklyWindowStart)
	require.Equal(t, now, *stub.sub.MonthlyWindowStart)
	require.Len(t, entitlementRepo.resetCalls, 1)
	require.Equal(t, entitlementID, entitlementRepo.resetCalls[0].id)
	require.True(t, entitlementRepo.resetCalls[0].resetDaily)
	require.True(t, entitlementRepo.resetCalls[0].resetWeekly)
	require.True(t, entitlementRepo.resetCalls[0].resetMonthly)
	require.Equal(t, float64(0), entitlementRepo.entitlements[entitlementID].DailyUsageUSD)
	require.Equal(t, float64(0), entitlementRepo.entitlements[entitlementID].WeeklyUsageUSD)
	require.Equal(t, float64(0), entitlementRepo.entitlements[entitlementID].MonthlyUsageUSD)
}

func TestAdminResetQuota_LinkedEntitlementValidatesAliasBeforeMutation(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 37, 42, 0, time.UTC)
	const (
		entitlementID = int64(192)
		userID        = int64(10)
		groupID       = int64(28)
	)
	deletedAt := now.Add(-time.Hour)

	tests := []struct {
		name          string
		legacyID      int64
		aliasRepo     *resetQuotaUserSubRepoStub
		wantErr       error
		configureRepo bool
	}{
		{
			name:          "invalid linked ID",
			legacyID:      0,
			aliasRepo:     &resetQuotaUserSubRepoStub{},
			wantErr:       ErrSubscriptionEntitlementAliasUnavailable,
			configureRepo: true,
		},
		{
			name:          "missing repository",
			legacyID:      991,
			wantErr:       ErrSubscriptionEntitlementAliasUnavailable,
			configureRepo: false,
		},
		{
			name:      "missing alias",
			legacyID:  992,
			aliasRepo: &resetQuotaUserSubRepoStub{},
			wantErr:   ErrSubscriptionEntitlementAliasUnavailable,
		},
		{
			name:     "cross-user alias",
			legacyID: 993,
			aliasRepo: &resetQuotaUserSubRepoStub{sub: &UserSubscription{
				ID:      993,
				UserID:  userID + 1,
				GroupID: groupID,
			}},
			wantErr: ErrSubscriptionEntitlementNotFound,
		},
		{
			name:     "soft-deleted alias",
			legacyID: 994,
			aliasRepo: &resetQuotaUserSubRepoStub{sub: &UserSubscription{
				ID:        994,
				UserID:    userID,
				GroupID:   groupID,
				DeletedAt: &deletedAt,
			}},
			wantErr: ErrSubscriptionEntitlementAliasUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entitlementRepo := newFakeSubscriptionEntitlementRepo(now)
			legacyID := tt.legacyID
			primaryGroupID := groupID
			entitlementRepo.entitlements[entitlementID] = &SubscriptionEntitlement{
				ID:                   entitlementID,
				UserID:               userID,
				LegacySubscriptionID: &legacyID,
				PrimaryGroupID:       &primaryGroupID,
				Status:               SubscriptionStatusActive,
				StartsAt:             now.Add(-time.Hour),
				ExpiresAt:            now.Add(24 * time.Hour),
				DailyUsageUSD:        9,
				GroupGrants:          testGroupGrants([]int64{groupID}),
			}

			var userSubRepo UserSubscriptionRepository
			if tt.configureRepo || tt.aliasRepo != nil {
				userSubRepo = tt.aliasRepo
			}
			svc := NewSubscriptionService(groupRepoNoop{}, userSubRepo, nil, nil, nil)
			svc.entitlementSvc = NewSubscriptionEntitlementService(entitlementRepo, nil)
			svc.entitlementSvc.SetLegacySubscriptionRepository(userSubRepo)
			svc.entitlementSvc.SetNowFunc(func() time.Time { return now })

			_, err := svc.AdminResetQuota(context.Background(), -entitlementID, true, false, false)

			require.ErrorIs(t, err, tt.wantErr)
			require.Empty(t, entitlementRepo.resetCalls, "alias validation must complete before entitlement usage mutates")
			require.Equal(t, 9.0, entitlementRepo.entitlements[entitlementID].DailyUsageUSD)
		})
	}
}

func TestAdminResetQuota_ResetDailyOnly(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 2, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 2, true, false, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, stub.resetDailyCalled, "应调用 ResetDailyUsage")
	require.False(t, stub.resetWeeklyCalled, "不应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_ResetWeeklyOnly(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 3, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 3, false, true, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, stub.resetDailyCalled, "不应调用 ResetDailyUsage")
	require.True(t, stub.resetWeeklyCalled, "应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_BothFalseReturnsError(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 7, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 7, false, false, false)

	require.ErrorIs(t, err, ErrInvalidInput)
	require.False(t, stub.resetDailyCalled)
	require.False(t, stub.resetWeeklyCalled)
	require.False(t, stub.resetMonthlyCalled)
}

func TestAdminResetQuota_SubscriptionNotFound(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{sub: nil}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 999, true, true, true)

	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	require.False(t, stub.resetDailyCalled)
	require.False(t, stub.resetWeeklyCalled)
	require.False(t, stub.resetMonthlyCalled)
}

func TestAdminResetQuota_ResetDailyUsageError(t *testing.T) {
	dbErr := errors.New("db error")
	stub := &resetQuotaUserSubRepoStub{
		sub:           &UserSubscription{ID: 4, UserID: 10, GroupID: 20},
		resetDailyErr: dbErr,
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 4, true, true, false)

	require.ErrorIs(t, err, dbErr)
	require.True(t, stub.resetDailyCalled)
	require.True(t, stub.resetWeeklyCalled, "原子重置应在一次调用中提交所选窗口")
}

func TestAdminResetQuota_ResetWeeklyUsageError(t *testing.T) {
	dbErr := errors.New("db error")
	stub := &resetQuotaUserSubRepoStub{
		sub:            &UserSubscription{ID: 5, UserID: 10, GroupID: 20},
		resetWeeklyErr: dbErr,
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 5, false, true, false)

	require.ErrorIs(t, err, dbErr)
	require.True(t, stub.resetWeeklyCalled)
}

func TestAdminResetQuota_ResetMonthlyOnly(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 8, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 8, false, false, true)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, stub.resetDailyCalled, "不应调用 ResetDailyUsage")
	require.False(t, stub.resetWeeklyCalled, "不应调用 ResetWeeklyUsage")
	require.True(t, stub.resetMonthlyCalled, "应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_BeforeStartsAtSameDayPreservesAutomaticBoundary(t *testing.T) {
	startsAt := time.Date(2026, 7, 1, 15, 0, 0, 0, time.UTC)
	resetAt := time.Date(2026, 7, 1, 10, 37, 42, 123, time.UTC)
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{
			ID:        10,
			UserID:    10,
			GroupID:   20,
			StartsAt:  startsAt,
			ExpiresAt: startsAt.Add(45 * 24 * time.Hour),
		},
	}
	svc := newResetQuotaSvc(stub)
	svc.now = func() time.Time { return resetAt }

	result, err := svc.AdminResetQuota(context.Background(), 10, false, false, true)

	require.NoError(t, err)
	require.Equal(t, resetAt, *result.MonthlyWindowStart)
	boundary, ok := result.automaticWindowStartAt(result.MonthlyWindowStart, 30*24*time.Hour, resetAt.Add(30*24*time.Hour))
	require.True(t, ok)
	require.Equal(t, resetAt.Add(30*24*time.Hour), boundary)
}

func TestAdminResetQuota_ResetMonthlyUsageError(t *testing.T) {
	dbErr := errors.New("db error")
	stub := &resetQuotaUserSubRepoStub{
		sub:             &UserSubscription{ID: 9, UserID: 10, GroupID: 20},
		resetMonthlyErr: dbErr,
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 9, false, false, true)

	require.ErrorIs(t, err, dbErr)
	require.True(t, stub.resetMonthlyCalled)
}

func TestAdminResetQuota_ReturnsRefreshedSub(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{
			ID:            6,
			UserID:        10,
			GroupID:       20,
			DailyUsageUSD: 99.9,
		},
	}

	svc := newResetQuotaSvc(stub)
	result, err := svc.AdminResetQuota(context.Background(), 6, true, false, false)

	require.NoError(t, err)
	// ResetUsageWindows stub 会将 sub.DailyUsageUSD 归零，
	// 服务应返回第二次 GetByID 的刷新值而非初始的 99.9
	require.Equal(t, float64(0), result.DailyUsageUSD, "返回的订阅应反映已归零的用量")
	require.True(t, stub.resetDailyCalled)
}

func TestAdminResetQuota_MonthlyResetBecomesNewCycleAnchor(t *testing.T) {
	startsAt := time.Date(2026, 5, 1, 15, 30, 0, 0, time.UTC)
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{
			ID:                 10,
			UserID:             10,
			GroupID:            20,
			StartsAt:           startsAt,
			ExpiresAt:          startsAt.AddDate(0, 0, 90),
			MonthlyUsageUSD:    99.9,
			MonthlyWindowStart: func() *time.Time { t := startsAt; return &t }(),
		},
	}

	svc := newResetQuotaSvc(stub)
	result, err := svc.AdminResetQuota(context.Background(), 10, false, false, true)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, stub.resetMonthlyCalled)
	require.NotNil(t, result.MonthlyWindowStart)
	resetAt := result.MonthlyResetTime()
	require.NotNil(t, resetAt)
	require.Equal(t, result.MonthlyWindowStart.Add(30*24*time.Hour), *resetAt)
}
