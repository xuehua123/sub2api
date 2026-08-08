package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration197RequiresLegacyWritersStoppedAndFailsClosed(t *testing.T) {
	content, err := FS.ReadFile("197_payment_order_reversal_components.sql")
	require.NoError(t, err)

	sql := string(content)
	upperSQL := strings.ToUpper(sql)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS provider_refund_amount DECIMAL(20,2) NOT NULL DEFAULT 0")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS chargeback_amount DECIMAL(20,2) NOT NULL DEFAULT 0")
	require.Contains(t, sql, "all legacy payment writers MUST be stopped")
	require.Contains(t, sql, "NOT shared-database blue-green")
	require.Contains(t, sql, "SET provider_refund_amount = LEAST(GREATEST(refund_amount, 0), GREATEST(amount, 0))")
	require.Contains(t, sql, "status IN ('PARTIALLY_REFUNDED', 'REFUNDED')")
	require.Contains(t, sql, "CHARGEBACK_EVENT_")
	require.Contains(t, sql, "EXTERNAL_CHARGEBACK_SYNCED")
	require.Contains(t, sql, "REFUND_EVENT_")
	require.Contains(t, sql, "reversal evidence does not match settled projection")
	require.Contains(t, sql, "FROM recharge_orders AS r")
	require.Contains(t, sql, "r.chargeback_amount")
	require.Contains(t, sql, "chk_payment_orders_reversal_components")
	require.Contains(t, sql, "provider_refund_amount + chargeback_amount <= amount")
	require.Contains(t, sql, "chk_payment_orders_reversal_projection")
	require.Contains(t, sql, "provider_refund_amount + chargeback_amount = refund_amount")
	require.Contains(t, sql, ") NOT VALID")
	require.Contains(t, sql, "VALIDATE CONSTRAINT chk_payment_orders_reversal_components")
	require.Contains(t, sql, "VALIDATE CONSTRAINT chk_payment_orders_reversal_projection")
	require.NotContains(t, upperSQL, "DROP COLUMN")
	require.NotContains(t, upperSQL, "DROP TABLE")
	require.NotContains(t, upperSQL, "ALTER COLUMN REFUND_AMOUNT")
	require.NotContains(t, upperSQL, "UPDATE USERS")
	require.NotContains(t, upperSQL, "UPDATE USER_SUBSCRIPTIONS")
	require.NotContains(t, upperSQL, "UPDATE SUBSCRIPTION_ENTITLEMENTS")
	require.NotContains(t, upperSQL, "UPDATE USER_AFFILIATES")
}
