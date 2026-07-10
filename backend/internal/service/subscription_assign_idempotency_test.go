package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/dgraph-io/ristretto"
	"github.com/stretchr/testify/require"
)

func TestWithSubscriptionUpdateTx_ReusesExistingTransaction(t *testing.T) {
	existingTx := &dbent.Tx{}
	ctx := dbent.NewTxContext(context.Background(), existingTx)
	svc := &SubscriptionService{entClient: &dbent.Client{}}

	called := false
	err := svc.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		called = true
		require.Same(t, existingTx, dbent.TxFromContext(txCtx))
		return nil
	})

	require.NoError(t, err)
	require.True(t, called)
}

func TestMaybeInvalidateAssignmentCaches_DefersForOuterTransactionOwner(t *testing.T) {
	cache, err := ristretto.NewCache(&ristretto.Config{NumCounters: 1_000, MaxCost: 100, BufferItems: 64})
	require.NoError(t, err)
	t.Cleanup(cache.Close)

	svc := &SubscriptionService{subCacheL1: cache}
	key := subCacheKey(7, 9)
	require.True(t, cache.Set(key, &UserSubscription{ID: 42}, 1))
	cache.Wait()

	svc.maybeInvalidateAssignmentCaches(7, 9, true)
	_, cachedBeforeCommit := cache.Get(key)
	require.True(t, cachedBeforeCommit, "outer transaction must retain caches until its owner commits")

	svc.maybeInvalidateAssignmentCaches(7, 9, false)
	cache.Wait()
	_, cachedAfterCommit := cache.Get(key)
	require.False(t, cachedAfterCommit, "post-commit invalidation must remove the cached subscription")
}

type groupRepoNoop struct{}

func (groupRepoNoop) Create(context.Context, *Group) error { panic("unexpected Create call") }
func (groupRepoNoop) GetByID(context.Context, int64) (*Group, error) {
	panic("unexpected GetByID call")
}
func (groupRepoNoop) GetByIDLite(context.Context, int64) (*Group, error) {
	panic("unexpected GetByIDLite call")
}
func (groupRepoNoop) Update(context.Context, *Group) error { panic("unexpected Update call") }
func (groupRepoNoop) Delete(context.Context, int64) error  { panic("unexpected Delete call") }
func (groupRepoNoop) DeleteCascade(context.Context, int64) ([]int64, error) {
	panic("unexpected DeleteCascade call")
}
func (groupRepoNoop) List(context.Context, pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (groupRepoNoop) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}
func (groupRepoNoop) ListActive(context.Context) ([]Group, error) {
	panic("unexpected ListActive call")
}
func (groupRepoNoop) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
}
func (groupRepoNoop) ExistsByName(context.Context, string) (bool, error) {
	panic("unexpected ExistsByName call")
}
func (groupRepoNoop) GetAccountCount(context.Context, int64) (int64, int64, error) {
	panic("unexpected GetAccountCount call")
}
func (groupRepoNoop) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
}
func (groupRepoNoop) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs call")
}
func (groupRepoNoop) BindAccountsToGroup(context.Context, int64, []int64) error {
	panic("unexpected BindAccountsToGroup call")
}
func (groupRepoNoop) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	panic("unexpected UpdateSortOrders call")
}

type subscriptionGroupRepoStub struct {
	groupRepoNoop
	group *Group
}

func (s *subscriptionGroupRepoStub) GetByID(context.Context, int64) (*Group, error) {
	return s.group, nil
}

type userSubRepoNoop struct{}

func (userSubRepoNoop) Create(context.Context, *UserSubscription) error {
	panic("unexpected Create call")
}
func (userSubRepoNoop) GetByID(context.Context, int64) (*UserSubscription, error) {
	panic("unexpected GetByID call")
}
func (userSubRepoNoop) GetByIDIncludeDeleted(context.Context, int64) (*UserSubscription, error) {
	panic("unexpected GetByIDIncludeDeleted call")
}
func (userSubRepoNoop) GetByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	panic("unexpected GetByUserIDAndGroupID call")
}
func (userSubRepoNoop) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	panic("unexpected GetActiveByUserIDAndGroupID call")
}
func (userSubRepoNoop) Update(context.Context, *UserSubscription) error {
	panic("unexpected Update call")
}
func (userSubRepoNoop) Delete(context.Context, int64) error { panic("unexpected Delete call") }
func (userSubRepoNoop) Restore(context.Context, int64, string) (*UserSubscription, error) {
	panic("unexpected Restore call")
}
func (userSubRepoNoop) ListByUserID(context.Context, int64) ([]UserSubscription, error) {
	panic("unexpected ListByUserID call")
}
func (userSubRepoNoop) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	panic("unexpected ListActiveByUserID call")
}
func (userSubRepoNoop) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
}
func (userSubRepoNoop) List(context.Context, pagination.PaginationParams, *int64, *int64, string, string, string, string) ([]UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (userSubRepoNoop) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	panic("unexpected ExistsByUserIDAndGroupID call")
}
func (userSubRepoNoop) ExistsActiveByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	panic("unexpected ExistsActiveByUserIDAndGroupID call")
}
func (userSubRepoNoop) ExtendExpiry(context.Context, int64, time.Time) error {
	panic("unexpected ExtendExpiry call")
}
func (userSubRepoNoop) UpdateStatus(context.Context, int64, string) error {
	panic("unexpected UpdateStatus call")
}
func (userSubRepoNoop) UpdateNotes(context.Context, int64, string) error {
	panic("unexpected UpdateNotes call")
}
func (userSubRepoNoop) ActivateWindows(context.Context, int64, time.Time) error {
	panic("unexpected ActivateWindows call")
}
func (userSubRepoNoop) ResetUsageWindows(context.Context, int64, bool, bool, bool, time.Time) error {
	panic("unexpected ResetUsageWindows call")
}
func (userSubRepoNoop) ResetDailyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetDailyUsage call")
}
func (userSubRepoNoop) ResetWeeklyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetWeeklyUsage call")
}
func (userSubRepoNoop) ResetMonthlyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetMonthlyUsage call")
}
func (userSubRepoNoop) IncrementUsage(context.Context, int64, float64) error {
	panic("unexpected IncrementUsage call")
}
func (userSubRepoNoop) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	panic("unexpected BatchUpdateExpiredStatus call")
}

type subscriptionUserSubRepoStub struct {
	userSubRepoNoop

	nextID      int64
	byID        map[int64]*UserSubscription
	byUserGroup map[string]*UserSubscription
	createCalls int
	deleteCalls int
}

func newSubscriptionUserSubRepoStub() *subscriptionUserSubRepoStub {
	return &subscriptionUserSubRepoStub{
		nextID:      1,
		byID:        make(map[int64]*UserSubscription),
		byUserGroup: make(map[string]*UserSubscription),
	}
}

func (s *subscriptionUserSubRepoStub) key(userID, groupID int64) string {
	return strconvFormatInt(userID) + ":" + strconvFormatInt(groupID)
}

func (s *subscriptionUserSubRepoStub) seed(sub *UserSubscription) {
	if sub == nil {
		return
	}
	cp := *sub
	if cp.ID == 0 {
		cp.ID = s.nextID
		s.nextID++
	}
	s.byID[cp.ID] = &cp
	s.byUserGroup[s.key(cp.UserID, cp.GroupID)] = &cp
}

func (s *subscriptionUserSubRepoStub) setEntitlementLink(id int64, link *UserSubscriptionEntitlementLink) {
	sub := s.byID[id]
	if sub == nil {
		return
	}
	cp := *sub
	cp.EntitlementLink = link
	s.byID[cp.ID] = &cp
	s.byUserGroup[s.key(cp.UserID, cp.GroupID)] = &cp
}

func (s *subscriptionUserSubRepoStub) ExistsByUserIDAndGroupID(_ context.Context, userID, groupID int64) (bool, error) {
	_, ok := s.byUserGroup[s.key(userID, groupID)]
	return ok, nil
}

func (s *subscriptionUserSubRepoStub) GetByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	sub := s.byUserGroup[s.key(userID, groupID)]
	if sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (s *subscriptionUserSubRepoStub) Create(_ context.Context, sub *UserSubscription) error {
	if sub == nil {
		return nil
	}
	s.createCalls++
	cp := *sub
	if cp.ID == 0 {
		cp.ID = s.nextID
		s.nextID++
	}
	sub.ID = cp.ID
	s.byID[cp.ID] = &cp
	s.byUserGroup[s.key(cp.UserID, cp.GroupID)] = &cp
	return nil
}

func (s *subscriptionUserSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	sub := s.byID[id]
	if sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (s *subscriptionUserSubRepoStub) Update(_ context.Context, sub *UserSubscription) error {
	if sub == nil {
		return ErrSubscriptionNilInput
	}
	existing := s.byID[sub.ID]
	if existing == nil {
		return ErrSubscriptionNotFound
	}
	oldKey := s.key(existing.UserID, existing.GroupID)
	cp := *sub
	s.byID[cp.ID] = &cp
	if oldKey != s.key(cp.UserID, cp.GroupID) {
		delete(s.byUserGroup, oldKey)
	}
	s.byUserGroup[s.key(cp.UserID, cp.GroupID)] = &cp
	return nil
}

func TestAssignSubscriptionReuseWhenSemanticsMatch(t *testing.T) {
	start := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	subRepo.seed(&UserSubscription{
		ID:        10,
		UserID:    1001,
		GroupID:   1,
		StartsAt:  start,
		ExpiresAt: start.AddDate(0, 0, 30),
		Notes:     "init",
	})

	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       1001,
		GroupID:      1,
		ValidityDays: 30,
		Notes:        "init",
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), sub.ID)
	require.Equal(t, 0, subRepo.createCalls, "reuse should not create new subscription")
}

func TestAssignSubscriptionConflictWhenSemanticsMismatch(t *testing.T) {
	start := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	subRepo.seed(&UserSubscription{
		ID:        11,
		UserID:    2001,
		GroupID:   1,
		StartsAt:  start,
		ExpiresAt: start.AddDate(0, 0, 30),
		Notes:     "old-note",
	})

	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	_, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       2001,
		GroupID:      1,
		ValidityDays: 30,
		Notes:        "new-note",
	})
	require.Error(t, err)
	require.Equal(t, "SUBSCRIPTION_ASSIGN_CONFLICT", infraerrorsReason(err))
	require.Equal(t, 0, subRepo.createCalls, "conflict should not create or mutate existing subscription")
}

func TestBulkAssignSubscriptionCreatedReusedAndConflict(t *testing.T) {
	start := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	// user 1: 语义一致，可 reused
	subRepo.seed(&UserSubscription{
		ID:        21,
		UserID:    1,
		GroupID:   1,
		StartsAt:  start,
		ExpiresAt: start.AddDate(0, 0, 30),
		Notes:     "same-note",
	})
	// user 3: 语义冲突（有效期不一致），应 failed
	subRepo.seed(&UserSubscription{
		ID:        23,
		UserID:    3,
		GroupID:   1,
		StartsAt:  start,
		ExpiresAt: start.AddDate(0, 0, 60),
		Notes:     "same-note",
	})

	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	result, err := svc.BulkAssignSubscription(context.Background(), &BulkAssignSubscriptionInput{
		UserIDs:      []int64{1, 2, 3},
		GroupID:      1,
		ValidityDays: 30,
		AssignedBy:   9,
		Notes:        "same-note",
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.SuccessCount)
	require.Equal(t, 1, result.CreatedCount)
	require.Equal(t, 1, result.ReusedCount)
	require.Equal(t, 1, result.FailedCount)
	require.Equal(t, "reused", result.Statuses[1])
	require.Equal(t, "created", result.Statuses[2])
	require.Equal(t, "failed", result.Statuses[3])
	require.Equal(t, 1, subRepo.createCalls)
}

func TestAssignSubscriptionKeepsWorkingWhenIdempotencyStoreUnavailable(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	SetDefaultIdempotencyCoordinator(NewIdempotencyCoordinator(failingIdempotencyRepo{}, DefaultIdempotencyConfig()))
	t.Cleanup(func() {
		SetDefaultIdempotencyCoordinator(nil)
	})

	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       9001,
		GroupID:      1,
		ValidityDays: 30,
		Notes:        "new",
	})
	require.NoError(t, err)
	require.NotNil(t, sub)
	require.Equal(t, 1, subRepo.createCalls, "semantic idempotent endpoint should not depend on idempotency store availability")
}

func TestAssignSubscriptionV2OffWithPlanIDKeepsLegacyOnly(t *testing.T) {
	entRepo := newFakeSubscriptionEntitlementRepo(time.Now())
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		77: testEntitlementPlan(77, []int64{1, 2}, nil),
	}}
	svc := newAssignSubscriptionEntitlementTestService(false, entRepo, planRepo)

	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       3001,
		GroupID:      1,
		PlanID:       77,
		ValidityDays: 30,
		AssignedBy:   9,
		Notes:        "admin-plan",
	})

	require.NoError(t, err)
	require.NotZero(t, sub.ID)
	subRepo, ok := svc.userSubRepo.(*subscriptionUserSubRepoStub)
	require.True(t, ok)
	require.Equal(t, 1, subRepo.createCalls)
	require.Zero(t, entRepo.createCount)
	require.Zero(t, entRepo.eventCount)
}

func TestAssignSubscriptionV2PlanCreatesEntitlementWithLegacySubscriptionID(t *testing.T) {
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		77: testEntitlementPlan(77, []int64{1, 2}, nil),
	}}
	svc := newAssignSubscriptionEntitlementTestService(true, entRepo, planRepo)

	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       3002,
		GroupID:      1,
		PlanID:       77,
		ValidityDays: 30,
		AssignedBy:   9,
		Notes:        "admin-plan",
	})

	require.NoError(t, err)
	require.NotZero(t, sub.ID)
	require.Equal(t, 1, entRepo.createCount)
	require.Equal(t, 1, entRepo.eventCount)
	require.Len(t, entRepo.entitlements, 1)
	var ent *SubscriptionEntitlement
	for _, candidate := range entRepo.entitlements {
		ent = candidate
	}
	require.NotNil(t, ent)
	require.NotNil(t, ent.LegacySubscriptionID)
	require.Equal(t, sub.ID, *ent.LegacySubscriptionID)
	require.Equal(t, []int64{1, 2}, entitlementGroupIDs(ent))
	require.NotNil(t, ent.SourceExternalID)
	require.Equal(t, adminAssignEntitlementSourceExternalID(sub.ID, 77), *ent.SourceExternalID)
}

func TestAssignSubscriptionV2PlanReplayDoesNotDuplicateEntitlement(t *testing.T) {
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		77: testEntitlementPlan(77, []int64{1, 2}, nil),
	}}
	svc := newAssignSubscriptionEntitlementTestService(true, entRepo, planRepo)
	input := &AssignSubscriptionInput{
		UserID:       3003,
		GroupID:      1,
		PlanID:       77,
		ValidityDays: 30,
		AssignedBy:   9,
		Notes:        "admin-plan",
	}

	first, err := svc.AssignSubscription(context.Background(), input)
	require.NoError(t, err)
	second, err := svc.AssignSubscription(context.Background(), input)
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID)
	require.Equal(t, 1, entRepo.createCount)
	require.Equal(t, 1, entRepo.eventCount)
	require.Len(t, entRepo.entitlements, 1)
}

func TestAssignSubscriptionV2PlanFallbacksToEntitlementOnlyWhenExistingLegacyBelongsToDifferentPlan(t *testing.T) {
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	existingPlanID := int64(10)
	groupID := int64(1)
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		9:  testEntitlementPlan(9, []int64{groupID, 2}, nil),
		10: testEntitlementPlan(10, []int64{groupID, 2}, nil),
	}}
	svc := newAssignSubscriptionEntitlementTestService(true, entRepo, planRepo)
	subRepo := requireSubscriptionUserSubRepoStub(t, svc)
	subRepo.seed(&UserSubscription{
		ID:        43,
		UserID:    3005,
		GroupID:   groupID,
		StartsAt:  now,
		ExpiresAt: now.AddDate(0, 0, 30),
		Status:    SubscriptionStatusActive,
		Notes:     "admin-plan",
		EntitlementLink: &UserSubscriptionEntitlementLink{
			EntitlementID:   91,
			PlanID:          &existingPlanID,
			Status:          SubscriptionStatusActive,
			ExpiresAt:       now.AddDate(0, 0, 30),
			PrimaryGroupID:  &groupID,
			OveragePolicy:   SubscriptionEntitlementOverageBalanceFallback,
			MonthlyUsageUSD: 0,
		},
	})

	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       3005,
		GroupID:      groupID,
		PlanID:       9,
		ValidityDays: 30,
		AssignedBy:   9,
		Notes:        "admin-plan",
	})

	require.NoError(t, err)
	require.NotNil(t, sub)
	require.True(t, sub.EntitlementOnly)
	require.Negative(t, sub.ID)
	require.NotNil(t, sub.EntitlementLink)
	require.NotNil(t, sub.EntitlementLink.PlanID)
	require.Equal(t, int64(9), *sub.EntitlementLink.PlanID)
	require.Equal(t, 0, subRepo.createCalls)
	require.Equal(t, 1, entRepo.createCount)
	require.Len(t, entRepo.entitlements, 1)
	for _, ent := range entRepo.entitlements {
		require.Nil(t, ent.LegacySubscriptionID)
		require.Equal(t, int64(9), *ent.PlanID)
	}
}

func TestAssignSubscriptionV2PlanFallbacksToEntitlementOnlyWhenExistingLegacyPlanIsUnknown(t *testing.T) {
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	groupID := int64(1)
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		9: testEntitlementPlan(9, []int64{groupID, 2}, nil),
	}}
	svc := newAssignSubscriptionEntitlementTestService(true, entRepo, planRepo)
	subRepo := requireSubscriptionUserSubRepoStub(t, svc)
	subRepo.seed(&UserSubscription{
		ID:        44,
		UserID:    3007,
		GroupID:   groupID,
		StartsAt:  now,
		ExpiresAt: now.AddDate(0, 0, 30),
		Status:    SubscriptionStatusActive,
		Notes:     "admin-plan",
		EntitlementLink: &UserSubscriptionEntitlementLink{
			EntitlementID:  92,
			Status:         SubscriptionStatusActive,
			ExpiresAt:      now.AddDate(0, 0, 30),
			PrimaryGroupID: &groupID,
		},
	})

	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       3007,
		GroupID:      groupID,
		PlanID:       9,
		ValidityDays: 30,
		AssignedBy:   9,
		Notes:        "admin-plan",
	})

	require.NoError(t, err)
	require.NotNil(t, sub)
	require.True(t, sub.EntitlementOnly)
	require.Negative(t, sub.ID)
	require.NotNil(t, sub.EntitlementLink)
	require.NotNil(t, sub.EntitlementLink.PlanID)
	require.Equal(t, int64(9), *sub.EntitlementLink.PlanID)
	require.Equal(t, 0, subRepo.createCalls)
	require.Equal(t, 1, entRepo.createCount)
	require.Len(t, entRepo.entitlements, 1)
	for _, ent := range entRepo.entitlements {
		require.Nil(t, ent.LegacySubscriptionID)
		require.Equal(t, int64(9), *ent.PlanID)
	}
}

func TestAssignSubscriptionV2PlanReassignAfterLinkedRevocationUsesNewLegacySubscription(t *testing.T) {
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	groupID := int64(1)
	wrongPlanID := int64(10)
	rightPlanID := int64(9)
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		rightPlanID: testEntitlementPlan(rightPlanID, []int64{groupID, 2}, nil),
		wrongPlanID: testEntitlementPlan(wrongPlanID, []int64{groupID, 2}, nil),
	}}
	svc := newAssignSubscriptionEntitlementTestService(true, entRepo, planRepo)
	subRepo := requireSubscriptionUserSubRepoStub(t, svc)

	wrong, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       3006,
		GroupID:      groupID,
		PlanID:       wrongPlanID,
		ValidityDays: 30,
		AssignedBy:   9,
		Notes:        "wrong-plan",
	})
	require.NoError(t, err)
	wrongEnt := requireTestEntitlementByLegacy(t, entRepo, wrong.ID, wrongPlanID)
	subRepo.setEntitlementLink(wrong.ID, &UserSubscriptionEntitlementLink{
		EntitlementID:  wrongEnt.ID,
		PlanID:         &wrongPlanID,
		Status:         SubscriptionStatusActive,
		ExpiresAt:      wrongEnt.ExpiresAt,
		PrimaryGroupID: &groupID,
	})

	err = svc.RevokeSubscription(context.Background(), wrong.ID)
	require.NoError(t, err)
	require.Equal(t, 1, subRepo.deleteCalls)
	exists, err := subRepo.ExistsByUserIDAndGroupID(context.Background(), 3006, groupID)
	require.NoError(t, err)
	require.False(t, exists)
	revokedWrongEnt, err := entRepo.GetByID(context.Background(), wrongEnt.ID)
	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusRevoked, revokedWrongEnt.Status)

	right, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       3006,
		GroupID:      groupID,
		PlanID:       rightPlanID,
		ValidityDays: 30,
		AssignedBy:   9,
		Notes:        "right-plan",
	})
	require.NoError(t, err)
	require.NotEqual(t, wrong.ID, right.ID)
	require.Equal(t, 2, subRepo.createCalls)
	require.Equal(t, 2, entRepo.createCount)
	rightEnt := requireTestEntitlementByLegacy(t, entRepo, right.ID, rightPlanID)
	require.NotNil(t, rightEnt.SourceExternalID)
	require.Equal(t, adminAssignEntitlementSourceExternalID(right.ID, rightPlanID), *rightEnt.SourceExternalID)
	require.NotNil(t, revokedWrongEnt.LegacySubscriptionID)
	require.Equal(t, wrong.ID, *revokedWrongEnt.LegacySubscriptionID)
}

func TestAssignSubscriptionV2PlanReusesExistingEntitlementAndBackfillsLegacyID(t *testing.T) {
	now := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	planID := int64(77)
	require.NoError(t, entRepo.Create(context.Background(), &SubscriptionEntitlement{
		UserID:    3004,
		PlanID:    &planID,
		Status:    SubscriptionStatusActive,
		StartsAt:  now.Add(-time.Hour),
		ExpiresAt: now.Add(24 * time.Hour),
		Name:      "existing",
	}, []int64{1, 2}))
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		77: testEntitlementPlan(77, []int64{1, 2}, nil),
	}}
	svc := newAssignSubscriptionEntitlementTestService(true, entRepo, planRepo)

	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       3004,
		GroupID:      1,
		PlanID:       77,
		ValidityDays: 30,
		AssignedBy:   9,
		Notes:        "admin-plan",
	})

	require.NoError(t, err)
	require.Equal(t, 1, entRepo.createCount, "only the seeded entitlement should exist")
	require.Equal(t, 1, entRepo.updateTermCount)
	require.Equal(t, 1, entRepo.eventCount)
	require.Len(t, entRepo.entitlements, 1)
	ent, err := entRepo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, ent.LegacySubscriptionID)
	require.Equal(t, sub.ID, *ent.LegacySubscriptionID)
	require.NotNil(t, ent.SourceExternalID)
	require.Equal(t, adminAssignEntitlementSourceExternalID(sub.ID, 77), *ent.SourceExternalID)
}

func newAssignSubscriptionEntitlementTestService(enabled bool, entRepo *fakeSubscriptionEntitlementRepo, planRepo *fakeSubscriptionEntitlementPlanRepo) *SubscriptionService {
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{
			ID:                  1,
			Status:              StatusActive,
			SubscriptionType:    SubscriptionTypeSubscription,
			SubscriptionEnabled: true,
		},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	entSvc := NewSubscriptionEntitlementService(entRepo, planRepo)
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	svc.SetSubscriptionEntitlementAliasDependencies(subscriptionAliasRuntimeProviderStub{
		runtime: SubscriptionEntitlementsRuntime{Enabled: enabled},
	}, entSvc)
	return svc
}

func requireSubscriptionUserSubRepoStub(t *testing.T, svc *SubscriptionService) *subscriptionUserSubRepoStub {
	t.Helper()
	subRepo, ok := svc.userSubRepo.(*subscriptionUserSubRepoStub)
	require.True(t, ok)
	return subRepo
}

func TestNormalizeAssignValidityDays(t *testing.T) {
	require.Equal(t, 30, normalizeAssignValidityDays(0))
	require.Equal(t, 30, normalizeAssignValidityDays(-5))
	require.Equal(t, MaxValidityDays, normalizeAssignValidityDays(MaxValidityDays+100))
	require.Equal(t, 7, normalizeAssignValidityDays(7))
}

func TestDetectAssignSemanticConflictCases(t *testing.T) {
	start := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	base := &UserSubscription{
		UserID:    1,
		GroupID:   1,
		StartsAt:  start,
		ExpiresAt: start.AddDate(0, 0, 30),
		Notes:     "same",
	}

	reason, conflict := detectAssignSemanticConflict(base, &AssignSubscriptionInput{
		UserID:       1,
		GroupID:      1,
		ValidityDays: 30,
		Notes:        "same",
	})
	require.False(t, conflict)
	require.Equal(t, "", reason)

	reason, conflict = detectAssignSemanticConflict(base, &AssignSubscriptionInput{
		UserID:       1,
		GroupID:      1,
		ValidityDays: 60,
		Notes:        "same",
	})
	require.True(t, conflict)
	require.Equal(t, "validity_days_mismatch", reason)

	reason, conflict = detectAssignSemanticConflict(base, &AssignSubscriptionInput{
		UserID:       1,
		GroupID:      1,
		ValidityDays: 30,
		Notes:        "other",
	})
	require.True(t, conflict)
	require.Equal(t, "notes_mismatch", reason)

	base.EntitlementLink = &UserSubscriptionEntitlementLink{EntitlementID: 91}
	reason, conflict = detectAssignSemanticConflict(base, &AssignSubscriptionInput{
		UserID:       1,
		GroupID:      1,
		PlanID:       9,
		ValidityDays: 30,
		Notes:        "same",
	})
	require.True(t, conflict)
	require.Equal(t, "plan_id_mismatch", reason)
}

func TestAssignSubscriptionGroupTypeValidation(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeStandard},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	_, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       1,
		GroupID:      1,
		ValidityDays: 30,
	})
	require.Error(t, err)
	require.Equal(t, infraerrors.Code(ErrGroupNotSubscriptionType), infraerrors.Code(err))
}

func strconvFormatInt(v int64) string {
	return strconv.FormatInt(v, 10)
}

func infraerrorsReason(err error) string {
	return infraerrors.Reason(err)
}

func requireTestEntitlementByLegacy(t *testing.T, repo *fakeSubscriptionEntitlementRepo, legacySubscriptionID, planID int64) *SubscriptionEntitlement {
	t.Helper()
	repo.mu.Lock()
	defer repo.mu.Unlock()
	for _, ent := range repo.entitlements {
		if ent.LegacySubscriptionID != nil && *ent.LegacySubscriptionID == legacySubscriptionID && ent.PlanID != nil && *ent.PlanID == planID {
			return cloneTestEntitlement(ent)
		}
	}
	t.Fatalf("entitlement not found for legacy subscription %d and plan %d", legacySubscriptionID, planID)
	return nil
}
