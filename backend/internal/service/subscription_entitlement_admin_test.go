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
	legacySubscriptionID := int64(912)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[93] = &SubscriptionEntitlement{
		ID:                   93,
		UserID:               7,
		LegacySubscriptionID: &legacySubscriptionID,
		PrimaryGroupID:       &groupID,
		Name:                 "Pro Plan",
		Status:               SubscriptionStatusActive,
		StartsAt:             now.Add(-time.Hour),
		ExpiresAt:            now.Add(24 * time.Hour),
		GroupGrants:          testGroupGrants([]int64{groupID}),
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
	require.NotNil(t, userSubs.sub.DeletedAt)
}

func TestSubscriptionEntitlementServiceRevokeUserEntitlementDeletesLinkedAlias(t *testing.T) {
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	groupID := int64(28)
	legacySubscriptionID := int64(915)
	dailyWindow := now.Add(-6 * time.Hour)
	weeklyWindow := now.AddDate(0, 0, -3)
	monthlyWindow := now.AddDate(0, 0, -20)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[99] = &SubscriptionEntitlement{
		ID:                   99,
		UserID:               7,
		LegacySubscriptionID: &legacySubscriptionID,
		PrimaryGroupID:       &groupID,
		Name:                 "Pro Plan",
		Status:               SubscriptionStatusActive,
		StartsAt:             now.AddDate(0, 0, -20),
		ExpiresAt:            now.AddDate(0, 0, 10),
		DailyWindowStart:     &dailyWindow,
		WeeklyWindowStart:    &weeklyWindow,
		MonthlyWindowStart:   &monthlyWindow,
		DailyUsageUSD:        1.25,
		WeeklyUsageUSD:       2.5,
		MonthlyUsageUSD:      3.75,
		GroupGrants:          testGroupGrants([]int64{groupID}),
	}
	userSubs := &linkedEntitlementUserSubRepoStub{sub: &UserSubscription{
		ID:        legacySubscriptionID,
		UserID:    7,
		GroupID:   groupID,
		StartsAt:  now.AddDate(0, 0, -30),
		ExpiresAt: now.AddDate(0, 0, 1),
		Status:    SubscriptionStatusActive,
	}}
	entitlementSvc := NewSubscriptionEntitlementService(repo, nil)
	entitlementSvc.SetLegacySubscriptionRepository(userSubs)

	err := entitlementSvc.RevokeUserEntitlement(context.Background(), 7, 99, now)

	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusRevoked, repo.entitlements[99].Status)
	require.Equal(t, legacySubscriptionID, userSubs.deletedID)
	require.NotNil(t, userSubs.sub.DeletedAt)
	require.Equal(t, now.AddDate(0, 0, -20), userSubs.sub.StartsAt)
	require.Equal(t, now.AddDate(0, 0, 10), userSubs.sub.ExpiresAt)
	require.Equal(t, dailyWindow, *userSubs.sub.DailyWindowStart)
	require.Equal(t, weeklyWindow, *userSubs.sub.WeeklyWindowStart)
	require.Equal(t, monthlyWindow, *userSubs.sub.MonthlyWindowStart)
	require.Equal(t, 1.25, userSubs.sub.DailyUsageUSD)
	require.Equal(t, 2.5, userSubs.sub.WeeklyUsageUSD)
	require.Equal(t, 3.75, userSubs.sub.MonthlyUsageUSD)
}

func TestSubscriptionServiceAdjustLinkedEntitlementSyncsLegacyLifecycle(t *testing.T) {
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	groupID := int64(28)
	legacySubscriptionID := int64(916)
	dailyWindow := now.Add(-6 * time.Hour)
	weeklyWindow := now.AddDate(0, 0, -3)
	monthlyWindow := now.AddDate(0, 0, -20)
	originalExpiresAt := now.AddDate(0, 0, 10)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[100] = &SubscriptionEntitlement{
		ID:                   100,
		UserID:               7,
		LegacySubscriptionID: &legacySubscriptionID,
		PrimaryGroupID:       &groupID,
		Name:                 "Pro Plan",
		Status:               SubscriptionStatusActive,
		StartsAt:             now.AddDate(0, 0, -20),
		ExpiresAt:            originalExpiresAt,
		DailyWindowStart:     &dailyWindow,
		WeeklyWindowStart:    &weeklyWindow,
		MonthlyWindowStart:   &monthlyWindow,
		DailyUsageUSD:        1.25,
		WeeklyUsageUSD:       2.5,
		MonthlyUsageUSD:      3.75,
		GroupGrants:          testGroupGrants([]int64{groupID}),
	}
	userSubs := &linkedEntitlementUserSubRepoStub{sub: &UserSubscription{
		ID:        legacySubscriptionID,
		UserID:    7,
		GroupID:   groupID,
		StartsAt:  now.AddDate(0, 0, -30),
		ExpiresAt: now.AddDate(0, 0, 1),
		Status:    SubscriptionStatusExpired,
	}}
	svc := newSubscriptionServiceWithLinkedEntitlementRepo(repo, userSubs)
	svc.entitlementSvc.SetNowFunc(func() time.Time { return now })

	got, err := svc.ExtendSubscription(context.Background(), -100, 5)

	require.NoError(t, err)
	require.Equal(t, originalExpiresAt.AddDate(0, 0, 5), got.ExpiresAt)
	require.Equal(t, repo.entitlements[100].StartsAt, userSubs.sub.StartsAt)
	require.Equal(t, repo.entitlements[100].ExpiresAt, userSubs.sub.ExpiresAt)
	require.Equal(t, repo.entitlements[100].Status, userSubs.sub.Status)
	require.Equal(t, dailyWindow, *userSubs.sub.DailyWindowStart)
	require.Equal(t, weeklyWindow, *userSubs.sub.WeeklyWindowStart)
	require.Equal(t, monthlyWindow, *userSubs.sub.MonthlyWindowStart)
	require.Equal(t, 1.25, userSubs.sub.DailyUsageUSD)
	require.Equal(t, 2.5, userSubs.sub.WeeklyUsageUSD)
	require.Equal(t, 3.75, userSubs.sub.MonthlyUsageUSD)
}

type rollbackAwareSubscriptionEntitlementRepo struct {
	*fakeSubscriptionEntitlementRepo
}

func (r *rollbackAwareSubscriptionEntitlementRepo) WithUserEntitlementMutationTx(ctx context.Context, _ int64, fn func(context.Context) error) error {
	r.mu.Lock()
	snapshot := make(map[int64]*SubscriptionEntitlement, len(r.entitlements))
	for id, ent := range r.entitlements {
		snapshot[id] = cloneTestEntitlement(ent)
	}
	r.mu.Unlock()

	err := fn(ctx)
	if err == nil {
		return nil
	}
	r.mu.Lock()
	r.entitlements = snapshot
	r.mu.Unlock()
	return err
}

type nilLinkedAliasRepoStub struct {
	userSubRepoNoop
}

func (nilLinkedAliasRepoStub) GetByIDIncludeDeletedForUpdate(context.Context, int64) (*UserSubscription, error) {
	return nil, nil
}

func TestSubscriptionServiceAdjustLinkedEntitlementMissingAliasRollsBackTerm(t *testing.T) {
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	const (
		entitlementID = int64(110)
		legacyID      = int64(920)
		groupID       = int64(28)
	)

	tests := []struct {
		name      string
		aliasRepo UserSubscriptionRepository
	}{
		{name: "not found", aliasRepo: &linkedEntitlementUserSubRepoStub{}},
		{name: "nil row", aliasRepo: nilLinkedAliasRepoStub{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseRepo := newFakeSubscriptionEntitlementRepo(now)
			originalExpiresAt := now.AddDate(0, 0, 10)
			baseRepo.entitlements[entitlementID] = &SubscriptionEntitlement{
				ID:                   entitlementID,
				UserID:               7,
				LegacySubscriptionID: func() *int64 { v := legacyID; return &v }(),
				PrimaryGroupID:       func() *int64 { v := groupID; return &v }(),
				Status:               SubscriptionStatusActive,
				StartsAt:             now.AddDate(0, 0, -20),
				ExpiresAt:            originalExpiresAt,
				GroupGrants:          testGroupGrants([]int64{groupID}),
			}
			repo := &rollbackAwareSubscriptionEntitlementRepo{fakeSubscriptionEntitlementRepo: baseRepo}
			svc := NewSubscriptionService(groupRepoNoop{}, tt.aliasRepo, nil, nil, nil)
			svc.entitlementSvc = NewSubscriptionEntitlementService(repo, nil)
			svc.entitlementSvc.SetLegacySubscriptionRepository(tt.aliasRepo)
			svc.entitlementSvc.SetNowFunc(func() time.Time { return now })

			_, err := svc.ExtendSubscription(context.Background(), -entitlementID, 5)

			require.ErrorIs(t, err, ErrSubscriptionEntitlementAliasUnavailable)
			stored, getErr := repo.GetByID(context.Background(), entitlementID)
			require.NoError(t, getErr)
			require.Equal(t, originalExpiresAt, stored.ExpiresAt, "alias sync failure must roll back the entitlement term")
		})
	}
}

func TestSubscriptionServiceRestoreLinkedAliasUsesAdjustedLifecycleSnapshot(t *testing.T) {
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	groupID := int64(28)
	legacySubscriptionID := int64(917)
	originalExpiresAt := now.AddDate(0, 0, 10)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[101] = &SubscriptionEntitlement{
		ID:                   101,
		UserID:               7,
		LegacySubscriptionID: &legacySubscriptionID,
		PrimaryGroupID:       &groupID,
		Name:                 "Pro Plan",
		Status:               SubscriptionStatusActive,
		StartsAt:             now.AddDate(0, 0, -20),
		ExpiresAt:            originalExpiresAt,
		MonthlyUsageUSD:      8.5,
		GroupGrants:          testGroupGrants([]int64{groupID}),
	}
	userSubs := &linkedEntitlementUserSubRepoStub{sub: &UserSubscription{
		ID:        legacySubscriptionID,
		UserID:    7,
		GroupID:   groupID,
		StartsAt:  now.AddDate(0, 0, -30),
		ExpiresAt: now.AddDate(0, 0, 1),
		Status:    SubscriptionStatusActive,
		EntitlementLink: &UserSubscriptionEntitlementLink{
			EntitlementID: 101,
		},
	}}
	svc := newSubscriptionServiceWithLinkedEntitlementRepo(repo, userSubs)
	svc.entitlementSvc.SetNowFunc(func() time.Time { return now })

	_, err := svc.ExtendSubscription(context.Background(), -101, 5)
	require.NoError(t, err)
	adjustedExpiresAt := originalExpiresAt.AddDate(0, 0, 5)
	require.Equal(t, adjustedExpiresAt, userSubs.sub.ExpiresAt)
	require.NoError(t, svc.RevokeSubscription(context.Background(), legacySubscriptionID))
	require.NotNil(t, userSubs.sub.DeletedAt)

	restored, err := svc.RestoreSubscription(context.Background(), legacySubscriptionID)

	require.NoError(t, err)
	require.Nil(t, restored.DeletedAt)
	require.Equal(t, adjustedExpiresAt, restored.ExpiresAt)
	require.Equal(t, adjustedExpiresAt, repo.entitlements[101].ExpiresAt)
	require.Equal(t, 8.5, restored.MonthlyUsageUSD)
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

func TestSubscriptionServiceRestoreSubscription_NegativeLinkedEntitlementRestoresAliasLifecycle(t *testing.T) {
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	deletedAt := now.Add(-time.Hour)
	groupID := int64(28)
	legacySubscriptionID := int64(919)
	dailyWindow := now.Add(-6 * time.Hour)
	weeklyWindow := now.AddDate(0, 0, -3)
	monthlyWindow := now.AddDate(0, 0, -20)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[102] = &SubscriptionEntitlement{
		ID:                   102,
		UserID:               7,
		LegacySubscriptionID: &legacySubscriptionID,
		PrimaryGroupID:       &groupID,
		Name:                 "Pro Plan",
		Status:               SubscriptionStatusRevoked,
		StartsAt:             now.AddDate(0, 0, -20),
		ExpiresAt:            now.AddDate(0, 0, 10),
		DailyWindowStart:     &dailyWindow,
		WeeklyWindowStart:    &weeklyWindow,
		MonthlyWindowStart:   &monthlyWindow,
		DailyUsageUSD:        1.25,
		WeeklyUsageUSD:       2.5,
		MonthlyUsageUSD:      3.75,
		GroupGrants:          testGroupGrants([]int64{groupID}),
	}
	userSubs := &linkedEntitlementUserSubRepoStub{sub: &UserSubscription{
		ID:        legacySubscriptionID,
		UserID:    7,
		GroupID:   groupID,
		StartsAt:  now.AddDate(0, 0, -40),
		ExpiresAt: now.AddDate(0, 0, -1),
		Status:    SubscriptionStatusExpired,
		DeletedAt: &deletedAt,
	}}
	svc := newSubscriptionServiceWithLinkedEntitlementRepo(repo, userSubs)
	svc.entitlementSvc.SetNowFunc(func() time.Time { return now })

	got, err := svc.RestoreSubscription(context.Background(), -102)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(-102), got.ID)
	require.Equal(t, legacySubscriptionID, userSubs.restoredID)
	require.Nil(t, userSubs.sub.DeletedAt)
	require.Equal(t, SubscriptionStatusActive, userSubs.sub.Status)
	require.Equal(t, repo.entitlements[102].StartsAt, userSubs.sub.StartsAt)
	require.Equal(t, repo.entitlements[102].ExpiresAt, userSubs.sub.ExpiresAt)
	require.Equal(t, dailyWindow, *userSubs.sub.DailyWindowStart)
	require.Equal(t, weeklyWindow, *userSubs.sub.WeeklyWindowStart)
	require.Equal(t, monthlyWindow, *userSubs.sub.MonthlyWindowStart)
	require.Equal(t, 1.25, userSubs.sub.DailyUsageUSD)
	require.Equal(t, 2.5, userSubs.sub.WeeklyUsageUSD)
	require.Equal(t, 3.75, userSubs.sub.MonthlyUsageUSD)
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

func TestSubscriptionServiceRestoreSubscription_PreservesRevokedLinkedAliasLifecycle(t *testing.T) {
	now := time.Now().UTC()
	deletedAt := now.Add(-time.Hour)
	groupID := int64(28)
	aliasDailyWindow := timezone.StartOfDay(now.AddDate(0, 0, -2))
	aliasWeeklyWindow := now.AddDate(0, 0, -6)
	aliasMonthlyWindow := now.AddDate(0, 0, -20)
	entitlementDailyWindow := aliasDailyWindow.AddDate(0, 0, -1)
	entitlementWeeklyWindow := aliasWeeklyWindow.AddDate(0, 0, -7)
	entitlementMonthlyWindow := aliasMonthlyWindow.AddDate(0, -1, 0)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[97] = &SubscriptionEntitlement{
		ID:                 97,
		UserID:             7,
		PrimaryGroupID:     &groupID,
		Name:               "Pro Plan",
		Status:             SubscriptionStatusRevoked,
		StartsAt:           now.AddDate(0, 0, -60),
		ExpiresAt:          now.AddDate(0, 0, -30),
		DailyWindowStart:   &entitlementDailyWindow,
		WeeklyWindowStart:  &entitlementWeeklyWindow,
		MonthlyWindowStart: &entitlementMonthlyWindow,
		DailyUsageUSD:      1,
		WeeklyUsageUSD:     2,
		MonthlyUsageUSD:    3,
		GroupGrants:        testGroupGrants([]int64{groupID}),
	}
	userSubs := &linkedEntitlementUserSubRepoStub{
		sub: &UserSubscription{
			ID:                 912,
			UserID:             7,
			GroupID:            groupID,
			StartsAt:           now.AddDate(0, 0, -30),
			ExpiresAt:          now.AddDate(0, 0, 1),
			Status:             SubscriptionStatusActive,
			DailyWindowStart:   &aliasDailyWindow,
			WeeklyWindowStart:  &aliasWeeklyWindow,
			MonthlyWindowStart: &aliasMonthlyWindow,
			DailyUsageUSD:      4,
			WeeklyUsageUSD:     5,
			MonthlyUsageUSD:    6,
			DeletedAt:          &deletedAt,
			EntitlementLink: &UserSubscriptionEntitlementLink{
				EntitlementID: 97,
			},
		},
	}
	svc := newSubscriptionServiceWithLinkedEntitlementRepo(repo, userSubs)
	svc.entitlementSvc.SetNowFunc(func() time.Time { return now })

	got, err := svc.RestoreSubscription(context.Background(), 912)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, SubscriptionStatusActive, got.Status)
	require.Equal(t, userSubs.sub.StartsAt, repo.entitlements[97].StartsAt)
	require.Equal(t, userSubs.sub.ExpiresAt, repo.entitlements[97].ExpiresAt)
	require.Equal(t, aliasDailyWindow, *got.DailyWindowStart)
	require.Equal(t, aliasWeeklyWindow, *got.WeeklyWindowStart)
	require.Equal(t, aliasMonthlyWindow, *got.MonthlyWindowStart)
	require.Equal(t, 4.0, got.DailyUsageUSD)
	require.Equal(t, 5.0, got.WeeklyUsageUSD)
	require.Equal(t, 6.0, got.MonthlyUsageUSD)
}

func TestSubscriptionServiceRestoreSubscription_PreservesReactivatedLinkedEntitlementTerm(t *testing.T) {
	now := time.Now().UTC()
	deletedAt := now.Add(-time.Hour)
	groupID := int64(28)
	latestStartsAt := now.Add(-time.Minute)
	latestExpiresAt := now.AddDate(0, 0, 30)
	latestDailyWindow := timezone.StartOfDay(latestStartsAt)
	latestWeeklyWindow := latestStartsAt.Add(-2 * time.Hour)
	latestMonthlyWindow := latestStartsAt.Add(-3 * time.Hour)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[98] = &SubscriptionEntitlement{
		ID:                 98,
		UserID:             7,
		PrimaryGroupID:     &groupID,
		Name:               "Pro Plan",
		Status:             SubscriptionStatusActive,
		StartsAt:           latestStartsAt,
		ExpiresAt:          latestExpiresAt,
		DailyWindowStart:   &latestDailyWindow,
		WeeklyWindowStart:  &latestWeeklyWindow,
		MonthlyWindowStart: &latestMonthlyWindow,
		DailyUsageUSD:      1.25,
		WeeklyUsageUSD:     2.5,
		MonthlyUsageUSD:    3.75,
		Notes:              "reactivated by purchase",
		GroupGrants:        testGroupGrants([]int64{groupID}),
	}
	userSubs := &linkedEntitlementUserSubRepoStub{
		sub: &UserSubscription{
			ID:        914,
			UserID:    7,
			GroupID:   groupID,
			StartsAt:  now.AddDate(0, 0, -30),
			ExpiresAt: now.AddDate(0, 0, 1),
			Status:    SubscriptionStatusActive,
			DeletedAt: &deletedAt,
			EntitlementLink: &UserSubscriptionEntitlementLink{
				EntitlementID: 98,
			},
		},
	}
	svc := newSubscriptionServiceWithLinkedEntitlementRepo(repo, userSubs)
	svc.entitlementSvc.SetNowFunc(func() time.Time { return now })

	got, err := svc.RestoreSubscription(context.Background(), 914)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, SubscriptionStatusActive, userSubs.restoredStatus)
	require.Equal(t, latestStartsAt, repo.entitlements[98].StartsAt)
	require.Equal(t, latestExpiresAt, repo.entitlements[98].ExpiresAt)
	require.Equal(t, "reactivated by purchase", repo.entitlements[98].Notes)
	require.Equal(t, latestStartsAt, got.StartsAt)
	require.Equal(t, latestExpiresAt, got.ExpiresAt)
	require.Equal(t, SubscriptionStatusActive, got.Status)
	require.Equal(t, latestDailyWindow, *got.DailyWindowStart)
	require.Equal(t, latestWeeklyWindow, *got.WeeklyWindowStart)
	require.Equal(t, latestMonthlyWindow, *got.MonthlyWindowStart)
	require.Equal(t, 1.25, got.DailyUsageUSD)
	require.Equal(t, 2.5, got.WeeklyUsageUSD)
	require.Equal(t, 3.75, got.MonthlyUsageUSD)
	require.Equal(t, latestStartsAt, userSubs.sub.StartsAt)
	require.Equal(t, latestExpiresAt, userSubs.sub.ExpiresAt)
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

func (r *linkedEntitlementUserSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id || r.sub.DeletedAt != nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *linkedEntitlementUserSubRepoStub) GetByIDIncludeDeleted(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *linkedEntitlementUserSubRepoStub) GetByIDIncludeDeletedForUpdate(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *linkedEntitlementUserSubRepoStub) Update(_ context.Context, sub *UserSubscription) error {
	if r.sub == nil || sub == nil || r.sub.ID != sub.ID {
		return ErrSubscriptionNotFound
	}
	cp := *sub
	r.sub = &cp
	return nil
}

func (r *linkedEntitlementUserSubRepoStub) Delete(_ context.Context, id int64) error {
	if r.sub == nil || r.sub.ID != id {
		return ErrSubscriptionNotFound
	}
	r.deletedID = id
	if r.sub.DeletedAt == nil {
		deletedAt := time.Now()
		cp := *r.sub
		cp.DeletedAt = &deletedAt
		r.sub = &cp
	}
	return nil
}

func (r *linkedEntitlementUserSubRepoStub) ExistsActiveByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return false, nil
}

func (r *linkedEntitlementUserSubRepoStub) Restore(_ context.Context, id int64, restoredStatus string) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	r.restoredID = id
	r.restoredStatus = restoredStatus
	cp := *r.sub
	cp.Status = restoredStatus
	cp.DeletedAt = nil
	r.sub = &cp
	return &cp, nil
}

func (r *linkedEntitlementUserSubRepoStub) RestoreWithLifecycle(_ context.Context, id int64, state UserSubscriptionLifecycleState) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	r.restoredID = id
	r.restoredStatus = state.Status
	cp := *r.sub
	cp.StartsAt = state.StartsAt
	cp.ExpiresAt = state.ExpiresAt
	cp.Status = state.Status
	cp.DailyWindowStart = state.DailyWindowStart
	cp.WeeklyWindowStart = state.WeeklyWindowStart
	cp.MonthlyWindowStart = state.MonthlyWindowStart
	cp.DailyUsageUSD = state.DailyUsageUSD
	cp.WeeklyUsageUSD = state.WeeklyUsageUSD
	cp.MonthlyUsageUSD = state.MonthlyUsageUSD
	cp.DeletedAt = nil
	r.sub = &cp
	return &cp, nil
}

func newSubscriptionServiceWithLinkedEntitlementRepo(repo *fakeSubscriptionEntitlementRepo, userSubRepo UserSubscriptionRepository) *SubscriptionService {
	svc := NewSubscriptionService(groupRepoNoop{}, userSubRepo, nil, nil, nil)
	svc.entitlementSvc = NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{}})
	svc.entitlementSvc.SetLegacySubscriptionRepository(userSubRepo)
	return svc
}
