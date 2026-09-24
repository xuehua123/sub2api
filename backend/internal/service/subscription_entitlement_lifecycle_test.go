//go:build unit

package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestTryAutoAdvanceMonthlyCycleRules(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		mutate func(*SubscriptionEntitlement)
		want   bool
	}{
		{name: "exhausted", want: true},
		{name: "overdrawn", want: true, mutate: func(e *SubscriptionEntitlement) { e.MonthlyUsageUSD = 110 }},
		{name: "manual threshold only", mutate: func(e *SubscriptionEntitlement) { e.MonthlyUsageUSD = 95 }},
		{name: "disabled", mutate: func(e *SubscriptionEntitlement) { e.AutoAdvanceMonthly = false }},
		{name: "daily exhausted", mutate: func(e *SubscriptionEntitlement) { e.DailyUsageUSD = 10 }},
		{name: "weekly exhausted", mutate: func(e *SubscriptionEntitlement) { e.WeeklyUsageUSD = 50 }},
		{name: "no next cycle", mutate: func(e *SubscriptionEntitlement) { e.ExpiresAt = e.StartsAt.Add(monthlyCycleDuration) }},
		{name: "normal reset first", mutate: func(e *SubscriptionEntitlement) {
			v := now.Add(-31 * 24 * time.Hour)
			e.StartsAt = v
			e.MonthlyWindowStart = &v
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := newAdvanceEntitlementMonthlyCycleRepo(now)
			limit, daily, weekly := 100.0, 10.0, 50.0
			start, day, week := now.Add(-10*24*time.Hour), now.Add(-time.Hour), now.Add(-24*time.Hour)
			ent := &SubscriptionEntitlement{ID: 1, UserID: 42, Status: SubscriptionStatusActive, StartsAt: start, ExpiresAt: start.Add(120 * 24 * time.Hour),
				AutoAdvanceMonthly: true, MonthlyLimitUSD: &limit, DailyLimitUSD: &daily, WeeklyLimitUSD: &weekly,
				MonthlyUsageUSD: 100, DailyUsageUSD: 3, WeeklyUsageUSD: 4, MonthlyWindowStart: &start, DailyWindowStart: &day, WeeklyWindowStart: &week}
			if tc.mutate != nil {
				tc.mutate(ent)
			}
			require.NoError(t, repo.Create(ctx, ent, []int64{1}))
			svc := NewSubscriptionEntitlementService(repo, nil)
			svc.SetNowFunc(func() time.Time { return now })
			stale, err := repo.GetByID(ctx, ent.ID)
			require.NoError(t, err)
			got, err := svc.TryAutoAdvanceMonthlyCycle(ctx, stale)
			require.NoError(t, err)
			if tc.want {
				require.Len(t, repo.resetLogs, 1)
				require.True(t, repo.resetLogs[0].Automatic)
				require.Zero(t, got.MonthlyUsageUSD)
				require.Equal(t, 3.0, got.DailyUsageUSD)
				require.Equal(t, 4.0, got.WeeklyUsageUSD)
				// A stale request must not reset the replacement cycle even after it is used.
				repo.mu.Lock()
				repo.entitlements[ent.ID].MonthlyUsageUSD = 100
				repo.mu.Unlock()
				_, err = svc.TryAutoAdvanceMonthlyCycle(ctx, stale)
				require.NoError(t, err)
				require.Len(t, repo.resetLogs, 1)
			} else {
				require.Empty(t, repo.resetLogs)
				require.Equal(t, ent.ExpiresAt, got.ExpiresAt)
			}
		})
	}
}

func TestDeferredMonthlyAdmissionRunsAfterRateLimitsAndOnlyOnce(t *testing.T) {
	calls := 0
	state := &entitlementGenerationAdmission{id: 1, admit: func(context.Context, float64) (*APIKeyEntitlementAuthResult, error) {
		calls++
		return &APIKeyEntitlementAuthResult{Entitlement: &SubscriptionEntitlement{ID: 1}}, nil
	}}
	ctx := context.WithValue(context.Background(), entitlementGenerationAdmissionKey{}, state)
	svc := &BillingCacheService{cfg: &config.Config{}, userRPMCache: &userRPMCacheStub{userGroupCounts: []int{2, 1, 1}}}
	ent := &SubscriptionEntitlement{ID: 1}
	user := &User{ID: 1}
	group := &Group{ID: 1, RPMLimit: 1}
	require.ErrorIs(t, svc.CheckBillingEligibilityWithEntitlement(ctx, user, nil, group, nil, ent, "openai"), ErrGroupRPMExceeded)
	require.Zero(t, calls)
	require.ErrorIs(t, CompleteEntitlementGenerationAdmission(ctx, ent, 0), ErrBillingServiceUnavailable)
	require.NoError(t, svc.CheckBillingEligibilityWithEntitlement(ctx, user, nil, group, nil, ent, "openai"))
	require.Zero(t, calls, "eligibility alone must not consume a cycle")
	require.NoError(t, CompleteEntitlementGenerationAdmission(ctx, ent, 0))
	require.Equal(t, 1, calls)
	require.NoError(t, svc.CheckBillingEligibilityWithEntitlement(ctx, user, nil, group, nil, ent, "openai"))
	require.Equal(t, 1, calls)
}

func TestAdvanceEntitlementMonthlyCycleRejectsStalePreview(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	limit := 100.0
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	ent := &SubscriptionEntitlement{ID: 1, UserID: 42, Status: SubscriptionStatusActive, StartsAt: start, ExpiresAt: start.Add(90 * 24 * time.Hour), MonthlyLimitUSD: &limit, MonthlyUsageUSD: 95, MonthlyWindowStart: &start}
	require.NoError(t, repo.Create(context.Background(), ent, nil))
	svc := NewSubscriptionEntitlementService(repo, nil)
	svc.SetNowFunc(func() time.Time { return now })
	_, err := svc.AdvanceMonthlyCycle(context.Background(), 42, 1, &EntitlementMonthlyCycleExpectedState{MonthlyWindowStart: &start, ExpiresAt: ent.ExpiresAt.Add(time.Hour)})
	require.ErrorIs(t, err, ErrSubscriptionEntitlementTermConflict)
	require.Empty(t, repo.resetLogs)
}

func TestAutomaticAdvanceRejectsOversizedHoldWithoutChangingValidity(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	start := now.Add(-10 * 24 * time.Hour)
	limit := 100.0
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	ent := &SubscriptionEntitlement{ID: 1, UserID: 42, Status: SubscriptionStatusActive, StartsAt: start, ExpiresAt: start.Add(90 * 24 * time.Hour), AutoAdvanceMonthly: true, MonthlyLimitUSD: &limit, MonthlyUsageUSD: 101, MonthlyWindowStart: &start}
	require.NoError(t, repo.Create(context.Background(), ent, nil))
	svc := NewSubscriptionEntitlementService(repo, nil)
	svc.SetNowFunc(func() time.Time { return now })
	_, err := svc.TryAutoAdvanceMonthlyCycle(context.Background(), ent, 101)
	require.ErrorIs(t, err, ErrSubscriptionEntitlementQuotaExceeded)
	require.Empty(t, repo.resetLogs)
	current, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, ent.ExpiresAt, current.ExpiresAt)
	_, err = svc.TryAutoAdvanceMonthlyCycle(context.Background(), current, 30)
	require.NoError(t, err)
	require.Len(t, repo.resetLogs, 1)
}

func TestAutomaticCyclesHaveUniqueMarkersWithinOneSecond(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	start := now.Add(-10 * 24 * time.Hour)
	limit := 100.0
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	ent := &SubscriptionEntitlement{ID: 1, UserID: 42, Status: SubscriptionStatusActive, StartsAt: start, ExpiresAt: start.Add(150 * 24 * time.Hour), AutoAdvanceMonthly: true, MonthlyLimitUSD: &limit, MonthlyUsageUSD: 101, MonthlyWindowStart: &start}
	require.NoError(t, repo.Create(context.Background(), ent, nil))
	svc := NewSubscriptionEntitlementService(repo, nil)
	svc.SetNowFunc(func() time.Time { return now })
	first, err := svc.TryAutoAdvanceMonthlyCycle(context.Background(), ent)
	require.NoError(t, err)
	repo.entitlements[1].MonthlyUsageUSD = 101
	stale, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	second, err := svc.TryAutoAdvanceMonthlyCycle(context.Background(), stale)
	require.NoError(t, err)
	require.True(t, second.MonthlyWindowStart.After(*first.MonthlyWindowStart))
	repo.entitlements[1].MonthlyUsageUSD = 101
	_, err = svc.TryAutoAdvanceMonthlyCycle(context.Background(), stale)
	require.NoError(t, err)
	require.Len(t, repo.resetLogs, 2, "a stale request from the previous cycle must not advance again")
}

func TestAutomaticAdvanceAcceptsEntitlementStartedInCurrentSecond(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 900000000, time.UTC)
	start := now.Add(-200 * time.Millisecond)
	limit := 100.0
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	ent := &SubscriptionEntitlement{ID: 1, UserID: 42, Status: SubscriptionStatusActive, StartsAt: start, ExpiresAt: start.Add(90 * 24 * time.Hour), AutoAdvanceMonthly: true, MonthlyLimitUSD: &limit, MonthlyUsageUSD: 101, MonthlyWindowStart: &start}
	require.NoError(t, repo.Create(context.Background(), ent, nil))
	svc := NewSubscriptionEntitlementService(repo, nil)
	svc.SetNowFunc(func() time.Time { return now })
	got, err := svc.TryAutoAdvanceMonthlyCycle(context.Background(), ent)
	require.NoError(t, err)
	require.Len(t, repo.resetLogs, 1)
	require.Equal(t, now, *got.MonthlyWindowStart)
}

func TestPreviewEntitlementMonthlyCycleIgnoresExpiredWindowUsage(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	start := now.Add(-31 * 24 * time.Hour)
	limit := 100.0
	ent := &SubscriptionEntitlement{Status: SubscriptionStatusActive, StartsAt: start, ExpiresAt: now.Add(90 * 24 * time.Hour), MonthlyLimitUSD: &limit, MonthlyUsageUSD: 100, MonthlyWindowStart: &start}
	p := PreviewEntitlementMonthlyCycle(ent, now)
	require.False(t, p.CanAdvance)
	require.Equal(t, "below_threshold", p.Reason)
	require.Equal(t, 100.0, p.RemainingQuota)
}

func TestAPIKeyEntitlementAutoAdvanceAdmission(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		auto, deferred, groupUnavailable bool
		wantReset                        bool
		wantError                        bool
	}{
		{name: "read only", wantError: true},
		{name: "generation", auto: true},
		{name: "unavailable group", auto: true, groupUnavailable: true, wantError: true},
		{name: "websocket handshake", auto: true, deferred: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
			start := now.Add(-10 * 24 * time.Hour)
			limit := 100.0
			ent := testBindingEntitlement(1, 1, start, start.Add(90*24*time.Hour), SubscriptionStatusActive, []int64{20})
			ent.AutoAdvanceMonthly = true
			ent.MonthlyLimitUSD = &limit
			ent.MonthlyUsageUSD = 101
			ent.MonthlyWindowStart = &start
			svc, _, _ := newAPIKeyEntitlementBindingFixture(t, now, true, ent)
			repo := &advanceEntitlementMonthlyCycleRepo{fakeSubscriptionEntitlementRepo: svc.subscriptionEntitlementSvc.entitlementRepo.(*fakeSubscriptionEntitlementRepo)}
			svc.subscriptionEntitlementSvc.entitlementRepo = repo
			id := int64(1)
			key := &APIKey{ID: 10, UserID: 1, User: &User{ID: 1, Status: StatusActive}, Group: &Group{ID: 20, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}, SubscriptionEntitlementID: &id}
			got, err := svc.ResolveEntitlementForAPIKeyAuth(context.Background(), key, SubscriptionSwitchRequest{}, tc.groupUnavailable, tc.auto, tc.deferred)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
			}
			require.Len(t, repo.resetLogs, map[bool]int{true: 1, false: 0}[tc.wantReset])
			if tc.auto && !tc.wantError {
				require.Nil(t, got.LegacySubscription)
				admitted, err := svc.AdmitEntitlementGeneration(context.Background(), key, 1)
				require.NoError(t, err)
				require.Zero(t, admitted.Entitlement.MonthlyUsageUSD)
				require.Len(t, repo.resetLogs, 1)
			}
		})
	}
}

func TestAdmissionReadsSettingsAfterWebSocketHandshake(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	start := now.Add(-10 * 24 * time.Hour)
	limit := 100.0
	ent := testBindingEntitlement(1, 1, start, start.Add(90*24*time.Hour), SubscriptionStatusActive, []int64{20})
	ent.MonthlyLimitUSD = &limit
	ent.MonthlyUsageUSD = 50
	ent.MonthlyWindowStart = &start
	svc, _, _ := newAPIKeyEntitlementBindingFixture(t, now, true, ent)
	repo := &advanceEntitlementMonthlyCycleRepo{fakeSubscriptionEntitlementRepo: svc.subscriptionEntitlementSvc.entitlementRepo.(*fakeSubscriptionEntitlementRepo)}
	svc.subscriptionEntitlementSvc.entitlementRepo = repo
	id := int64(1)
	key := &APIKey{ID: 10, UserID: 1, User: &User{ID: 1, Status: StatusActive}, Group: &Group{ID: 20, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}, SubscriptionEntitlementID: &id}
	ctx := svc.WithEntitlementGenerationAdmission(context.Background(), key, 1)
	snapshot, err := repo.GetByID(ctx, 1)
	require.NoError(t, err)
	require.NoError(t, PrepareEntitlementGenerationAdmission(ctx, snapshot))
	require.Empty(t, repo.resetLogs)
	repo.entitlements[1].AutoAdvanceMonthly = true
	repo.entitlements[1].MonthlyUsageUSD = 101
	admitted, err := svc.AdmitEntitlementGeneration(ctx, key, 1)
	require.NoError(t, err)
	require.True(t, admitted.Entitlement.AutoAdvanceMonthly)
	require.Len(t, repo.resetLogs, 1)
	repo.entitlements[1].AutoAdvanceMonthly = false
	repo.entitlements[1].MonthlyUsageUSD = 101
	_, err = svc.AdmitEntitlementGeneration(ctx, key, 1)
	require.ErrorIs(t, err, ErrSubscriptionEntitlementQuotaExceeded)
	require.Len(t, repo.resetLogs, 1)
	repo.entitlements[1].AutoAdvanceMonthly = true
	_, err = svc.AdmitEntitlementGeneration(ctx, key, 1)
	require.NoError(t, err)
	require.Len(t, repo.resetLogs, 2)
}
