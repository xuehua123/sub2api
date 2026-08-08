package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration196AddsBoundedAffiliateReversedAmountAdditively(t *testing.T) {
	content, err := FS.ReadFile("196_affiliate_ledger_reversal.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS reversed_amount DECIMAL(20,8) NOT NULL DEFAULT 0")
	require.Contains(t, sql, "chk_user_affiliate_ledger_accrue_reversed_amount")
	require.Contains(t, sql, "reversed_amount >= 0")
	require.Contains(t, sql, "(action = 'accrue' AND reversed_amount <= amount)")
	require.Contains(t, sql, "OR (action <> 'accrue' AND reversed_amount = 0)")
	require.Contains(t, sql, ") NOT VALID")
	require.Contains(t, sql, "VALIDATE CONSTRAINT chk_user_affiliate_ledger_accrue_reversed_amount")
	require.NotContains(t, strings.ToUpper(sql), "CONCURRENTLY")
}

func TestMigration196aBuildsAffiliateAccrueOrderUniquenessOnline(t *testing.T) {
	const name = "196a_affiliate_ledger_accrue_order_unique_notx.sql"
	content, err := FS.ReadFile(name)
	require.NoError(t, err)

	sql := string(content)
	require.True(t, strings.HasSuffix(name, "_notx.sql"))
	require.Contains(t, sql, "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_user_affiliate_ledger_accrue_order_uniq")
	require.Contains(t, sql, "ON user_affiliate_ledger (source_order_id)")
	require.Contains(t, sql, "WHERE action = 'accrue' AND source_order_id IS NOT NULL")
	require.NotContains(t, strings.ToUpper(sql), "DELETE FROM")
}
