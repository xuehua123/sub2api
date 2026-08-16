//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration224KeepsCodexFingerprintModeExplicitAcrossMixedVersionWrites(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	migrationSQL, err := dbmigrations.FS.ReadFile("224_codex_fingerprint_mode_explicit_off.sql")
	require.NoError(t, err)

	schemaName := fmt.Sprintf("migration_224_%d", time.Now().UnixNano())
	_, err = tx.ExecContext(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `SET LOCAL search_path TO `+schemaName+`, pg_catalog`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
CREATE TABLE accounts (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    platform TEXT NOT NULL,
    type TEXT NOT NULL,
    extra JSONB NOT NULL DEFAULT '{}'::jsonb,
    deleted_at TIMESTAMPTZ
);
CREATE TABLE scheduler_outbox (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    account_id BIGINT
);
`)
	require.NoError(t, err)

	ids := map[string]int64{}
	for _, row := range []struct {
		name      string
		platform  string
		accountTy string
		extra     string
		deleted   bool
	}{
		{"missing", "openai", "oauth", `{"preserve":"missing"}`, false},
		{"boolean", "openai", "oauth", `{"codex_fingerprint_mode":true,"preserve":"boolean"}`, false},
		{"blank", "openai", "oauth", `{"codex_fingerprint_mode":"   ","preserve":"blank"}`, false},
		{"unknown", "openai", "oauth", `{"codex_fingerprint_mode":"legacy","preserve":"unknown"}`, false},
		{"off", "openai", "oauth", `{"codex_fingerprint_mode":"off","preserve":"off"}`, false},
		{"device", "openai", "oauth", `{"codex_fingerprint_mode":"device","preserve":"device"}`, false},
		{"session", "openai", "oauth", `{"codex_fingerprint_mode":"session","preserve":"session"}`, false},
		{"full", "openai", "oauth", `{"codex_fingerprint_mode":"full","preserve":"full"}`, false},
		{"apikey", "openai", "apikey", `{"codex_fingerprint_mode":"legacy"}`, false},
		{"anthropic", "anthropic", "oauth", `{"codex_fingerprint_mode":"legacy"}`, false},
		{"deleted", "openai", "oauth", `{"codex_fingerprint_mode":"legacy"}`, true},
		{"nonobject-nontarget", "anthropic", "oauth", `[]`, false},
	} {
		query := `
INSERT INTO accounts (name, platform, type, extra, deleted_at)
VALUES ($1, $2, $3, $4::jsonb, CASE WHEN $5 THEN NOW() ELSE NULL END)
RETURNING id
`
		var id int64
		require.NoError(t, tx.QueryRowContext(ctx, query, row.name, row.platform, row.accountTy, row.extra, row.deleted).Scan(&id))
		ids[row.name] = id
	}

	// A re-run must be safe and must not emit a second account_changed event.
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	for _, name := range []string{"missing", "boolean", "blank", "unknown"} {
		var (
			mode     string
			preserve string
		)
		require.NoError(t, tx.QueryRowContext(ctx, `
SELECT extra->>'codex_fingerprint_mode', extra->>'preserve'
FROM accounts
WHERE id = $1
`, ids[name]).Scan(&mode, &preserve))
		require.Equal(t, "off", mode, name)
		require.Equal(t, name, preserve, name)
	}

	for _, name := range []string{"off", "device", "session", "full"} {
		var mode string
		require.NoError(t, tx.QueryRowContext(ctx, `
SELECT extra->>'codex_fingerprint_mode'
FROM accounts
WHERE id = $1
`, ids[name]).Scan(&mode))
		require.Equal(t, name, mode)
	}

	var backfillEvents int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM scheduler_outbox
WHERE event_type = 'account_changed'
`).Scan(&backfillEvents))
	require.Equal(t, 4, backfillEvents)

	for _, name := range []string{"apikey", "anthropic", "deleted"} {
		var mode string
		require.NoError(t, tx.QueryRowContext(ctx, `
SELECT extra->>'codex_fingerprint_mode'
FROM accounts
WHERE id = $1
`, ids[name]).Scan(&mode))
		require.Equal(t, "legacy", mode, name)
	}

	var nonTargetExtra string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT extra::text FROM accounts WHERE id = $1`, ids["nonobject-nontarget"]).Scan(&nonTargetExtra))
	require.JSONEq(t, `[]`, nonTargetExtra)

	// Simulate a v0.1.176.2 writer replacing extra without the new key.
	_, err = tx.ExecContext(ctx, `
UPDATE accounts
SET extra = '{"legacy_writer_replaced_extra":true}'::jsonb
WHERE id = $1
`, ids["session"])
	require.NoError(t, err)
	var legacyWriterMode string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT extra->>'codex_fingerprint_mode'
FROM accounts
WHERE id = $1
`, ids["session"]).Scan(&legacyWriterMode))
	require.Equal(t, "off", legacyWriterMode)

	var insertedMode string
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('mixed-version-insert', 'openai', 'oauth', '{"other":true}'::jsonb)
RETURNING extra->>'codex_fingerprint_mode'
`).Scan(&insertedMode))
	require.Equal(t, "off", insertedMode)

	// Soft-deleted rows stay untouched; restoring one is a live transition and
	// therefore repairs its invalid legacy value.
	_, err = tx.ExecContext(ctx, `UPDATE accounts SET deleted_at = NULL WHERE id = $1`, ids["deleted"])
	require.NoError(t, err)
	var restoredMode string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT extra->>'codex_fingerprint_mode'
FROM accounts
WHERE id = $1
`, ids["deleted"]).Scan(&restoredMode))
	require.Equal(t, "off", restoredMode)

	// A live non-object extra cannot be repaired by overwriting it.  Use a
	// savepoint because PostgreSQL marks the current transaction failed after a
	// trigger exception.
	_, err = tx.ExecContext(ctx, `SAVEPOINT invalid_live_extra`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('invalid-live-extra', 'openai', 'oauth', '[]'::jsonb)
`)
	require.ErrorContains(t, err, "must be a JSON object before enforcing codex_fingerprint_mode")
	_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT invalid_live_extra`)
	require.NoError(t, err)
}

func TestMigration224FailsClosedForHistoricalLiveNonObjectExtra(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	migrationSQL, err := dbmigrations.FS.ReadFile("224_codex_fingerprint_mode_explicit_off.sql")
	require.NoError(t, err)

	schemaName := fmt.Sprintf("migration_224_nonobject_%d", time.Now().UnixNano())
	_, err = tx.ExecContext(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `SET LOCAL search_path TO `+schemaName+`, pg_catalog`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
CREATE TABLE accounts (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    platform TEXT NOT NULL,
    type TEXT NOT NULL,
    extra JSONB NOT NULL DEFAULT '{}'::jsonb,
    deleted_at TIMESTAMPTZ
);
CREATE TABLE scheduler_outbox (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    account_id BIGINT
);
INSERT INTO accounts (name, platform, type, extra)
VALUES ('historical-invalid-live-extra', 'openai', 'oauth', '[]'::jsonb);
`)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `SAVEPOINT historical_invalid_live_extra`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.ErrorContains(t, err, "must be a JSON object before enforcing codex_fingerprint_mode")
	_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT historical_invalid_live_extra`)
	require.NoError(t, err)

	var extra string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT extra::text FROM accounts WHERE name = 'historical-invalid-live-extra'`).Scan(&extra))
	require.JSONEq(t, `[]`, extra)

	var events int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM scheduler_outbox`).Scan(&events))
	require.Zero(t, events)
}
