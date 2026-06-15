package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestValidateConfigRequiresExecuteForWriteModes(t *testing.T) {
	for _, mode := range []string{modeApply, modeSnapshot, modeResumeAPIKeys, modeRollback} {
		t.Run(mode, func(t *testing.T) {
			cfg := config{
				Mode:             mode,
				Env:              envStaging,
				DatabaseURL:      "postgres://user:secret@127.0.0.1/db?sslmode=disable",
				MappingVersion:   "test-version",
				StatementTimeout: time.Minute,
			}

			err := validateConfig(cfg)
			if err == nil || !strings.Contains(err.Error(), "requires -execute") {
				t.Fatalf("expected execute error, got %v", err)
			}
		})
	}
}

func TestValidateConfigRequiresProductionConfirmationForWriteModes(t *testing.T) {
	for _, mode := range []string{modeApply, modeSnapshot, modeResumeAPIKeys, modeRollback} {
		t.Run(mode, func(t *testing.T) {
			cfg := config{
				Mode:             mode,
				Env:              envProduction,
				DatabaseURL:      "postgres://user:secret@127.0.0.1/db?sslmode=disable",
				MappingVersion:   "test-version",
				Execute:          true,
				StatementTimeout: time.Minute,
			}

			err := validateConfig(cfg)
			if err == nil || !strings.Contains(err.Error(), "confirm-production") {
				t.Fatalf("expected production confirmation error, got %v", err)
			}

			cfg.ConfirmProduction = confirmProduction
			if err := validateConfig(cfg); err != nil {
				t.Fatalf("expected valid production write config after confirmation, got %v", err)
			}
		})
	}
}

func TestValidateConfigRejectsProductionDryRunExecute(t *testing.T) {
	cfg := config{
		Mode:             modeDryRun,
		Env:              envProduction,
		DatabaseURL:      "postgres://user:secret@127.0.0.1/db?sslmode=disable",
		MappingVersion:   "test-version",
		Execute:          true,
		StatementTimeout: time.Minute,
	}

	err := validateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "must not use -execute") {
		t.Fatalf("expected production dry-run execute error, got %v", err)
	}
}

func TestRedactSecretHidesDatabasePassword(t *testing.T) {
	got := redactSecret("postgres://user:super-secret@db.internal:5432/sub2api?sslmode=require")
	if strings.Contains(got, "super-secret") {
		t.Fatalf("redacted URL leaked password: %s", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("expected redaction marker, got %s", got)
	}
}

func TestRuntimeGroupKeyIsStableAndDoesNotExposeGroupName(t *testing.T) {
	group := legacyGroup{
		ID:               42,
		Name:             "VIP customer specific tier",
		Platform:         "OpenAI/Codex",
		RateMultiplier:   1.5,
		AccountSignature: "1:50,2:50",
	}

	first := runtimeGroupKey(group)
	second := runtimeGroupKey(group)
	if first != second {
		t.Fatalf("runtime key is not stable: %s != %s", first, second)
	}
	if strings.Contains(first, "VIP") || strings.Contains(first, "customer") {
		t.Fatalf("runtime key leaked source group name: %s", first)
	}
}

func TestRunCLIHelpDoesNotRequireDatabaseURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"-h"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected help exit code 0, got %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "legacy-entitlement-backfill") {
		t.Fatalf("help output missing command name: %s", stdout.String())
	}
}

func TestNullableFloat(t *testing.T) {
	if got := nullableFloat(sql.NullFloat64{}); got != nil {
		t.Fatalf("expected nil for invalid float, got %#v", got)
	}
	if got := nullableFloat(sql.NullFloat64{Valid: true, Float64: 1.25}); got != 1.25 {
		t.Fatalf("expected valid float, got %#v", got)
	}
}

func TestWritePreconditionsRejectWriteModesWhenFlagsAreNotFalse(t *testing.T) {
	for _, mode := range []string{modeApply, modeSnapshot, modeResumeAPIKeys, modeRollback} {
		t.Run(mode, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()

			mock.ExpectQuery("SELECT key, value").
				WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
					AddRow("subscription_entitlements_v2_enabled", "true").
					AddRow("sub2_payment_page_legacy_mapping_enabled", "false"))

			err = ensureWritePreconditions(context.Background(), db, mode, "test-version")
			if err == nil || !strings.Contains(err.Error(), "subscription_entitlements_v2_enabled") {
				t.Fatalf("expected flag gate error, got %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestResumeAPIKeysWritePreconditionsDoNotUseApplyUsageGuard(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT key, value").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
			AddRow("subscription_entitlements_v2_enabled", "false").
			AddRow("sub2_payment_page_legacy_mapping_enabled", "false"))

	if err := ensureWritePreconditions(context.Background(), db, modeResumeAPIKeys, "test-version"); err != nil {
		t.Fatalf("resume-api-keys should pass flag-only write preconditions, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyRejectsWhenTargetedEntitlementUsageAlreadyExists(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT key, value").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
			AddRow("subscription_entitlements_v2_enabled", "false").
			AddRow("sub2_payment_page_legacy_mapping_enabled", "false"))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\)").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(3)))

	err = ensureWritePreconditions(context.Background(), db, modeApply, "test-version")
	if err == nil || !strings.Contains(err.Error(), `targeted by mapping_version "test-version"`) {
		t.Fatalf("expected existing entitlement usage error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyAllowsUnrelatedEntitlementUsage(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT key, value").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
			AddRow("subscription_entitlements_v2_enabled", "false").
			AddRow("sub2_payment_page_legacy_mapping_enabled", "false"))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\)").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	if err := ensureWritePreconditions(context.Background(), db, modeApply, "test-version"); err != nil {
		t.Fatalf("unrelated entitlement usage should not block apply, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDryRunAndReconcileDoNotUseWriteProtection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	for _, mode := range []string{modeDryRun, modeReconcile} {
		if err := ensureWritePreconditions(context.Background(), db, mode, "test-version"); err != nil {
			t.Fatalf("%s should not require write protection, got %v", mode, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotBeforeApplyWritesEligibleLegacySubscriptionKeys(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO api_key_legacy_backfill_snapshots").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"api_key_id"}).
			AddRow(int64(101)).
			AddRow(int64(102)))
	mock.ExpectQuery("SELECT\\s+ak\\.id AS api_key_id").
		WillReturnRows(sqlmock.NewRows([]string{"api_key_id", "user_id", "old_group_id", "mapping_version"}).
			AddRow(int64(101), int64(201), int64(301), "test-version").
			AddRow(int64(102), int64(202), int64(302), "test-version"))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := snapshotEligibleAPIKeys(context.Background(), tx, "test-version")
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("snapshot before apply failed: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if coverage.Captured != 2 || coverage.Reused != 0 || coverage.Covered != 2 || coverage.Missing != 0 {
		t.Fatalf("unexpected coverage: %+v", coverage)
	}
	if len(coverage.Details) != 2 || coverage.Details[0].Status != "newly_captured" || coverage.Details[1].Status != "newly_captured" {
		t.Fatalf("expected newly captured details, got %+v", coverage.Details)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotReusesExistingCoverageFromDifferentMappingVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO api_key_legacy_backfill_snapshots").
		WithArgs("new-version").
		WillReturnRows(sqlmock.NewRows([]string{"api_key_id"}))
	mock.ExpectQuery("SELECT\\s+ak\\.id AS api_key_id").
		WillReturnRows(sqlmock.NewRows([]string{"api_key_id", "user_id", "old_group_id", "mapping_version"}).
			AddRow(int64(101), int64(201), int64(301), "old-version"))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := snapshotEligibleAPIKeys(context.Background(), tx, "new-version")
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("snapshot reuse failed: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if coverage.Captured != 0 || coverage.Reused != 1 || coverage.Covered != 1 || coverage.Missing != 0 {
		t.Fatalf("unexpected coverage: %+v", coverage)
	}
	if len(coverage.Details) != 1 {
		t.Fatalf("expected one detail, got %+v", coverage.Details)
	}
	detail := coverage.Details[0]
	if detail.Status != "reused_existing_snapshot" || detail.SnapshotMappingVersion != "old-version" {
		t.Fatalf("expected reused old snapshot detail, got %+v", detail)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureSnapshotCoverageAllowsExistingCoverageAndRejectsMissing(t *testing.T) {
	if err := ensureSnapshotCoverage(snapshotCoverage{Covered: 1, Reused: 1}); err != nil {
		t.Fatalf("existing snapshot coverage should allow apply, got %v", err)
	}
	err := ensureSnapshotCoverage(snapshotCoverage{
		Covered: 1,
		Missing: 1,
		Details: []snapshotAPIKey{{
			APIKeyID:   11,
			UserID:     22,
			OldGroupID: 33,
			Status:     "missing_snapshot",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "lack snapshot coverage") {
		t.Fatalf("expected missing snapshot coverage error, got %v", err)
	}
}

func TestApplySnapshotCoverageRecordsRedactedMissingDetails(t *testing.T) {
	var sum summary
	applySnapshotCoverage(&sum, snapshotCoverage{
		Captured: 1,
		Reused:   1,
		Covered:  2,
		Missing:  1,
		Details: []snapshotAPIKey{
			{APIKeyID: 10, UserID: 20, OldGroupID: 30, Status: "newly_captured", SnapshotMappingVersion: "new-version"},
			{APIKeyID: 11, UserID: 21, OldGroupID: 31, Status: "reused_existing_snapshot", SnapshotMappingVersion: "old-version"},
			{APIKeyID: 12, UserID: 22, OldGroupID: 32, Status: "missing_snapshot"},
		},
	})
	if sum.CapturedAPIKeys != 1 || sum.ReusedExistingSnapshots != 1 || sum.CoveredAPIKeys != 2 || sum.MissingSnapshotAPIKeys != 1 {
		t.Fatalf("unexpected summary counts: %+v", sum)
	}
	if len(sum.MissingSnapshotAPIKeyDetails) != 1 || sum.MissingSnapshotAPIKeyDetails[0].APIKeyID != 12 {
		t.Fatalf("expected missing snapshot detail, got %+v", sum.MissingSnapshotAPIKeyDetails)
	}
}

func TestMergeApplySummaryPreservesResumeFieldsAndWarnings(t *testing.T) {
	dst := summary{Warnings: []string{"existing"}}
	src := summary{
		CandidateAPIKeys: 3,
		UpdatedAPIKeys:   2,
		SkippedAPIKeys:   1,
		RestartRequired:  true,
		Warnings:         []string{"existing", "restart required"},
	}

	mergeApplySummary(&dst, src)

	if dst.CandidateAPIKeys != 3 || dst.UpdatedAPIKeys != 2 || dst.SkippedAPIKeys != 1 {
		t.Fatalf("resume counters were not merged: %+v", dst)
	}
	if !dst.RestartRequired {
		t.Fatalf("restart_required was not merged: %+v", dst)
	}
	if len(dst.Warnings) != 2 {
		t.Fatalf("warnings should be deduped and merged, got %+v", dst.Warnings)
	}
}

func TestResumeAPIKeysUpdatesCoveredKeysAndRequiresRestart(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\)\\s+FROM subscription_legacy_backfill_mappings").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("WITH runtime_groups AS").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery("WITH source_candidates AS").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("WITH source_candidates AS").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"api_key_id", "user_id", "old_group_id", "mapping_version"}).
			AddRow(int64(101), int64(201), int64(301), "old-version"))
	mock.ExpectQuery("WITH source_candidates AS").
		WithArgs("test-version", sourceLegacyBackfill).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectExec("WITH source_candidates AS").
		WithArgs("test-version", sourceLegacyBackfill).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := resumeAPIKeys(context.Background(), tx, "test-version")
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("resume-api-keys failed: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	if sum.CandidateAPIKeys != 1 || sum.UpdatedAPIKeys != 1 || sum.SkippedAPIKeys != 0 {
		t.Fatalf("unexpected resume counters: %+v", sum)
	}
	if sum.ReusedExistingSnapshots != 1 || sum.CoveredAPIKeys != 1 || sum.MissingSnapshotAPIKeys != 0 {
		t.Fatalf("unexpected snapshot coverage: %+v", sum)
	}
	if !sum.RestartRequired || len(sum.Warnings) == 0 {
		t.Fatalf("resume should flag API key cache restart requirement: %+v", sum)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResumeAPIKeysRejectsMissingSnapshotCoverage(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\)\\s+FROM subscription_legacy_backfill_mappings").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("WITH runtime_groups AS").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery("WITH source_candidates AS").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("WITH source_candidates AS").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"api_key_id", "user_id", "old_group_id", "mapping_version"}).
			AddRow(int64(101), int64(201), int64(301), nil))
	mock.ExpectRollback()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = resumeAPIKeys(context.Background(), tx, "test-version")
	if err == nil || !strings.Contains(err.Error(), "lack snapshot coverage") {
		_ = tx.Rollback()
		t.Fatalf("expected missing snapshot coverage error, got %v", err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResumeAPIKeysRejectsInvalidEntitlementTarget(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\)\\s+FROM subscription_legacy_backfill_mappings").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("WITH runtime_groups AS").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery("WITH source_candidates AS").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("WITH source_candidates AS").
		WithArgs("test-version").
		WillReturnRows(sqlmock.NewRows([]string{"api_key_id", "user_id", "old_group_id", "mapping_version"}).
			AddRow(int64(101), int64(201), int64(301), "old-version"))
	mock.ExpectQuery("WITH source_candidates AS").
		WithArgs("test-version", sourceLegacyBackfill).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectRollback()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = resumeAPIKeys(context.Background(), tx, "test-version")
	if err == nil || !strings.Contains(err.Error(), "do not have exactly one valid legacy entitlement") {
		_ = tx.Rollback()
		t.Fatalf("expected invalid entitlement target error, got %v", err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResumeAPIKeysRejectsMissingMappingVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\)\\s+FROM subscription_legacy_backfill_mappings").
		WithArgs("missing-version").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectRollback()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = resumeAPIKeys(context.Background(), tx, "missing-version")
	if err == nil || !strings.Contains(err.Error(), "has no subscription_legacy_backfill_mappings rows") {
		_ = tx.Rollback()
		t.Fatalf("expected missing mapping version error, got %v", err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWriteEvidenceWritesAmbiguousAPIKeyDetailsWithoutSecrets(t *testing.T) {
	dir := t.TempDir()
	sum := summary{
		Mode:           modeDryRun,
		Env:            envStaging,
		MappingVersion: "test-version",
		AmbiguousAPIKeyDetails: []ambiguousAPIKey{
			{
				APIKeyID:                 11,
				UserID:                   22,
				OldGroupID:               33,
				Reason:                   "multiple_active_legacy_subscriptions",
				CandidateSubscriptionIDs: []int64{44, 55},
				ProposedRuntimeGroupKey:  "openai-rate-1_5000-pool-deadbeef",
			},
		},
	}

	if err := writeEvidence(dir, sum); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "legacy-backfill-ambiguous-api-keys-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one ambiguous evidence file, got %d", len(matches))
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, want := range []string{`"api_key_id": 11`, `"user_id": 22`, `"old_group_id": 33`, `"candidate_subscription_ids"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("ambiguous evidence missing %s: %s", want, text)
		}
	}
	for _, forbidden := range []string{"sk-", "token", "password", "email", "source_external_id", "notes"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("ambiguous evidence leaked forbidden marker %q: %s", forbidden, text)
		}
	}
}

func TestRollbackSQLOnlyRestoresAPIKeys(t *testing.T) {
	upper := strings.ToUpper(rollbackAPIKeysSQL)
	if !strings.Contains(upper, "UPDATE API_KEYS") {
		t.Fatalf("rollback SQL should update api_keys: %s", rollbackAPIKeysSQL)
	}
	if strings.Contains(upper, "S.MAPPING_VERSION") {
		t.Fatalf("rollback SQL must reuse snapshots across mapping versions: %s", rollbackAPIKeysSQL)
	}
	for _, forbidden := range []string{
		"DELETE FROM SUBSCRIPTION_ENTITLEMENTS",
		"DELETE FROM SUBSCRIPTION_ENTITLEMENT_FULFILLMENTS",
		"UPDATE SUBSCRIPTION_ENTITLEMENTS",
		"UPDATE SUBSCRIPTION_ENTITLEMENT_FULFILLMENTS",
	} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("rollback SQL must not mutate entitlement history, found %s", forbidden)
		}
	}
}

func TestEnsureMappingRejectsVersionMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT legacy_group_id, plan_id, runtime_group_id, runtime_group_key, mapping_version").
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{
			"legacy_group_id", "plan_id", "runtime_group_id", "runtime_group_key", "mapping_version",
		}).AddRow(int64(42), int64(100), int64(200), "openai-rate-1_0000-pool-old", "old-version"))
	mock.ExpectRollback()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, err = ensureMapping(context.Background(), tx, "requested-version", legacyGroup{ID: 42})
	if err == nil || !strings.Contains(err.Error(), `legacy_group_id 42`) || !strings.Contains(err.Error(), `"old-version"`) || !strings.Contains(err.Error(), `"requested-version"`) {
		t.Fatalf("expected mapping version mismatch error, got %v", err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyBackfillRejectsActiveButUnschedulableAccountPool(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT\\s+g\\.id,").
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"name",
			"description",
			"platform",
			"rate_multiplier",
			"daily_limit_usd",
			"weekly_limit_usd",
			"monthly_limit_usd",
			"default_validity_days",
			"sort_order",
			"is_exclusive",
			"status",
			"account_signature",
			"active_account_count",
			"schedulable_accounts",
		}).AddRow(
			int64(7),
			"legacy tier",
			nil,
			"openai",
			float64(1.5),
			nil,
			nil,
			nil,
			30,
			0,
			false,
			"active",
			"101:50",
			int64(1),
			int64(0),
		))
	mock.ExpectRollback()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = applyBackfill(context.Background(), tx, config{MappingVersion: "test-version"})
	if err == nil || !strings.Contains(err.Error(), "no active schedulable account pool") {
		t.Fatalf("expected no schedulable account pool error, got %v", err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
