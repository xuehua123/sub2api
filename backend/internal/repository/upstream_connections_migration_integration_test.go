//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
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
	requireColumnAbsent(t, tx, "upstream_connections", "legacy_migration_key")
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
	requireIndexAbsent(t, tx, "upstream_connections", "idx_upstream_connections_legacy_migration_key")
	requireIndex(t, tx, "upstream_groups", "upstream_groups_connection_name_unique")
	requireIndex(t, tx, "upstream_account_bindings", "upstream_account_bindings_account_unique")
	requireIndex(t, tx, "upstream_account_bindings", "idx_upstream_account_bindings_sync_due")
	requireIndex(t, tx, "upstream_account_bindings", "idx_upstream_account_bindings_status")

}

func TestRetiredAccountProbeDataMigration(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	migrationSQL, err := dbmigrations.FS.ReadFile("184_remove_legacy_account_probe_data.sql")
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `
		ALTER TABLE upstream_connections
			ADD COLUMN legacy_migration_key VARCHAR(128);
		CREATE UNIQUE INDEX idx_upstream_connections_legacy_migration_key
			ON upstream_connections (legacy_migration_key)
			WHERE legacy_migration_key IS NOT NULL;
	`)
	require.NoError(t, err)

	var accountID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
		INSERT INTO accounts (name, platform, type, credentials, extra)
		VALUES (
			'migration-184-retired-probe',
			'openai',
			'api_key',
			'{"api_key":"keep","upstream_management_auth":"drop","upstream_management_base_url":"https://drop.example"}'::jsonb,
			'{"custom":"keep","balance_probe_notified_at":"drop","upstream_billing_probe":{"status":"ok"},"upstream_rate_multiplier_sync_enabled":true}'::jsonb
		)
		RETURNING id
	`).Scan(&accountID))

	var connectionID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
		INSERT INTO upstream_connections (
			name, auth_mode, management_base_url, credential_encrypted, legacy_migration_key
		)
		VALUES (
			'migration-184-compatibility-column', 'access_token', 'https://upstream.example', 'ciphertext', 'legacy-source-1'
		)
		RETURNING id
	`).Scan(&connectionID))

	_, err = tx.ExecContext(ctx, `
		INSERT INTO settings (key, value)
		VALUES
			('upstream_billing_probe_settings', '{"enabled":true}'),
			('ops_alert_runtime_settings', '{"evaluation_interval_seconds":60,"account_balance":{"enabled":true},"unrelated":"keep"}')
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
	`)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err, "migration must be idempotent")

	var keepCredential, retiredAuth, retiredBaseURL bool
	var keepExtra, retiredBalance, retiredBilling, retiredMultiplier bool
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT
			credentials ? 'api_key',
			credentials ? 'upstream_management_auth',
			credentials ? 'upstream_management_base_url',
			extra ? 'custom',
			extra ? 'balance_probe_notified_at',
			extra ? 'upstream_billing_probe',
			extra ? 'upstream_rate_multiplier_sync_enabled'
		FROM accounts
		WHERE id = $1
	`, accountID).Scan(
		&keepCredential, &retiredAuth, &retiredBaseURL,
		&keepExtra, &retiredBalance, &retiredBilling, &retiredMultiplier,
	))
	require.True(t, keepCredential)
	require.True(t, keepExtra)
	require.False(t, retiredAuth)
	require.False(t, retiredBaseURL)
	require.False(t, retiredBalance)
	require.False(t, retiredBilling)
	require.False(t, retiredMultiplier)

	requireColumnAbsent(t, tx, "upstream_connections", "legacy_migration_key")
	requireIndexAbsent(t, tx, "upstream_connections", "idx_upstream_connections_legacy_migration_key")

	var connectionCount int
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT count(*) FROM upstream_connections WHERE id = $1",
		connectionID,
	).Scan(&connectionCount))
	require.Equal(t, 1, connectionCount)

	var oldSettingCount int
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT count(*) FROM settings WHERE key = 'upstream_billing_probe_settings'",
	).Scan(&oldSettingCount))
	require.Zero(t, oldSettingCount)

	var hasAccountBalance, keepsUnrelated bool
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT value::jsonb ? 'account_balance', value::jsonb ->> 'unrelated' = 'keep'
		FROM settings
		WHERE key = 'ops_alert_runtime_settings'
	`).Scan(&hasAccountBalance, &keepsUnrelated))
	require.False(t, hasAccountBalance)
	require.True(t, keepsUnrelated)
}

func requireColumnAbsent(t *testing.T, tx *sql.Tx, table, column string) {
	t.Helper()

	var exists bool
	err := tx.QueryRowContext(context.Background(), `
SELECT EXISTS (
	SELECT 1
	FROM information_schema.columns
	WHERE table_schema = 'public'
	  AND table_name = $1
	  AND column_name = $2
)
`, table, column).Scan(&exists)
	require.NoError(t, err, "query information_schema.columns for %s.%s", table, column)
	require.False(t, exists, "expected column %s.%s to be absent", table, column)
}
