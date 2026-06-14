//go:build integration

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	_ "github.com/lib/pq"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestApplyBackfillTypedParametersIntegration(t *testing.T) {
	ctx := context.Background()
	if !legacyBackfillDockerAvailable(ctx) {
		if os.Getenv("CI") != "" {
			t.Fatal("docker is not available in CI")
		}
		t.Skip("docker is not available")
	}

	pgContainer, err := tcpostgres.Run(
		ctx,
		"postgres:18.1-alpine3.23",
		tcpostgres.WithDatabase("sub2api_backfill_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
	if err != nil {
		t.Fatalf("postgres dsn: %v", err)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	if err := repository.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	userID, legacyGroupID, apiKeyID := seedLegacyBackfillApplyFixture(t, ctx, db)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin apply tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	sum, err := applyBackfill(ctx, tx, config{MappingVersion: fmt.Sprintf("it-param-types-%d", time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("apply backfill should not hit postgres parameter type inference errors: %v", err)
	}
	if sum.CreatedRuntimeGroups != 1 || sum.CreatedPlans != 1 || sum.CreatedMappings != 1 {
		t.Fatalf("unexpected mapping summary: %+v", sum)
	}
	if sum.UpdatedEntitlements != 1 || sum.UpdatedEntitlementGrants != 1 || sum.UpsertedFulfillments != 1 || sum.UpdatedAPIKeys != 1 {
		t.Fatalf("unexpected apply summary: %+v", sum)
	}

	var accessSource string
	var entitlementID sql.NullInt64
	var runtimeGroupID int64
	if err := tx.QueryRowContext(ctx, `
SELECT ak.access_source, ak.subscription_entitlement_id, ak.group_id
FROM api_keys ak
WHERE ak.id = $1`, apiKeyID).Scan(&accessSource, &entitlementID, &runtimeGroupID); err != nil {
		t.Fatalf("query migrated api key: %v", err)
	}
	if accessSource != "entitlement" || !entitlementID.Valid || runtimeGroupID == legacyGroupID {
		t.Fatalf("api key was not migrated to entitlement runtime group: access_source=%s entitlement=%v runtime=%d legacy=%d", accessSource, entitlementID, runtimeGroupID, legacyGroupID)
	}

	var entitlementUserID int64
	if err := tx.QueryRowContext(ctx, `
SELECT user_id
FROM subscription_entitlements
WHERE id = $1`, entitlementID.Int64).Scan(&entitlementUserID); err != nil {
		t.Fatalf("query entitlement: %v", err)
	}
	if entitlementUserID != userID {
		t.Fatalf("entitlement user mismatch: got %d want %d", entitlementUserID, userID)
	}
}

func seedLegacyBackfillApplyFixture(t *testing.T, ctx context.Context, db *sql.DB) (int64, int64, int64) {
	t.Helper()
	suffix := time.Now().UnixNano()

	var userID int64
	if err := db.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, status)
VALUES ($1, 'hash', 'active')
RETURNING id`, fmt.Sprintf("legacy-backfill-it-%d@example.invalid", suffix)).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	var legacyGroupID int64
	if err := db.QueryRowContext(ctx, `
INSERT INTO groups (
    name, description, platform, rate_multiplier, is_exclusive, status,
    subscription_type, daily_limit_usd, weekly_limit_usd, monthly_limit_usd,
    default_validity_days, balance_enabled, subscription_enabled, plan_auto_grant_enabled
) VALUES (
    $1, 'legacy tier fixture', 'openai', 1.5, TRUE, 'active',
    'subscription', 10, 20, 30,
    30, FALSE, TRUE, FALSE
) RETURNING id`, fmt.Sprintf("legacy-backfill-group-%d", suffix)).Scan(&legacyGroupID); err != nil {
		t.Fatalf("insert legacy group: %v", err)
	}

	var accountID int64
	if err := db.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, credentials, extra, status, schedulable, auto_pause_on_expired)
VALUES ($1, 'openai', 'apikey', '{}'::jsonb, '{}'::jsonb, 'active', TRUE, FALSE)
RETURNING id`, fmt.Sprintf("legacy-backfill-account-%d", suffix)).Scan(&accountID); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO account_groups (account_id, group_id, priority)
VALUES ($1, $2, 50)`, accountID, legacyGroupID); err != nil {
		t.Fatalf("insert account group: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO user_subscriptions (
    user_id, group_id, starts_at, expires_at, status,
    daily_window_start, weekly_window_start, monthly_window_start,
    daily_usage_usd, weekly_usage_usd, monthly_usage_usd
) VALUES (
    $1, $2, NOW() - INTERVAL '1 day', NOW() + INTERVAL '30 days', 'active',
    NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day',
    1.25, 2.25, 3.25
)`, userID, legacyGroupID); err != nil {
		t.Fatalf("insert user subscription: %v", err)
	}

	var apiKeyID int64
	if err := db.QueryRowContext(ctx, `
INSERT INTO api_keys (user_id, key, name, group_id, status, access_source)
VALUES ($1, $2, 'legacy backfill key', $3, 'active', 'balance')
RETURNING id`, userID, fmt.Sprintf("sk-it-%d", suffix), legacyGroupID).Scan(&apiKeyID); err != nil {
		t.Fatalf("insert api key: %v", err)
	}

	return userID, legacyGroupID, apiKeyID
}

func legacyBackfillDockerAvailable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "docker", "info")
	cmd.Env = os.Environ()
	return cmd.Run() == nil
}
