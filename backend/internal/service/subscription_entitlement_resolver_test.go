package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubscriptionEntitlementResolver_ExplicitEntitlementIDHasPriority(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	repo := newFakeSubscriptionEntitlementRepo(now)
	svc := NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{})
	svc.SetNowFunc(func() time.Time { return now })
	repo.entitlements[1] = testActiveEntitlement(1, 42, []int64{101}, now, now.AddDate(0, 0, 30))
	repo.entitlements[2] = testActiveEntitlement(2, 42, []int64{101}, now, now.AddDate(0, 0, 5))
	explicitID := int64(1)

	resolved, err := svc.Resolve(context.Background(), ResolveSubscriptionEntitlementInput{
		UserID:        42,
		GroupID:       101,
		EntitlementID: &explicitID,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), resolved.Entitlement.ID)

	defaultResolved, err := svc.ResolveForGroup(context.Background(), 42, 101, 0)
	require.NoError(t, err)
	require.Equal(t, int64(2), defaultResolved.Entitlement.ID)
}

func TestSubscriptionEntitlementResolver_GroupNotCoveredReturnsGroupNotAllowed(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	repo := newFakeSubscriptionEntitlementRepo(now)
	svc := NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{})
	repo.entitlements[1] = testActiveEntitlement(1, 42, []int64{101}, now, now.AddDate(0, 0, 30))
	explicitID := int64(1)

	_, err := svc.Resolve(context.Background(), ResolveSubscriptionEntitlementInput{
		UserID:        42,
		GroupID:       202,
		EntitlementID: &explicitID,
		Now:           now,
	})
	require.ErrorIs(t, err, ErrGroupNotAllowed)
}

func TestSubscriptionEntitlementResolver_DisabledGrantDoesNotFallBackToGroupsEdge(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	repo := newFakeSubscriptionEntitlementRepo(now)
	svc := NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{})
	svc.SetNowFunc(func() time.Time { return now })
	ent := testActiveEntitlement(1, 42, []int64{101}, now, now.AddDate(0, 0, 30))
	ent.GroupGrants[0].Enabled = false
	repo.entitlements[1] = ent
	explicitID := int64(1)

	_, err := svc.Resolve(context.Background(), ResolveSubscriptionEntitlementInput{
		UserID:        42,
		GroupID:       101,
		EntitlementID: &explicitID,
		Now:           now,
	})
	require.ErrorIs(t, err, ErrGroupNotAllowed)

	_, err = svc.ResolveForGroup(context.Background(), 42, 101, 0)
	require.ErrorIs(t, err, ErrGroupNotAllowed)

	// The production repository filters disabled rows out of GroupGrants but
	// preserves that grant rows were configured. This shape must not be treated
	// as a legacy Groups-only entitlement.
	filtered := testActiveEntitlement(2, 42, []int64{202}, now, now.AddDate(0, 0, 30))
	filtered.GroupGrants = nil
	filtered.GroupGrantsConfigured = true
	repo.entitlements[2] = filtered
	filteredID := int64(2)

	_, err = svc.Resolve(context.Background(), ResolveSubscriptionEntitlementInput{
		UserID:        42,
		GroupID:       202,
		EntitlementID: &filteredID,
		Now:           now,
	})
	require.ErrorIs(t, err, ErrGroupNotAllowed)
	require.Empty(t, entitlementCoveredGroupIDs(filtered))

	legacy := &SubscriptionEntitlement{Groups: []Group{{ID: 303}}}
	require.True(t, entitlementCoversGroup(legacy, 303), "true legacy objects retain the Groups-only fallback")
}

func TestSubscriptionEntitlementResolver_SharedMonthlyUsageExhaustedAcrossGroups(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	monthlyLimit := 5.0
	repo := newFakeSubscriptionEntitlementRepo(now)
	svc := NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{})
	svc.SetNowFunc(func() time.Time { return now })
	ent := testActiveEntitlement(1, 42, []int64{101, 202}, now, now.AddDate(0, 0, 30))
	ent.MonthlyLimitUSD = &monthlyLimit
	ent.MonthlyUsageUSD = 5
	repo.entitlements[1] = ent

	_, err := svc.ResolveForGroup(context.Background(), 42, 202, 0.01)
	require.ErrorIs(t, err, ErrSubscriptionEntitlementQuotaExceeded)
}

func TestSubscriptionEntitlementResolver_BalanceFallbackDoesNotBypassQuota(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	monthlyLimit := 5.0
	repo := newFakeSubscriptionEntitlementRepo(now)
	svc := NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{})
	svc.SetNowFunc(func() time.Time { return now })
	ent := testActiveEntitlement(1, 42, []int64{101}, now, now.AddDate(0, 0, 30))
	ent.OveragePolicy = SubscriptionEntitlementOverageBalanceFallback
	ent.MonthlyLimitUSD = &monthlyLimit
	ent.MonthlyUsageUSD = 5
	repo.entitlements[1] = ent

	_, err := svc.ResolveForGroup(context.Background(), 42, 101, 0.01)
	require.ErrorIs(t, err, ErrSubscriptionEntitlementQuotaExceeded)
}

func TestSubscriptionEntitlementResolver_FutureStartAndExpiredAreUnavailable(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	repo := newFakeSubscriptionEntitlementRepo(now)
	svc := NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{})
	future := testActiveEntitlement(1, 42, []int64{101}, now.Add(time.Hour), now.AddDate(0, 0, 30))
	expired := testActiveEntitlement(2, 42, []int64{101}, now.AddDate(0, 0, -30), now.Add(-time.Hour))
	repo.entitlements[1] = future
	repo.entitlements[2] = expired

	futureID := int64(1)
	_, err := svc.Resolve(context.Background(), ResolveSubscriptionEntitlementInput{
		UserID:        42,
		GroupID:       101,
		EntitlementID: &futureID,
		Now:           now,
	})
	require.ErrorIs(t, err, ErrSubscriptionEntitlementExpired)

	expiredID := int64(2)
	_, err = svc.Resolve(context.Background(), ResolveSubscriptionEntitlementInput{
		UserID:        42,
		GroupID:       101,
		EntitlementID: &expiredID,
		Now:           now,
	})
	require.ErrorIs(t, err, ErrSubscriptionEntitlementExpired)
	require.Empty(t, repo.resetCalls)
}

func testActiveEntitlement(id, userID int64, groupIDs []int64, startsAt, expiresAt time.Time) *SubscriptionEntitlement {
	planID := int64(1)
	return &SubscriptionEntitlement{
		ID:                 id,
		UserID:             userID,
		PlanID:             &planID,
		Name:               "Pro",
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          expiresAt,
		DailyWindowStart:   cloneTimeValue(startsAt),
		WeeklyWindowStart:  cloneTimeValue(startsAt),
		MonthlyWindowStart: cloneTimeValue(startsAt),
		OveragePolicy:      SubscriptionEntitlementOverageBlock,
		GroupGrants:        testGroupGrants(groupIDs),
		Groups:             testGroups(groupIDs),
	}
}
