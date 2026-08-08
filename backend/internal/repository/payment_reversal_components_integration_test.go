//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestPaymentReversalComponentsPersistIndependentlyInPostgres(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{
		Email:   uniqueTestEmail("payment-reversal-components"),
		Balance: 100,
	})
	now := time.Now().UTC().Truncate(time.Microsecond)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName("payment-reversal-components").
		SetAmount(100).
		SetPayAmount(100).
		SetRechargeCode(uniqueTestName("payment-reversal-code")).
		SetOutTradeNo(uniqueTestName("payment-reversal-order")).
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo(uniqueTestName("payment-reversal-trade")).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(service.OrderStatusCompleted).
		SetPaidAt(now).
		SetCompletedAt(now).
		SetExpiresAt(now.Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("integration.test").
		Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM payment_audit_logs WHERE order_id = $1", fmt.Sprintf("%d", order.ID))
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM payment_orders WHERE id = $1", order.ID)
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM users WHERE id = $1", user.ID)
	})

	paymentService := service.NewPaymentService(
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
		nil,
	)
	for _, notification := range []*payment.PaymentNotification{
		{
			EventID:        "evt_pg_refund_total_30",
			OrderID:        order.OutTradeNo,
			TradeNo:        order.PaymentTradeNo,
			Amount:         30,
			AmountSemantic: payment.NotificationAmountTotal,
			Status:         payment.NotificationStatusRefunded,
		},
		{
			EventID:        "evt_pg_chargeback_total_20",
			OrderID:        order.OutTradeNo,
			TradeNo:        order.PaymentTradeNo,
			Amount:         20,
			AmountSemantic: payment.NotificationAmountTotal,
			Status:         payment.NotificationStatusChargeback,
		},
	} {
		require.NoError(t, paymentService.HandlePaymentNotification(ctx, notification, payment.TypeStripe))
	}

	updated, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, 30.0, updated.ProviderRefundAmount)
	require.Equal(t, 20.0, updated.ChargebackAmount)
	require.Equal(t, 50.0, updated.RefundAmount)

	updatedUser, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, 50.0, updatedUser.Balance)

	// Provider retry is serialized by the payment row lock and remains a no-op.
	require.NoError(t, paymentService.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		EventID:        "evt_pg_refund_total_30",
		OrderID:        order.OutTradeNo,
		TradeNo:        order.PaymentTradeNo,
		Amount:         99,
		AmountSemantic: payment.NotificationAmountTotal,
		Status:         payment.NotificationStatusRefunded,
		RawData:        `{"retry":true}`,
	}, payment.TypeStripe))
	retried, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, 30.0, retried.ProviderRefundAmount)
	require.Equal(t, 20.0, retried.ChargebackAmount)
	require.Equal(t, 50.0, retried.RefundAmount)
}

func TestMigration197BackfillsKnownReferralChargeback(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	createPaymentReversalMigrationTestTables(t, ctx, tx)
	_, err = tx.ExecContext(ctx, `
INSERT INTO payment_orders (id, amount, pay_amount, refund_amount, out_trade_no, payment_type, status)
VALUES
    (1, 100, 100, 50, 'legacy-mixed-order', 'stripe', 'PARTIALLY_REFUNDED'),
    (3, 100, 100, 50, 'legacy-pending-order', 'stripe', 'REFUND_PENDING');
INSERT INTO recharge_orders (external_order_id, provider, paid_amount, chargeback_amount)
VALUES ('legacy-mixed-order', 'stripe', 100, 20);
`)
	require.NoError(t, err)

	applyPaymentReversalMigration(t, ctx, tx)

	var providerRefundAmount, chargebackAmount, combinedAmount float64
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT provider_refund_amount, chargeback_amount, refund_amount
FROM payment_orders
WHERE id = 1
`).Scan(&providerRefundAmount, &chargebackAmount, &combinedAmount))
	require.Equal(t, 30.0, providerRefundAmount)
	require.Equal(t, 20.0, chargebackAmount)
	require.Equal(t, 50.0, combinedAmount)

	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT provider_refund_amount, chargeback_amount
FROM payment_orders
WHERE id = 3
`).Scan(&providerRefundAmount, &chargebackAmount))
	require.Zero(t, providerRefundAmount, "a pending requested amount is not settled provider money")
	require.Zero(t, chargebackAmount)
}

func TestMigration197BackfillsAuditChargebacksWithoutReferralRows(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	createPaymentReversalMigrationTestTables(t, ctx, tx)
	_, err = tx.ExecContext(ctx, `
INSERT INTO payment_orders (id, amount, pay_amount, refund_amount, out_trade_no, payment_type, status)
VALUES
    (11, 120, 100, 24, 'audit-chargeback-only', 'stripe', 'PARTIALLY_REFUNDED'),
    (12, 120, 100, 60, 'audit-mixed-reversal', 'stripe', 'PARTIALLY_REFUNDED');

INSERT INTO payment_audit_logs (order_id, action, detail, created_at)
VALUES
    ('11', 'EXTERNAL_CHARGEBACK_SYNCED',
     '{"gatewayAmount":20,"amountSemantic":"total","status":"chargeback","refundAmountTotal":24,"creditedDelta":24}',
     NOW()),
    ('12', 'REFUND_EVENT_refund_total_30',
     '{"gatewayAmount":30,"amountSemantic":"total","status":"refunded","refundAmountTotal":36,"creditedDelta":36}',
     NOW()),
    ('12', 'CHARGEBACK_EVENT_chargeback_delta_20',
     '{"gatewayAmount":20,"amountSemantic":"delta","status":"chargeback","refundAmountTotal":60,"creditedDelta":24}',
     NOW() + INTERVAL '1 second'),
    ('12', 'EXTERNAL_CHARGEBACK_SYNCED',
     '{"gatewayAmount":20,"amountSemantic":"delta","status":"chargeback","refundAmountTotal":60,"creditedDelta":24}',
     NOW() + INTERVAL '1 second');
`)
	require.NoError(t, err)

	applyPaymentReversalMigration(t, ctx, tx)

	assertPaymentReversalComponents(t, ctx, tx, 11, 0, 24, 24)
	assertPaymentReversalComponents(t, ctx, tx, 12, 36, 24, 60)
}

func TestMigration197RejectsMixedEvidenceAboveSettledProjection(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	createPaymentReversalMigrationTestTables(t, ctx, tx)
	_, err = tx.ExecContext(ctx, `
INSERT INTO payment_orders (id, amount, pay_amount, refund_amount, out_trade_no, payment_type, status)
VALUES (21, 100, 100, 30, 'ambiguous-old-mixed-reversal', 'stripe', 'PARTIALLY_REFUNDED');

INSERT INTO payment_audit_logs (order_id, action, detail, created_at)
VALUES
    ('21', 'REFUND_EVENT_refund_total_30',
     '{"gatewayAmount":30,"amountSemantic":"total","status":"refunded","refundAmountTotal":30,"creditedDelta":30}',
     NOW()),
    ('21', 'CHARGEBACK_EVENT_chargeback_total_20',
     '{"gatewayAmount":20,"amountSemantic":"total","status":"chargeback","refundAmountTotal":30,"creditedDelta":0}',
     NOW() + INTERVAL '1 second');
`)
	require.NoError(t, err)

	migrationSQL, err := migrations.FS.ReadFile("197_payment_order_reversal_components.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.Error(t, err)
	require.Contains(t, err.Error(), "reversal evidence does not match settled projection")
}

func TestMigration197RejectsLegacySettledProjectionOnlyWriter(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	createPaymentReversalMigrationTestTables(t, ctx, tx)
	_, err = tx.ExecContext(ctx, `
INSERT INTO payment_orders (id, amount, pay_amount, refund_amount, out_trade_no, payment_type, status)
VALUES (31, 100, 100, 0, 'legacy-writer-after-contract', 'stripe', 'COMPLETED');
`)
	require.NoError(t, err)

	applyPaymentReversalMigration(t, ctx, tx)
	require.NoError(t, savepoint(ctx, tx, "before_legacy_writer"))
	_, err = tx.ExecContext(ctx, `
UPDATE payment_orders
SET status = 'PARTIALLY_REFUNDED',
    refund_amount = 20
WHERE id = 31
`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "chk_payment_orders_reversal_projection")
	require.NoError(t, rollbackToSavepoint(ctx, tx, "before_legacy_writer"))
}

func createPaymentReversalMigrationTestTables(t *testing.T, ctx context.Context, tx *sql.Tx) {
	t.Helper()
	_, err := tx.ExecContext(ctx, `
CREATE TEMP TABLE payment_orders (
    id BIGINT PRIMARY KEY,
    amount DECIMAL(20,2) NOT NULL,
    pay_amount DECIMAL(20,2) NOT NULL,
    refund_amount DECIMAL(20,2) NOT NULL DEFAULT 0,
    out_trade_no VARCHAR(64) NOT NULL,
    payment_type VARCHAR(30) NOT NULL,
    status VARCHAR(30) NOT NULL
);
CREATE TEMP TABLE recharge_orders (
    external_order_id VARCHAR(128) NOT NULL,
    provider VARCHAR(32) NOT NULL,
    paid_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    chargeback_amount DECIMAL(20,8) NOT NULL DEFAULT 0
);
CREATE TEMP TABLE payment_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    order_id VARCHAR(64) NOT NULL,
    action VARCHAR(50) NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`)
	require.NoError(t, err)
}

func applyPaymentReversalMigration(t *testing.T, ctx context.Context, tx *sql.Tx) {
	t.Helper()
	migrationSQL, err := migrations.FS.ReadFile("197_payment_order_reversal_components.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
}

func assertPaymentReversalComponents(t *testing.T, ctx context.Context, tx *sql.Tx, orderID int64, provider, chargeback, combined float64) {
	t.Helper()
	var gotProvider, gotChargeback, gotCombined float64
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT provider_refund_amount, chargeback_amount, refund_amount
FROM payment_orders
WHERE id = $1
`, orderID).Scan(&gotProvider, &gotChargeback, &gotCombined))
	require.Equal(t, provider, gotProvider)
	require.Equal(t, chargeback, gotChargeback)
	require.Equal(t, combined, gotCombined)
}

func savepoint(ctx context.Context, tx *sql.Tx, name string) error {
	_, err := tx.ExecContext(ctx, "SAVEPOINT "+name)
	return err
}

func rollbackToSavepoint(ctx context.Context, tx *sql.Tx, name string) error {
	_, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+name)
	return err
}
