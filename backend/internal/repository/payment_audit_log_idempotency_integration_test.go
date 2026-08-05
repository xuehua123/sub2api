//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/stretchr/testify/require"
)

func TestPaymentAuditLogDuplicateActionKeepsTransactionUsable(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	orderID := uniqueTestName("payment-audit-idempotency")
	t.Cleanup(func() {
		_, _ = client.PaymentAuditLog.Delete().
			Where(paymentauditlog.OrderIDEQ(orderID)).
			Exec(context.Background())
	})

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	_, err = tx.PaymentAuditLog.Create().
		SetOrderID(orderID).
		SetAction("REFUND_SUCCESS").
		SetOperator("admin").
		SetDetail(`{"attempt":1}`).
		Save(ctx)
	require.NoError(t, err)

	err = tx.PaymentAuditLog.Create().
		SetOrderID(orderID).
		SetAction("REFUND_SUCCESS").
		SetOperator("admin").
		SetDetail(`{"attempt":2}`).
		OnConflictColumns(paymentauditlog.FieldOrderID, paymentauditlog.FieldAction).
		Ignore().
		Exec(ctx)
	require.NoError(t, err)

	_, err = tx.PaymentAuditLog.Create().
		SetOrderID(orderID).
		SetAction("REFUND_REFERRAL_SYNC_FAILED").
		SetOperator("admin").
		SetDetail(`{"detail":"retry later"}`).
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	logs, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(orderID)).
		Order(paymentauditlog.ByID()).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, logs, 2)
	require.Equal(t, "REFUND_SUCCESS", logs[0].Action)
	require.JSONEq(t, `{"attempt":1}`, logs[0].Detail)
	require.Equal(t, "REFUND_REFERRAL_SYNC_FAILED", logs[1].Action)
}
