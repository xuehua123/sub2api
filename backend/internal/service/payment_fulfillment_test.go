//go:build unit

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// resolveRedeemAction — pure idempotency decision logic
// ---------------------------------------------------------------------------

func TestResolveRedeemAction_CodeNotFound(t *testing.T) {
	t.Parallel()
	action := resolveRedeemAction(nil, nil)
	assert.Equal(t, redeemActionCreate, action, "nil code with nil error should create")
}

func TestResolveRedeemAction_LookupError(t *testing.T) {
	t.Parallel()
	action := resolveRedeemAction(nil, errors.New("db connection lost"))
	assert.Equal(t, redeemActionCreate, action, "lookup error should fall back to create")
}

func TestResolveRedeemAction_LookupErrorWithNonNilCode(t *testing.T) {
	t.Parallel()
	// Edge case: both code and error are non-nil (shouldn't happen in practice,
	// but the function should still treat error as authoritative)
	code := &RedeemCode{Status: StatusUnused}
	action := resolveRedeemAction(code, errors.New("partial error"))
	assert.Equal(t, redeemActionCreate, action, "non-nil error should always result in create regardless of code")
}

func TestResolveRedeemAction_CodeExistsAndUsed(t *testing.T) {
	t.Parallel()
	code := &RedeemCode{
		Code:   "test-code-123",
		Status: StatusUsed,
		Type:   RedeemTypeBalance,
		Value:  10.0,
	}
	action := resolveRedeemAction(code, nil)
	assert.Equal(t, redeemActionSkipCompleted, action, "used code should skip to completed")
}

func TestResolveRedeemAction_CodeExistsAndUnused(t *testing.T) {
	t.Parallel()
	code := &RedeemCode{
		Code:   "test-code-456",
		Status: StatusUnused,
		Type:   RedeemTypeBalance,
		Value:  25.0,
	}
	action := resolveRedeemAction(code, nil)
	assert.Equal(t, redeemActionRedeem, action, "unused code should skip creation and proceed to redeem")
}

func TestResolveRedeemAction_CodeExistsWithExpiredStatus(t *testing.T) {
	t.Parallel()
	// A code with a non-standard status (neither "unused" nor "used")
	// should NOT be treated as used, so it falls through to redeemActionRedeem.
	code := &RedeemCode{
		Code:   "expired-code",
		Status: StatusExpired,
	}
	action := resolveRedeemAction(code, nil)
	assert.Equal(t, redeemActionRedeem, action, "expired-status code is not IsUsed(), should redeem")
}

// ---------------------------------------------------------------------------
// Table-driven comprehensive test
// ---------------------------------------------------------------------------

func TestResolveRedeemAction_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     *RedeemCode
		err      error
		expected redeemAction
	}{
		{
			name:     "nil code, nil error — first run",
			code:     nil,
			err:      nil,
			expected: redeemActionCreate,
		},
		{
			name:     "nil code, lookup error — treat as not found",
			code:     nil,
			err:      ErrRedeemCodeNotFound,
			expected: redeemActionCreate,
		},
		{
			name:     "nil code, generic DB error — treat as not found",
			code:     nil,
			err:      errors.New("connection refused"),
			expected: redeemActionCreate,
		},
		{
			name:     "code exists, used — previous run completed redeem",
			code:     &RedeemCode{Status: StatusUsed},
			err:      nil,
			expected: redeemActionSkipCompleted,
		},
		{
			name:     "code exists, unused — previous run created code but crashed before redeem",
			code:     &RedeemCode{Status: StatusUnused},
			err:      nil,
			expected: redeemActionRedeem,
		},
		{
			name:     "code exists but error also set — error takes precedence",
			code:     &RedeemCode{Status: StatusUsed},
			err:      errors.New("unexpected"),
			expected: redeemActionCreate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveRedeemAction(tt.code, tt.err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ---------------------------------------------------------------------------
// redeemAction enum value sanity
// ---------------------------------------------------------------------------

func TestRedeemAction_DistinctValues(t *testing.T) {
	t.Parallel()
	// Ensure the three actions have distinct values (iota correctness)
	assert.NotEqual(t, redeemActionCreate, redeemActionRedeem)
	assert.NotEqual(t, redeemActionCreate, redeemActionSkipCompleted)
	assert.NotEqual(t, redeemActionRedeem, redeemActionSkipCompleted)
}

// ---------------------------------------------------------------------------
// RedeemCode.IsUsed / CanUse interaction with resolveRedeemAction
// ---------------------------------------------------------------------------

func TestComputeExternalCreditedRefund_UsesCreditRatioForPartialGatewayRefund(t *testing.T) {
	t.Parallel()

	creditedDelta, refundTotal := computeExternalCreditedRefund(&dbent.PaymentOrder{
		Amount:       120,
		PayAmount:    100,
		RefundAmount: 0,
	}, 50, payment.NotificationAmountDelta)

	assert.Equal(t, 60.0, creditedDelta)
	assert.Equal(t, 60.0, refundTotal)
}

func TestComputeExternalCreditedRefund_CapsAtRemainingCreditedBalance(t *testing.T) {
	t.Parallel()

	creditedDelta, refundTotal := computeExternalCreditedRefund(&dbent.PaymentOrder{
		Amount:       120,
		PayAmount:    100,
		RefundAmount: 100,
	}, 50, payment.NotificationAmountDelta)

	assert.Equal(t, 20.0, creditedDelta)
	assert.Equal(t, 120.0, refundTotal)
}

func TestComputeExternalCreditedRefund_UsesTotalSemanticAsCumulativeGatewayAmount(t *testing.T) {
	t.Parallel()

	creditedDelta, refundTotal := computeExternalCreditedRefund(&dbent.PaymentOrder{
		Amount:       120,
		PayAmount:    100,
		RefundAmount: 24,
	}, 50, payment.NotificationAmountTotal)

	assert.Equal(t, 36.0, creditedDelta)
	assert.Equal(t, 60.0, refundTotal)
}

func TestAccumulateExternalReversalAmount_AddsIncrementallyAndCapsTotal(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 70.0, accumulateExternalReversalAmount(40, 30, 100))
	assert.Equal(t, 100.0, accumulateExternalReversalAmount(80, 50, 100))
	assert.Equal(t, 100.0, accumulateExternalReversalAmount(120, 10, 100))
}

func TestReconcileExternalReversalAmount_UsesTotalSemanticAsCumulativeAmount(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 70.0, reconcileExternalReversalAmount(40, 70, 100, payment.NotificationAmountTotal))
	assert.Equal(t, 100.0, reconcileExternalReversalAmount(40, 120, 100, payment.NotificationAmountTotal))
}

type paymentRefundUserRepoStub struct {
	*withdrawalUserRepoStub
	deductedBalances map[int64]float64
	deductErr        error
}

func newPaymentRefundUserRepoStub() *paymentRefundUserRepoStub {
	return &paymentRefundUserRepoStub{withdrawalUserRepoStub: &withdrawalUserRepoStub{}}
}

func (s *paymentRefundUserRepoStub) DeductBalance(ctx context.Context, id int64, amount float64) error {
	if s.deductErr != nil {
		return s.deductErr
	}
	if s.deductedBalances == nil {
		s.deductedBalances = map[int64]float64{}
	}
	s.deductedBalances[id] += amount
	return nil
}

type failingRechargeOrderRepoStub struct {
	*rechargeOrderRepoStub
	err error
}

func (s *failingRechargeOrderRepoStub) GetByProviderAndExternalOrderID(ctx context.Context, provider, externalOrderID string) (*RechargeOrder, error) {
	return nil, s.err
}

func newPaymentServiceEntClient(t *testing.T) *dbent.Client {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() {
		require.NoError(t, client.Close())
		require.NoError(t, db.Close())
	})
	return client
}

func createPaymentRefundTestUser(t *testing.T, ctx context.Context, client *dbent.Client, balance float64) *dbent.User {
	t.Helper()

	user, err := client.User.Create().
		SetEmail(fmt.Sprintf("%s@example.com", strings.ToLower(strings.ReplaceAll(t.Name(), "/", "_")))).
		SetPasswordHash("hash").
		SetRole("user").
		SetUsername("refund-test").
		SetBalance(balance).
		Save(ctx)
	require.NoError(t, err)
	return user
}

func createPaymentOrderForRefundTest(t *testing.T, ctx context.Context, client *dbent.Client, user *dbent.User, amount float64, payAmount float64, refundAmount float64, status string, outTradeNo string, paymentTradeNo string) *dbent.PaymentOrder {
	t.Helper()

	paidAt := time.Now().Add(-time.Hour)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(amount).
		SetPayAmount(payAmount).
		SetRechargeCode("code-" + strings.ReplaceAll(outTradeNo, "_", "-")).
		SetOutTradeNo(outTradeNo).
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo(paymentTradeNo).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(status).
		SetRefundAmount(refundAmount).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(paidAt).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		Save(ctx)
	require.NoError(t, err)
	return order
}

func requireAuditActionsForOrder(t *testing.T, ctx context.Context, client *dbent.Client, orderID int64, expectedActions ...string) []*dbent.PaymentAuditLog {
	t.Helper()

	logs, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(orderID, 10))).
		Order(paymentauditlog.ByCreatedAt()).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, logs, len(expectedActions))
	for i, action := range expectedActions {
		require.Equal(t, action, logs[i].Action)
	}
	return logs
}

func TestHandlePaymentNotification_RefundedTotalSyncsOrderBalanceAndReferralState(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(t, ctx, client, user, 120, 100, 24, OrderStatusCompleted, "refund_total_order", "trade_refund_total")

	userRepo := newPaymentRefundUserRepoStub()
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["stripe::refund_total_order"] = &RechargeOrder{
		ID:              101,
		UserID:          user.ID,
		Provider:        payment.TypeStripe,
		ExternalOrderID: order.OutTradeNo,
		PaidAmount:      100,
		RefundedAmount:  20,
		Status:          RechargeOrderStatusCredited,
		Currency:        ReferralSettlementCurrencyCNY,
	}
	service := &PaymentService{
		entClient:         client,
		userRepo:          userRepo,
		referralRefundSvc: NewReferralRefundService(rechargeRepo, &commissionRepoStub{}, nil, nil),
	}

	err := service.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		Amount:         50,
		AmountSemantic: payment.NotificationAmountTotal,
		Status:         payment.NotificationStatusRefunded,
	}, payment.TypeStripe)
	require.NoError(t, err)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, updatedOrder.Status)
	require.Equal(t, 60.0, updatedOrder.RefundAmount)
	require.NotNil(t, updatedOrder.RefundAt)
	require.Equal(t, 36.0, userRepo.deductedBalances[user.ID])

	updatedRecharge, err := rechargeRepo.GetByProviderAndExternalOrderID(ctx, payment.TypeStripe, order.OutTradeNo)
	require.NoError(t, err)
	require.Equal(t, 50.0, updatedRecharge.RefundedAmount)
	require.Equal(t, 0.0, updatedRecharge.ChargebackAmount)
	require.Equal(t, RechargeOrderStatusPartiallyRefunded, updatedRecharge.Status)

	logs := requireAuditActionsForOrder(t, ctx, client, order.ID, "EXTERNAL_REFUND_SYNCED")
	require.Contains(t, logs[0].Detail, `"amountSemantic":"total"`)
}

func TestHandlePaymentNotification_ChargebackDeltaSyncsOrderBalanceAndReferralState(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(t, ctx, client, user, 100, 100, 30, OrderStatusCompleted, "chargeback_delta_order", "trade_chargeback_delta")

	userRepo := newPaymentRefundUserRepoStub()
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["stripe::chargeback_delta_order"] = &RechargeOrder{
		ID:               102,
		UserID:           user.ID,
		Provider:         payment.TypeStripe,
		ExternalOrderID:  order.OutTradeNo,
		PaidAmount:       100,
		RefundedAmount:   30,
		ChargebackAmount: 0,
		Status:           RechargeOrderStatusPartiallyRefunded,
		Currency:         ReferralSettlementCurrencyCNY,
	}
	service := &PaymentService{
		entClient:         client,
		userRepo:          userRepo,
		referralRefundSvc: NewReferralRefundService(rechargeRepo, &commissionRepoStub{}, nil, nil),
	}

	err := service.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		Amount:         20,
		AmountSemantic: payment.NotificationAmountDelta,
		Status:         payment.NotificationStatusChargeback,
	}, payment.TypeStripe)
	require.NoError(t, err)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, updatedOrder.Status)
	require.Equal(t, 50.0, updatedOrder.RefundAmount)
	require.NotNil(t, updatedOrder.RefundAt)
	require.Equal(t, 20.0, userRepo.deductedBalances[user.ID])

	updatedRecharge, err := rechargeRepo.GetByProviderAndExternalOrderID(ctx, payment.TypeStripe, order.OutTradeNo)
	require.NoError(t, err)
	require.Equal(t, 30.0, updatedRecharge.RefundedAmount)
	require.Equal(t, 20.0, updatedRecharge.ChargebackAmount)
	require.Equal(t, RechargeOrderStatusChargeback, updatedRecharge.Status)

	logs := requireAuditActionsForOrder(t, ctx, client, order.ID, "EXTERNAL_CHARGEBACK_SYNCED")
	require.Contains(t, logs[0].Detail, `"amountSemantic":"delta"`)
}

func TestHandlePaymentNotification_RefundedFallsBackToTradeNoLookup(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(t, ctx, client, user, 120, 100, 0, OrderStatusCompleted, "refund_trade_lookup_order", "trade_refund_lookup")

	userRepo := newPaymentRefundUserRepoStub()
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["stripe::refund_trade_lookup_order"] = &RechargeOrder{
		ID:              103,
		UserID:          user.ID,
		Provider:        payment.TypeStripe,
		ExternalOrderID: order.OutTradeNo,
		PaidAmount:      100,
		RefundedAmount:  0,
		Status:          RechargeOrderStatusCredited,
		Currency:        ReferralSettlementCurrencyCNY,
	}
	service := &PaymentService{
		entClient:         client,
		userRepo:          userRepo,
		referralRefundSvc: NewReferralRefundService(rechargeRepo, &commissionRepoStub{}, nil, nil),
	}

	err := service.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		TradeNo:        order.PaymentTradeNo,
		Amount:         50,
		AmountSemantic: payment.NotificationAmountTotal,
		Status:         payment.NotificationStatusRefunded,
	}, payment.TypeStripe)
	require.NoError(t, err)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, updatedOrder.Status)
	require.Equal(t, 60.0, updatedOrder.RefundAmount)
	require.Equal(t, 60.0, userRepo.deductedBalances[user.ID])

	updatedRecharge, err := rechargeRepo.GetByProviderAndExternalOrderID(ctx, payment.TypeStripe, order.OutTradeNo)
	require.NoError(t, err)
	require.Equal(t, 50.0, updatedRecharge.RefundedAmount)

	logs := requireAuditActionsForOrder(t, ctx, client, order.ID, "EXTERNAL_REFUND_SYNCED")
	require.Contains(t, logs[0].Detail, `"tradeNo":"trade_refund_lookup"`)
}

func TestMarkRefundOk_PersistsSuccessWhenReferralSyncFails(t *testing.T) {
	ctx := context.Background()
	client := newPaymentServiceEntClient(t)
	user := createPaymentRefundTestUser(t, ctx, client, 0)
	order := createPaymentOrderForRefundTest(t, ctx, client, user, 50, 50, 0, OrderStatusRefunding, "mark_refund_ok_order", "trade_mark_refund_ok")

	service := &PaymentService{
		entClient: client,
		referralRefundSvc: NewReferralRefundService(
			&failingRechargeOrderRepoStub{rechargeOrderRepoStub: newRechargeOrderRepoStub(), err: errors.New("referral lookup failed")},
			&commissionRepoStub{},
			nil,
			nil,
		),
	}

	result, err := service.markRefundOk(ctx, &RefundPlan{
		OrderID:       order.ID,
		Order:         order,
		RefundAmount:  50,
		GatewayAmount: 50,
		Reason:        "gateway success",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Contains(t, result.Warning, "referral sync failed")

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefunded, updatedOrder.Status)
	require.Equal(t, 50.0, updatedOrder.RefundAmount)
	require.NotNil(t, updatedOrder.RefundAt)
	require.NotNil(t, updatedOrder.RefundReason)
	require.Equal(t, "gateway success", *updatedOrder.RefundReason)

	requireAuditActionsForOrder(t, ctx, client, order.ID, "REFUND_SUCCESS", "REFUND_REFERRAL_SYNC_FAILED")
}
