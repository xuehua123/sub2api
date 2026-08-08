//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlement"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type paymentEntitlementMutationBarrierRepository struct {
	service.SubscriptionEntitlementRepository

	once     sync.Once
	acquired chan int
	release  chan struct{}
}

func newPaymentEntitlementMutationBarrierRepository(repo service.SubscriptionEntitlementRepository) *paymentEntitlementMutationBarrierRepository {
	return &paymentEntitlementMutationBarrierRepository{
		SubscriptionEntitlementRepository: repo,
		acquired:                          make(chan int, 1),
		release:                           make(chan struct{}),
	}
}

func (r *paymentEntitlementMutationBarrierRepository) WithUserEntitlementMutationTx(
	ctx context.Context,
	userID int64,
	fn func(context.Context) error,
) error {
	return r.SubscriptionEntitlementRepository.WithUserEntitlementMutationTx(ctx, userID, func(txCtx context.Context) error {
		first := false
		r.once.Do(func() { first = true })
		if first {
			pid, err := paymentTransactionBackendPID(txCtx)
			if err != nil {
				return err
			}
			r.acquired <- pid
			select {
			case <-r.release:
			case <-txCtx.Done():
				return txCtx.Err()
			}
		}
		return fn(txCtx)
	})
}

func (r *paymentEntitlementMutationBarrierRepository) CompareAndSwapTerm(
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
		return time.Time{}, false, errors.New("payment entitlement repository does not support term compare-and-swap")
	}
	return repo.CompareAndSwapTerm(ctx, id, expectedUpdatedAt, startsAt, expiresAt, status, notes)
}

func TestPaymentV2FulfillmentSerializesExternalRefundBeforeGrantingEntitlement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newPaymentFulfillmentConcurrencyFixture(t)

	legacyRepo := NewUserSubscriptionRepository(fixture.client)
	legacyExpiry := time.Now().UTC().AddDate(0, 0, 45).Truncate(time.Microsecond)
	legacySub := &service.UserSubscription{
		UserID:     fixture.userID,
		GroupID:    fixture.groupID,
		StartsAt:   time.Now().UTC().Add(-24 * time.Hour),
		ExpiresAt:  legacyExpiry,
		Status:     service.SubscriptionStatusActive,
		AssignedAt: time.Now().UTC().Add(-24 * time.Hour),
	}
	require.NoError(t, legacyRepo.Create(ctx, legacySub))

	barrierRepo := newPaymentEntitlementMutationBarrierRepository(NewSubscriptionEntitlementRepository(fixture.client))
	paymentSvc := newPaymentFulfillmentConcurrencyService(t, fixture, barrierRepo, legacyRepo)

	fulfillmentDone := make(chan error, 1)
	go func() {
		fulfillmentDone <- paymentSvc.ExecuteSubscriptionFulfillment(ctx, fixture.order.ID)
	}()
	fulfillmentPID := receivePaymentBarrierPID(t, ctx, barrierRepo.acquired)

	refundDone := make(chan error, 1)
	go func() {
		refundDone <- sendFullExternalSubscriptionRefund(ctx, paymentSvc, fixture.order)
	}()

	waitForPaymentRefundCommitOrBlockedWaiter(t, ctx, fixture.order.ID, fulfillmentPID)
	close(barrierRepo.release)

	fulfillmentErr := receivePaymentConcurrencyResult(t, fulfillmentDone)
	refundErr := receivePaymentConcurrencyResult(t, refundDone)
	require.NoError(t, refundErr)
	require.NotContains(t, strings.ToLower(errorString(fulfillmentErr)), "deadlock detected")

	order, err := fixture.client.PaymentOrder.Get(ctx, fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, service.OrderStatusRefunded, order.Status)
	if order.SubscriptionEntitlementID != nil {
		entitlement, getErr := fixture.client.SubscriptionEntitlement.Get(ctx, *order.SubscriptionEntitlementID)
		require.NoError(t, getErr)
		require.NotEqual(t, service.SubscriptionStatusActive, entitlement.Status)
		require.False(t, entitlement.ExpiresAt.After(time.Now()), "refunded order retained an active entitlement")
	}
	activeCount, err := activePaymentEntitlementCount(ctx, fixture.client, fixture.userID)
	require.NoError(t, err)
	require.Zero(t, activeCount, "refunded order retained active entitlement access")

	legacyAfter, err := legacyRepo.GetByID(ctx, legacySub.ID)
	require.NoError(t, err)
	require.WithinDuration(t, legacyExpiry, legacyAfter.ExpiresAt, time.Microsecond, "V2 refund without an entitlement id deducted an unrelated legacy subscription")
}

func TestPaymentV2FulfillmentRetryAndExternalRefundDoNotDeadlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newPaymentFulfillmentConcurrencyFixture(t)

	baseRepo := NewSubscriptionEntitlementRepository(fixture.client)
	paymentSvc := newPaymentFulfillmentConcurrencyService(t, fixture, baseRepo, nil)
	require.NoError(t, paymentSvc.ExecuteSubscriptionFulfillment(ctx, fixture.order.ID))

	completed, err := fixture.client.PaymentOrder.Get(ctx, fixture.order.ID)
	require.NoError(t, err)
	require.NotNil(t, completed.SubscriptionEntitlementID)
	leaseRetryAt := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)
	_, err = fixture.client.PaymentOrder.UpdateOneID(fixture.order.ID).
		SetStatus(service.OrderStatusFailed).
		SetUpdatedAt(leaseRetryAt).
		ClearCompletedAt().
		Save(ctx)
	require.NoError(t, err)

	blockerTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = blockerTx.Rollback() }()
	var blockerPID int
	require.NoError(t, blockerTx.QueryRowContext(ctx, `
		SELECT pg_backend_pid()
		FROM payment_orders
		WHERE id = $1
		FOR UPDATE
	`, fixture.order.ID).Scan(&blockerPID))

	paymentSvc = newPaymentFulfillmentConcurrencyService(t, fixture, baseRepo, nil)
	refundDone := make(chan error, 1)
	go func() {
		refundDone <- sendFullExternalSubscriptionRefund(ctx, paymentSvc, fixture.order)
	}()
	require.Eventually(t, func() bool {
		return paymentWaiterCountBlockedByPID(ctx, blockerPID) >= 1
	}, 5*time.Second, 20*time.Millisecond, "external refund did not wait on the held order lock")

	// A retry may finish from its existing fulfillment source without locking the
	// order again. Start it while the refund is definitely blocked so the test
	// exercises the concurrency contract without requiring a non-contractual lock.
	fulfillmentDone := make(chan error, 1)
	go func() {
		fulfillmentDone <- paymentSvc.ExecuteSubscriptionFulfillment(ctx, fixture.order.ID)
	}()
	require.NoError(t, blockerTx.Commit())

	fulfillmentErr := receivePaymentConcurrencyResult(t, fulfillmentDone)
	refundErr := receivePaymentConcurrencyResult(t, refundDone)
	require.NotContains(t, strings.ToLower(errorString(fulfillmentErr)), "deadlock detected")
	require.NotContains(t, strings.ToLower(errorString(refundErr)), "deadlock detected")
	require.NoError(t, refundErr)

	order, err := fixture.client.PaymentOrder.Get(ctx, fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, service.OrderStatusRefunded, order.Status)
	require.NotNil(t, order.SubscriptionEntitlementID)
	entitlement, err := fixture.client.SubscriptionEntitlement.Get(ctx, *order.SubscriptionEntitlementID)
	require.NoError(t, err)
	require.NotEqual(t, service.SubscriptionStatusActive, entitlement.Status)
	require.False(t, entitlement.ExpiresAt.After(time.Now()), "refunded retry retained an active entitlement")
}

type paymentFulfillmentConcurrencyFixture struct {
	client  *dbent.Client
	userID  int64
	groupID int64
	planID  int64
	order   *dbent.PaymentOrder
}

func newPaymentFulfillmentConcurrencyFixture(t *testing.T) paymentFulfillmentConcurrencyFixture {
	t.Helper()
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{
		Email:    uniqueTestEmail("payment-fulfillment-concurrency"),
		Username: uniqueTestName("payment-fulfillment-user"),
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:                uniqueTestName("payment-fulfillment-group"),
		Platform:            service.PlatformOpenAI,
		SubscriptionType:    service.SubscriptionTypeSubscription,
		SubscriptionEnabled: true,
	})
	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName(uniqueTestName("payment-fulfillment-plan")).
		SetPrice(30).
		SetCurrency("CNY").
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		Save(ctx)
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Microsecond)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(30).
		SetPayAmount(30).
		SetRechargeCode(uniqueTestName("pay-sub")).
		SetOutTradeNo(uniqueTestName("payment-fulfillment-order")).
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo(uniqueTestName("payment-fulfillment-trade")).
		SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).
		SetSubscriptionGroupID(group.ID).
		SetSubscriptionDays(30).
		SetStatus(service.OrderStatusPaid).
		SetProviderSnapshot(map[string]any{
			"currency":                             "CNY",
			"subscription_entitlements_v2_enabled": true,
		}).
		SetPaidAt(now).
		SetExpiresAt(now.Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("integration.test").
		Save(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM payment_audit_logs WHERE order_id = $1", strconv.FormatInt(order.ID, 10))
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM payment_orders WHERE id = $1", order.ID)
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM users WHERE id = $1", user.ID)
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM subscription_plans WHERE id = $1", plan.ID)
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM groups WHERE id = $1", group.ID)
	})

	return paymentFulfillmentConcurrencyFixture{
		client:  client,
		userID:  user.ID,
		groupID: group.ID,
		planID:  plan.ID,
		order:   order,
	}
}

func newPaymentFulfillmentConcurrencyService(
	t *testing.T,
	fixture paymentFulfillmentConcurrencyFixture,
	entitlementRepo service.SubscriptionEntitlementRepository,
	legacyRepo service.UserSubscriptionRepository,
) *service.PaymentService {
	t.Helper()
	entitlementSvc := service.NewSubscriptionEntitlementService(entitlementRepo, NewSubscriptionEntitlementPlanRepository(fixture.client))
	var subscriptionSvc *service.SubscriptionService
	if legacyRepo != nil {
		subscriptionSvc = service.NewSubscriptionService(NewGroupRepository(fixture.client, integrationDB), legacyRepo, nil, fixture.client, nil)
		t.Cleanup(subscriptionSvc.Stop)
	}
	return service.NewPaymentService(
		fixture.client,
		nil,
		nil,
		nil,
		subscriptionSvc,
		entitlementSvc,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}

func sendFullExternalSubscriptionRefund(ctx context.Context, paymentSvc *service.PaymentService, order *dbent.PaymentOrder) error {
	return paymentSvc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		Amount:         order.PayAmount,
		AmountSemantic: payment.NotificationAmountTotal,
		Status:         payment.NotificationStatusRefunded,
	}, payment.TypeStripe)
}

func paymentTransactionBackendPID(ctx context.Context) (int, error) {
	tx := dbent.TxFromContext(ctx)
	if tx == nil {
		return 0, errors.New("payment entitlement mutation did not reuse a transaction")
	}
	rows, err := tx.Client().QueryContext(ctx, "SELECT pg_backend_pid()")
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, fmt.Errorf("query payment transaction backend pid: %w", err)
		}
		return 0, errors.New("query payment transaction backend pid returned no rows")
	}
	var pid int
	if err := rows.Scan(&pid); err != nil {
		return 0, err
	}
	return pid, rows.Err()
}

func waitForPaymentRefundCommitOrBlockedWaiter(t *testing.T, ctx context.Context, orderID int64, blockerPID int) {
	t.Helper()
	require.Eventually(t, func() bool {
		var status string
		if err := integrationDB.QueryRowContext(ctx, "SELECT status FROM payment_orders WHERE id = $1", orderID).Scan(&status); err != nil {
			return false
		}
		return status == service.OrderStatusRefunded || paymentHasWaiterBlockedByPID(ctx, blockerPID)
	}, 5*time.Second, 20*time.Millisecond, "external refund neither committed nor waited on the fulfillment transaction")
}

func paymentHasWaiterBlockedByPID(ctx context.Context, blockerPID int) bool {
	var blocked bool
	err := integrationDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_stat_activity
			WHERE $1 = ANY(pg_blocking_pids(pid))
		)
	`, blockerPID).Scan(&blocked)
	return err == nil && blocked
}

func receivePaymentConcurrencyResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("payment concurrency operation did not complete")
		return nil
	}
}

func receivePaymentBarrierPID(t *testing.T, ctx context.Context, acquired <-chan int) int {
	t.Helper()
	select {
	case pid := <-acquired:
		return pid
	case <-ctx.Done():
		t.Fatalf("payment concurrency barrier was not acquired: %v", ctx.Err())
		return 0
	case <-time.After(10 * time.Second):
		t.Fatal("payment concurrency barrier was not acquired")
		return 0
	}
}

func paymentWaiterCountBlockedByPID(ctx context.Context, blockerPID int) int {
	var count int
	if err := integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM pg_stat_activity
		WHERE $1 = ANY(pg_blocking_pids(pid))
	`, blockerPID).Scan(&count); err != nil {
		return 0
	}
	return count
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func activePaymentEntitlementCount(ctx context.Context, client *dbent.Client, userID int64) (int, error) {
	return client.SubscriptionEntitlement.Query().
		Where(
			subscriptionentitlement.UserIDEQ(userID),
			subscriptionentitlement.StatusEQ(service.SubscriptionStatusActive),
			subscriptionentitlement.DeletedAtIsNil(),
		).
		Count(ctx)
}
