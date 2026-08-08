//go:build integration

package repository

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlement"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlementfulfillment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type entitlementAdminPurchaseRaceRepository struct {
	service.SubscriptionEntitlementRepository

	adminReadReady chan struct{}
	purchaseReady  chan struct{}
	release        chan struct{}
	firstGet       atomic.Bool
	listOnce       sync.Once
}

type entitlementRestoreReadBarrierRepository struct {
	service.UserSubscriptionRepository

	firstRead atomic.Bool
	readReady chan struct{}
	release   chan struct{}
}

type entitlementRestoreAliasLockBarrierRepository struct {
	service.UserSubscriptionRepository

	lockReady chan struct{}
	release   chan struct{}
	once      sync.Once
}

func newEntitlementRestoreAliasLockBarrierRepository(repo service.UserSubscriptionRepository) *entitlementRestoreAliasLockBarrierRepository {
	return &entitlementRestoreAliasLockBarrierRepository{
		UserSubscriptionRepository: repo,
		lockReady:                  make(chan struct{}),
		release:                    make(chan struct{}),
	}
}

func (r *entitlementRestoreAliasLockBarrierRepository) GetByIDIncludeDeletedForUpdate(ctx context.Context, id int64) (*service.UserSubscription, error) {
	sub, err := r.UserSubscriptionRepository.GetByIDIncludeDeletedForUpdate(ctx, id)
	if err == nil {
		r.once.Do(func() {
			close(r.lockReady)
			select {
			case <-r.release:
			case <-ctx.Done():
			}
		})
	}
	return sub, err
}

func newEntitlementRestoreReadBarrierRepository(repo service.UserSubscriptionRepository) *entitlementRestoreReadBarrierRepository {
	return &entitlementRestoreReadBarrierRepository{
		UserSubscriptionRepository: repo,
		readReady:                  make(chan struct{}),
		release:                    make(chan struct{}),
	}
}

func (r *entitlementRestoreReadBarrierRepository) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.UserSubscription, error) {
	sub, err := r.UserSubscriptionRepository.GetByIDIncludeDeleted(ctx, id)
	if err == nil && r.firstRead.CompareAndSwap(false, true) {
		close(r.readReady)
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return sub, err
}

func newEntitlementAdminPurchaseRaceRepository(repo service.SubscriptionEntitlementRepository) *entitlementAdminPurchaseRaceRepository {
	return &entitlementAdminPurchaseRaceRepository{
		SubscriptionEntitlementRepository: repo,
		adminReadReady:                    make(chan struct{}),
		purchaseReady:                     make(chan struct{}),
		release:                           make(chan struct{}),
	}
}

func (r *entitlementAdminPurchaseRaceRepository) GetByID(ctx context.Context, id int64) (*service.SubscriptionEntitlement, error) {
	ent, err := r.SubscriptionEntitlementRepository.GetByID(ctx, id)
	if err == nil && r.firstGet.CompareAndSwap(false, true) {
		close(r.adminReadReady)
		<-r.release
	}
	return ent, err
}

func (r *entitlementAdminPurchaseRaceRepository) ListByUserPlanID(ctx context.Context, userID, planID int64) ([]service.SubscriptionEntitlement, error) {
	r.waitForPurchaseRelease()
	return r.SubscriptionEntitlementRepository.ListByUserPlanID(ctx, userID, planID)
}

func (r *entitlementAdminPurchaseRaceRepository) ListByUserPlanIDForUpdate(ctx context.Context, userID, planID int64) ([]service.SubscriptionEntitlement, error) {
	r.waitForPurchaseRelease()
	return r.SubscriptionEntitlementRepository.ListByUserPlanIDForUpdate(ctx, userID, planID)
}

func (r *entitlementAdminPurchaseRaceRepository) CompareAndSwapTerm(
	ctx context.Context,
	id int64,
	expectedUpdatedAt time.Time,
	startsAt time.Time,
	expiresAt time.Time,
	status string,
	notes string,
) (time.Time, bool, error) {
	repo, ok := r.SubscriptionEntitlementRepository.(interface {
		CompareAndSwapTerm(context.Context, int64, time.Time, time.Time, time.Time, string, string) (time.Time, bool, error)
	})
	if !ok {
		return time.Time{}, false, service.ErrSubscriptionEntitlementTermConflict
	}
	return repo.CompareAndSwapTerm(ctx, id, expectedUpdatedAt, startsAt, expiresAt, status, notes)
}

func (r *entitlementAdminPurchaseRaceRepository) waitForPurchaseRelease() {
	r.listOnce.Do(func() {
		close(r.purchaseReady)
		<-r.release
	})
}

func TestSubscriptionEntitlementConcurrentFirstPurchasesCreateOneLedger(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, userID, planID, _ := newEntitlementConcurrencyFixture(t)
	baseRepo := NewSubscriptionEntitlementRepository(client)
	svc := service.NewSubscriptionEntitlementService(baseRepo, NewSubscriptionEntitlementPlanRepository(client))
	now := time.Now().UTC().Truncate(time.Second)

	results, errs := runConcurrentEntitlementAssignments(ctx, svc,
		service.AssignEntitlementFromPlanInput{UserID: userID, PlanID: planID, OrderID: 71001, Now: now},
		service.AssignEntitlementFromPlanInput{UserID: userID, PlanID: planID, OrderID: 71002, Now: now},
	)
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.NotNil(t, results[0])
	require.NotNil(t, results[1])
	require.Equal(t, results[0].ID, results[1].ID)

	ents, err := baseRepo.ListByUserPlanID(ctx, userID, planID)
	require.NoError(t, err)
	require.Len(t, ents, 1)
	require.Equal(t, now.AddDate(0, 0, 60), ents[0].ExpiresAt)
	require.Equal(t, 2, countEntitlementFulfillments(t, ctx, client, ents[0].ID))
}

func TestSubscriptionEntitlementConcurrentRenewalsAccumulateBothTerms(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, userID, planID, _ := newEntitlementConcurrencyFixture(t)
	baseRepo := NewSubscriptionEntitlementRepository(client)
	planRepo := NewSubscriptionEntitlementPlanRepository(client)
	now := time.Now().UTC().Truncate(time.Second)
	seedSvc := service.NewSubscriptionEntitlementService(baseRepo, planRepo)
	seed, _, err := seedSvc.AssignOrExtendFromPlan(ctx, service.AssignEntitlementFromPlanInput{
		UserID: userID, PlanID: planID, OrderID: 72000, Now: now,
	})
	require.NoError(t, err)

	svc := service.NewSubscriptionEntitlementService(baseRepo, planRepo)
	results, errs := runConcurrentEntitlementAssignments(ctx, svc,
		service.AssignEntitlementFromPlanInput{UserID: userID, PlanID: planID, OrderID: 72001, Now: now},
		service.AssignEntitlementFromPlanInput{UserID: userID, PlanID: planID, OrderID: 72002, Now: now},
	)
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.Equal(t, seed.ID, results[0].ID)
	require.Equal(t, seed.ID, results[1].ID)

	got, err := baseRepo.GetByID(ctx, seed.ID)
	require.NoError(t, err)
	require.Equal(t, now.AddDate(0, 0, 90), got.ExpiresAt)
	require.Equal(t, 3, countEntitlementFulfillments(t, ctx, client, seed.ID))
}

func TestSubscriptionEntitlementConcurrentSourceReplayGrantsOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, userID, planID, _ := newEntitlementConcurrencyFixture(t)
	repo := NewSubscriptionEntitlementRepository(client)
	svc := service.NewSubscriptionEntitlementService(repo, NewSubscriptionEntitlementPlanRepository(client))
	now := time.Now().UTC().Truncate(time.Second)
	input := service.AssignEntitlementFromPlanInput{UserID: userID, PlanID: planID, OrderID: 73001, Now: now}

	results, errs := runConcurrentEntitlementAssignments(ctx, svc, input, input)
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.Equal(t, results[0].ID, results[1].ID)

	got, err := repo.GetByID(ctx, results[0].ID)
	require.NoError(t, err)
	require.Equal(t, now.AddDate(0, 0, 30), got.ExpiresAt)
	require.Equal(t, 1, countEntitlementFulfillments(t, ctx, client, got.ID))
}

func TestSubscriptionEntitlementMutationLockDoesNotDeadlockBalanceFallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, userID, planID, _ := newEntitlementConcurrencyFixture(t)
	repo := NewSubscriptionEntitlementRepository(client)
	svc := service.NewSubscriptionEntitlementService(repo, NewSubscriptionEntitlementPlanRepository(client))
	now := time.Now().UTC().Truncate(time.Second)
	entitlement, _, err := svc.AssignOrExtendFromPlan(ctx, service.AssignEntitlementFromPlanInput{
		UserID: userID, PlanID: planID, OrderID: 73501, Now: now,
	})
	require.NoError(t, err)
	require.NotNil(t, entitlement)
	_, err = integrationDB.ExecContext(ctx, "UPDATE users SET balance = 100 WHERE id = $1", userID)
	require.NoError(t, err)

	billingTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = billingTx.Rollback() }()
	_, err = lockUsageBillingEntitlement(ctx, billingTx, entitlement.ID)
	require.NoError(t, err)

	mutationEntered := make(chan struct{})
	mutationDone := make(chan error, 1)
	go func() {
		mutationDone <- repo.WithUserEntitlementMutationTx(ctx, userID, func(txCtx context.Context) error {
			close(mutationEntered)
			_, lockErr := repo.GetByIDForUpdate(txCtx, entitlement.ID)
			return lockErr
		})
	}()
	select {
	case <-mutationEntered:
	case <-ctx.Done():
		t.Fatal("entitlement mutation did not acquire its user mutex")
	}

	newBalance, err := deductUsageBillingBalanceStrict(ctx, billingTx, userID, 1)
	require.NoError(t, err)
	require.InDelta(t, 99, newBalance, 0.000001)
	require.NoError(t, billingTx.Commit())
	require.NoError(t, <-mutationDone)
}

func TestSubscriptionAliasAssignmentAcquiresUserMutexBeforeLegacyRowLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, userID, planID, groupID := newEntitlementConcurrencyFixture(t)
	userSubRepo := NewUserSubscriptionRepository(client)
	now := time.Now().UTC().Truncate(time.Second)
	legacySub := &service.UserSubscription{
		UserID:     userID,
		GroupID:    groupID,
		StartsAt:   now.AddDate(0, 0, -40),
		ExpiresAt:  now.AddDate(0, 0, -10),
		Status:     service.SubscriptionStatusExpired,
		AssignedAt: now.AddDate(0, 0, -40),
	}
	require.NoError(t, userSubRepo.Create(ctx, legacySub))

	entitlementRepo := NewSubscriptionEntitlementRepository(client)
	entitlementSvc := service.NewSubscriptionEntitlementService(entitlementRepo, NewSubscriptionEntitlementPlanRepository(client))
	subscriptionSvc := service.NewSubscriptionService(NewGroupRepository(client, integrationDB), userSubRepo, nil, client, nil)
	t.Cleanup(subscriptionSvc.Stop)
	subscriptionSvc.SetSubscriptionEntitlementAliasDependencies(enabledSubscriptionEntitlementsRuntime{}, entitlementSvc)

	mutexKey := subscriptionEntitlementUserMutationLockKey(userID)
	blockerTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = blockerTx.Rollback() }()
	rows, err := blockerTx.QueryContext(ctx, "SELECT pg_advisory_xact_lock($1)", mutexKey)
	require.NoError(t, err)
	require.NoError(t, rows.Close())

	assignDone := make(chan error, 1)
	go func() {
		_, assignErr := subscriptionSvc.AssignSubscription(ctx, &service.AssignSubscriptionInput{
			UserID:       userID,
			GroupID:      groupID,
			PlanID:       planID,
			ValidityDays: 30,
		})
		assignDone <- assignErr
	}()

	lockBits := uint64(mutexKey)
	classID := int64(uint32(lockBits >> 32))
	objectID := int64(uint32(lockBits))
	require.Eventually(t, func() bool {
		var waiting bool
		err := integrationDB.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_locks
				WHERE locktype = 'advisory'
					AND classid::bigint = $1
					AND objid::bigint = $2
					AND objsubid = 1
					AND NOT granted
			)
		`, classID, objectID).Scan(&waiting)
		return err == nil && waiting
	}, 5*time.Second, 20*time.Millisecond, "alias assignment never waited on the user advisory mutex")

	probeTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	var lockedAliasID int64
	err = probeTx.QueryRowContext(ctx, `
		SELECT id
		FROM user_subscriptions
		WHERE id = $1
		FOR UPDATE NOWAIT
	`, legacySub.ID).Scan(&lockedAliasID)
	require.NoError(t, err, "alias row was locked before the user advisory mutex")
	require.Equal(t, legacySub.ID, lockedAliasID)
	require.NoError(t, probeTx.Rollback())

	require.NoError(t, blockerTx.Commit())
	require.NoError(t, <-assignDone)
}

func TestSubscriptionEntitlementAdminAdjustmentSerializesWithPurchase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, userID, planID, _ := newEntitlementConcurrencyFixture(t)
	baseRepo := NewSubscriptionEntitlementRepository(client)
	planRepo := NewSubscriptionEntitlementPlanRepository(client)
	now := time.Now().UTC().Truncate(time.Second)
	seedSvc := service.NewSubscriptionEntitlementService(baseRepo, planRepo)
	seed, _, err := seedSvc.AssignOrExtendFromPlan(ctx, service.AssignEntitlementFromPlanInput{
		UserID: userID, PlanID: planID, OrderID: 74000, Now: now,
	})
	require.NoError(t, err)

	raceRepo := newEntitlementAdminPurchaseRaceRepository(baseRepo)
	entitlementSvc := service.NewSubscriptionEntitlementService(raceRepo, planRepo)
	subscriptionSvc := service.NewSubscriptionService(nil, nil, nil, client, nil)
	subscriptionSvc.SetSubscriptionEntitlementAliasDependencies(enabledSubscriptionEntitlementsRuntime{}, entitlementSvc)
	errCh := make(chan error, 2)
	go func() {
		_, adjustErr := subscriptionSvc.ExtendSubscription(ctx, -seed.ID, 5)
		errCh <- adjustErr
	}()
	select {
	case <-raceRepo.adminReadReady:
	case <-ctx.Done():
		t.Fatal("admin adjustment did not reach its initial read")
	}
	go func() {
		_, _, purchaseErr := entitlementSvc.AssignOrExtendFromPlan(ctx, service.AssignEntitlementFromPlanInput{
			UserID: userID, PlanID: planID, OrderID: 74001, Now: now,
		})
		errCh <- purchaseErr
	}()
	select {
	case <-raceRepo.purchaseReady:
	case <-ctx.Done():
		t.Fatal("purchase did not reach the entitlement lookup")
	}
	close(raceRepo.release)
	require.NoError(t, <-errCh)
	require.NoError(t, <-errCh)

	got, err := baseRepo.GetByID(ctx, seed.ID)
	require.NoError(t, err)
	require.Equal(t, now.AddDate(0, 0, 65), got.ExpiresAt)
}

func TestSubscriptionEntitlementRefundSerializesWithPurchase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, userID, planID, _ := newEntitlementConcurrencyFixture(t)
	baseRepo := NewSubscriptionEntitlementRepository(client)
	planRepo := NewSubscriptionEntitlementPlanRepository(client)
	now := time.Now().UTC().Truncate(time.Second)
	seedSvc := service.NewSubscriptionEntitlementService(baseRepo, planRepo)
	seed, _, err := seedSvc.AssignOrExtendFromPlan(ctx, service.AssignEntitlementFromPlanInput{
		UserID: userID, PlanID: planID, OrderID: 76000, Now: now,
	})
	require.NoError(t, err)

	raceRepo := newEntitlementAdminPurchaseRaceRepository(baseRepo)
	svc := service.NewSubscriptionEntitlementService(raceRepo, planRepo)
	errCh := make(chan error, 2)
	go func() {
		_, refundErr := svc.ShortenForRefund(ctx, seed.ID, 5, now)
		errCh <- refundErr
	}()
	select {
	case <-raceRepo.adminReadReady:
	case <-ctx.Done():
		t.Fatal("refund did not reach its initial read")
	}
	go func() {
		_, _, purchaseErr := svc.AssignOrExtendFromPlan(ctx, service.AssignEntitlementFromPlanInput{
			UserID: userID, PlanID: planID, OrderID: 76001, Now: now,
		})
		errCh <- purchaseErr
	}()
	select {
	case <-raceRepo.purchaseReady:
	case <-ctx.Done():
		t.Fatal("purchase did not reach the entitlement lookup")
	}
	close(raceRepo.release)
	require.NoError(t, <-errCh)
	require.NoError(t, <-errCh)

	got, err := baseRepo.GetByID(ctx, seed.ID)
	require.NoError(t, err)
	require.Equal(t, now.AddDate(0, 0, 55), got.ExpiresAt)
}

func TestSubscriptionEntitlementLinkedRestorePreservesConcurrentPurchaseTerm(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, userID, planID, groupID := newEntitlementConcurrencyFixture(t)
	entitlementRepo := NewSubscriptionEntitlementRepository(client)
	planRepo := NewSubscriptionEntitlementPlanRepository(client)
	entitlementSvc := service.NewSubscriptionEntitlementService(entitlementRepo, planRepo)
	userSubRepo := NewUserSubscriptionRepository(client)
	entitlementSvc.SetLegacySubscriptionRepository(userSubRepo)
	now := time.Now().UTC().Truncate(time.Second)
	legacySub := &service.UserSubscription{
		UserID:     userID,
		GroupID:    groupID,
		StartsAt:   now.AddDate(0, 0, -30),
		ExpiresAt:  now.AddDate(0, 0, 1),
		Status:     service.SubscriptionStatusActive,
		AssignedAt: now.AddDate(0, 0, -30),
	}
	require.NoError(t, userSubRepo.Create(ctx, legacySub))
	legacyID := legacySub.ID
	seedSource := uniqueTestName("linked-restore-seed")
	seed, _, err := entitlementSvc.AssignOrExtendFromPlan(ctx, service.AssignEntitlementFromPlanInput{
		UserID:               userID,
		PlanID:               planID,
		LegacySubscriptionID: &legacyID,
		SourceType:           service.SubscriptionEntitlementSourceAdminAssign,
		SourceExternalID:     &seedSource,
		ValidityDaysOverride: 31,
		Now:                  legacySub.StartsAt,
	})
	require.NoError(t, err)

	subscriptionSvc := service.NewSubscriptionService(nil, userSubRepo, nil, client, nil)
	t.Cleanup(subscriptionSvc.Stop)
	subscriptionSvc.SetSubscriptionEntitlementAliasDependencies(enabledSubscriptionEntitlementsRuntime{}, entitlementSvc)
	require.NoError(t, subscriptionSvc.RevokeSubscription(ctx, legacySub.ID))
	revoked, err := entitlementRepo.GetByID(ctx, seed.ID)
	require.NoError(t, err)
	require.Equal(t, service.SubscriptionStatusRevoked, revoked.Status)

	barrierRepo := newEntitlementRestoreReadBarrierRepository(userSubRepo)
	restoreSvc := service.NewSubscriptionService(nil, barrierRepo, nil, client, nil)
	t.Cleanup(restoreSvc.Stop)
	restoreSvc.SetSubscriptionEntitlementAliasDependencies(enabledSubscriptionEntitlementsRuntime{}, entitlementSvc)
	restoreDone := make(chan error, 1)
	go func() {
		_, restoreErr := restoreSvc.RestoreSubscription(ctx, legacySub.ID)
		restoreDone <- restoreErr
	}()
	select {
	case <-barrierRepo.readReady:
	case <-ctx.Done():
		t.Fatal("linked restore did not read its deleted alias")
	}

	purchased, _, err := entitlementSvc.AssignOrExtendFromPlan(ctx, service.AssignEntitlementFromPlanInput{
		UserID:  userID,
		PlanID:  planID,
		OrderID: 76501,
		Now:     time.Now().UTC().Add(time.Second).Truncate(time.Second),
	})
	require.NoError(t, err)
	require.Equal(t, seed.ID, purchased.ID)
	close(barrierRepo.release)
	require.NoError(t, <-restoreDone)

	got, err := entitlementRepo.GetByID(ctx, seed.ID)
	require.NoError(t, err)
	require.Equal(t, service.SubscriptionStatusActive, got.Status)
	require.Equal(t, purchased.StartsAt, got.StartsAt)
	require.Equal(t, purchased.ExpiresAt, got.ExpiresAt)
	restoredAlias, err := userSubRepo.GetByID(ctx, legacySub.ID)
	require.NoError(t, err)
	require.Equal(t, got.Status, restoredAlias.Status)
	require.Equal(t, got.StartsAt, restoredAlias.StartsAt)
	require.Equal(t, got.ExpiresAt, restoredAlias.ExpiresAt)
	require.Equal(t, got.DailyWindowStart, restoredAlias.DailyWindowStart)
	require.Equal(t, got.WeeklyWindowStart, restoredAlias.WeeklyWindowStart)
	require.Equal(t, got.MonthlyWindowStart, restoredAlias.MonthlyWindowStart)
	require.Equal(t, got.DailyUsageUSD, restoredAlias.DailyUsageUSD)
	require.Equal(t, got.WeeklyUsageUSD, restoredAlias.WeeklyUsageUSD)
	require.Equal(t, got.MonthlyUsageUSD, restoredAlias.MonthlyUsageUSD)
	require.Equal(t, 2, countEntitlementFulfillments(t, ctx, client, got.ID))
}

func TestSubscriptionEntitlementLinkedRestoreLocksDeletedAliasRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, userID, planID, groupID := newEntitlementConcurrencyFixture(t)
	entitlementRepo := NewSubscriptionEntitlementRepository(client)
	entitlementSvc := service.NewSubscriptionEntitlementService(entitlementRepo, NewSubscriptionEntitlementPlanRepository(client))
	userSubRepo := NewUserSubscriptionRepository(client)
	entitlementSvc.SetLegacySubscriptionRepository(userSubRepo)
	now := time.Now().UTC().Truncate(time.Second)
	legacySub := &service.UserSubscription{
		UserID:     userID,
		GroupID:    groupID,
		StartsAt:   now.AddDate(0, 0, -30),
		ExpiresAt:  now.AddDate(0, 0, 1),
		Status:     service.SubscriptionStatusActive,
		AssignedAt: now.AddDate(0, 0, -30),
	}
	require.NoError(t, userSubRepo.Create(ctx, legacySub))
	legacyID := legacySub.ID
	source := uniqueTestName("linked-restore-row-lock")
	_, _, err := entitlementSvc.AssignOrExtendFromPlan(ctx, service.AssignEntitlementFromPlanInput{
		UserID:               userID,
		PlanID:               planID,
		LegacySubscriptionID: &legacyID,
		SourceType:           service.SubscriptionEntitlementSourceAdminAssign,
		SourceExternalID:     &source,
		ValidityDaysOverride: 31,
		Now:                  legacySub.StartsAt,
	})
	require.NoError(t, err)

	subscriptionSvc := service.NewSubscriptionService(nil, userSubRepo, nil, client, nil)
	t.Cleanup(subscriptionSvc.Stop)
	subscriptionSvc.SetSubscriptionEntitlementAliasDependencies(enabledSubscriptionEntitlementsRuntime{}, entitlementSvc)
	require.NoError(t, subscriptionSvc.RevokeSubscription(ctx, legacySub.ID))

	barrierRepo := newEntitlementRestoreAliasLockBarrierRepository(userSubRepo)
	restoreSvc := service.NewSubscriptionService(nil, barrierRepo, nil, client, nil)
	t.Cleanup(restoreSvc.Stop)
	restoreSvc.SetSubscriptionEntitlementAliasDependencies(enabledSubscriptionEntitlementsRuntime{}, entitlementSvc)
	released := false
	defer func() {
		if !released {
			close(barrierRepo.release)
		}
	}()
	restoreDone := make(chan error, 1)
	go func() {
		_, restoreErr := restoreSvc.RestoreSubscription(ctx, legacySub.ID)
		restoreDone <- restoreErr
	}()

	select {
	case <-barrierRepo.lockReady:
	case <-ctx.Done():
		t.Fatal("linked restore did not lock its deleted alias")
	}
	probeTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	var lockedAliasID int64
	err = probeTx.QueryRowContext(ctx, `
		SELECT id
		FROM user_subscriptions
		WHERE id = $1
		FOR UPDATE NOWAIT
	`, legacySub.ID).Scan(&lockedAliasID)
	require.Error(t, err, "deleted alias row was not locked during linked restore")
	require.NoError(t, probeTx.Rollback())

	close(barrierRepo.release)
	released = true
	require.NoError(t, <-restoreDone)
	restoredAlias, err := userSubRepo.GetByID(ctx, legacySub.ID)
	require.NoError(t, err)
	require.Nil(t, restoredAlias.DeletedAt)
}

func TestSubscriptionEntitlementAssignmentRollsBackLedgerAndFulfillment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, userID, planID, groupID := newEntitlementConcurrencyFixture(t)
	repo := NewSubscriptionEntitlementRepository(client)
	svc := service.NewSubscriptionEntitlementService(repo, NewSubscriptionEntitlementPlanRepository(client))
	now := time.Now().UTC().Truncate(time.Second)
	missingGroupID := int64(9_000_000_000)
	plan := &service.SubscriptionEntitlementPlan{
		ID: planID, GroupID: groupID, Name: "rollback plan", ValidityDays: 30, ValidityUnit: "day", ForSale: true,
		Groups: []service.SubscriptionEntitlementPlanGroup{
			{GroupID: groupID, Enabled: true, Group: &service.Group{ID: groupID, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeSubscription}},
			{GroupID: missingGroupID, Enabled: true, Group: &service.Group{ID: missingGroupID, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeSubscription}},
		},
	}

	_, _, err := svc.AssignOrExtendFromPlan(ctx, service.AssignEntitlementFromPlanInput{
		UserID: userID, PlanID: planID, OrderID: 75001, PlanSnapshot: plan, Now: now,
	})
	require.Error(t, err)

	entitlementCount, countErr := client.SubscriptionEntitlement.Query().
		Where(subscriptionentitlement.UserIDEQ(userID), subscriptionentitlement.PlanIDEQ(planID)).
		Count(ctx)
	require.NoError(t, countErr)
	require.Zero(t, entitlementCount)
	fulfillmentCount, countErr := client.SubscriptionEntitlementFulfillment.Query().
		Where(subscriptionentitlementfulfillment.UserIDEQ(userID), subscriptionentitlementfulfillment.PlanIDEQ(planID)).
		Count(ctx)
	require.NoError(t, countErr)
	require.Zero(t, fulfillmentCount)
}

func runConcurrentEntitlementAssignments(
	ctx context.Context,
	svc *service.SubscriptionEntitlementService,
	inputs ...service.AssignEntitlementFromPlanInput,
) ([]*service.SubscriptionEntitlement, []error) {
	results := make([]*service.SubscriptionEntitlement, len(inputs))
	errs := make([]error, len(inputs))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range inputs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index], _, errs[index] = svc.AssignOrExtendFromPlan(ctx, inputs[index])
		}(i)
	}
	close(start)
	wg.Wait()
	return results, errs
}

func newEntitlementConcurrencyFixture(t *testing.T) (*dbent.Client, int64, int64, int64) {
	t.Helper()
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{Email: uniqueTestEmail("entitlement-concurrency")})
	group := mustCreateGroup(t, client, &service.Group{
		Name:                uniqueTestName("entitlement-concurrency-group"),
		Platform:            service.PlatformOpenAI,
		SubscriptionType:    service.SubscriptionTypeSubscription,
		SubscriptionEnabled: true,
	})
	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName(uniqueTestName("entitlement-concurrency-plan")).
		SetPrice(29.9).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM subscription_plans WHERE id = $1", plan.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM groups WHERE id = $1", group.ID)
	})
	return client, user.ID, plan.ID, group.ID
}

func countEntitlementFulfillments(t *testing.T, ctx context.Context, client *dbent.Client, entitlementID int64) int {
	t.Helper()
	count, err := client.SubscriptionEntitlementFulfillment.Query().
		Where(subscriptionentitlementfulfillment.EntitlementIDEQ(entitlementID)).
		Count(ctx)
	require.NoError(t, err)
	return count
}
