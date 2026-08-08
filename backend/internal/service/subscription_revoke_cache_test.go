//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type revokeCacheUserSubRepoStub struct {
	userSubRepoNoop

	sub            *UserSubscription
	deleted        bool
	getActiveCalls int
	onUpdate       func()
	onDelete       func()
}

func (r *revokeCacheUserSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id || r.deleted {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *revokeCacheUserSubRepoStub) Delete(_ context.Context, id int64) error {
	if r.sub == nil || r.sub.ID != id || r.deleted {
		return ErrSubscriptionNotFound
	}
	if r.onDelete != nil {
		r.onDelete()
	}
	r.deleted = true
	deletedAt := time.Now()
	r.sub.DeletedAt = &deletedAt
	return nil
}

func (r *revokeCacheUserSubRepoStub) GetByIDIncludeDeletedForUpdate(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *revokeCacheUserSubRepoStub) Update(_ context.Context, sub *UserSubscription) error {
	if r.sub == nil || sub == nil || r.sub.ID != sub.ID {
		return ErrSubscriptionNotFound
	}
	if r.onUpdate != nil {
		r.onUpdate()
	}
	cp := *sub
	r.sub = &cp
	return nil
}

func (r *revokeCacheUserSubRepoStub) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	r.getActiveCalls++
	if r.deleted || r.sub == nil || r.sub.UserID != userID || r.sub.GroupID != groupID {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *revokeCacheUserSubRepoStub) ListActiveByUserID(_ context.Context, userID int64) ([]UserSubscription, error) {
	if r.deleted || r.sub == nil || r.sub.UserID != userID {
		return nil, nil
	}
	cp := *r.sub
	return []UserSubscription{cp}, nil
}

func TestRevokeSubscription_InvalidatesL1CacheSynchronously(t *testing.T) {
	repo := &revokeCacheUserSubRepoStub{
		sub: &UserSubscription{
			ID:        1,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, &config.Config{
		SubscriptionCache: config.SubscriptionCacheConfig{
			L1Size:       16,
			L1TTLSeconds: 60,
		},
	})
	t.Cleanup(svc.Stop)

	_, err := svc.GetActiveSubscription(context.Background(), 10, 20)
	require.NoError(t, err)
	svc.subCacheL1.Wait()
	require.Equal(t, 1, repo.getActiveCalls)

	err = svc.RevokeSubscription(context.Background(), 1)
	require.NoError(t, err)

	_, err = svc.GetActiveSubscription(context.Background(), 10, 20)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	require.Equal(t, 2, repo.getActiveCalls, "撤销后应回源确认订阅已不存在，不能命中旧 L1")
}

func TestRevokeUserEntitlement_InvalidatesLinkedAliasCacheAfterMutation(t *testing.T) {
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	const (
		userID               = int64(10)
		groupID              = int64(20)
		legacySubscriptionID = int64(30)
		entitlementID        = int64(40)
	)
	userSubs := &revokeCacheUserSubRepoStub{sub: &UserSubscription{
		ID:        legacySubscriptionID,
		UserID:    userID,
		GroupID:   groupID,
		Status:    SubscriptionStatusActive,
		StartsAt:  now.Add(-time.Hour),
		ExpiresAt: now.Add(24 * time.Hour),
	}}
	legacyID := legacySubscriptionID
	primaryGroupID := groupID
	entitlementRepo := newFakeSubscriptionEntitlementRepo(now)
	entitlementRepo.entitlements[entitlementID] = &SubscriptionEntitlement{
		ID:                   entitlementID,
		UserID:               userID,
		LegacySubscriptionID: &legacyID,
		PrimaryGroupID:       &primaryGroupID,
		Status:               SubscriptionStatusActive,
		StartsAt:             now.Add(-time.Hour),
		ExpiresAt:            now.Add(24 * time.Hour),
		GroupGrants:          testGroupGrants([]int64{groupID}),
	}
	entitlementSvc := NewSubscriptionEntitlementService(entitlementRepo, nil)
	svc := NewSubscriptionService(groupRepoNoop{}, userSubs, nil, nil, &config.Config{
		SubscriptionCache: config.SubscriptionCacheConfig{L1Size: 1024, L1TTLSeconds: 60},
	})
	t.Cleanup(svc.Stop)
	svc.SetSubscriptionEntitlementAliasDependencies(nil, entitlementSvc)
	seedLinkedAliasCacheEntries(t, svc, userID, groupID)
	userSubs.onDelete = func() {
		requireLinkedAliasCacheEntries(t, svc, userID, groupID, true, "cache invalidation must wait until the entitlement transaction finishes")
	}

	err := entitlementSvc.RevokeUserEntitlement(context.Background(), userID, entitlementID, now)

	require.NoError(t, err)
	requireLinkedAliasCacheEntries(t, svc, userID, groupID, false, "direct entitlement revoke must invalidate both linked alias cache keys")
}

func TestAdvanceEntitlementMonthlyCycle_InvalidatesLinkedAliasCacheAfterMutation(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	const (
		userID               = int64(11)
		groupID              = int64(21)
		legacySubscriptionID = int64(31)
		entitlementID        = int64(41)
	)
	monthlyLimit := 100.0
	startsAt := now.Add(-10 * 24 * time.Hour)
	monthlyWindowStart := startsAt
	expiresAt := startsAt.Add(120 * 24 * time.Hour)
	userSubs := &revokeCacheUserSubRepoStub{sub: &UserSubscription{
		ID:                 legacySubscriptionID,
		UserID:             userID,
		GroupID:            groupID,
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          expiresAt,
		MonthlyWindowStart: &monthlyWindowStart,
		MonthlyUsageUSD:    monthlyLimit,
	}}
	entitlementRepo := newAdvanceEntitlementMonthlyCycleRepo(now)
	require.NoError(t, entitlementRepo.Create(context.Background(), &SubscriptionEntitlement{
		ID:                   entitlementID,
		UserID:               userID,
		LegacySubscriptionID: func() *int64 { v := legacySubscriptionID; return &v }(),
		PrimaryGroupID:       func() *int64 { v := groupID; return &v }(),
		Status:               SubscriptionStatusActive,
		StartsAt:             startsAt,
		ExpiresAt:            expiresAt,
		MonthlyLimitUSD:      &monthlyLimit,
		MonthlyUsageUSD:      monthlyLimit,
		MonthlyWindowStart:   &monthlyWindowStart,
	}, []int64{groupID}))
	entitlementSvc := NewSubscriptionEntitlementService(entitlementRepo, nil)
	entitlementSvc.SetNowFunc(func() time.Time { return now })
	svc := NewSubscriptionService(groupRepoNoop{}, userSubs, nil, nil, &config.Config{
		SubscriptionCache: config.SubscriptionCacheConfig{L1Size: 1024, L1TTLSeconds: 60},
	})
	t.Cleanup(svc.Stop)
	svc.SetSubscriptionEntitlementAliasDependencies(nil, entitlementSvc)
	seedLinkedAliasCacheEntries(t, svc, userID, groupID)
	userSubs.onUpdate = func() {
		requireLinkedAliasCacheEntries(t, svc, userID, groupID, true, "cache invalidation must not run inside the entitlement transaction")
	}

	result, err := entitlementSvc.AdvanceMonthlyCycle(context.Background(), userID, entitlementID)

	require.NoError(t, err)
	require.NotNil(t, result)
	requireLinkedAliasCacheEntries(t, svc, userID, groupID, false, "direct monthly-cycle advance must invalidate both linked alias cache keys")
}

func seedLinkedAliasCacheEntries(t *testing.T, svc *SubscriptionService, userID, groupID int64) {
	t.Helper()
	require.NotNil(t, svc.subCacheL1)
	_, err := svc.GetActiveSubscription(context.Background(), userID, groupID)
	require.NoError(t, err)
	_, err = svc.listActiveSubscriptionsForSwitch(context.Background(), userID)
	require.NoError(t, err)
	svc.subCacheL1.Wait()
	requireLinkedAliasCacheEntries(t, svc, userID, groupID, true, "cache fixture was not populated")
}

func requireLinkedAliasCacheEntries(t *testing.T, svc *SubscriptionService, userID, groupID int64, wantPresent bool, message string) {
	t.Helper()
	_, subscriptionPresent := svc.subCacheL1.Get(subCacheKey(userID, groupID))
	_, listPresent := svc.subCacheL1.Get(activeSubscriptionsCacheKey(userID))
	require.Equal(t, wantPresent, subscriptionPresent, message)
	require.Equal(t, wantPresent, listPresent, message)
}

type restoreUserSubRepoStub struct {
	userSubRepoNoop

	sub            *UserSubscription
	existsActive   bool
	restoreCalls   int
	restoredStatus string
}

func (r *restoreUserSubRepoStub) GetByIDIncludeDeleted(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *restoreUserSubRepoStub) ExistsActiveByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return r.existsActive, nil
}

func (r *restoreUserSubRepoStub) Restore(_ context.Context, id int64, restoredStatus string) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	r.restoreCalls++
	r.restoredStatus = restoredStatus
	cp := *r.sub
	cp.Status = restoredStatus
	cp.DeletedAt = nil
	r.sub = &cp
	return &cp, nil
}

func TestRestoreSubscription_ExpiredActiveRestoresAsExpired(t *testing.T) {
	deletedAt := time.Now().Add(-time.Hour)
	repo := &restoreUserSubRepoStub{
		sub: &UserSubscription{
			ID:        1,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(-time.Minute),
			DeletedAt: &deletedAt,
		},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)

	restored, err := svc.RestoreSubscription(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 1, repo.restoreCalls)
	require.Equal(t, SubscriptionStatusExpired, repo.restoredStatus)
	require.Equal(t, SubscriptionStatusExpired, restored.Status)
	require.Nil(t, restored.DeletedAt)
}

func TestRestoreSubscription_NotRevokedReturnsConflict(t *testing.T) {
	repo := &restoreUserSubRepoStub{
		sub: &UserSubscription{
			ID:        1,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)

	_, err := svc.RestoreSubscription(context.Background(), 1)
	require.ErrorIs(t, err, ErrSubscriptionNotRevoked)
	require.Zero(t, repo.restoreCalls)
}

func TestRestoreSubscription_LiveSubscriptionConflict(t *testing.T) {
	deletedAt := time.Now().Add(-time.Hour)
	repo := &restoreUserSubRepoStub{
		existsActive: true,
		sub: &UserSubscription{
			ID:        1,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusExpired,
			ExpiresAt: time.Now().Add(-time.Hour),
			DeletedAt: &deletedAt,
		},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)

	_, err := svc.RestoreSubscription(context.Background(), 1)
	require.ErrorIs(t, err, ErrSubscriptionRestoreConflict)
	require.Zero(t, repo.restoreCalls)
}
