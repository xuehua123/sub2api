package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPreviewMonthlyCycleAdjustmentAdvanceNextCycleLinkedEntitlement(t *testing.T) {
	now := time.Date(2026, 6, 17, 11, 18, 43, 0, time.UTC)
	groupID := int64(28)
	monthlyLimit := 100.0
	startsAt := now.Add(-20 * 24 * time.Hour)
	monthlyWindowStart := startsAt
	expiresAt := startsAt.Add(90 * 24 * time.Hour)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	repo.entitlements[91] = &SubscriptionEntitlement{
		ID:                 91,
		UserID:             7,
		PrimaryGroupID:     &groupID,
		Name:               "Pro Plan",
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          expiresAt,
		MonthlyLimitUSD:    &monthlyLimit,
		MonthlyUsageUSD:    95,
		MonthlyWindowStart: &monthlyWindowStart,
		GroupGrants:        testGroupGrants([]int64{groupID}),
	}
	userSubs := &linkedEntitlementUserSubRepoStub{
		sub: &UserSubscription{
			ID:      912,
			UserID:  7,
			GroupID: groupID,
			Status:  SubscriptionStatusActive,
			EntitlementLink: &UserSubscriptionEntitlementLink{
				EntitlementID: 91,
			},
		},
	}
	svc := newSubscriptionServiceWithLinkedEntitlementCycleRepo(repo, userSubs)

	preview, err := svc.PreviewMonthlyCycleAdjustment(context.Background(), 912, MonthlyCycleAdjustmentInput{
		Mode: MonthlyCycleAdjustmentAdvanceNextCycle,
		Now:  now,
	})

	require.NoError(t, err)
	require.True(t, preview.CanApply)
	require.Equal(t, MonthlyCycleAdjustmentTargetEntitlement, preview.TargetType)
	require.NotNil(t, preview.EntitlementID)
	require.Equal(t, int64(91), *preview.EntitlementID)
	require.Equal(t, now, preview.NewMonthlyWindowStart)
	require.Equal(t, int64((10*24*time.Hour)/time.Second), preview.DeductedSeconds)
	require.Equal(t, expiresAt.Add(-10*24*time.Hour), preview.NewExpiresAt)
	require.Equal(t, 95.0, preview.CurrentMonthlyUsageUSD)
	require.Zero(t, preview.NewMonthlyUsageUSD)
}

func TestApplyMonthlyCycleAdjustmentCompensateResetLinkedEntitlement(t *testing.T) {
	now := time.Date(2026, 6, 17, 11, 18, 43, 0, time.UTC)
	groupID := int64(28)
	legacySubscriptionID := int64(912)
	monthlyLimit := 100.0
	startsAt := now.Add(-10 * 24 * time.Hour)
	monthlyWindowStart := startsAt
	expiresAt := startsAt.Add(60 * 24 * time.Hour)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	repo.entitlements[91] = &SubscriptionEntitlement{
		ID:                   91,
		UserID:               7,
		LegacySubscriptionID: &legacySubscriptionID,
		PrimaryGroupID:       &groupID,
		Name:                 "Pro Plan",
		Status:               SubscriptionStatusActive,
		StartsAt:             startsAt,
		ExpiresAt:            expiresAt,
		MonthlyLimitUSD:      &monthlyLimit,
		MonthlyUsageUSD:      52,
		MonthlyWindowStart:   &monthlyWindowStart,
		GroupGrants:          testGroupGrants([]int64{groupID}),
	}
	userSubs := &linkedEntitlementUserSubRepoStub{
		sub: &UserSubscription{
			ID:              legacySubscriptionID,
			UserID:          7,
			GroupID:         groupID,
			StartsAt:        startsAt.Add(-24 * time.Hour),
			ExpiresAt:       expiresAt.Add(24 * time.Hour),
			Status:          SubscriptionStatusExpired,
			MonthlyUsageUSD: 12,
			EntitlementLink: &UserSubscriptionEntitlementLink{
				EntitlementID: 91,
			},
		},
	}
	svc := newSubscriptionServiceWithLinkedEntitlementCycleRepo(repo, userSubs)

	preview, err := svc.ApplyMonthlyCycleAdjustment(context.Background(), 912, MonthlyCycleAdjustmentInput{
		Mode:   MonthlyCycleAdjustmentCompensateReset,
		Reason: "payment callback recovered by support",
		Now:    now,
	})

	require.NoError(t, err)
	require.True(t, preview.CanApply)
	require.Equal(t, expiresAt, preview.NewExpiresAt)
	require.Zero(t, preview.DeductedSeconds)
	require.True(t, preview.ResetMonthlyUsage)
	require.Len(t, repo.updateCalls, 1)
	require.Len(t, repo.resetLogs, 1)
	require.Equal(t, expiresAt, repo.entitlements[91].ExpiresAt)
	require.Equal(t, now, *repo.entitlements[91].MonthlyWindowStart)
	require.Zero(t, repo.entitlements[91].MonthlyUsageUSD)
	require.Zero(t, repo.resetLogs[0].DeductedSeconds)
	require.True(t, repo.resetLogs[0].ResetMonthlyUsage)
	require.Equal(t, MonthlyCycleAdjustmentCompensateReset, repo.resetLogs[0].Mode)
	require.Equal(t, "payment callback recovered by support", repo.resetLogs[0].Reason)
	require.Nil(t, repo.resetLogs[0].AdminID)
	require.Equal(t, startsAt, userSubs.sub.StartsAt)
	require.Equal(t, expiresAt, userSubs.sub.ExpiresAt)
	require.Equal(t, SubscriptionStatusActive, userSubs.sub.Status)
	require.Equal(t, now, *userSubs.sub.MonthlyWindowStart)
	require.Zero(t, userSubs.sub.MonthlyUsageUSD)
}

func TestApplyMonthlyCycleAdjustmentCompensateResetLinkedEntitlementRecordsAuditFields(t *testing.T) {
	now := time.Date(2026, 6, 17, 11, 18, 43, 0, time.UTC)
	groupID := int64(28)
	monthlyLimit := 100.0
	startsAt := now.Add(-10 * 24 * time.Hour)
	monthlyWindowStart := startsAt
	expiresAt := startsAt.Add(60 * 24 * time.Hour)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	repo.entitlements[91] = &SubscriptionEntitlement{
		ID:                 91,
		UserID:             7,
		PrimaryGroupID:     &groupID,
		Name:               "Pro Plan",
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          expiresAt,
		MonthlyLimitUSD:    &monthlyLimit,
		MonthlyUsageUSD:    52,
		MonthlyWindowStart: &monthlyWindowStart,
		GroupGrants:        testGroupGrants([]int64{groupID}),
	}
	userSubs := &linkedEntitlementUserSubRepoStub{
		sub: &UserSubscription{
			ID:      912,
			UserID:  7,
			GroupID: groupID,
			Status:  SubscriptionStatusActive,
			EntitlementLink: &UserSubscriptionEntitlementLink{
				EntitlementID: 91,
			},
		},
	}
	svc := newSubscriptionServiceWithLinkedEntitlementCycleRepo(repo, userSubs)

	preview, err := svc.ApplyMonthlyCycleAdjustment(context.Background(), 912, MonthlyCycleAdjustmentInput{
		Mode:    MonthlyCycleAdjustmentCompensateReset,
		Reason:  "  support verified payment callback  ",
		AdminID: 66,
		Now:     now,
	})

	require.NoError(t, err)
	require.True(t, preview.CanApply)
	require.Equal(t, "support verified payment callback", preview.Reason)
	require.Len(t, repo.resetLogs, 1)
	require.True(t, repo.resetLogs[0].ResetMonthlyUsage)
	require.Equal(t, MonthlyCycleAdjustmentCompensateReset, repo.resetLogs[0].Mode)
	require.Equal(t, "support verified payment callback", repo.resetLogs[0].Reason)
	require.NotNil(t, repo.resetLogs[0].AdminID)
	require.Equal(t, int64(66), *repo.resetLogs[0].AdminID)
}

func TestApplyMonthlyCycleAdjustmentAlignToExpiryPreservesMonthlyUsageByDefault(t *testing.T) {
	now := time.Date(2026, 6, 17, 11, 18, 43, 0, time.UTC)
	groupID := int64(28)
	monthlyLimit := 100.0
	startsAt := now.Add(-20 * 24 * time.Hour)
	monthlyWindowStart := startsAt
	expiresAt := now.Add(40 * 24 * time.Hour)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	repo.entitlements[91] = &SubscriptionEntitlement{
		ID:                 91,
		UserID:             7,
		PrimaryGroupID:     &groupID,
		Name:               "Pro Plan",
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          expiresAt,
		MonthlyLimitUSD:    &monthlyLimit,
		MonthlyUsageUSD:    52,
		MonthlyWindowStart: &monthlyWindowStart,
		GroupGrants:        testGroupGrants([]int64{groupID}),
	}
	userSubs := &linkedEntitlementUserSubRepoStub{
		sub: &UserSubscription{
			ID:      912,
			UserID:  7,
			GroupID: groupID,
			Status:  SubscriptionStatusActive,
			EntitlementLink: &UserSubscriptionEntitlementLink{
				EntitlementID: 91,
			},
		},
	}
	svc := newSubscriptionServiceWithLinkedEntitlementCycleRepo(repo, userSubs)

	preview, err := svc.ApplyMonthlyCycleAdjustment(context.Background(), 912, MonthlyCycleAdjustmentInput{
		Mode:       MonthlyCycleAdjustmentAlignToExpiry,
		CycleCount: 2,
		Now:        now,
	})

	require.NoError(t, err)
	require.True(t, preview.CanApply)
	require.False(t, preview.ResetMonthlyUsage)
	require.Equal(t, 52.0, preview.NewMonthlyUsageUSD)
	require.Equal(t, 52.0, repo.entitlements[91].MonthlyUsageUSD)
	require.Len(t, repo.updateCalls, 1)
	require.Equal(t, 52.0, repo.updateCalls[0].NewMonthlyUsageUSD)
	require.Len(t, repo.resetLogs, 1)
	require.False(t, repo.resetLogs[0].ResetMonthlyUsage)
	require.NotContains(t, preview.Warnings, "monthly_usage_will_reset")
}

func TestPreviewMonthlyCycleAdjustmentAlignToResetCanResetMonthlyUsage(t *testing.T) {
	now := time.Date(2026, 6, 17, 11, 18, 43, 0, time.UTC)
	groupID := int64(28)
	monthlyLimit := 100.0
	monthlyWindowStart := now.Add(-12 * 24 * time.Hour)
	expiresAt := monthlyWindowStart.Add(90 * 24 * time.Hour)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	repo.entitlements[91] = &SubscriptionEntitlement{
		ID:                 91,
		UserID:             7,
		PrimaryGroupID:     &groupID,
		Name:               "Pro Plan",
		Status:             SubscriptionStatusActive,
		StartsAt:           monthlyWindowStart,
		ExpiresAt:          expiresAt,
		MonthlyLimitUSD:    &monthlyLimit,
		MonthlyUsageUSD:    12,
		MonthlyWindowStart: &monthlyWindowStart,
		GroupGrants:        testGroupGrants([]int64{groupID}),
	}
	svc := newSubscriptionServiceWithEntitlementRepo(repo.fakeSubscriptionEntitlementRepo)
	resetMonthlyUsage := true

	preview, err := svc.PreviewMonthlyCycleAdjustment(context.Background(), -91, MonthlyCycleAdjustmentInput{
		Mode:              MonthlyCycleAdjustmentAlignToReset,
		CycleCount:        3,
		ResetMonthlyUsage: &resetMonthlyUsage,
		Now:               now,
	})

	require.NoError(t, err)
	require.True(t, preview.CanApply)
	require.True(t, preview.ResetMonthlyUsage)
	require.Zero(t, preview.NewMonthlyUsageUSD)
	require.Contains(t, preview.Warnings, "monthly_usage_will_reset")
}

func TestPreviewMonthlyCycleAdjustmentAlignToExpiryInfersCurrentCycleCount(t *testing.T) {
	now := time.Date(2026, 6, 17, 11, 18, 43, 0, time.UTC)
	groupID := int64(28)
	monthlyLimit := 100.0
	monthlyWindowStart := now.Add(-12 * 24 * time.Hour)
	expiresAt := monthlyWindowStart.Add(90 * 24 * time.Hour)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	repo.entitlements[91] = &SubscriptionEntitlement{
		ID:                 91,
		UserID:             7,
		PrimaryGroupID:     &groupID,
		Name:               "Pro Plan",
		Status:             SubscriptionStatusActive,
		StartsAt:           monthlyWindowStart,
		ExpiresAt:          expiresAt,
		MonthlyLimitUSD:    &monthlyLimit,
		MonthlyUsageUSD:    12,
		MonthlyWindowStart: &monthlyWindowStart,
		GroupGrants:        testGroupGrants([]int64{groupID}),
	}
	svc := newSubscriptionServiceWithEntitlementRepo(repo.fakeSubscriptionEntitlementRepo)

	preview, err := svc.PreviewMonthlyCycleAdjustment(context.Background(), -91, MonthlyCycleAdjustmentInput{
		Mode: MonthlyCycleAdjustmentAlignToExpiry,
		Now:  now,
	})

	require.NoError(t, err)
	require.True(t, preview.CanApply)
	require.Equal(t, 3, preview.CycleCount)
	require.Equal(t, expiresAt, preview.NewExpiresAt)
	require.Equal(t, monthlyWindowStart, preview.NewMonthlyWindowStart)
	require.False(t, preview.ResetMonthlyUsage)
	require.Equal(t, 12.0, preview.NewMonthlyUsageUSD)
	require.NotContains(t, preview.Warnings, "monthly_usage_will_reset")
}

func TestPreviewMonthlyCycleAdjustmentAlignToExpiryRejectsFutureWindowStart(t *testing.T) {
	now := time.Date(2026, 6, 17, 11, 18, 43, 0, time.UTC)
	groupID := int64(28)
	monthlyLimit := 100.0
	monthlyWindowStart := now.Add(-2 * 24 * time.Hour)
	expiresAt := now.Add(90 * 24 * time.Hour)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	repo.entitlements[91] = &SubscriptionEntitlement{
		ID:                 91,
		UserID:             7,
		PrimaryGroupID:     &groupID,
		Name:               "Pro Plan",
		Status:             SubscriptionStatusActive,
		StartsAt:           monthlyWindowStart,
		ExpiresAt:          expiresAt,
		MonthlyLimitUSD:    &monthlyLimit,
		MonthlyUsageUSD:    12,
		MonthlyWindowStart: &monthlyWindowStart,
		GroupGrants:        testGroupGrants([]int64{groupID}),
	}
	svc := newSubscriptionServiceWithEntitlementRepo(repo.fakeSubscriptionEntitlementRepo)

	preview, err := svc.PreviewMonthlyCycleAdjustment(context.Background(), -91, MonthlyCycleAdjustmentInput{
		Mode:       MonthlyCycleAdjustmentAlignToExpiry,
		CycleCount: 1,
		Now:        now,
	})

	require.NoError(t, err)
	require.False(t, preview.CanApply)
	require.Equal(t, "monthly_window_start_in_future", preview.UnavailableReason)
}

func TestPreviewMonthlyCycleAdjustmentRejectsTooLargeCycleCount(t *testing.T) {
	now := time.Date(2026, 6, 17, 11, 18, 43, 0, time.UTC)
	groupID := int64(28)
	monthlyLimit := 100.0
	monthlyWindowStart := now.Add(-12 * 24 * time.Hour)
	expiresAt := monthlyWindowStart.Add(90 * 24 * time.Hour)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	repo.entitlements[91] = &SubscriptionEntitlement{
		ID:                 91,
		UserID:             7,
		PrimaryGroupID:     &groupID,
		Name:               "Pro Plan",
		Status:             SubscriptionStatusActive,
		StartsAt:           monthlyWindowStart,
		ExpiresAt:          expiresAt,
		MonthlyLimitUSD:    &monthlyLimit,
		MonthlyUsageUSD:    12,
		MonthlyWindowStart: &monthlyWindowStart,
		GroupGrants:        testGroupGrants([]int64{groupID}),
	}
	svc := newSubscriptionServiceWithEntitlementRepo(repo.fakeSubscriptionEntitlementRepo)

	preview, err := svc.PreviewMonthlyCycleAdjustment(context.Background(), -91, MonthlyCycleAdjustmentInput{
		Mode:       MonthlyCycleAdjustmentAlignToReset,
		CycleCount: maxMonthlyCycleAdjustmentCycleCount + 1,
		Now:        now,
	})

	require.NoError(t, err)
	require.False(t, preview.CanApply)
	require.Equal(t, "cycle_count_out_of_range", preview.UnavailableReason)
}

func TestPreviewMonthlyCycleAdjustmentRejectsCustomExpiresAfterMax(t *testing.T) {
	now := time.Date(2026, 6, 17, 11, 18, 43, 0, time.UTC)
	groupID := int64(28)
	monthlyLimit := 100.0
	monthlyWindowStart := now.Add(-12 * 24 * time.Hour)
	expiresAt := monthlyWindowStart.Add(90 * 24 * time.Hour)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	repo.entitlements[91] = &SubscriptionEntitlement{
		ID:                 91,
		UserID:             7,
		PrimaryGroupID:     &groupID,
		Name:               "Pro Plan",
		Status:             SubscriptionStatusActive,
		StartsAt:           monthlyWindowStart,
		ExpiresAt:          expiresAt,
		MonthlyLimitUSD:    &monthlyLimit,
		MonthlyUsageUSD:    12,
		MonthlyWindowStart: &monthlyWindowStart,
		GroupGrants:        testGroupGrants([]int64{groupID}),
	}
	svc := newSubscriptionServiceWithEntitlementRepo(repo.fakeSubscriptionEntitlementRepo)
	customWindowStart := now.Add(-time.Hour)
	customExpiresAt := MaxExpiresAt.Add(time.Second)

	preview, err := svc.PreviewMonthlyCycleAdjustment(context.Background(), -91, MonthlyCycleAdjustmentInput{
		Mode:                     MonthlyCycleAdjustmentCustom,
		CustomMonthlyWindowStart: &customWindowStart,
		CustomExpiresAt:          &customExpiresAt,
		Now:                      now,
	})

	require.NoError(t, err)
	require.False(t, preview.CanApply)
	require.Equal(t, "expires_at_after_max", preview.UnavailableReason)
}

func TestPreviewMonthlyCycleAdjustmentRejectsLongReason(t *testing.T) {
	now := time.Date(2026, 6, 17, 11, 18, 43, 0, time.UTC)
	groupID := int64(28)
	monthlyLimit := 100.0
	monthlyWindowStart := now.Add(-12 * 24 * time.Hour)
	expiresAt := monthlyWindowStart.Add(90 * 24 * time.Hour)
	repo := newAdvanceEntitlementMonthlyCycleRepo(now)
	repo.entitlements[91] = &SubscriptionEntitlement{
		ID:                 91,
		UserID:             7,
		PrimaryGroupID:     &groupID,
		Name:               "Pro Plan",
		Status:             SubscriptionStatusActive,
		StartsAt:           monthlyWindowStart,
		ExpiresAt:          expiresAt,
		MonthlyLimitUSD:    &monthlyLimit,
		MonthlyUsageUSD:    12,
		MonthlyWindowStart: &monthlyWindowStart,
		GroupGrants:        testGroupGrants([]int64{groupID}),
	}
	svc := newSubscriptionServiceWithEntitlementRepo(repo.fakeSubscriptionEntitlementRepo)

	preview, err := svc.PreviewMonthlyCycleAdjustment(context.Background(), -91, MonthlyCycleAdjustmentInput{
		Mode:   MonthlyCycleAdjustmentCompensateReset,
		Reason: strings.Repeat("a", monthlyCycleAdjustmentReasonMaxLength+1),
		Now:    now,
	})

	require.NoError(t, err)
	require.False(t, preview.CanApply)
	require.Equal(t, "reason_too_long", preview.UnavailableReason)
}

func newSubscriptionServiceWithLinkedEntitlementCycleRepo(repo SubscriptionEntitlementRepository, userSubRepo UserSubscriptionRepository) *SubscriptionService {
	svc := NewSubscriptionService(groupRepoNoop{}, userSubRepo, nil, nil, nil)
	svc.entitlementSvc = NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{}})
	svc.entitlementSvc.SetLegacySubscriptionRepository(userSubRepo)
	return svc
}
