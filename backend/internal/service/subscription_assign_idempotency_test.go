package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
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

type assignmentInvalidationCacheStub struct {
	billingCacheWorkerStub
	invalidated chan struct{}
	published   chan string
}

func (s *assignmentInvalidationCacheStub) InvalidateSubscriptionCache(context.Context, int64, int64) error {
	s.invalidated <- struct{}{}
	return nil
}

func (s *assignmentInvalidationCacheStub) PublishSubscriptionCacheInvalidation(_ context.Context, cacheKey string) error {
	s.published <- cacheKey
	return nil
}

func (s *assignmentInvalidationCacheStub) SubscribeSubscriptionCacheInvalidation(context.Context, func(string)) error {
	return nil
}

func TestMaybeInvalidateAssignmentCachesPublishesCrossInstanceInvalidation(t *testing.T) {
	cacheBackend := &assignmentInvalidationCacheStub{
		invalidated: make(chan struct{}, 1),
		published:   make(chan string, 2),
	}
	svc := &SubscriptionService{
		billingCacheService: &BillingCacheService{cache: cacheBackend},
	}

	svc.maybeInvalidateAssignmentCaches(7, 9, false)

	select {
	case <-cacheBackend.invalidated:
	case <-time.After(time.Second):
		t.Fatal("subscription cache invalidation did not run")
	}
	published := make([]string, 0, 2)
	for len(published) < 2 {
		select {
		case key := <-cacheBackend.published:
			published = append(published, key)
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("missing cross-instance subscription cache invalidation: got %v", published)
		}
	}
	require.ElementsMatch(t, []string{subCacheKey(7, 9), activeSubscriptionsCacheKey(7)}, published)
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
func (userSubRepoNoop) GetByIDForUpdate(context.Context, int64) (*UserSubscription, error) {
	panic("unexpected GetByIDForUpdate call")
}
func (userSubRepoNoop) GetByIDIncludeDeleted(context.Context, int64) (*UserSubscription, error) {
	panic("unexpected GetByIDIncludeDeleted call")
}
func (userSubRepoNoop) GetByIDIncludeDeletedForUpdate(context.Context, int64) (*UserSubscription, error) {
	panic("unexpected GetByIDIncludeDeletedForUpdate call")
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
func (userSubRepoNoop) RestoreWithLifecycle(context.Context, int64, UserSubscriptionLifecycleState) (*UserSubscription, error) {
	panic("unexpected RestoreWithLifecycle call")
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
func (userSubRepoNoop) ActivateWindows(context.Context, int64, time.Time, time.Time) error {
	panic("unexpected ActivateWindows call")
}
func (userSubRepoNoop) ResetUsageWindows(context.Context, int64, bool, bool, bool, time.Time, time.Time) error {
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
	if sub == nil || sub.DeletedAt != nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (s *subscriptionUserSubRepoStub) GetByIDForUpdate(ctx context.Context, id int64) (*UserSubscription, error) {
	return s.GetByID(ctx, id)
}

func (s *subscriptionUserSubRepoStub) GetByIDIncludeDeleted(ctx context.Context, id int64) (*UserSubscription, error) {
	sub := s.byID[id]
	if sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (s *subscriptionUserSubRepoStub) GetByIDIncludeDeletedForUpdate(ctx context.Context, id int64) (*UserSubscription, error) {
	return s.GetByIDIncludeDeleted(ctx, id)
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
	start := time.Now().Add(-time.Hour)
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
		Status:    SubscriptionStatusActive,
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
	require.Equal(t, start, sub.StartsAt)
	require.Equal(t, start.AddDate(0, 0, 30), sub.ExpiresAt)
}

func TestAssignSubscriptionDoesNotReactivateFutureSuspendedSubscription(t *testing.T) {
	start := time.Now().Add(-time.Hour)
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	subRepo.seed(&UserSubscription{
		ID:        13,
		UserID:    1003,
		GroupID:   1,
		StartsAt:  start,
		ExpiresAt: start.AddDate(0, 0, 30),
		Status:    SubscriptionStatusSuspended,
		Notes:     "assignment",
	})

	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       1003,
		GroupID:      1,
		ValidityDays: 30,
		Notes:        "assignment",
	})

	require.NoError(t, err)
	require.Equal(t, int64(13), sub.ID)
	require.Equal(t, SubscriptionStatusSuspended, sub.Status)
	require.Equal(t, start, sub.StartsAt)
	require.Equal(t, start.AddDate(0, 0, 30), sub.ExpiresAt)
	require.Equal(t, "assignment", sub.Notes)
	require.Equal(t, 0, subRepo.createCalls)
}

func TestAssignSubscriptionDoesNotReactivatePastExpirySuspendedSubscription(t *testing.T) {
	start := time.Now().AddDate(0, 0, -31)
	expiresAt := start.AddDate(0, 0, 30)
	windowStart := startOfDay(start)
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	subRepo.seed(&UserSubscription{
		ID:                 15,
		UserID:             1005,
		GroupID:            1,
		StartsAt:           start,
		ExpiresAt:          expiresAt,
		Status:             SubscriptionStatusSuspended,
		DailyWindowStart:   &windowStart,
		WeeklyWindowStart:  &windowStart,
		MonthlyWindowStart: &windowStart,
		DailyUsageUSD:      1,
		WeeklyUsageUSD:     2,
		MonthlyUsageUSD:    3,
		Notes:              "suspended assignment",
	})

	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       1005,
		GroupID:      1,
		ValidityDays: 30,
		Notes:        "suspended assignment",
	})

	require.NoError(t, err)
	require.Equal(t, int64(15), sub.ID)
	require.Equal(t, SubscriptionStatusSuspended, sub.Status)
	require.Equal(t, start, sub.StartsAt)
	require.Equal(t, expiresAt, sub.ExpiresAt)
	require.Equal(t, "suspended assignment", sub.Notes)
	require.Equal(t, &windowStart, sub.DailyWindowStart)
	require.Equal(t, &windowStart, sub.WeeklyWindowStart)
	require.Equal(t, &windowStart, sub.MonthlyWindowStart)
	require.Equal(t, float64(1), sub.DailyUsageUSD)
	require.Equal(t, float64(2), sub.WeeklyUsageUSD)
	require.Equal(t, float64(3), sub.MonthlyUsageUSD)
	require.Equal(t, 0, subRepo.createCalls)
}

func TestAssignSubscriptionRenewsExpiredSemanticMatch(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	oldStart := time.Now().Add(-time.Hour)
	oldWindowStart := startOfDay(oldStart)
	subRepo.seed(&UserSubscription{
		ID:                 12,
		UserID:             1002,
		GroupID:            1,
		StartsAt:           oldStart,
		ExpiresAt:          oldStart.AddDate(0, 0, 30),
		Status:             SubscriptionStatusExpired,
		DailyWindowStart:   &oldWindowStart,
		WeeklyWindowStart:  &oldWindowStart,
		MonthlyWindowStart: &oldWindowStart,
		DailyUsageUSD:      1,
		WeeklyUsageUSD:     2,
		MonthlyUsageUSD:    3,
		Notes:              " assignment ",
	})

	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	before := time.Now()
	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       1002,
		GroupID:      1,
		ValidityDays: 30,
		Notes:        "assignment",
	})
	after := time.Now()

	require.NoError(t, err)
	require.Equal(t, int64(12), sub.ID)
	require.Equal(t, 0, subRepo.createCalls)
	require.Equal(t, SubscriptionStatusActive, sub.Status)
	require.False(t, sub.StartsAt.Before(before))
	require.False(t, sub.StartsAt.After(after))
	require.Equal(t, sub.StartsAt.AddDate(0, 0, 30), sub.ExpiresAt)
	require.Equal(t, timezone.StartOfDay(sub.StartsAt), *sub.DailyWindowStart, "续期后日窗口应锚定当天 0 点")
	require.Equal(t, sub.StartsAt, *sub.WeeklyWindowStart)
	require.Equal(t, sub.StartsAt, *sub.MonthlyWindowStart)
	require.Zero(t, sub.DailyUsageUSD)
	require.Zero(t, sub.WeeklyUsageUSD)
	require.Zero(t, sub.MonthlyUsageUSD)
	require.Equal(t, " assignment ", sub.Notes)
}

func TestAssignSubscriptionRenewsExpiredAndAppendsDifferentNotes(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	oldStart := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	subRepo.seed(&UserSubscription{
		ID:        14,
		UserID:    1004,
		GroupID:   1,
		StartsAt:  oldStart,
		ExpiresAt: oldStart.AddDate(0, 0, 30),
		Status:    SubscriptionStatusExpired,
		Notes:     "old assignment",
	})

	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       1004,
		GroupID:      1,
		ValidityDays: 30,
		Notes:        "new assignment",
	})

	require.NoError(t, err)
	require.Equal(t, "old assignment\nnew assignment", sub.Notes)
}

func TestAssignSubscriptionConflictWhenSemanticsMismatch(t *testing.T) {
	start := time.Now().Add(-time.Hour)
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
		Status:    SubscriptionStatusActive,
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
	start := time.Now().Add(-time.Hour)
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
		Status:    SubscriptionStatusActive,
		Notes:     "same-note",
	})
	// user 3: 语义冲突（有效期不一致），应 failed
	subRepo.seed(&UserSubscription{
		ID:        23,
		UserID:    3,
		GroupID:   1,
		StartsAt:  start,
		ExpiresAt: start.AddDate(0, 0, 60),
		Status:    SubscriptionStatusActive,
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

func TestBulkAssignSubscriptionRenewsExpiredSemanticMatch(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	oldStart := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	subRepo.seed(&UserSubscription{
		ID:              24,
		UserID:          4,
		GroupID:         1,
		StartsAt:        oldStart,
		ExpiresAt:       oldStart.AddDate(0, 0, 7),
		Status:          SubscriptionStatusExpired,
		DailyUsageUSD:   1,
		WeeklyUsageUSD:  2,
		MonthlyUsageUSD: 3,
		Notes:           "bulk",
	})

	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	before := time.Now()
	result, err := svc.BulkAssignSubscription(context.Background(), &BulkAssignSubscriptionInput{
		UserIDs:      []int64{4},
		GroupID:      1,
		ValidityDays: 7,
		Notes:        "bulk",
	})
	after := time.Now()

	require.NoError(t, err)
	require.Equal(t, 1, result.SuccessCount)
	require.Equal(t, 0, result.CreatedCount)
	require.Equal(t, 1, result.ReusedCount)
	require.Equal(t, "reused", result.Statuses[4])
	require.Len(t, result.Subscriptions, 1)
	renewed := result.Subscriptions[0]
	require.Equal(t, SubscriptionStatusActive, renewed.Status)
	require.False(t, renewed.StartsAt.Before(before))
	require.False(t, renewed.StartsAt.After(after))
	require.Equal(t, renewed.StartsAt.AddDate(0, 0, 7), renewed.ExpiresAt)
	require.Zero(t, renewed.DailyUsageUSD)
	require.Zero(t, renewed.WeeklyUsageUSD)
	require.Zero(t, renewed.MonthlyUsageUSD)
	require.Equal(t, "bulk", renewed.Notes)
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

func TestAssignSubscriptionV2PlanUsesLegacyDefaultValidityForBothAliasRowsWhenOverrideOmitted(t *testing.T) {
	now := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	plan := testEntitlementPlan(77, []int64{1, 2}, nil)
	plan.ValidityDays = 2
	plan.ValidityUnit = validityUnitWeek
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{77: plan}}
	svc := newAssignSubscriptionEntitlementTestService(true, entRepo, planRepo)

	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:     3012,
		GroupID:    1,
		PlanID:     77,
		AssignedBy: 9,
		Notes:      "admin-plan-default-validity",
	})

	require.NoError(t, err)
	require.Equal(t, sub.StartsAt.AddDate(0, 0, 30), sub.ExpiresAt)
	ent := requireTestEntitlementByLegacy(t, entRepo, sub.ID, 77)
	require.Equal(t, ent.StartsAt.AddDate(0, 0, 30), ent.ExpiresAt)
	require.WithinDuration(t, sub.ExpiresAt, ent.ExpiresAt, time.Second)
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

func TestAssignSubscriptionV2RepairsMissingAliasAndInvalidatesCacheAfterCommit(t *testing.T) {
	now := time.Now().UTC()
	startsAt := now.AddDate(0, 0, -10)
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		77: testEntitlementPlan(77, []int64{1, 2}, nil),
	}}
	svc := newAssignSubscriptionEntitlementTestService(true, entRepo, planRepo)
	subRepo := requireSubscriptionUserSubRepoStub(t, svc)
	cache, err := ristretto.NewCache(&ristretto.Config{NumCounters: 1_000, MaxCost: 100, BufferItems: 64})
	require.NoError(t, err)
	t.Cleanup(cache.Close)
	svc.subCacheL1 = cache

	input := &AssignSubscriptionInput{
		UserID:       3009,
		GroupID:      1,
		PlanID:       77,
		ValidityDays: 30,
		AssignedBy:   9,
		Notes:        "admin-plan",
	}
	existing := &UserSubscription{
		ID:        45,
		UserID:    input.UserID,
		GroupID:   input.GroupID,
		StartsAt:  startsAt,
		ExpiresAt: startsAt.AddDate(0, 0, input.ValidityDays),
		Status:    SubscriptionStatusActive,
		Notes:     input.Notes,
	}
	subRepo.seed(existing)
	cacheKey := subCacheKey(input.UserID, input.GroupID)
	require.True(t, cache.Set(cacheKey, existing, 1))
	cache.Wait()

	reused, err := svc.AssignSubscription(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, existing.ID, reused.ID)
	require.Equal(t, 1, entRepo.createCount)
	repairedEntitlement := requireTestEntitlementByLegacy(t, entRepo, existing.ID, input.PlanID)
	require.Equal(t, existing.StartsAt, repairedEntitlement.StartsAt)
	require.Equal(t, existing.ExpiresAt, repairedEntitlement.ExpiresAt)
	cache.Wait()
	_, cachedAfterCommit := cache.Get(cacheKey)
	require.False(t, cachedAfterCommit)
}

func TestAssignSubscriptionV2ExpiredPlanRenewsLegacyAndEntitlementExactlyOnce(t *testing.T) {
	now := time.Now().UTC()
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		77: testEntitlementPlan(77, []int64{1, 2}, nil),
	}}
	svc := newAssignSubscriptionEntitlementTestService(true, entRepo, planRepo)
	subRepo := requireSubscriptionUserSubRepoStub(t, svc)
	cache, err := ristretto.NewCache(&ristretto.Config{NumCounters: 1_000, MaxCost: 100, BufferItems: 64})
	require.NoError(t, err)
	t.Cleanup(cache.Close)
	svc.subCacheL1 = cache

	input := &AssignSubscriptionInput{
		UserID:       3010,
		GroupID:      1,
		PlanID:       77,
		ValidityDays: 30,
		AssignedBy:   9,
		Notes:        "admin-plan",
	}
	first, err := svc.AssignSubscription(context.Background(), input)
	require.NoError(t, err)
	firstEnt := requireTestEntitlementByLegacy(t, entRepo, first.ID, input.PlanID)

	expiredStart := now.AddDate(0, 0, -60)
	expiredAt := expiredStart.AddDate(0, 0, 30)
	expiredSub := *first
	expiredSub.StartsAt = expiredStart
	expiredSub.ExpiresAt = expiredAt
	expiredSub.Status = SubscriptionStatusExpired
	planID := input.PlanID
	expiredSub.EntitlementLink = &UserSubscriptionEntitlementLink{
		EntitlementID: firstEnt.ID,
		PlanID:        &planID,
		Status:        SubscriptionStatusExpired,
		ExpiresAt:     expiredAt,
	}
	subRepo.seed(&expiredSub)

	entRepo.mu.Lock()
	storedEnt := entRepo.entitlements[firstEnt.ID]
	storedEnt.StartsAt = expiredStart
	storedEnt.ExpiresAt = expiredAt
	storedEnt.Status = SubscriptionStatusExpired
	storedEnt.DailyUsageUSD = 1
	storedEnt.WeeklyUsageUSD = 2
	storedEnt.MonthlyUsageUSD = 3
	entRepo.mu.Unlock()

	cacheKey := subCacheKey(input.UserID, input.GroupID)
	require.True(t, cache.Set(cacheKey, &expiredSub, 1))
	cache.Wait()
	cachePresentDuringEntitlementExtension := false
	entRepo.beforeExtend = func() {
		_, cachePresentDuringEntitlementExtension = cache.Get(cacheKey)
	}

	renewed, err := svc.AssignSubscription(context.Background(), input)
	require.NoError(t, err)
	require.True(t, cachePresentDuringEntitlementExtension, "cache invalidation must wait until the alias transaction completes")
	cache.Wait()
	_, cachedAfterCommit := cache.Get(cacheKey)
	require.False(t, cachedAfterCommit)

	renewedEnt := requireTestEntitlementByLegacy(t, entRepo, renewed.ID, input.PlanID)
	require.Equal(t, renewed.StartsAt, renewedEnt.StartsAt)
	require.Equal(t, renewed.ExpiresAt, renewedEnt.ExpiresAt)
	require.Equal(t, SubscriptionStatusActive, renewedEnt.Status)
	require.Zero(t, renewedEnt.DailyUsageUSD)
	require.Zero(t, renewedEnt.WeeklyUsageUSD)
	require.Zero(t, renewedEnt.MonthlyUsageUSD)
	require.Equal(t, 1, entRepo.createCount)
	require.Equal(t, 1, entRepo.updateTermCount)
	require.Equal(t, 2, entRepo.eventCount)
	require.NotNil(t, renewedEnt.SourceExternalID)
	require.Equal(t, adminAssignEntitlementRenewalSourceExternalID(renewed.ID, input.PlanID, renewed.StartsAt), *renewedEnt.SourceExternalID)

	replayed, err := svc.AssignSubscription(context.Background(), input)
	require.NoError(t, err)
	replayedEnt := requireTestEntitlementByLegacy(t, entRepo, replayed.ID, input.PlanID)
	require.Equal(t, renewed.StartsAt, replayed.StartsAt)
	require.Equal(t, renewed.ExpiresAt, replayed.ExpiresAt)
	require.Equal(t, renewedEnt.ExpiresAt, replayedEnt.ExpiresAt)
	require.Equal(t, 1, entRepo.updateTermCount)
	require.Equal(t, 2, entRepo.eventCount)
}

func TestAssignSubscriptionV2ExpiredLegacyDoesNotExtendActiveEntitlement(t *testing.T) {
	now := time.Now().UTC()
	entRepo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		77: testEntitlementPlan(77, []int64{1, 2}, nil),
	}}
	svc := newAssignSubscriptionEntitlementTestService(true, entRepo, planRepo)
	subRepo := requireSubscriptionUserSubRepoStub(t, svc)
	input := &AssignSubscriptionInput{
		UserID:       3011,
		GroupID:      1,
		PlanID:       77,
		ValidityDays: 30,
		AssignedBy:   9,
		Notes:        "admin-plan",
	}
	first, err := svc.AssignSubscription(context.Background(), input)
	require.NoError(t, err)
	firstEnt := requireTestEntitlementByLegacy(t, entRepo, first.ID, input.PlanID)
	originalEntitlementExpiry := firstEnt.ExpiresAt

	expiredSub := *first
	expiredSub.StartsAt = now.AddDate(0, 0, -60)
	expiredSub.ExpiresAt = expiredSub.StartsAt.AddDate(0, 0, 30)
	expiredSub.Status = SubscriptionStatusExpired
	planID := input.PlanID
	expiredSub.EntitlementLink = &UserSubscriptionEntitlementLink{
		EntitlementID: firstEnt.ID,
		PlanID:        &planID,
		Status:        SubscriptionStatusActive,
		ExpiresAt:     originalEntitlementExpiry,
	}
	subRepo.seed(&expiredSub)

	// Legacy-migrated aliases do not have the admin-assignment fulfillment.
	// A replay must still treat the active linked entitlement as already granted.
	entRepo.mu.Lock()
	storedEnt, ok := entRepo.entitlements[firstEnt.ID]
	if ok {
		storedEnt.SourceType = "legacy_migration"
		storedEnt.SourceExternalID = nil
	}
	entRepo.fulfillments = make(map[int64]*SubscriptionEntitlementFulfillment)
	entRepo.mu.Unlock()
	require.True(t, ok)

	renewed, err := svc.AssignSubscription(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusActive, renewed.Status)
	renewedEnt := requireTestEntitlementByLegacy(t, entRepo, renewed.ID, input.PlanID)
	require.Equal(t, originalEntitlementExpiry, renewedEnt.ExpiresAt)
	require.Equal(t, 0, entRepo.updateTermCount)
	require.Equal(t, 1, entRepo.eventCount)
}

func TestAssignSubscriptionV2PlanFallbacksToEntitlementOnlyWhenExistingLegacyBelongsToDifferentPlan(t *testing.T) {
	now := time.Now()
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

func TestAssignSubscriptionV2PlanFallbacksToEntitlementOnlyWhenExpiredLegacyPlanIsUnknown(t *testing.T) {
	now := time.Now()
	legacyStart := now.AddDate(0, 0, -31)
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
		StartsAt:  legacyStart,
		ExpiresAt: legacyStart.AddDate(0, 0, 30),
		Status:    SubscriptionStatusExpired,
		Notes:     "admin-plan",
		EntitlementLink: &UserSubscriptionEntitlementLink{
			EntitlementID:  92,
			Status:         SubscriptionStatusActive,
			ExpiresAt:      legacyStart.AddDate(0, 0, 30),
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
	legacy, err := subRepo.GetByID(context.Background(), 44)
	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusExpired, legacy.Status)
	require.Equal(t, legacyStart, legacy.StartsAt)
	require.Equal(t, legacyStart.AddDate(0, 0, 30), legacy.ExpiresAt)
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
