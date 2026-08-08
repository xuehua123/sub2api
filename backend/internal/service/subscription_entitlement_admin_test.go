package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionServiceExtendSubscription_AdjustsSyntheticEntitlementRow(t *testing.T) {
	now := time.Now().UTC()
	planID := int64(9)
	groupID := int64(28)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[91] = &SubscriptionEntitlement{
		ID:                 91,
		UserID:             7,
		PlanID:             &planID,
		PrimaryGroupID:     &groupID,
		Name:               "Pro Plan",
		Status:             SubscriptionStatusActive,
		StartsAt:           now.Add(-time.Hour),
		ExpiresAt:          now.Add(24 * time.Hour),
		DailyWindowStart:   cloneTimeValue(now.Add(-time.Hour)),
		WeeklyWindowStart:  cloneTimeValue(now.Add(-time.Hour)),
		MonthlyWindowStart: cloneTimeValue(now.Add(-time.Hour)),
		DailyUsageUSD:      1.25,
		WeeklyUsageUSD:     2.5,
		MonthlyUsageUSD:    3.75,
		GroupGrants:        testGroupGrants([]int64{groupID}),
	}
	svc := newSubscriptionServiceWithEntitlementRepo(repo)
	svc.entitlementSvc.SetNowFunc(func() time.Time { return now })

	got, err := svc.ExtendSubscription(context.Background(), -91, 7)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(-91), got.ID)
	require.True(t, got.EntitlementOnly)
	require.Equal(t, int64(91), got.EntitlementLink.EntitlementID)
	require.Equal(t, SubscriptionStatusActive, repo.entitlements[91].Status)
	require.True(t, repo.entitlements[91].ExpiresAt.After(now.Add(7*24*time.Hour)))
	require.Equal(t, 1.25, repo.entitlements[91].DailyUsageUSD)
	require.Equal(t, 2.5, repo.entitlements[91].WeeklyUsageUSD)
	require.Equal(t, 3.75, repo.entitlements[91].MonthlyUsageUSD)
}

func TestSubscriptionServiceExtendSubscription_RevivesExpiredEntitlementWithFreshUsageWindows(t *testing.T) {
	now := time.Date(2026, 6, 16, 16, 45, 0, 0, timezone.Location())

	tests := []struct {
		name          string
		entitlementID int64
		days          int
		oneTimeDaily  bool
	}{
		{name: "one day card", entitlementID: 96, days: 1, oneTimeDaily: true},
		{name: "multi day card", entitlementID: 97, days: 7, oneTimeDaily: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groupID := int64(28)
			oldWindowStart := now.AddDate(0, 0, -10)
			repo := newFakeSubscriptionEntitlementRepo(now)
			repo.entitlements[tt.entitlementID] = &SubscriptionEntitlement{
				ID:                 tt.entitlementID,
				UserID:             7,
				PrimaryGroupID:     &groupID,
				Name:               "Expired Plan",
				Status:             SubscriptionStatusExpired,
				StartsAt:           now.AddDate(0, 0, -10),
				ExpiresAt:          now.AddDate(0, 0, -2),
				DailyWindowStart:   &oldWindowStart,
				WeeklyWindowStart:  &oldWindowStart,
				MonthlyWindowStart: &oldWindowStart,
				DailyUsageUSD:      8.5,
				WeeklyUsageUSD:     18.5,
				MonthlyUsageUSD:    28.5,
				GroupGrants:        testGroupGrants([]int64{groupID}),
			}
			svc := newSubscriptionServiceWithEntitlementRepo(repo)
			svc.entitlementSvc.SetNowFunc(func() time.Time { return now })

			got, err := svc.ExtendSubscription(context.Background(), -tt.entitlementID, tt.days)

			require.NoError(t, err)
			require.NotNil(t, got)
			updated := repo.entitlements[tt.entitlementID]
			require.Equal(t, SubscriptionStatusActive, updated.Status)
			require.Equal(t, now, updated.StartsAt)
			require.Equal(t, now.AddDate(0, 0, tt.days), updated.ExpiresAt)
			require.Equal(t, tt.oneTimeDaily, updated.HasOneTimeDailyQuota())
			require.Zero(t, updated.DailyUsageUSD)
			require.Zero(t, updated.WeeklyUsageUSD)
			require.Zero(t, updated.MonthlyUsageUSD)
			require.Equal(t, timezone.StartOfDay(now), *updated.DailyWindowStart)
			require.Equal(t, now, *updated.WeeklyWindowStart)
			require.Equal(t, now, *updated.MonthlyWindowStart)
			require.Empty(t, repo.resetCalls, "expired revival must reset term and usage atomically")
		})
	}
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
	now := time.Date(2026, 6, 11, 16, 45, 0, 0, timezone.Location())
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
	svc.entitlementSvc.SetNowFunc(func() time.Time { return now })

	got, err := svc.AdminResetQuota(context.Background(), -92, true, true, true)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, float64(0), got.DailyUsageUSD)
	require.Equal(t, float64(0), got.WeeklyUsageUSD)
	require.Equal(t, float64(0), got.MonthlyUsageUSD)
	require.Len(t, repo.resetCalls, 1)
	require.Equal(t, int64(92), repo.resetCalls[0].id)
	require.Equal(t, timezone.StartOfDay(now), repo.resetCalls[0].dailyStart)
	require.Equal(t, now, repo.resetCalls[0].periodicStart)
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

func TestSubscriptionServiceRestoreSubscription_RestoresSyntheticEntitlementRow(t *testing.T) {
	now := time.Now().UTC()
	groupID := int64(28)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[95] = &SubscriptionEntitlement{
		ID:             95,
		UserID:         7,
		PrimaryGroupID: &groupID,
		Name:           "Pro Plan",
		Status:         SubscriptionStatusRevoked,
		StartsAt:       now.Add(-time.Hour),
		ExpiresAt:      now.Add(24 * time.Hour),
		GroupGrants:    testGroupGrants([]int64{groupID}),
	}
	svc := newSubscriptionServiceWithEntitlementRepo(repo)

	got, err := svc.RestoreSubscription(context.Background(), -95)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(-95), got.ID)
	require.True(t, got.EntitlementOnly)
	require.Equal(t, int64(95), got.EntitlementLink.EntitlementID)
	require.Equal(t, SubscriptionStatusActive, repo.entitlements[95].Status)
	require.Contains(t, repo.entitlements[95].Notes, "restored by admin")
}

func TestSubscriptionServiceRestoreSubscription_RestoresLinkedEntitlementRow(t *testing.T) {
	now := time.Now().UTC()
	deletedAt := now.Add(-time.Hour)
	groupID := int64(28)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[94] = &SubscriptionEntitlement{
		ID:             94,
		UserID:         7,
		PrimaryGroupID: &groupID,
		Name:           "Pro Plan",
		Status:         SubscriptionStatusRevoked,
		StartsAt:       now.Add(-time.Hour),
		ExpiresAt:      now.Add(24 * time.Hour),
		GroupGrants:    testGroupGrants([]int64{groupID}),
	}
	userSubs := &linkedEntitlementUserSubRepoStub{
		sub: &UserSubscription{
			ID:        913,
			UserID:    7,
			GroupID:   groupID,
			StartsAt:  now.Add(-time.Hour),
			ExpiresAt: now.Add(24 * time.Hour),
			Status:    SubscriptionStatusActive,
			DeletedAt: &deletedAt,
			EntitlementLink: &UserSubscriptionEntitlementLink{
				EntitlementID: 94,
			},
		},
	}
	svc := newSubscriptionServiceWithLinkedEntitlementRepo(repo, userSubs)

	got, err := svc.RestoreSubscription(context.Background(), 913)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(913), userSubs.restoredID)
	require.Equal(t, SubscriptionStatusActive, userSubs.restoredStatus)
	require.Equal(t, SubscriptionStatusActive, repo.entitlements[94].Status)
	require.Contains(t, repo.entitlements[94].Notes, "restored by admin")
	require.Nil(t, got.DeletedAt)
}

func newSubscriptionServiceWithEntitlementRepo(repo *fakeSubscriptionEntitlementRepo) *SubscriptionService {
	svc := NewSubscriptionService(groupRepoNoop{}, userSubRepoNoop{}, nil, nil, nil)
	svc.entitlementSvc = NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{}})
	return svc
}

type linkedEntitlementUserSubRepoStub struct {
	userSubRepoNoop
	sub            *UserSubscription
	deletedID      int64
	restoredID     int64
	restoredStatus string
}

func (r *linkedEntitlementUserSubRepoStub) GetByID(context.Context, int64) (*UserSubscription, error) {
	cp := *r.sub
	return &cp, nil
}

func (r *linkedEntitlementUserSubRepoStub) GetByIDIncludeDeleted(context.Context, int64) (*UserSubscription, error) {
	cp := *r.sub
	return &cp, nil
}

func (r *linkedEntitlementUserSubRepoStub) Delete(_ context.Context, id int64) error {
	r.deletedID = id
	return nil
}

func (r *linkedEntitlementUserSubRepoStub) ExistsActiveByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return false, nil
}

func (r *linkedEntitlementUserSubRepoStub) Restore(_ context.Context, id int64, restoredStatus string) (*UserSubscription, error) {
	r.restoredID = id
	r.restoredStatus = restoredStatus
	cp := *r.sub
	cp.Status = restoredStatus
	cp.DeletedAt = nil
	r.sub = &cp
	return &cp, nil
}

func newSubscriptionServiceWithLinkedEntitlementRepo(repo *fakeSubscriptionEntitlementRepo, userSubRepo UserSubscriptionRepository) *SubscriptionService {
	svc := NewSubscriptionService(groupRepoNoop{}, userSubRepo, nil, nil, nil)
	svc.entitlementSvc = NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{}})
	return svc
}
