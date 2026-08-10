//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

const (
	legacyMigration220Checksum  = "353c8e8e1805f2a6fd61311e03118e7dd8388f264cfd9af9e0cabe2a696388c4"
	currentMigration220Checksum = "cf4dbfa75ac27d93a30a6a14439fe7dccfc911c043358363d5ec47946aa0e28b"
)

func TestMigration221RestoresOnlyUntouchedCompositePricingClearedByLegacy220(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	migrationSQL, err := dbmigrations.FS.ReadFile("221_group_media_pricing_auth_cache_invalidation.sql")
	require.NoError(t, err)

	schemaName := fmt.Sprintf("migration_221_%d", time.Now().UnixNano())
	_, err = tx.ExecContext(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `SET LOCAL search_path TO `+schemaName+`, pg_catalog`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
CREATE TABLE schema_migrations (
    filename TEXT PRIMARY KEY,
    checksum TEXT NOT NULL
);
CREATE TABLE groups (
    id BIGINT PRIMARY KEY,
    platform TEXT,
    updated_at TIMESTAMPTZ,
    video_price_480p NUMERIC(20,8),
    video_price_720p NUMERIC(20,8),
    video_price_1080p NUMERIC(20,8),
    video_model_prices JSONB,
    search_price_per_1k NUMERIC(20,8),
    audio_realtime_price_per_min NUMERIC(20,8),
    audio_tts_price_per_million_chars NUMERIC(20,8),
    audio_stt_price_per_hour NUMERIC(20,8)
);
CREATE TABLE groups_video_price_backup_220 (
    group_id BIGINT PRIMARY KEY,
    platform TEXT,
    video_price_480p NUMERIC(20,8),
    video_price_720p NUMERIC(20,8),
    video_price_1080p NUMERIC(20,8),
    video_model_prices JSONB,
    backed_up_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE api_keys (
    group_id BIGINT,
    key TEXT,
    deleted_at TIMESTAMPTZ
);
CREATE TABLE auth_cache_invalidation_outbox (
    cache_key CHAR(64) NOT NULL
);

INSERT INTO groups (
    id, platform, updated_at, video_price_480p, video_price_720p,
    video_price_1080p, video_model_prices
) VALUES
    (1, 'composite', NOW() - INTERVAL '2 hours', NULL, NULL, NULL, NULL),
    (2, 'composite', NOW(), NULL, NULL, NULL, NULL),
    (3, 'composite', NOW() - INTERVAL '2 hours', NULL, 9.99, NULL, NULL),
    (4, 'openai', NOW() - INTERVAL '2 hours', NULL, NULL, NULL, NULL);

INSERT INTO groups_video_price_backup_220 (
    group_id, platform, video_price_480p, video_price_720p,
    video_price_1080p, video_model_prices, backed_up_at
) VALUES
    (1, 'composite', 0.11, 0.22, 0.33, '{"grok-imagine-video":{"720p":0.44}}'::jsonb, NOW() - INTERVAL '1 hour'),
    (2, 'composite', 1.11, 1.22, 1.33, '{"grok-imagine-video":{"720p":1.44}}'::jsonb, NOW() - INTERVAL '1 hour'),
    (3, 'composite', 2.11, 2.22, 2.33, '{"grok-imagine-video":{"720p":2.44}}'::jsonb, NOW() - INTERVAL '1 hour'),
    (4, 'openai', 3.11, 3.22, 3.33, '{"grok-imagine-video":{"720p":3.44}}'::jsonb, NOW() - INTERVAL '1 hour');
`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
INSERT INTO schema_migrations (filename, checksum)
VALUES ('220_clear_non_grok_video_generation_config.sql', $1)
`, legacyMigration220Checksum)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	var (
		price480P  sql.NullFloat64
		price720P  sql.NullFloat64
		price1080P sql.NullFloat64
		modelPrice []byte
	)
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT video_price_480p::double precision,
       video_price_720p::double precision,
       video_price_1080p::double precision,
       video_model_prices
FROM groups
WHERE id = 1
`).Scan(&price480P, &price720P, &price1080P, &modelPrice))
	require.Equal(t, sql.NullFloat64{Float64: 0.11, Valid: true}, price480P)
	require.Equal(t, sql.NullFloat64{Float64: 0.22, Valid: true}, price720P)
	require.Equal(t, sql.NullFloat64{Float64: 0.33, Valid: true}, price1080P)
	require.JSONEq(t, `{"grok-imagine-video":{"720p":0.44}}`, string(modelPrice))

	assertAllVideoPricesNull(t, ctx, tx, 2, "operator update after backup must win")
	var partialPrice sql.NullFloat64
	require.NoError(t, tx.QueryRowContext(ctx,
		`SELECT video_price_720p::double precision FROM groups WHERE id = 3`).Scan(&partialPrice))
	require.Equal(t, sql.NullFloat64{Float64: 9.99, Valid: true}, partialPrice,
		"a partial operator override must not be mixed with the stale snapshot")
	assertAllVideoPricesNull(t, ctx, tx, 4, "non-composite groups must not be restored")

	_, err = tx.ExecContext(ctx, `
UPDATE groups
SET video_price_480p = NULL,
    video_price_720p = NULL,
    video_price_1080p = NULL,
    video_model_prices = NULL,
    updated_at = NOW() - INTERVAL '2 hours'
WHERE id = 1
`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
UPDATE schema_migrations
SET checksum = $1
WHERE filename = '220_clear_non_grok_video_generation_config.sql'
`, currentMigration220Checksum)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	assertAllVideoPricesNull(t, ctx, tx, 1, "the corrected 220 checksum must not trigger legacy restoration")

	_, err = tx.ExecContext(ctx, `DROP TABLE groups_video_price_backup_220`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err, "221 must tolerate operators dropping the documented disposable backup table")
	var backupTableExists bool
	require.NoError(t, tx.QueryRowContext(ctx,
		`SELECT to_regclass('groups_video_price_backup_220') IS NOT NULL`).Scan(&backupTableExists))
	require.True(t, backupTableExists)
}

func assertAllVideoPricesNull(t *testing.T, ctx context.Context, tx *sql.Tx, groupID int64, message string) {
	t.Helper()
	var allNull bool
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT video_price_480p IS NULL
   AND video_price_720p IS NULL
   AND video_price_1080p IS NULL
   AND video_model_prices IS NULL
FROM groups
WHERE id = $1
`, groupID).Scan(&allNull))
	require.True(t, allNull, message)
}
