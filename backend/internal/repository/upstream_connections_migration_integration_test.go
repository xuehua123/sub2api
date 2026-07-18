//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpstreamConnectionsMigrationSchema(t *testing.T) {
	tx := testTx(t)

	for _, table := range []string{
		"upstream_connections",
		"upstream_groups",
		"upstream_account_bindings",
	} {
		var regclass sql.NullString
		require.NoError(t, tx.QueryRowContext(
			context.Background(),
			"SELECT to_regclass('public.' || $1)",
			table,
		).Scan(&regclass))
		require.True(t, regclass.Valid, "expected %s table to exist", table)
	}

	requireColumn(t, tx, "upstream_connections", "credential_encrypted", "text", 0, false)
	requireColumn(t, tx, "upstream_connections", "legacy_migration_key", "character varying", 128, true)
	requireColumn(t, tx, "upstream_connections", "remote_user_id", "character varying", 128, false)
	requireColumn(t, tx, "upstream_connections", "wallet_amount", "numeric", 0, true)
	requireColumn(t, tx, "upstream_connections", "wallet_raw", "jsonb", 0, false)
	requireColumn(t, tx, "upstream_groups", "rate_multiplier", "numeric", 0, true)
	requireColumn(t, tx, "upstream_account_bindings", "observed_multiplier", "numeric", 0, true)
	requireColumn(t, tx, "upstream_account_bindings", "apply_policy", "character varying", 32, false)

	requireForeignKeyOnDelete(t, tx, "upstream_connections", "proxy_id", "proxies", "SET NULL")
	requireForeignKeyOnDelete(t, tx, "upstream_groups", "connection_id", "upstream_connections", "CASCADE")
	requireForeignKeyOnDelete(t, tx, "upstream_account_bindings", "connection_id", "upstream_connections", "CASCADE")
	requireForeignKeyOnDelete(t, tx, "upstream_account_bindings", "account_id", "accounts", "CASCADE")

	requireIndex(t, tx, "upstream_connections", "idx_upstream_connections_sync_due")
	requireIndex(t, tx, "upstream_connections", "idx_upstream_connections_status")
	requireIndex(t, tx, "upstream_connections", "idx_upstream_connections_legacy_migration_key")
	requireIndex(t, tx, "upstream_groups", "upstream_groups_connection_name_unique")
	requireIndex(t, tx, "upstream_account_bindings", "upstream_account_bindings_account_unique")
	requireIndex(t, tx, "upstream_account_bindings", "idx_upstream_account_bindings_sync_due")
	requireIndex(t, tx, "upstream_account_bindings", "idx_upstream_account_bindings_status")
}
