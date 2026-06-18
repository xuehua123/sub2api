package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubscriptionServiceExtendSubscription_AdjustsSyntheticEntitlementRow(t *testing.T) {
	now := time.Now().UTC()
	planID := int64(9)
	groupID := int64(28)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[91] = &SubscriptionEntitlement{
		ID:             91,
		UserID:         7,
		PlanID:         &planID,
		PrimaryGroupID: &groupID,
		Name:           "Pro Plan",
		Status:         SubscriptionStatusActive,
		StartsAt:       now.Add(-time.Hour),
		ExpiresAt:      now.Add(24 * time.Hour),
		GroupGrants:    testGroupGrants([]int64{groupID}),
	}
	svc := newSubscriptionServiceWithEntitlementRepo(repo)

	got, err := svc.ExtendSubscription(context.Background(), -91, 7)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(-91), got.ID)
	require.True(t, got.EntitlementOnly)
	require.Equal(t, int64(91), got.EntitlementLink.EntitlementID)
	require.Equal(t, SubscriptionStatusActive, repo.entitlements[91].Status)
	require.True(t, repo.entitlements[91].ExpiresAt.After(now.Add(7*24*time.Hour)))
}

func TestSubscriptionServiceExtendSubscription_AdjustsLinkedEntitlementRow(t *testing.T) {
	now := time.Now().UTC()
	groupID := int64(28)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[91] = &SubscriptionEntitlement{
		ID:             91,
		UserID:         7,
		PrimaryGroupID: &groupID,
		Name:           "Pro Plan",
		Status:         SubscriptionStatusActive,
		StartsAt:       now.Add(-time.Hour),
		ExpiresAt:      now.Add(24 * time.Hour),
		GroupGrants:    testGroupGrants([]int64{groupID}),
	}
	userSubs := &linkedEntitlementUserSubRepoStub{
		sub: &UserSubscription{
			ID:        912,
			UserID:    7,
			GroupID:   groupID,
			StartsAt:  now.Add(-time.Hour),
			ExpiresAt: now.Add(24 * time.Hour),
			Status:    SubscriptionStatusActive,
			EntitlementLink: &UserSubscriptionEntitlementLink{
				EntitlementID: 91,
			},
		},
	}
	svc := newSubscriptionServiceWithLinkedEntitlementRepo(repo, userSubs)

	got, err := svc.ExtendSubscription(context.Background(), 912, 7)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(-91), got.ID)
	require.Equal(t, int64(91), got.EntitlementLink.EntitlementID)
	require.True(t, repo.entitlements[91].ExpiresAt.After(now.Add(7*24*time.Hour)))
}

func TestSubscriptionServiceAdminResetQuota_ResetsSyntheticEntitlementRow(t *testing.T) {
	now := time.Now().UTC()
	groupID := int64(28)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[92] = &SubscriptionEntitlement{
		ID:              92,
		UserID:          7,
		PrimaryGroupID:  &groupID,
		Name:            "Pro Plan",
		Status:          SubscriptionStatusActive,
		StartsAt:        now.Add(-time.Hour),
		ExpiresAt:       now.Add(24 * time.Hour),
		DailyUsageUSD:   1.1,
		WeeklyUsageUSD:  2.2,
		MonthlyUsageUSD: 3.3,
		GroupGrants:     testGroupGrants([]int64{groupID}),
	}
	svc := newSubscriptionServiceWithEntitlementRepo(repo)

	got, err := svc.AdminResetQuota(context.Background(), -92, true, true, true)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, float64(0), got.DailyUsageUSD)
	require.Equal(t, float64(0), got.WeeklyUsageUSD)
	require.Equal(t, float64(0), got.MonthlyUsageUSD)
	require.Len(t, repo.resetCalls, 1)
	require.Equal(t, int64(92), repo.resetCalls[0].id)
}

func TestSubscriptionServiceRevokeSubscription_RevokesSyntheticEntitlementRow(t *testing.T) {
	now := time.Now().UTC()
	groupID := int64(28)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[93] = &SubscriptionEntitlement{
		ID:             93,
		UserID:         7,
		PrimaryGroupID: &groupID,
		Name:           "Pro Plan",
		Status:         SubscriptionStatusActive,
		StartsAt:       now.Add(-time.Hour),
		ExpiresAt:      now.Add(24 * time.Hour),
		GroupGrants:    testGroupGrants([]int64{groupID}),
	}
	svc := newSubscriptionServiceWithEntitlementRepo(repo)

	err := svc.RevokeSubscription(context.Background(), -93)

	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusRevoked, repo.entitlements[93].Status)
	require.Contains(t, repo.entitlements[93].Notes, "revoked by admin")
}

func TestSubscriptionServiceRevokeSubscription_RevokesLinkedEntitlementRow(t *testing.T) {
	now := time.Now().UTC()
	groupID := int64(28)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[93] = &SubscriptionEntitlement{
		ID:             93,
		UserID:         7,
		PrimaryGroupID: &groupID,
		Name:           "Pro Plan",
		Status:         SubscriptionStatusActive,
		StartsAt:       now.Add(-time.Hour),
		ExpiresAt:      now.Add(24 * time.Hour),
		GroupGrants:    testGroupGrants([]int64{groupID}),
	}
	userSubs := &linkedEntitlementUserSubRepoStub{
		sub: &UserSubscription{
			ID:        912,
			UserID:    7,
			GroupID:   groupID,
			StartsAt:  now.Add(-time.Hour),
			ExpiresAt: now.Add(24 * time.Hour),
			Status:    SubscriptionStatusActive,
			EntitlementLink: &UserSubscriptionEntitlementLink{
				EntitlementID: 93,
			},
		},
	}
	svc := newSubscriptionServiceWithLinkedEntitlementRepo(repo, userSubs)

	err := svc.RevokeSubscription(context.Background(), 912)

	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusRevoked, repo.entitlements[93].Status)
	require.Contains(t, repo.entitlements[93].Notes, "revoked by admin")
	require.Equal(t, int64(912), userSubs.deletedID)
}

func newSubscriptionServiceWithEntitlementRepo(repo *fakeSubscriptionEntitlementRepo) *SubscriptionService {
	svc := NewSubscriptionService(groupRepoNoop{}, userSubRepoNoop{}, nil, nil, nil)
	svc.entitlementSvc = NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{}})
	return svc
}

type linkedEntitlementUserSubRepoStub struct {
	userSubRepoNoop
	sub       *UserSubscription
	deletedID int64
}

func (r *linkedEntitlementUserSubRepoStub) GetByID(context.Context, int64) (*UserSubscription, error) {
	cp := *r.sub
	return &cp, nil
}

func (r *linkedEntitlementUserSubRepoStub) Delete(_ context.Context, id int64) error {
	r.deletedID = id
	return nil
}

func newSubscriptionServiceWithLinkedEntitlementRepo(repo *fakeSubscriptionEntitlementRepo, userSubRepo UserSubscriptionRepository) *SubscriptionService {
	svc := NewSubscriptionService(groupRepoNoop{}, userSubRepo, nil, nil, nil)
	svc.entitlementSvc = NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{}})
	return svc
}
