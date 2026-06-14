package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

const (
	modeDryRun    = "dry-run"
	modeApply     = "apply"
	modeSnapshot  = "snapshot"
	modeRollback  = "rollback"
	modeReconcile = "reconcile"

	envLocal      = "local"
	envStaging    = "staging"
	envProduction = "production"

	sourceLegacyBackfill = "legacy_subscription_backfill"
	confirmProduction    = "CONFIRM_PRODUCTION_LEGACY_ENTITLEMENT_BACKFILL"
)

const rollbackAPIKeysSQL = `
UPDATE api_keys ak
SET
    group_id = s.old_group_id,
    access_source = COALESCE(NULLIF(s.old_access_source, ''), CASE WHEN s.old_subscription_entitlement_id IS NULL THEN 'balance' ELSE 'entitlement' END),
    subscription_entitlement_id = s.old_subscription_entitlement_id,
    updated_at = NOW()
FROM api_key_legacy_backfill_snapshots s
WHERE ak.id = s.api_key_id
  AND (
      ak.group_id IN (
          SELECT runtime_group_id FROM subscription_legacy_backfill_mappings WHERE mapping_version = $1
      )
      OR ak.subscription_entitlement_id IN (
          SELECT se2.id
          FROM subscription_entitlements se2
          JOIN user_subscriptions us2 ON us2.id = se2.legacy_subscription_id
          JOIN subscription_legacy_backfill_mappings m2 ON m2.legacy_group_id = us2.group_id
          WHERE m2.mapping_version = $1
            AND se2.source_type = $2
            AND se2.deleted_at IS NULL
      )
  )`

type config struct {
	Mode                string
	Env                 string
	DatabaseURL         string
	DatabaseURLEnv      string
	MappingVersion      string
	OutputDir           string
	Execute             bool
	ConfirmProduction   string
	StatementTimeout    time.Duration
	PrintReconcileSQL   bool
	AllowNoAccountPools bool
}

type summary struct {
	Mode                          string            `json:"mode"`
	Env                           string            `json:"env"`
	MappingVersion                string            `json:"mapping_version"`
	DryRun                        bool              `json:"dry_run"`
	GeneratedAt                   string            `json:"generated_at"`
	DatabaseURL                   string            `json:"database_url"`
	RuntimeGroupCandidates        int64             `json:"runtime_group_candidates"`
	LegacyExclusiveSources        int64             `json:"legacy_exclusive_sources"`
	PlanCandidates                int64             `json:"plan_candidates"`
	ActiveLegacySubscriptions     int64             `json:"active_legacy_subscriptions"`
	SkippedLegacySubscriptions    int64             `json:"skipped_legacy_subscriptions"`
	APIKeyAutoMigrationCandidates int64             `json:"api_key_auto_migration_candidates"`
	AmbiguousAPIKeys              int64             `json:"ambiguous_api_keys"`
	ReviewGroups                  int64             `json:"review_groups"`
	CreatedRuntimeGroups          int64             `json:"created_runtime_groups,omitempty"`
	CreatedPlans                  int64             `json:"created_plans,omitempty"`
	CreatedMappings               int64             `json:"created_mappings,omitempty"`
	UpdatedEntitlements           int64             `json:"updated_entitlements,omitempty"`
	UpdatedEntitlementGrants      int64             `json:"updated_entitlement_grants,omitempty"`
	UpsertedFulfillments          int64             `json:"upserted_fulfillments,omitempty"`
	SnapshottedAPIKeys            int64             `json:"snapshotted_api_keys,omitempty"`
	CapturedAPIKeys               int64             `json:"captured_api_keys,omitempty"`
	ReusedExistingSnapshots       int64             `json:"reused_existing_snapshots,omitempty"`
	CoveredAPIKeys                int64             `json:"covered_api_keys,omitempty"`
	MissingSnapshotAPIKeys        int64             `json:"missing_snapshot_api_keys,omitempty"`
	UpdatedAPIKeys                int64             `json:"updated_api_keys,omitempty"`
	RolledBackAPIKeys             int64             `json:"rolled_back_api_keys,omitempty"`
	Reconciliation                map[string]int64  `json:"reconciliation,omitempty"`
	ReviewReasons                 map[string]int64  `json:"review_reasons,omitempty"`
	AmbiguousAPIKeyDetails        []ambiguousAPIKey `json:"ambiguous_api_key_details,omitempty"`
	SnapshotAPIKeyDetails         []snapshotAPIKey  `json:"snapshot_api_key_details,omitempty"`
	MissingSnapshotAPIKeyDetails  []snapshotAPIKey  `json:"missing_snapshot_api_key_details,omitempty"`
	Warnings                      []string          `json:"warnings,omitempty"`
	PostWriteReconciliationSQL    []string          `json:"post_write_reconciliation_sql,omitempty"`
}

type ambiguousAPIKey struct {
	APIKeyID                 int64   `json:"api_key_id"`
	UserID                   int64   `json:"user_id"`
	OldGroupID               int64   `json:"old_group_id"`
	Reason                   string  `json:"reason"`
	CandidateSubscriptionIDs []int64 `json:"candidate_subscription_ids"`
	ProposedRuntimeGroupID   *int64  `json:"proposed_runtime_group_id,omitempty"`
	ProposedRuntimeGroupKey  string  `json:"proposed_runtime_group_key"`
	ExistingMappingVersion   string  `json:"existing_mapping_version,omitempty"`
}

type snapshotAPIKey struct {
	APIKeyID               int64  `json:"api_key_id"`
	UserID                 int64  `json:"user_id"`
	OldGroupID             int64  `json:"old_group_id"`
	Status                 string `json:"status"`
	SnapshotMappingVersion string `json:"snapshot_mapping_version,omitempty"`
}

type snapshotCoverage struct {
	Captured int64
	Reused   int64
	Covered  int64
	Missing  int64
	Details  []snapshotAPIKey
}

type legacyGroup struct {
	ID                  int64
	Name                string
	Description         sql.NullString
	Platform            string
	RateMultiplier      float64
	DailyLimitUSD       sql.NullFloat64
	WeeklyLimitUSD      sql.NullFloat64
	MonthlyLimitUSD     sql.NullFloat64
	DefaultValidityDays int
	SortOrder           int
	IsExclusive         bool
	Status              string
	AccountSignature    string
	ActiveAccountCount  int64
	SchedulableAccounts int64
}

type mapping struct {
	LegacyGroupID   int64
	RuntimeGroupID  int64
	PlanID          int64
	RuntimeGroupKey string
	MappingVersion  string
	Existed         bool
}

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	cfg, helpRequested, err := parseConfig(args, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if helpRequested {
		return 0
	}
	if err := validateConfig(cfg); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.StatementTimeout)
	defer cancel()

	sum, err := execute(ctx, cfg)
	if err != nil {
		if cfg.OutputDir != "" {
			_ = writeEvidence(cfg.OutputDir, sum)
		}
		if len(sum.MissingSnapshotAPIKeyDetails) > 0 {
			writeSummary(stdout, sum)
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	writeSummary(stdout, sum)
	if cfg.OutputDir != "" {
		if err := writeEvidence(cfg.OutputDir, sum); err != nil {
			fmt.Fprintf(stderr, "error writing evidence: %v\n", err)
			return 1
		}
	}
	return 0
}

func parseConfig(args []string, output io.Writer) (config, bool, error) {
	cfg := config{
		Mode:             modeDryRun,
		Env:              envStaging,
		DatabaseURLEnv:   "DATABASE_URL",
		MappingVersion:   "legacy-backfill-v1",
		StatementTimeout: 5 * time.Minute,
	}
	fs := flag.NewFlagSet("legacy-entitlement-backfill", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&cfg.Mode, "mode", cfg.Mode, "Mode: dry-run, apply, snapshot, rollback, reconcile")
	fs.StringVar(&cfg.Env, "env", cfg.Env, "Target environment label: local, staging, production")
	fs.StringVar(&cfg.DatabaseURL, "database-url", "", "PostgreSQL URL. Prefer DATABASE_URL env var; value is never printed")
	fs.StringVar(&cfg.DatabaseURLEnv, "database-url-env", cfg.DatabaseURLEnv, "Environment variable that contains PostgreSQL URL")
	fs.StringVar(&cfg.MappingVersion, "mapping-version", cfg.MappingVersion, "Backfill mapping version/audit label")
	fs.StringVar(&cfg.OutputDir, "output-dir", "", "Optional evidence directory for redacted JSON summary")
	fs.BoolVar(&cfg.Execute, "execute", false, "Required for write modes: apply, snapshot, rollback")
	fs.StringVar(&cfg.ConfirmProduction, "confirm-production", "", "Required exact confirmation string for production write modes")
	fs.DurationVar(&cfg.StatementTimeout, "timeout", cfg.StatementTimeout, "Maximum runtime for DB operations")
	fs.BoolVar(&cfg.PrintReconcileSQL, "print-reconcile-sql", false, "Include post-write reconciliation SQL snippets in output")
	fs.BoolVar(&cfg.AllowNoAccountPools, "allow-no-account-pools", false, "Allow apply even when a legacy subscription group has no active schedulable accounts")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return cfg, true, nil
		}
		return cfg, false, err
	}
	if cfg.DatabaseURL == "" && cfg.DatabaseURLEnv != "" {
		cfg.DatabaseURL = os.Getenv(cfg.DatabaseURLEnv)
	}
	return cfg, false, nil
}

func validateConfig(cfg config) error {
	cfg.Mode = strings.ToLower(strings.TrimSpace(cfg.Mode))
	switch cfg.Mode {
	case modeDryRun, modeApply, modeSnapshot, modeRollback, modeReconcile:
	default:
		return fmt.Errorf("unsupported mode %q", cfg.Mode)
	}

	cfg.Env = strings.ToLower(strings.TrimSpace(cfg.Env))
	switch cfg.Env {
	case envLocal, envStaging, envProduction:
	default:
		return fmt.Errorf("unsupported env %q", cfg.Env)
	}

	if strings.TrimSpace(cfg.MappingVersion) == "" {
		return errors.New("mapping-version is required")
	}
	if len(cfg.MappingVersion) > 64 {
		return errors.New("mapping-version must be 64 characters or fewer")
	}
	if cfg.StatementTimeout <= 0 {
		return errors.New("timeout must be positive")
	}
	writeMode := cfg.Mode == modeApply || cfg.Mode == modeSnapshot || cfg.Mode == modeRollback
	if writeMode && !cfg.Execute {
		return fmt.Errorf("%s mode requires -execute", cfg.Mode)
	}
	if cfg.Env == envProduction {
		if cfg.Mode == modeDryRun && cfg.Execute {
			return errors.New("production dry-run must not use -execute")
		}
		if writeMode && cfg.ConfirmProduction != confirmProduction {
			return fmt.Errorf("production %s requires -confirm-production %s", cfg.Mode, confirmProduction)
		}
	}
	if cfg.DatabaseURL == "" {
		return errors.New("database URL is required via -database-url or DATABASE_URL")
	}
	return nil
}

func execute(ctx context.Context, cfg config) (summary, error) {
	sum := summary{
		Mode:                       cfg.Mode,
		Env:                        cfg.Env,
		MappingVersion:             cfg.MappingVersion,
		DryRun:                     cfg.Mode == modeDryRun || !cfg.Execute,
		GeneratedAt:                time.Now().UTC().Format(time.RFC3339),
		DatabaseURL:                redactSecret(cfg.DatabaseURL),
		ReviewReasons:              map[string]int64{},
		Reconciliation:             map[string]int64{},
		Warnings:                   []string{},
		PostWriteReconciliationSQL: nil,
	}
	if cfg.PrintReconcileSQL {
		sum.PostWriteReconciliationSQL = reconciliationSQL(cfg.MappingVersion)
	}

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return sum, err
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		return sum, err
	}

	if err := collectReadOnlySummary(ctx, db, &sum); err != nil {
		return sum, err
	}
	details, err := collectAmbiguousAPIKeys(ctx, db)
	if err != nil {
		return sum, err
	}
	sum.AmbiguousAPIKeyDetails = details

	switch cfg.Mode {
	case modeDryRun:
		return sum, nil
	case modeReconcile:
		if err := collectReconciliation(ctx, db, cfg.MappingVersion, &sum); err != nil {
			return sum, err
		}
		return sum, nil
	case modeSnapshot:
		if err := ensureWritePreconditions(ctx, db, cfg.Mode, cfg.MappingVersion); err != nil {
			return sum, err
		}
		if err := withTx(ctx, db, false, func(tx *sql.Tx) error {
			coverage, err := snapshotEligibleAPIKeys(ctx, tx, cfg.MappingVersion)
			applySnapshotCoverage(&sum, coverage)
			return err
		}); err != nil {
			return sum, err
		}
	case modeRollback:
		if err := ensureWritePreconditions(ctx, db, cfg.Mode, cfg.MappingVersion); err != nil {
			return sum, err
		}
		if err := withTx(ctx, db, false, func(tx *sql.Tx) error {
			n, err := rollbackAPIKeys(ctx, tx, cfg.MappingVersion)
			sum.RolledBackAPIKeys = n
			return err
		}); err != nil {
			return sum, err
		}
	case modeApply:
		if err := ensureWritePreconditions(ctx, db, cfg.Mode, cfg.MappingVersion); err != nil {
			return sum, err
		}
		if err := withTx(ctx, db, false, func(tx *sql.Tx) error {
			coverage, err := snapshotEligibleAPIKeys(ctx, tx, cfg.MappingVersion)
			applySnapshotCoverage(&sum, coverage)
			if err != nil {
				return err
			}
			return ensureSnapshotCoverage(coverage)
		}); err != nil {
			return sum, err
		}
		if err := withTx(ctx, db, false, func(tx *sql.Tx) error {
			applySum, err := applyBackfill(ctx, tx, cfg)
			if err != nil {
				return err
			}
			mergeApplySummary(&sum, applySum)
			return nil
		}); err != nil {
			return sum, err
		}
	}
	if err := collectReconciliation(ctx, db, cfg.MappingVersion, &sum); err != nil {
		return sum, err
	}
	return sum, nil
}

func writeModeRequiresGate(mode string) bool {
	switch mode {
	case modeApply, modeSnapshot, modeRollback:
		return true
	default:
		return false
	}
}

func ensureWritePreconditions(ctx context.Context, db *sql.DB, mode, mappingVersion string) error {
	if !writeModeRequiresGate(mode) {
		return nil
	}
	if err := ensureFlagsDisabledForWrite(ctx, db); err != nil {
		return err
	}
	if mode == modeApply {
		if err := ensureNoExistingTargetEntitlementUsage(ctx, db, mappingVersion); err != nil {
			return err
		}
	}
	return nil
}

func ensureFlagsDisabledForWrite(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
SELECT key, value
FROM settings
WHERE key IN ('subscription_entitlements_v2_enabled', 'sub2_payment_page_legacy_mapping_enabled')`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		values[key] = strings.TrimSpace(strings.ToLower(value))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, key := range []string{"subscription_entitlements_v2_enabled", "sub2_payment_page_legacy_mapping_enabled"} {
		if values[key] != "false" {
			return fmt.Errorf("refusing write mode because %s is %q; both entitlement v2 flags must be false", key, values[key])
		}
	}
	return nil
}

func ensureNoExistingTargetEntitlementUsage(ctx context.Context, db *sql.DB, mappingVersion string) error {
	var count int64
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM usage_logs ul
JOIN subscription_entitlements se ON se.id = ul.entitlement_id
JOIN user_subscriptions us ON us.id = se.legacy_subscription_id
JOIN subscription_legacy_backfill_mappings m
  ON m.legacy_group_id = us.group_id
 AND m.mapping_version = $1
WHERE ul.entitlement_id IS NOT NULL
  AND se.deleted_at IS NULL
  AND us.deleted_at IS NULL
  AND us.status = 'active'
  AND us.expires_at > NOW()`, mappingVersion).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("refusing apply because %d usage_logs rows already reference entitlements targeted by mapping_version %q; run reconciliation/manual review before backfill", count, mappingVersion)
	}
	return nil
}

func withTx(ctx context.Context, db *sql.DB, readOnly bool, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: readOnly})
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func collectReadOnlySummary(ctx context.Context, db *sql.DB, sum *summary) error {
	row := db.QueryRowContext(ctx, `
WITH legacy_subscription_groups AS (
    SELECT
        g.id,
        g.is_exclusive,
        COALESCE(g.status, '') AS status,
        COUNT(DISTINCT a.id) FILTER (WHERE a.status = 'active' AND a.deleted_at IS NULL) AS active_accounts,
        COUNT(DISTINCT a.id) FILTER (
            WHERE a.status = 'active'
              AND a.deleted_at IS NULL
              AND a.schedulable = TRUE
              AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW())
              AND (a.expires_at IS NULL OR a.expires_at > NOW() OR a.auto_pause_on_expired = FALSE)
              AND (a.overload_until IS NULL OR a.overload_until <= NOW())
              AND (a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW())
        ) AS schedulable_accounts,
        COUNT(DISTINCT us.id) FILTER (WHERE us.status = 'active' AND us.expires_at > NOW() AND us.deleted_at IS NULL) AS active_subscriptions
    FROM groups g
    LEFT JOIN account_groups ag ON ag.group_id = g.id
    LEFT JOIN accounts a ON a.id = ag.account_id AND a.deleted_at IS NULL
    LEFT JOIN user_subscriptions us ON us.group_id = g.id
    WHERE g.deleted_at IS NULL
      AND g.subscription_type = 'subscription'
    GROUP BY g.id, g.is_exclusive, g.status
),
api_key_resolution AS (
    SELECT
        ak.id AS api_key_id,
        COUNT(us.id) FILTER (WHERE us.status = 'active' AND us.expires_at > NOW() AND us.deleted_at IS NULL) AS active_subscription_count
    FROM api_keys ak
    JOIN groups g ON g.id = ak.group_id
    LEFT JOIN user_subscriptions us ON us.user_id = ak.user_id AND us.group_id = ak.group_id
    WHERE ak.deleted_at IS NULL
      AND ak.status = 'active'
      AND g.deleted_at IS NULL
      AND g.subscription_type = 'subscription'
    GROUP BY ak.id
)
SELECT
    COALESCE(COUNT(*) FILTER (WHERE active_subscriptions > 0), 0) AS runtime_group_candidates,
    COALESCE(COUNT(*) FILTER (WHERE is_exclusive AND active_subscriptions > 0), 0) AS legacy_exclusive_sources,
    COALESCE(COUNT(*) FILTER (WHERE active_subscriptions > 0), 0) AS plan_candidates,
    COALESCE((SELECT COUNT(*) FROM user_subscriptions WHERE deleted_at IS NULL AND status = 'active' AND expires_at > NOW()), 0) AS active_legacy_subscriptions,
    COALESCE((SELECT COUNT(*) FROM user_subscriptions WHERE deleted_at IS NOT NULL OR status <> 'active' OR expires_at <= NOW()), 0) AS skipped_legacy_subscriptions,
    COALESCE((SELECT COUNT(*) FROM api_key_resolution WHERE active_subscription_count = 1), 0) AS api_key_auto_migration_candidates,
    COALESCE((SELECT COUNT(*) FROM api_key_resolution WHERE active_subscription_count <> 1), 0) AS ambiguous_api_keys,
    COALESCE(COUNT(*) FILTER (WHERE active_subscriptions > 0 AND (is_exclusive OR status <> 'active' OR schedulable_accounts = 0)), 0) AS review_groups
FROM legacy_subscription_groups`)
	if err := row.Scan(
		&sum.RuntimeGroupCandidates,
		&sum.LegacyExclusiveSources,
		&sum.PlanCandidates,
		&sum.ActiveLegacySubscriptions,
		&sum.SkippedLegacySubscriptions,
		&sum.APIKeyAutoMigrationCandidates,
		&sum.AmbiguousAPIKeys,
		&sum.ReviewGroups,
	); err != nil {
		return err
	}
	reasons, err := countReviewReasons(ctx, db)
	if err != nil {
		return err
	}
	sum.ReviewReasons = reasons
	if sum.LegacyExclusiveSources > 0 {
		sum.Warnings = append(sum.Warnings, "legacy exclusive subscription groups are used as plan template sources only; they are not opened as runtime groups")
	}
	return nil
}

func countReviewReasons(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, `
WITH legacy_groups AS (
    SELECT
        g.id,
        g.is_exclusive,
        g.status,
        COUNT(DISTINCT a.id) FILTER (WHERE a.status = 'active' AND a.deleted_at IS NULL) AS active_accounts,
        COUNT(DISTINCT a.id) FILTER (
            WHERE a.status = 'active'
              AND a.deleted_at IS NULL
              AND a.schedulable = TRUE
              AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW())
              AND (a.expires_at IS NULL OR a.expires_at > NOW() OR a.auto_pause_on_expired = FALSE)
              AND (a.overload_until IS NULL OR a.overload_until <= NOW())
              AND (a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW())
        ) AS schedulable_accounts,
        COUNT(DISTINCT us.id) FILTER (WHERE us.status = 'active' AND us.expires_at > NOW() AND us.deleted_at IS NULL) AS active_subscriptions
    FROM groups g
    LEFT JOIN account_groups ag ON ag.group_id = g.id
    LEFT JOIN accounts a ON a.id = ag.account_id AND a.deleted_at IS NULL
    LEFT JOIN user_subscriptions us ON us.group_id = g.id
    WHERE g.deleted_at IS NULL
      AND g.subscription_type = 'subscription'
    GROUP BY g.id, g.is_exclusive, g.status
),
reasons AS (
    SELECT 'legacy_exclusive_group_used_as_plan_template_source' AS reason
    FROM legacy_groups
    WHERE is_exclusive AND active_subscriptions > 0
    UNION ALL
    SELECT 'legacy_group_has_no_active_schedulable_accounts'
    FROM legacy_groups
    WHERE schedulable_accounts = 0 AND active_subscriptions > 0
    UNION ALL
    SELECT 'legacy_group_not_active'
    FROM legacy_groups
    WHERE status <> 'active' AND active_subscriptions > 0
)
SELECT reason, COUNT(*) FROM reasons GROUP BY reason ORDER BY reason`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	reasons := map[string]int64{}
	for rows.Next() {
		var reason string
		var count int64
		if err := rows.Scan(&reason, &count); err != nil {
			return nil, err
		}
		reasons[reason] = count
	}
	return reasons, rows.Err()
}

func collectAmbiguousAPIKeys(ctx context.Context, db *sql.DB) ([]ambiguousAPIKey, error) {
	rows, err := db.QueryContext(ctx, `
SELECT
    ak.id AS api_key_id,
    ak.user_id,
    ak.group_id AS old_group_id,
    CASE
        WHEN COUNT(us.id) = 0 THEN 'no_active_legacy_subscription'
        ELSE 'multiple_active_legacy_subscriptions'
    END AS reason,
    COALESCE(STRING_AGG(DISTINCT us.id::text, ',' ORDER BY us.id::text), '') AS candidate_subscription_ids,
    m.runtime_group_id,
    m.runtime_group_key,
    m.mapping_version,
    g.name,
    g.description,
    g.platform,
    COALESCE(g.rate_multiplier, 1.0),
    g.daily_limit_usd,
    g.weekly_limit_usd,
    g.monthly_limit_usd,
    COALESCE(g.default_validity_days, 30),
    COALESCE(g.sort_order, 0),
    COALESCE(g.is_exclusive, FALSE),
    COALESCE(g.status, 'active'),
    COALESCE(STRING_AGG(DISTINCT CONCAT(ag.account_id, ':', ag.priority), ',' ORDER BY CONCAT(ag.account_id, ':', ag.priority)), 'no_accounts') AS account_signature
FROM api_keys ak
JOIN groups g ON g.id = ak.group_id
LEFT JOIN user_subscriptions us
    ON us.user_id = ak.user_id
   AND us.group_id = ak.group_id
   AND us.deleted_at IS NULL
   AND us.status = 'active'
   AND us.expires_at > NOW()
LEFT JOIN subscription_legacy_backfill_mappings m ON m.legacy_group_id = ak.group_id
LEFT JOIN account_groups ag ON ag.group_id = g.id
WHERE ak.deleted_at IS NULL
  AND ak.status = 'active'
  AND g.deleted_at IS NULL
  AND g.subscription_type = 'subscription'
GROUP BY
    ak.id, ak.user_id, ak.group_id,
    m.runtime_group_id, m.runtime_group_key, m.mapping_version,
    g.id, g.name, g.description, g.platform, g.rate_multiplier,
    g.daily_limit_usd, g.weekly_limit_usd, g.monthly_limit_usd,
    g.default_validity_days, g.sort_order, g.is_exclusive, g.status
HAVING COUNT(us.id) <> 1
ORDER BY ak.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var details []ambiguousAPIKey
	for rows.Next() {
		var detail ambiguousAPIKey
		var candidateIDs string
		var runtimeGroupID sql.NullInt64
		var runtimeGroupKeyValue sql.NullString
		var mappingVersion sql.NullString
		var group legacyGroup
		if err := rows.Scan(
			&detail.APIKeyID,
			&detail.UserID,
			&detail.OldGroupID,
			&detail.Reason,
			&candidateIDs,
			&runtimeGroupID,
			&runtimeGroupKeyValue,
			&mappingVersion,
			&group.Name,
			&group.Description,
			&group.Platform,
			&group.RateMultiplier,
			&group.DailyLimitUSD,
			&group.WeeklyLimitUSD,
			&group.MonthlyLimitUSD,
			&group.DefaultValidityDays,
			&group.SortOrder,
			&group.IsExclusive,
			&group.Status,
			&group.AccountSignature,
		); err != nil {
			return nil, err
		}
		group.ID = detail.OldGroupID
		detail.CandidateSubscriptionIDs = parseIDList(candidateIDs)
		if runtimeGroupID.Valid {
			id := runtimeGroupID.Int64
			detail.ProposedRuntimeGroupID = &id
		}
		if runtimeGroupKeyValue.Valid && runtimeGroupKeyValue.String != "" {
			detail.ProposedRuntimeGroupKey = runtimeGroupKeyValue.String
		} else {
			detail.ProposedRuntimeGroupKey = runtimeGroupKey(group)
		}
		if mappingVersion.Valid {
			detail.ExistingMappingVersion = mappingVersion.String
		}
		details = append(details, detail)
	}
	return details, rows.Err()
}

func applyBackfill(ctx context.Context, tx *sql.Tx, cfg config) (summary, error) {
	sum := summary{
		ReviewReasons:  map[string]int64{},
		Reconciliation: map[string]int64{},
	}
	groups, err := loadLegacyGroups(ctx, tx)
	if err != nil {
		return sum, err
	}
	for _, group := range groups {
		if group.SchedulableAccounts == 0 && !cfg.AllowNoAccountPools {
			return sum, fmt.Errorf("legacy group %d has no active schedulable account pool; rerun after review or pass -allow-no-account-pools", group.ID)
		}
		m, createdRuntime, createdPlan, createdMapping, err := ensureMapping(ctx, tx, cfg.MappingVersion, group)
		if err != nil {
			return sum, err
		}
		if err := copyAccountPool(ctx, tx, group.ID, m.RuntimeGroupID); err != nil {
			return sum, err
		}
		if err := ensurePlanGrant(ctx, tx, m.PlanID, m.RuntimeGroupID); err != nil {
			return sum, err
		}
		if createdRuntime {
			sum.CreatedRuntimeGroups++
		}
		if createdPlan {
			sum.CreatedPlans++
		}
		if createdMapping {
			sum.CreatedMappings++
		}
	}
	n, err := upsertBackfillEntitlements(ctx, tx, cfg.MappingVersion)
	if err != nil {
		return sum, err
	}
	sum.UpdatedEntitlements = n

	n, err = replaceBackfillEntitlementGrants(ctx, tx, cfg.MappingVersion)
	if err != nil {
		return sum, err
	}
	sum.UpdatedEntitlementGrants = n

	n, err = upsertBackfillFulfillments(ctx, tx, cfg.MappingVersion)
	if err != nil {
		return sum, err
	}
	sum.UpsertedFulfillments = n

	n, err = migrateEligibleAPIKeys(ctx, tx, cfg.MappingVersion)
	if err != nil {
		return sum, err
	}
	sum.UpdatedAPIKeys = n
	return sum, nil
}

func loadLegacyGroups(ctx context.Context, tx *sql.Tx) ([]legacyGroup, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT
    g.id,
    g.name,
    g.description,
    g.platform,
    COALESCE(g.rate_multiplier, 1.0),
    g.daily_limit_usd,
    g.weekly_limit_usd,
    g.monthly_limit_usd,
    COALESCE(g.default_validity_days, 30),
    COALESCE(g.sort_order, 0),
    COALESCE(g.is_exclusive, FALSE),
    COALESCE(g.status, 'active'),
    COALESCE(STRING_AGG(CONCAT(ag.account_id, ':', ag.priority), ',' ORDER BY ag.account_id, ag.priority), 'no_accounts') AS account_signature,
    COUNT(DISTINCT a.id) FILTER (WHERE a.status = 'active' AND a.deleted_at IS NULL) AS active_account_count,
    COUNT(DISTINCT a.id) FILTER (
        WHERE a.status = 'active'
          AND a.deleted_at IS NULL
          AND a.schedulable = TRUE
          AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW())
          AND (a.expires_at IS NULL OR a.expires_at > NOW() OR a.auto_pause_on_expired = FALSE)
          AND (a.overload_until IS NULL OR a.overload_until <= NOW())
          AND (a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW())
    ) AS schedulable_accounts
FROM groups g
JOIN user_subscriptions us ON us.group_id = g.id
LEFT JOIN account_groups ag ON ag.group_id = g.id
LEFT JOIN accounts a ON a.id = ag.account_id AND a.deleted_at IS NULL
WHERE g.deleted_at IS NULL
  AND g.subscription_type = 'subscription'
  AND us.deleted_at IS NULL
  AND us.status = 'active'
  AND us.expires_at > NOW()
GROUP BY
    g.id, g.name, g.description, g.platform, g.rate_multiplier,
    g.daily_limit_usd, g.weekly_limit_usd, g.monthly_limit_usd,
    g.default_validity_days, g.sort_order, g.is_exclusive, g.status
ORDER BY g.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var groups []legacyGroup
	for rows.Next() {
		var g legacyGroup
		if err := rows.Scan(
			&g.ID,
			&g.Name,
			&g.Description,
			&g.Platform,
			&g.RateMultiplier,
			&g.DailyLimitUSD,
			&g.WeeklyLimitUSD,
			&g.MonthlyLimitUSD,
			&g.DefaultValidityDays,
			&g.SortOrder,
			&g.IsExclusive,
			&g.Status,
			&g.AccountSignature,
			&g.ActiveAccountCount,
			&g.SchedulableAccounts,
		); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func ensureMapping(ctx context.Context, tx *sql.Tx, version string, group legacyGroup) (mapping, bool, bool, bool, error) {
	existing, err := getMapping(ctx, tx, group.ID)
	if err == nil {
		if existing.MappingVersion != version {
			return mapping{}, false, false, false, fmt.Errorf("legacy_group_id %d already has mapping_version %q, requested %q", group.ID, existing.MappingVersion, version)
		}
		return existing, false, false, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return mapping{}, false, false, false, err
	}

	runtimeKey := runtimeGroupKey(group)
	runtimeGroupID, createdRuntime, err := ensureRuntimeGroup(ctx, tx, group, runtimeKey)
	if err != nil {
		return mapping{}, false, false, false, err
	}
	planID, createdPlan, err := ensureLegacyPlan(ctx, tx, group, runtimeGroupID)
	if err != nil {
		return mapping{}, false, false, false, err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO subscription_legacy_backfill_mappings (
    legacy_group_id, plan_id, runtime_group_id, runtime_group_key, mapping_version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
ON CONFLICT (legacy_group_id) DO NOTHING`,
		group.ID, planID, runtimeGroupID, runtimeKey, version)
	if err != nil {
		return mapping{}, false, false, false, err
	}
	return mapping{
		LegacyGroupID:   group.ID,
		RuntimeGroupID:  runtimeGroupID,
		PlanID:          planID,
		RuntimeGroupKey: runtimeKey,
		MappingVersion:  version,
	}, createdRuntime, createdPlan, true, nil
}

func getMapping(ctx context.Context, tx *sql.Tx, legacyGroupID int64) (mapping, error) {
	var m mapping
	err := tx.QueryRowContext(ctx, `
SELECT legacy_group_id, plan_id, runtime_group_id, runtime_group_key, mapping_version
FROM subscription_legacy_backfill_mappings
WHERE legacy_group_id = $1`, legacyGroupID).
		Scan(&m.LegacyGroupID, &m.PlanID, &m.RuntimeGroupID, &m.RuntimeGroupKey, &m.MappingVersion)
	m.Existed = err == nil
	return m, err
}

func ensureRuntimeGroup(ctx context.Context, tx *sql.Tx, group legacyGroup, runtimeKey string) (int64, bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
SELECT runtime_group_id
FROM subscription_legacy_backfill_mappings
WHERE runtime_group_key = $1
ORDER BY legacy_group_id
LIMIT 1`, runtimeKey).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}

	name := fmt.Sprintf("Legacy Runtime %s", runtimeKey)
	err = tx.QueryRowContext(ctx, `
SELECT id FROM groups
WHERE name = $1 AND deleted_at IS NULL
LIMIT 1`, name).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}

	description := fmt.Sprintf("Backfilled runtime group for legacy subscription groups using key %s. Legacy exclusive groups remain plan template sources only.", runtimeKey)
	err = tx.QueryRowContext(ctx, `
INSERT INTO groups (
    name, description, platform, rate_multiplier, is_exclusive, status,
    subscription_type, balance_enabled, subscription_enabled, plan_auto_grant_enabled,
    sort_order, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, FALSE, 'active',
    'subscription', FALSE, TRUE, FALSE,
    $5, NOW(), NOW()
) RETURNING id`, name, description, group.Platform, group.RateMultiplier, group.SortOrder).Scan(&id)
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func ensureLegacyPlan(ctx context.Context, tx *sql.Tx, group legacyGroup, runtimeGroupID int64) (int64, bool, error) {
	name := fmt.Sprintf("Legacy Backfill Plan #%d - %s", group.ID, truncate(group.Name, 58))
	var id int64
	err := tx.QueryRowContext(ctx, `
SELECT id FROM subscription_plans
WHERE name = $1
LIMIT 1`, name).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}

	description := "Generated by legacy subscription entitlement backfill. Not for sale."
	if group.Description.Valid && strings.TrimSpace(group.Description.String) != "" {
		description = group.Description.String
	}
	err = tx.QueryRowContext(ctx, `
INSERT INTO subscription_plans (
    group_id, name, description, price, validity_days, validity_unit,
    access_scope, allowed_platforms,
    daily_limit_usd, weekly_limit_usd, monthly_limit_usd,
    overage_policy, features, product_name, for_sale, sort_order, created_at, updated_at
) VALUES (
    $1, $2, $3, 0, $4, 'day',
    'explicit', '[]'::jsonb,
    $5, $6, $7,
    'block', '', '', FALSE, $8, NOW(), NOW()
) RETURNING id`,
		runtimeGroupID,
		name,
		description,
		group.DefaultValidityDays,
		nullableFloat(group.DailyLimitUSD),
		nullableFloat(group.WeeklyLimitUSD),
		nullableFloat(group.MonthlyLimitUSD),
		group.SortOrder,
	).Scan(&id)
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func copyAccountPool(ctx context.Context, tx *sql.Tx, legacyGroupID, runtimeGroupID int64) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO account_groups (account_id, group_id, priority, created_at)
SELECT account_id, $2, MIN(priority), NOW()
FROM account_groups
WHERE group_id = $1
GROUP BY account_id
ON CONFLICT (account_id, group_id) DO NOTHING`, legacyGroupID, runtimeGroupID)
	return err
}

func ensurePlanGrant(ctx context.Context, tx *sql.Tx, planID, runtimeGroupID int64) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO subscription_plan_groups (plan_id, group_id, sort_order, enabled, created_at, updated_at)
VALUES ($1, $2, 0, TRUE, NOW(), NOW())
ON CONFLICT (plan_id, group_id) DO UPDATE
SET enabled = TRUE, updated_at = NOW()`, planID, runtimeGroupID)
	return err
}

func upsertBackfillEntitlements(ctx context.Context, tx *sql.Tx, version string) (int64, error) {
	res, err := tx.ExecContext(ctx, `
WITH legacy_rows AS (
    SELECT
        us.id AS legacy_subscription_id,
        us.user_id,
        us.group_id AS legacy_group_id,
        m.plan_id,
        m.runtime_group_id,
        m.mapping_version,
        COALESCE(g.name, 'Legacy Subscription') AS entitlement_name,
        us.status,
        us.starts_at,
        us.expires_at,
        us.daily_window_start,
        us.weekly_window_start,
        us.monthly_window_start,
        g.daily_limit_usd,
        g.weekly_limit_usd,
        g.monthly_limit_usd,
        us.daily_usage_usd,
        us.weekly_usage_usd,
        us.monthly_usage_usd,
        us.assigned_by,
        us.assigned_at,
        us.created_at,
        us.updated_at
    FROM user_subscriptions us
    JOIN subscription_legacy_backfill_mappings m ON m.legacy_group_id = us.group_id
    JOIN groups g ON g.id = us.group_id
    WHERE us.deleted_at IS NULL
      AND us.status = 'active'
      AND us.expires_at > NOW()
      AND m.mapping_version = $1
)
INSERT INTO subscription_entitlements (
    user_id, plan_id, legacy_subscription_id, primary_group_id, name, source_type, source_id,
    status, starts_at, expires_at,
    daily_window_start, weekly_window_start, monthly_window_start,
    daily_limit_usd, weekly_limit_usd, monthly_limit_usd,
    daily_usage_usd, weekly_usage_usd, monthly_usage_usd,
    overage_policy, plan_snapshot, assigned_by, assigned_at, created_at, updated_at
)
SELECT
    user_id, plan_id, legacy_subscription_id, runtime_group_id, entitlement_name,
    $2, legacy_subscription_id,
    status, starts_at, expires_at,
    daily_window_start, weekly_window_start, monthly_window_start,
    daily_limit_usd, weekly_limit_usd, monthly_limit_usd,
    daily_usage_usd, weekly_usage_usd, monthly_usage_usd,
    'block',
    jsonb_build_object(
        'legacy_group_id', legacy_group_id,
        'runtime_group_id', runtime_group_id,
        'mapping_version', mapping_version,
        'source', $2
    ),
    assigned_by, assigned_at, created_at, updated_at
FROM legacy_rows
ON CONFLICT (legacy_subscription_id) DO UPDATE
SET
    user_id = EXCLUDED.user_id,
    plan_id = EXCLUDED.plan_id,
    primary_group_id = EXCLUDED.primary_group_id,
    name = EXCLUDED.name,
    source_type = EXCLUDED.source_type,
    source_id = EXCLUDED.source_id,
    status = EXCLUDED.status,
    starts_at = EXCLUDED.starts_at,
    expires_at = EXCLUDED.expires_at,
    daily_window_start = EXCLUDED.daily_window_start,
    weekly_window_start = EXCLUDED.weekly_window_start,
    monthly_window_start = EXCLUDED.monthly_window_start,
    daily_limit_usd = EXCLUDED.daily_limit_usd,
    weekly_limit_usd = EXCLUDED.weekly_limit_usd,
    monthly_limit_usd = EXCLUDED.monthly_limit_usd,
    daily_usage_usd = EXCLUDED.daily_usage_usd,
    weekly_usage_usd = EXCLUDED.weekly_usage_usd,
    monthly_usage_usd = EXCLUDED.monthly_usage_usd,
    overage_policy = EXCLUDED.overage_policy,
    plan_snapshot = EXCLUDED.plan_snapshot,
    assigned_by = EXCLUDED.assigned_by,
    assigned_at = EXCLUDED.assigned_at,
    updated_at = NOW()`, version, sourceLegacyBackfill)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func replaceBackfillEntitlementGrants(ctx context.Context, tx *sql.Tx, version string) (int64, error) {
	if _, err := tx.ExecContext(ctx, `
DELETE FROM subscription_entitlement_groups seg
USING subscription_entitlements se
JOIN subscription_legacy_backfill_mappings m ON m.plan_id = se.plan_id AND m.runtime_group_id = se.primary_group_id
WHERE seg.entitlement_id = se.id
  AND se.source_type = $2
  AND se.legacy_subscription_id IS NOT NULL
  AND m.mapping_version = $1`, version, sourceLegacyBackfill); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `
INSERT INTO subscription_entitlement_groups (entitlement_id, group_id, sort_order, enabled, created_at, updated_at)
SELECT se.id, m.runtime_group_id, 0, TRUE, NOW(), NOW()
FROM subscription_entitlements se
JOIN subscription_legacy_backfill_mappings m ON m.plan_id = se.plan_id AND m.runtime_group_id = se.primary_group_id
WHERE se.source_type = $2
  AND se.legacy_subscription_id IS NOT NULL
  AND m.mapping_version = $1
ON CONFLICT (entitlement_id, group_id) DO UPDATE
SET enabled = TRUE, updated_at = NOW()`, version, sourceLegacyBackfill)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func upsertBackfillFulfillments(ctx context.Context, tx *sql.Tx, version string) (int64, error) {
	res, err := tx.ExecContext(ctx, `
INSERT INTO subscription_entitlement_fulfillments (
    entitlement_id, user_id, plan_id, source_type, source_id, validity_days,
    starts_at, expires_at, assigned_by, assigned_at, notes, created_at, updated_at
)
SELECT
    se.id,
    se.user_id,
    se.plan_id,
    $2,
    se.legacy_subscription_id,
    GREATEST(0, CEIL(EXTRACT(EPOCH FROM (se.expires_at - se.starts_at)) / 86400.0)::INTEGER),
    se.starts_at,
    se.expires_at,
    se.assigned_by,
    se.assigned_at,
    'legacy subscription entitlement backfill',
    NOW(),
    NOW()
FROM subscription_entitlements se
JOIN subscription_legacy_backfill_mappings m ON m.plan_id = se.plan_id AND m.runtime_group_id = se.primary_group_id
WHERE se.source_type = $2
  AND se.legacy_subscription_id IS NOT NULL
  AND m.mapping_version = $1
ON CONFLICT (source_type, source_id) WHERE source_id IS NOT NULL DO UPDATE
SET
    entitlement_id = EXCLUDED.entitlement_id,
    user_id = EXCLUDED.user_id,
    plan_id = EXCLUDED.plan_id,
    validity_days = EXCLUDED.validity_days,
    starts_at = EXCLUDED.starts_at,
    expires_at = EXCLUDED.expires_at,
    assigned_by = EXCLUDED.assigned_by,
    assigned_at = EXCLUDED.assigned_at,
    updated_at = NOW()`, version, sourceLegacyBackfill)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func snapshotEligibleAPIKeys(ctx context.Context, tx *sql.Tx, version string) (snapshotCoverage, error) {
	rows, err := tx.QueryContext(ctx, `
WITH eligible AS (
    SELECT
        ak.id AS api_key_id,
        ak.user_id,
        ak.group_id AS old_group_id,
        ak.access_source AS old_access_source,
        ak.subscription_entitlement_id AS old_subscription_entitlement_id,
        ak.updated_at AS old_updated_at
    FROM api_keys ak
    JOIN groups g ON g.id = ak.group_id
    WHERE ak.deleted_at IS NULL
      AND ak.status = 'active'
      AND g.deleted_at IS NULL
      AND g.subscription_type = 'subscription'
)
INSERT INTO api_key_legacy_backfill_snapshots (
    api_key_id, user_id, old_group_id, old_access_source,
    old_subscription_entitlement_id, old_updated_at, mapping_version, created_at
)
SELECT
    api_key_id, user_id, old_group_id, old_access_source,
    old_subscription_entitlement_id, old_updated_at, $1, NOW()
FROM eligible
ON CONFLICT (api_key_id) DO NOTHING
RETURNING api_key_id`, version)
	if err != nil {
		return snapshotCoverage{}, err
	}
	capturedIDs := map[int64]struct{}{}
	for rows.Next() {
		var apiKeyID int64
		if err := rows.Scan(&apiKeyID); err != nil {
			_ = rows.Close()
			return snapshotCoverage{}, err
		}
		capturedIDs[apiKeyID] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return snapshotCoverage{}, err
	}
	if err := rows.Err(); err != nil {
		return snapshotCoverage{}, err
	}
	return collectSnapshotCoverage(ctx, tx, capturedIDs)
}

func collectSnapshotCoverage(ctx context.Context, tx *sql.Tx, capturedIDs map[int64]struct{}) (snapshotCoverage, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT
    ak.id AS api_key_id,
    ak.user_id,
    ak.group_id AS old_group_id,
    s.mapping_version
FROM api_keys ak
JOIN groups g ON g.id = ak.group_id
LEFT JOIN api_key_legacy_backfill_snapshots s ON s.api_key_id = ak.id
WHERE ak.deleted_at IS NULL
  AND ak.status = 'active'
  AND g.deleted_at IS NULL
  AND g.subscription_type = 'subscription'
ORDER BY ak.id`)
	if err != nil {
		return snapshotCoverage{}, err
	}
	defer func() { _ = rows.Close() }()

	coverage := snapshotCoverage{Captured: int64(len(capturedIDs))}
	for rows.Next() {
		var detail snapshotAPIKey
		var snapshotVersion sql.NullString
		if err := rows.Scan(&detail.APIKeyID, &detail.UserID, &detail.OldGroupID, &snapshotVersion); err != nil {
			return snapshotCoverage{}, err
		}
		if snapshotVersion.Valid {
			detail.SnapshotMappingVersion = snapshotVersion.String
			if _, ok := capturedIDs[detail.APIKeyID]; ok {
				detail.Status = "newly_captured"
			} else {
				detail.Status = "reused_existing_snapshot"
				coverage.Reused++
			}
			coverage.Covered++
		} else {
			detail.Status = "missing_snapshot"
			coverage.Missing++
		}
		coverage.Details = append(coverage.Details, detail)
	}
	if err := rows.Err(); err != nil {
		return snapshotCoverage{}, err
	}
	return coverage, nil
}

func ensureSnapshotCoverage(coverage snapshotCoverage) error {
	if coverage.Missing > 0 {
		return fmt.Errorf("refusing apply because %d api keys lack snapshot coverage", coverage.Missing)
	}
	return nil
}

func applySnapshotCoverage(sum *summary, coverage snapshotCoverage) {
	sum.SnapshottedAPIKeys += coverage.Captured
	sum.CapturedAPIKeys += coverage.Captured
	sum.ReusedExistingSnapshots += coverage.Reused
	sum.CoveredAPIKeys += coverage.Covered
	sum.MissingSnapshotAPIKeys += coverage.Missing
	sum.SnapshotAPIKeyDetails = append(sum.SnapshotAPIKeyDetails, coverage.Details...)
	for _, detail := range coverage.Details {
		if detail.Status == "missing_snapshot" {
			sum.MissingSnapshotAPIKeyDetails = append(sum.MissingSnapshotAPIKeyDetails, detail)
		}
	}
}

func migrateEligibleAPIKeys(ctx context.Context, tx *sql.Tx, version string) (int64, error) {
	res, err := tx.ExecContext(ctx, `
WITH eligible AS (
    SELECT
        ak.id AS api_key_id,
        ak.group_id AS old_group_id,
        se.id AS entitlement_id,
        m.runtime_group_id
    FROM api_keys ak
    JOIN groups g ON g.id = ak.group_id
    JOIN user_subscriptions us ON us.user_id = ak.user_id AND us.group_id = ak.group_id
    JOIN subscription_entitlements se ON se.legacy_subscription_id = us.id
    JOIN subscription_legacy_backfill_mappings m ON m.legacy_group_id = us.group_id
    WHERE ak.deleted_at IS NULL
      AND ak.status = 'active'
      AND g.subscription_type = 'subscription'
      AND us.deleted_at IS NULL
      AND us.status = 'active'
      AND us.expires_at > NOW()
      AND se.deleted_at IS NULL
      AND se.source_type = $2
      AND m.mapping_version = $1
      AND (
          SELECT COUNT(*)
          FROM user_subscriptions us2
          WHERE us2.user_id = ak.user_id
            AND us2.group_id = ak.group_id
            AND us2.deleted_at IS NULL
            AND us2.status = 'active'
            AND us2.expires_at > NOW()
      ) = 1
)
UPDATE api_keys ak
SET
    group_id = eligible.runtime_group_id,
    access_source = 'entitlement',
    subscription_entitlement_id = eligible.entitlement_id,
    updated_at = NOW()
FROM eligible
WHERE ak.id = eligible.api_key_id
  AND ak.group_id = eligible.old_group_id`, version, sourceLegacyBackfill)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func rollbackAPIKeys(ctx context.Context, tx *sql.Tx, version string) (int64, error) {
	res, err := tx.ExecContext(ctx, rollbackAPIKeysSQL, version, sourceLegacyBackfill)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func collectReconciliation(ctx context.Context, db *sql.DB, version string, sum *summary) error {
	checks := map[string]string{
		"entitlement_legacy_usage_mismatch": `
SELECT COUNT(*)
FROM subscription_entitlements se
JOIN user_subscriptions us ON us.id = se.legacy_subscription_id
JOIN subscription_legacy_backfill_mappings m ON m.plan_id = se.plan_id AND m.runtime_group_id = se.primary_group_id
WHERE m.mapping_version = $1
  AND se.deleted_at IS NULL
  AND (
      se.expires_at <> us.expires_at
      OR se.daily_usage_usd <> us.daily_usage_usd
      OR se.weekly_usage_usd <> us.weekly_usage_usd
      OR se.monthly_usage_usd <> us.monthly_usage_usd
  )`,
		"missing_entitlement_runtime_grant": `
SELECT COUNT(*)
FROM subscription_entitlements se
JOIN subscription_legacy_backfill_mappings m ON m.plan_id = se.plan_id AND m.runtime_group_id = se.primary_group_id
LEFT JOIN subscription_entitlement_groups seg ON seg.entitlement_id = se.id AND seg.group_id = m.runtime_group_id AND seg.enabled = TRUE
WHERE m.mapping_version = $1
  AND se.deleted_at IS NULL
  AND seg.entitlement_id IS NULL`,
		"missing_backfill_fulfillment": `
SELECT COUNT(*)
FROM subscription_entitlements se
JOIN subscription_legacy_backfill_mappings m ON m.plan_id = se.plan_id AND m.runtime_group_id = se.primary_group_id
LEFT JOIN subscription_entitlement_fulfillments sef ON sef.source_type = $2 AND sef.source_id = se.legacy_subscription_id
WHERE m.mapping_version = $1
  AND se.deleted_at IS NULL
  AND sef.id IS NULL`,
		"api_keys_without_snapshot_on_runtime_group": `
SELECT COUNT(*)
FROM api_keys ak
JOIN subscription_legacy_backfill_mappings m ON m.runtime_group_id = ak.group_id
LEFT JOIN api_key_legacy_backfill_snapshots s ON s.api_key_id = ak.id
WHERE m.mapping_version = $1
  AND ak.deleted_at IS NULL
  AND s.api_key_id IS NULL`,
	}
	if sum.Reconciliation == nil {
		sum.Reconciliation = map[string]int64{}
	}
	for name, query := range checks {
		var count int64
		args := []any{version}
		if strings.Contains(query, "$2") {
			args = append(args, sourceLegacyBackfill)
		}
		if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		sum.Reconciliation[name] = count
	}
	return nil
}

func mergeApplySummary(dst *summary, src summary) {
	dst.CreatedRuntimeGroups += src.CreatedRuntimeGroups
	dst.CreatedPlans += src.CreatedPlans
	dst.CreatedMappings += src.CreatedMappings
	dst.UpdatedEntitlements += src.UpdatedEntitlements
	dst.UpdatedEntitlementGrants += src.UpdatedEntitlementGrants
	dst.UpsertedFulfillments += src.UpsertedFulfillments
	dst.SnapshottedAPIKeys += src.SnapshottedAPIKeys
	dst.CapturedAPIKeys += src.CapturedAPIKeys
	dst.ReusedExistingSnapshots += src.ReusedExistingSnapshots
	dst.CoveredAPIKeys += src.CoveredAPIKeys
	dst.MissingSnapshotAPIKeys += src.MissingSnapshotAPIKeys
	dst.UpdatedAPIKeys += src.UpdatedAPIKeys
	dst.RolledBackAPIKeys += src.RolledBackAPIKeys
	dst.SnapshotAPIKeyDetails = append(dst.SnapshotAPIKeyDetails, src.SnapshotAPIKeyDetails...)
	dst.MissingSnapshotAPIKeyDetails = append(dst.MissingSnapshotAPIKeyDetails, src.MissingSnapshotAPIKeyDetails...)
}

func runtimeGroupKey(group legacyGroup) string {
	hash := sha256.Sum256([]byte(strings.Join([]string{
		group.Platform,
		fmt.Sprintf("%.4f", group.RateMultiplier),
		group.AccountSignature,
	}, "|")))
	short := hex.EncodeToString(hash[:])[:16]
	platform := sanitizeKeyPart(group.Platform)
	return fmt.Sprintf("%s-rate-%s-pool-%s", platform, strings.ReplaceAll(fmt.Sprintf("%.4f", group.RateMultiplier), ".", "_"), short)
}

func sanitizeKeyPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	re := regexp.MustCompile(`[^a-z0-9]+`)
	value = re.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "unknown"
	}
	if len(value) > 32 {
		return value[:32]
	}
	return value
}

func nullableFloat(v sql.NullFloat64) any {
	if !v.Valid {
		return nil
	}
	return v.Float64
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

func parseIDList(value string) []int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func writeSummary(w io.Writer, sum summary) {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(sum)
}

func writeEvidence(dir string, sum summary) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	if err := writeJSONFile(filepath.Join(dir, fmt.Sprintf("legacy-backfill-%s-%s.json", sum.Mode, timestamp)), sum); err != nil {
		return err
	}
	if len(sum.AmbiguousAPIKeyDetails) > 0 {
		if err := writeJSONFile(filepath.Join(dir, fmt.Sprintf("legacy-backfill-ambiguous-api-keys-%s.json", timestamp)), sum.AmbiguousAPIKeyDetails); err != nil {
			return err
		}
	}
	return nil
}

func writeJSONFile(path string, value any) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func redactSecret(value string) string {
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		return regexp.MustCompile(`://([^:@/]+):([^@/]+)@`).ReplaceAllString(value, "://$1:[REDACTED]@")
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"password=", "token=", "apikey=", "api_key=", "secret="} {
		if idx := strings.Index(lower, marker); idx >= 0 {
			end := strings.IndexAny(value[idx+len(marker):], "&; ")
			if end < 0 {
				return value[:idx+len(marker)] + "[REDACTED]"
			}
			return value[:idx+len(marker)] + "[REDACTED]" + value[idx+len(marker)+end:]
		}
	}
	if len(value) <= 12 {
		return "[REDACTED]"
	}
	return value[:6] + "[REDACTED]" + value[len(value)-4:]
}

func reconciliationSQL(mappingVersion string) []string {
	version := strings.ReplaceAll(mappingVersion, "'", "''")
	return []string{
		fmt.Sprintf("SELECT COUNT(*) FROM subscription_entitlements se JOIN user_subscriptions us ON us.id = se.legacy_subscription_id JOIN subscription_legacy_backfill_mappings m ON m.plan_id = se.plan_id AND m.runtime_group_id = se.primary_group_id WHERE m.mapping_version = '%s' AND se.deleted_at IS NULL AND (se.expires_at <> us.expires_at OR se.daily_usage_usd <> us.daily_usage_usd OR se.weekly_usage_usd <> us.weekly_usage_usd OR se.monthly_usage_usd <> us.monthly_usage_usd);", version),
		fmt.Sprintf("SELECT COUNT(*) FROM subscription_entitlements se JOIN subscription_legacy_backfill_mappings m ON m.plan_id = se.plan_id AND m.runtime_group_id = se.primary_group_id LEFT JOIN subscription_entitlement_groups seg ON seg.entitlement_id = se.id AND seg.group_id = m.runtime_group_id AND seg.enabled = TRUE WHERE m.mapping_version = '%s' AND se.deleted_at IS NULL AND seg.entitlement_id IS NULL;", version),
		fmt.Sprintf("SELECT COUNT(*) FROM subscription_entitlements se JOIN subscription_legacy_backfill_mappings m ON m.plan_id = se.plan_id AND m.runtime_group_id = se.primary_group_id LEFT JOIN subscription_entitlement_fulfillments sef ON sef.source_type = '%s' AND sef.source_id = se.legacy_subscription_id WHERE m.mapping_version = '%s' AND se.deleted_at IS NULL AND sef.id IS NULL;", sourceLegacyBackfill, version),
	}
}
