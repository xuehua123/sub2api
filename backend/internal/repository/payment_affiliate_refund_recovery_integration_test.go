//go:build integration

package repository

import (
	"context"
	"strconv"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

const affiliateRefundRecoveryCursorSettingKey = "payment_refund_reward_sync_recovery_cursor"

func TestPaymentAffiliateRefundRecoveryAcrossGateRestart(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	fixture := newAffiliateReversalFixture(t, 10, 24)
	orderID := strconv.FormatInt(fixture.orderID, 10)

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM settings WHERE key = $1", affiliateRefundRecoveryCursorSettingKey)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM settings WHERE key = $1", affiliateRefundRecoveryCursorSettingKey)
	})

	_, err = client.PaymentAuditLog.Create().
		SetOrderID(orderID).
		SetAction("AFFILIATE_REBATE_APPLIED").
		SetDetail(`{"baseAmount":100,"rebateAmount":10}`).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Get(ctx, fixture.orderID)
	require.NoError(t, err)
	require.InDelta(t, 0, affiliateAccrualReversedAmount(t, ctx, client, fixture.orderID), 1e-9)

	t.Setenv("AFFILIATE_REFUND_REVERSAL_ENABLED", "false")
	preContractService := newAffiliateRefundRecoveryPaymentService(client, fixture.repo)
	require.NoError(t, preContractService.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		Amount:         50,
		AmountSemantic: payment.NotificationAmountTotal,
		Status:         payment.NotificationStatusRefunded,
		RawData:        "affiliate-refund-gate-disabled",
	}, payment.TypeStripe))

	refunded, err := client.PaymentOrder.Get(ctx, fixture.orderID)
	require.NoError(t, err)
	require.Equal(t, service.OrderStatusPartiallyRefunded, refunded.Status)
	require.InDelta(t, 50, refunded.RefundAmount, 1e-9)
	require.Equal(t, 1, paymentAuditActionCount(t, ctx, client, orderID, "REFUND_AFFILIATE_SYNC_FAILED"))
	require.Equal(t, 0, paymentAuditActionCount(t, ctx, client, orderID, "REFUND_AFFILIATE_SYNC_RECOVERED"))
	require.InDelta(t, 0, affiliateAccrualReversedAmount(t, ctx, client, fixture.orderID), 1e-9,
		"the disabled gate must not write the expand-contract column")
	require.Equal(t, 0, affiliateRefundReverseLedgerCount(t, ctx, client, fixture.orderID))

	t.Setenv("AFFILIATE_REFUND_REVERSAL_ENABLED", "true")
	postContractService := newAffiliateRefundRecoveryPaymentService(client, fixture.repo)
	recovered, err := postContractService.RetryFailedRefundRewardSyncs(ctx, 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, recovered, 1)

	reloaded, err := client.PaymentOrder.Get(ctx, fixture.orderID)
	require.NoError(t, err)
	require.Equal(t, service.OrderStatusPartiallyRefunded, reloaded.Status)
	require.InDelta(t, 50, reloaded.RefundAmount, 1e-9)
	require.Equal(t, 0, paymentAuditActionCount(t, ctx, client, orderID, "REFUND_AFFILIATE_SYNC_FAILED"))
	require.Equal(t, 1, paymentAuditActionCount(t, ctx, client, orderID, "REFUND_AFFILIATE_SYNC_RECOVERED"))
	require.InDelta(t, 5, affiliateAccrualReversedAmount(t, ctx, client, fixture.orderID), 1e-9)
	require.Equal(t, 1, affiliateRefundReverseLedgerCount(t, ctx, client, fixture.orderID))
	require.InDelta(t, -5, affiliateRefundReverseLedgerAmount(t, ctx, client, fixture.orderID), 1e-9)
}

func newAffiliateRefundRecoveryPaymentService(client *dbent.Client, affiliateRepo service.AffiliateRepository) *service.PaymentService {
	return service.NewPaymentService(
		client,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		NewUserRepository(client, integrationDB),
		nil,
		nil,
		nil,
		service.NewAffiliateService(affiliateRepo, nil, nil, nil),
	)
}

func paymentAuditActionCount(t *testing.T, ctx context.Context, client *dbent.Client, orderID, action string) int {
	t.Helper()
	return querySingleInt(t, ctx, client,
		"SELECT COUNT(*) FROM payment_audit_logs WHERE order_id = $1 AND action = $2",
		orderID, action)
}

func affiliateAccrualReversedAmount(t *testing.T, ctx context.Context, client *dbent.Client, orderID int64) float64 {
	t.Helper()
	return querySingleFloat(t, ctx, client, `
		SELECT reversed_amount::double precision
		FROM user_affiliate_ledger
		WHERE source_order_id = $1 AND action = 'accrue'`, orderID)
}

func affiliateRefundReverseLedgerCount(t *testing.T, ctx context.Context, client *dbent.Client, orderID int64) int {
	t.Helper()
	return querySingleInt(t, ctx, client, `
		SELECT COUNT(*)
		FROM user_affiliate_ledger
		WHERE source_order_id = $1 AND action = 'refund_reverse'`, orderID)
}

func affiliateRefundReverseLedgerAmount(t *testing.T, ctx context.Context, client *dbent.Client, orderID int64) float64 {
	t.Helper()
	return querySingleFloat(t, ctx, client, `
		SELECT amount::double precision
		FROM user_affiliate_ledger
		WHERE source_order_id = $1 AND action = 'refund_reverse'`, orderID)
}
