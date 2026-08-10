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

type migration220BackupRow struct {
	platform         sql.NullString
	videoPrice480P   sql.NullFloat64
	videoPrice720P   sql.NullFloat64
	videoPrice1080P  sql.NullFloat64
	videoModelPrices []byte
	backedUpAt       time.Time
}

func TestMigration220SnapshotsAndClearsOnlyNonGrokVideoPricing(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	migrationSQL, err := dbmigrations.FS.ReadFile("220_clear_non_grok_video_generation_config.sql")
	require.NoError(t, err)

	schemaName := fmt.Sprintf("migration_220_%d", time.Now().UnixNano())
	_, err = tx.ExecContext(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `SET LOCAL search_path TO `+schemaName+`, pg_catalog`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
CREATE TABLE groups (
    id BIGINT PRIMARY KEY,
    name TEXT NOT NULL,
    platform TEXT,
    deleted_at TIMESTAMPTZ,
    video_price_480p NUMERIC(20,8),
    video_price_720p NUMERIC(20,8),
    video_price_1080p NUMERIC(20,8),
    video_model_prices JSONB
);

INSERT INTO groups (
    id, name, platform, deleted_at,
    video_price_480p, video_price_720p, video_price_1080p, video_model_prices
) VALUES
    (1, 'openai-priced', 'openai', NULL,
        0.11, 0.22, 0.33, '{"grok-imagine-video":{"480p":0.44}}'::jsonb),
    (2, 'anthropic-model-only', 'anthropic', NULL,
        NULL, NULL, NULL, '{"grok-imagine-video-1.5":{"1080p":0.55}}'::jsonb),
    (3, 'null-platform', NULL, NULL,
        NULL, 0.66, NULL, NULL),
    (4, 'soft-deleted-gemini', 'gemini', NOW(),
        NULL, NULL, 0.77, NULL),
    (5, 'grok-preserved', 'grok', NULL,
        1.11, 1.22, 1.33, '{"grok-imagine-video":{"720p":1.44}}'::jsonb),
    (6, 'composite-preserved', 'composite', NULL,
        2.11, 2.22, 2.33, '{"grok-imagine-video-1.5":{"1080p":2.44}}'::jsonb),
    (7, 'openai-unconfigured', 'openai', NULL,
        NULL, NULL, NULL, NULL);
`)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	var totalGroups int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups`).Scan(&totalGroups))
	require.Equal(t, 7, totalGroups, "migration must not delete group rows")

	var clearedGroups int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM groups
WHERE id IN (1, 2, 3, 4)
  AND video_price_480p IS NULL
  AND video_price_720p IS NULL
  AND video_price_1080p IS NULL
  AND video_model_prices IS NULL
`).Scan(&clearedGroups))
	require.Equal(t, 4, clearedGroups)

	var softDeletedStillPresent bool
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT deleted_at IS NOT NULL
FROM groups
WHERE id = 4
`).Scan(&softDeletedStillPresent))
	require.True(t, softDeletedStillPresent, "soft-deleted groups are cleared but not deleted")

	var nullPlatformStillNull bool
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT platform IS NULL
FROM groups
WHERE id = 3
`).Scan(&nullPlatformStillNull))
	require.True(t, nullPlatformStillNull)

	var preservedGroups int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM groups
WHERE (id = 5
       AND video_price_480p = 1.11
       AND video_price_720p = 1.22
       AND video_price_1080p = 1.33
       AND video_model_prices = '{"grok-imagine-video":{"720p":1.44}}'::jsonb)
   OR (id = 6
       AND video_price_480p = 2.11
       AND video_price_720p = 2.22
       AND video_price_1080p = 2.33
       AND video_model_prices = '{"grok-imagine-video-1.5":{"1080p":2.44}}'::jsonb)
`).Scan(&preservedGroups))
	require.Equal(t, 2, preservedGroups, "grok and composite pricing must remain unchanged")

	backupBeforeReapply := loadMigration220BackupRows(t, ctx, tx)
	require.Len(t, backupBeforeReapply, 4)
	require.NotContains(t, backupBeforeReapply, int64(5))
	require.NotContains(t, backupBeforeReapply, int64(6))
	require.NotContains(t, backupBeforeReapply, int64(7))

	openAIBackup := backupBeforeReapply[1]
	require.Equal(t, sql.NullString{String: "openai", Valid: true}, openAIBackup.platform)
	require.Equal(t, sql.NullFloat64{Float64: 0.11, Valid: true}, openAIBackup.videoPrice480P)
	require.Equal(t, sql.NullFloat64{Float64: 0.22, Valid: true}, openAIBackup.videoPrice720P)
	require.Equal(t, sql.NullFloat64{Float64: 0.33, Valid: true}, openAIBackup.videoPrice1080P)
	require.JSONEq(t, `{"grok-imagine-video":{"480p":0.44}}`, string(openAIBackup.videoModelPrices))
	require.False(t, openAIBackup.backedUpAt.IsZero())

	modelOnlyBackup := backupBeforeReapply[2]
	require.Equal(t, sql.NullString{String: "anthropic", Valid: true}, modelOnlyBackup.platform)
	require.False(t, modelOnlyBackup.videoPrice480P.Valid)
	require.False(t, modelOnlyBackup.videoPrice720P.Valid)
	require.False(t, modelOnlyBackup.videoPrice1080P.Valid)
	require.JSONEq(t, `{"grok-imagine-video-1.5":{"1080p":0.55}}`, string(modelOnlyBackup.videoModelPrices))

	nullPlatformBackup := backupBeforeReapply[3]
	require.False(t, nullPlatformBackup.platform.Valid)
	require.Equal(t, sql.NullFloat64{Float64: 0.66, Valid: true}, nullPlatformBackup.videoPrice720P)

	softDeletedBackup := backupBeforeReapply[4]
	require.Equal(t, sql.NullString{String: "gemini", Valid: true}, softDeletedBackup.platform)
	require.Equal(t, sql.NullFloat64{Float64: 0.77, Valid: true}, softDeletedBackup.videoPrice1080P)

	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err, "reapplying migration SQL must be safe")
	require.Equal(t, backupBeforeReapply, loadMigration220BackupRows(t, ctx, tx),
		"reapplying must retain the original snapshot without duplicate rows")
}

func loadMigration220BackupRows(
	t *testing.T,
	ctx context.Context,
	tx *sql.Tx,
) map[int64]migration220BackupRow {
	t.Helper()

	rows, err := tx.QueryContext(ctx, `
SELECT group_id,
       platform,
       video_price_480p::double precision,
       video_price_720p::double precision,
       video_price_1080p::double precision,
       video_model_prices,
       backed_up_at
FROM groups_video_price_backup_220
ORDER BY group_id
`)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, rows.Close())
	}()

	result := make(map[int64]migration220BackupRow)
	for rows.Next() {
		var (
			groupID int64
			row     migration220BackupRow
		)
		require.NoError(t, rows.Scan(
			&groupID,
			&row.platform,
			&row.videoPrice480P,
			&row.videoPrice720P,
			&row.videoPrice1080P,
			&row.videoModelPrices,
			&row.backedUpAt,
		))
		result[groupID] = row
	}
	require.NoError(t, rows.Err())
	return result
}
