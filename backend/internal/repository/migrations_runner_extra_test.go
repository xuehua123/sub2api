package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestApplyMigrations_NilDB(t *testing.T) {
	err := ApplyMigrations(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil sql db")
}

func TestApplyMigrations_DelegatesToApplyMigrationsFS(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT pg_try_advisory_lock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnError(errors.New("lock failed"))

	err = ApplyMigrations(context.Background(), db)
	require.Error(t, err)
	require.Contains(t, err.Error(), "acquire migrations lock")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLatestMigrationBaseline(t *testing.T) {
	t.Run("empty_fs_returns_baseline", func(t *testing.T) {
		version, description, hash, err := latestMigrationBaseline(fstest.MapFS{})
		require.NoError(t, err)
		require.Equal(t, "baseline", version)
		require.Equal(t, "baseline", description)
		require.Equal(t, "", hash)
	})

	t.Run("uses_latest_sorted_sql_file", func(t *testing.T) {
		fsys := fstest.MapFS{
			"001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE t1(id int);")},
			"010_final.sql": &fstest.MapFile{
				Data: []byte("CREATE TABLE t2(id int);"),
			},
		}
		version, description, hash, err := latestMigrationBaseline(fsys)
		require.NoError(t, err)
		require.Equal(t, "010_final", version)
		require.Equal(t, "010_final", description)
		require.Len(t, hash, 64)
	})

	t.Run("read_file_error", func(t *testing.T) {
		fsys := fstest.MapFS{
			"010_bad.sql": &fstest.MapFile{Mode: fs.ModeDir},
		}
		_, _, _, err := latestMigrationBaseline(fsys)
		require.Error(t, err)
	})
}

func TestIsMigrationChecksumCompatible_AdditionalCases(t *testing.T) {
	require.False(t, isMigrationChecksumCompatible("unknown.sql", "db", "file"))

	var (
		name string
		rule migrationChecksumCompatibilityRule
	)
	for n, r := range migrationChecksumCompatibilityRules {
		name = n
		rule = r
		break
	}
	require.NotEmpty(t, name)

	require.False(t, isMigrationChecksumCompatible(name, "db-not-accepted", "file-not-match"))
	require.False(t, isMigrationChecksumCompatible(name, "db-not-accepted", rule.fileChecksum))

	var accepted string
	for checksum := range rule.acceptedDBChecksum {
		accepted = checksum
		break
	}
	require.NotEmpty(t, accepted)
	require.True(t, isMigrationChecksumCompatible(name, accepted, rule.fileChecksum))
}

func TestMigrationChecksumCompatibilityRules_CoverEditedUpgradeCompatibilityMigrations(t *testing.T) {
	for _, name := range []string{
		"109_auth_identity_compat_backfill.sql",
		"110_pending_auth_and_provider_default_grants.sql",
		"112_add_payment_order_provider_key_snapshot.sql",
		"115_auth_identity_legacy_external_backfill.sql",
		"116_auth_identity_legacy_external_safety_reports.sql",
		"118_wechat_dual_mode_and_auth_source_defaults.sql",
		"120_enforce_payment_orders_out_trade_no_unique_notx.sql",
		"123_fix_legacy_auth_source_grant_on_signup_defaults.sql",
	} {
		rule, ok := migrationChecksumCompatibilityRules[name]
		require.Truef(t, ok, "missing compatibility rule for %s", name)
		require.NotEmpty(t, rule.fileChecksum)
		require.NotEmpty(t, rule.acceptedDBChecksum)
	}
}

func TestMigrationChecksumCompatibilityRules_MatchRunnerChecksums(t *testing.T) {
	knownHistoricalChecksums := map[string][]string{
		"109_auth_identity_compat_backfill.sql": {
			"748ddcdc60f93a1ac562ce8a66ee870f64ee594bf6dbedad55ed8baf3c75b28c",
			"0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace",
			"551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee",
		},
		"110_pending_auth_and_provider_default_grants.sql": {
			"301e90405b3424967b7d1931568b7a244902148fa82802f362c115ae4e2ae2ef",
			"32cf87ee787b1bb36b5c691367c96eee37518fa3eed6f3322cf68795e3745279",
			"e3d1f433be2b564cfbdc549adf98fce13c5c7b363ebc20fd05b765d0563b0925",
		},
		"112_add_payment_order_provider_key_snapshot.sql": {
			"d4476c67ceea871aa2d92ee2a603795a742d0379a58cf53938bb9aa559ff9caa",
			"b75f8f56d39455682787696a3d92ad25b055444ca328fb7fca9a460a15d68d99",
			"ffd3e8a2c9295fa9cbefefd629a78268877e5b51bc970a82d9b3f46ec4ebd15e",
		},
		"118_wechat_dual_mode_and_auth_source_defaults.sql": {
			"b4a5b7a28f6a7ac67aad214645761e5a8486c83f0f2a1a874d7f67085f83159b",
			"6395ad255f2be2219ad85813b72db6fa7783c81d747e42e098847ef3594f1674",
			"b54194d7a3e4fbf710e0a3590d22a2fe7966804c487052a356e0b55f53ef96b0",
			"e0cdf835d6c688d64100f483d31bc02ac9ebad414bf1837af239a84bf75b8227",
			"a38243ca0a72c3a01c0a92b7986423054d6133c0399441f853b99802852720fb",
		},
		"120_enforce_payment_orders_out_trade_no_unique_notx.sql": {
			"e77921f79d539bc24575cb9c16cbe566d2b23ce816190343d0a7568f6a3fcf61",
			"79ea6127a22e61b3bad6ea29347a8cc3ff005f8b486ef4a51bd04fdda906f931",
			"707431450603e70a43ce9fbd61e0c12fa67da4875158ccefabacea069587ab22",
			"04b082b5a239c525154fe9185d324ee2b05ff90da9297e10dba19f9be79aa59a",
		},
		"123_fix_legacy_auth_source_grant_on_signup_defaults.sql": {
			"ac0d79ca6feb449674f54f593a5eac5f7cc06751047c664b586c1892e19c60d5",
			"ea17c2767b937f08274e091d212a93acb7e2d62521129179830f073a291fbd97",
			"dea22b2899ae6530daf44419e7f44e40ccdcdc96d2bea7584af0c6c4c0ee461b",
			"2ce43c2cd89e9f9e1febd34a407ed9e84d177386c5544b6f02c1f58a21129f57",
			"6cd33422f215dcd1f486ab6f35c0ea5805d9ca69bb25906d94bc649156657145",
		},
		"195_channel_monitor_mode.sql": {
			"f20366e106e3a54c73d4a67df3ba87734427ed859bc4ae42b0708e4cbcbacb56",
			"13f3792f3e3e53ee96e26415c884cf8062c77172824b54fcc9a8c0c2b1f185ec",
			"4c74fe33ef2274cc72e1bb49671e651274532c034b29f5b2982c2a4c88d101a6",
		},
		"218_group_audio_voice_pricing.sql": {
			"343a955e52348ce92c35753e78ca3f8e5a76060c20af71061ca5e04c6ed84085",
			"40ee9f3a2af0e0a5e99dabc878fd0fe98be1011f26bcfcefcac7197f7081f0e7",
			"c2a5e5b4ffd6968ad1c10593289fbc11192cdea19fec3ed9bce3a84eff9a8351",
		},
		"219_group_search_price_per_1k.sql": {
			"833578274d0eed24d39355298d5659b33e5484c869b331ffd815187c221552d2",
			"e86786ebcc3b14206fd2d321380a4e50e80cdadbfcf4962c639255e6a14008db",
			"df6ffd71b97e30ec2c8fe7b95e15783042dea58c553e32701ee7c42a5619af80",
		},
		"220_clear_non_grok_video_generation_config.sql": {
			"353c8e8e1805f2a6fd61311e03118e7dd8388f264cfd9af9e0cabe2a696388c4",
			"85e320b9ec64f2d3fcd8cf705b2b4e76a7b49f7a57140c14bff97f32691c818b",
			"3da48c8fdffe6390325f43d08b8e353e0a365df43d44a78dbbe655d0deb18402",
		},
	}
	for name, rule := range migrationChecksumCompatibilityRules {
		t.Run(name, func(t *testing.T) {
			require.Regexp(t, `^[0-9a-f]{64}$`, rule.fileChecksum)
			for checksum := range rule.acceptedChecksums {
				require.Regexp(t, `^[0-9a-f]{64}$`, checksum)
			}

			content, err := dbmigrations.FS.ReadFile(name)
			require.NoError(t, err)
			sum := sha256.Sum256([]byte(strings.TrimSpace(string(content))))
			currentChecksum := hex.EncodeToString(sum[:])
			require.Equalf(t, currentChecksum, rule.fileChecksum,
				"compatibility rule must use the runner's trimmed checksum")
			for _, legacyChecksum := range knownHistoricalChecksums[name] {
				require.Truef(t, isMigrationChecksumCompatible(name, legacyChecksum, currentChecksum),
					"compatibility rule must accept legacy runner checksum %s", legacyChecksum)
			}
		})
	}

	t.Run("legacy_220_without_backup_is_rejected", func(t *testing.T) {
		const migrationName = "220_clear_non_grok_video_generation_config.sql"
		content, err := dbmigrations.FS.ReadFile(migrationName)
		require.NoError(t, err)
		sum := sha256.Sum256([]byte(strings.TrimSpace(string(content))))
		currentChecksum := hex.EncodeToString(sum[:])

		for _, unrecoverableChecksum := range []string{
			"3d08d905a7bca1f56f14b6d2a2a0dcb07480ff52c21393b4e2db1b3a3f83b3d0",
			"e7942a7201f9a0d35e78275fbbe4eca82ac25e4a3741920e45bcd1054e0522a8",
		} {
			require.False(t, isMigrationChecksumCompatible(migrationName, unrecoverableChecksum, currentChecksum),
				"220 versions without a recovery snapshot must remain blocked")
		}
	})

	t.Run("known_raw_checksum_is_preserved_exactly", func(t *testing.T) {
		const migrationName = "123_fix_legacy_auth_source_grant_on_signup_defaults.sql"
		rule := migrationChecksumCompatibilityRules[migrationName]
		_, ok := rule.acceptedChecksums["2ce43c2cd89e9f9e1febd34a407ed9e84d177386c5544b6f02c1f58a21129f57"]
		require.True(t, ok)
	})
}

func TestEnsureAtlasBaselineAligned(t *testing.T) {
	t.Run("skip_when_no_legacy_table", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("schema_migrations").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		err = ensureAtlasBaselineAligned(context.Background(), db, fstest.MapFS{})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("create_atlas_and_insert_baseline_when_empty", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("schema_migrations").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("atlas_schema_revisions").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS atlas_schema_revisions").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM atlas_schema_revisions").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectExec("INSERT INTO atlas_schema_revisions").
			WithArgs("002_next", "002_next", 1, sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		fsys := fstest.MapFS{
			"001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE t1(id int);")},
			"002_next.sql": &fstest.MapFile{Data: []byte("CREATE TABLE t2(id int);")},
		}
		err = ensureAtlasBaselineAligned(context.Background(), db, fsys)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error_when_checking_legacy_table", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("schema_migrations").
			WillReturnError(errors.New("exists failed"))

		err = ensureAtlasBaselineAligned(context.Background(), db, fstest.MapFS{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "check schema_migrations")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error_when_counting_atlas_rows", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("schema_migrations").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("atlas_schema_revisions").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM atlas_schema_revisions").
			WillReturnError(errors.New("count failed"))

		err = ensureAtlasBaselineAligned(context.Background(), db, fstest.MapFS{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "count atlas_schema_revisions")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error_when_creating_atlas_table", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("schema_migrations").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("atlas_schema_revisions").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS atlas_schema_revisions").
			WillReturnError(errors.New("create failed"))

		err = ensureAtlasBaselineAligned(context.Background(), db, fstest.MapFS{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "create atlas_schema_revisions")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error_when_inserting_baseline", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("schema_migrations").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT EXISTS \\(").
			WithArgs("atlas_schema_revisions").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM atlas_schema_revisions").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectExec("INSERT INTO atlas_schema_revisions").
			WithArgs("001_init", "001_init", 1, sqlmock.AnyArg()).
			WillReturnError(errors.New("insert failed"))

		fsys := fstest.MapFS{
			"001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE t(id int);")},
		}
		err = ensureAtlasBaselineAligned(context.Background(), db, fsys)
		require.Error(t, err)
		require.Contains(t, err.Error(), "insert atlas baseline")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestApplyMigrationsFS_ChecksumMismatchRejected(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs("001_init.sql").
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow("mismatched-checksum"))
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE t(id int);")},
	}
	err = applyMigrationsFS(context.Background(), db, fsys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "checksum mismatch")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_CheckMigrationQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs("001_err.sql").
		WillReturnError(errors.New("query failed"))
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"001_err.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
	}
	err = applyMigrationsFS(context.Background(), db, fsys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "check migration 001_err.sql")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_SkipEmptyAndAlreadyApplied(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)

	alreadySQL := "CREATE TABLE t(id int);"
	checksum := migrationChecksum(alreadySQL)
	mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").
		WithArgs("001_already.sql").
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(checksum))
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"000_empty.sql":   &fstest.MapFile{Data: []byte("   \n\t ")},
		"001_already.sql": &fstest.MapFile{Data: []byte(alreadySQL)},
	}
	err = applyMigrationsFS(context.Background(), db, fsys)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyMigrationsFS_ReadMigrationError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(migrationsAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	fsys := fstest.MapFS{
		"001_bad.sql": &fstest.MapFile{Mode: fs.ModeDir},
	}
	err = applyMigrationsFS(context.Background(), db, fsys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read migration 001_bad.sql")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPgAdvisoryLockAndUnlock_ErrorBranches(t *testing.T) {
	t.Run("context_cancelled_while_not_locked", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT pg_try_advisory_lock\\(\\$1\\)").
			WithArgs(migrationsAdvisoryLockID).
			WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()
		err = pgAdvisoryLock(ctx, db)
		require.Error(t, err)
		require.Contains(t, err.Error(), "acquire migrations lock")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("unlock_exec_error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
			WithArgs(migrationsAdvisoryLockID).
			WillReturnError(errors.New("unlock failed"))

		err = pgAdvisoryUnlock(context.Background(), db)
		require.Error(t, err)
		require.Contains(t, err.Error(), "release migrations lock")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("acquire_lock_after_retry", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("SELECT pg_try_advisory_lock\\(\\$1\\)").
			WithArgs(migrationsAdvisoryLockID).
			WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))
		mock.ExpectQuery("SELECT pg_try_advisory_lock\\(\\$1\\)").
			WithArgs(migrationsAdvisoryLockID).
			WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))

		ctx, cancel := context.WithTimeout(context.Background(), migrationsLockRetryInterval*3)
		defer cancel()
		start := time.Now()
		err = pgAdvisoryLock(ctx, db)
		require.NoError(t, err)
		require.GreaterOrEqual(t, time.Since(start), migrationsLockRetryInterval)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func migrationChecksum(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return hex.EncodeToString(sum[:])
}
