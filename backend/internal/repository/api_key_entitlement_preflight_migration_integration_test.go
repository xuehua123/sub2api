//go:build integration

package repository

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMigration154ClearsOnlyInvalidAPIKeyEntitlementBindings(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)

	now := time.Now().UTC().Truncate(time.Second)
	userID := insertPreflightUser(t, tx, "preflight-owner@example.com")
	otherUserID := insertPreflightUser(t, tx, "preflight-other@example.com")
	groupID := insertPreflightGroup(t, tx, "preflight-group")
	otherGroupID := insertPreflightGroup(t, tx, "preflight-other-group")

	validEntitlementID := insertPreflightEntitlement(t, tx, userID, groupID, "active", now.Add(-time.Hour), now.Add(24*time.Hour), true, false)
	expiredEntitlementID := insertPreflightEntitlement(t, tx, userID, groupID, "active", now.Add(-48*time.Hour), now.Add(-time.Hour), true, false)
	futureEntitlementID := insertPreflightEntitlement(t, tx, userID, groupID, "active", now.Add(time.Hour), now.Add(48*time.Hour), true, false)
	revokedEntitlementID := insertPreflightEntitlement(t, tx, userID, groupID, "revoked", now.Add(-time.Hour), now.Add(24*time.Hour), true, false)
	ownerMismatchEntitlementID := insertPreflightEntitlement(t, tx, otherUserID, groupID, "active", now.Add(-time.Hour), now.Add(24*time.Hour), true, false)
	groupNotCoveredEntitlementID := insertPreflightEntitlement(t, tx, userID, otherGroupID, "active", now.Add(-time.Hour), now.Add(24*time.Hour), true, false)
	disabledGrantEntitlementID := insertPreflightEntitlement(t, tx, userID, groupID, "active", now.Add(-time.Hour), now.Add(24*time.Hour), false, false)
	deletedEntitlementID := insertPreflightEntitlement(t, tx, userID, groupID, "active", now.Add(-time.Hour), now.Add(24*time.Hour), true, true)

	validKeyID := insertPreflightAPIKey(t, tx, userID, groupID, validEntitlementID, "valid")
	expiredKeyID := insertPreflightAPIKey(t, tx, userID, groupID, expiredEntitlementID, "expired")
	futureKeyID := insertPreflightAPIKey(t, tx, userID, groupID, futureEntitlementID, "future")
	revokedKeyID := insertPreflightAPIKey(t, tx, userID, groupID, revokedEntitlementID, "revoked")
	ownerMismatchKeyID := insertPreflightAPIKey(t, tx, userID, groupID, ownerMismatchEntitlementID, "owner-mismatch")
	groupNotCoveredKeyID := insertPreflightAPIKey(t, tx, userID, groupID, groupNotCoveredEntitlementID, "group-not-covered")
	disabledGrantKeyID := insertPreflightAPIKey(t, tx, userID, groupID, disabledGrantEntitlementID, "disabled-grant")
	deletedKeyID := insertPreflightAPIKey(t, tx, userID, groupID, deletedEntitlementID, "deleted")

	runMigration154InTx(t, ctx, tx)
	runMigration154InTx(t, ctx, tx)

	require.Equal(t, sql.NullInt64{Int64: validEntitlementID, Valid: true}, preflightAPIKeyEntitlementID(t, tx, validKeyID))
	for _, keyID := range []int64{
		expiredKeyID,
		futureKeyID,
		revokedKeyID,
		ownerMismatchKeyID,
		groupNotCoveredKeyID,
		disabledGrantKeyID,
		deletedKeyID,
	} {
		require.False(t, preflightAPIKeyEntitlementID(t, tx, keyID).Valid, "api key %d binding should be cleared", keyID)
	}
}

func TestMigration155BackfillsAccessSourceAndGroupCapabilities(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)

	now := time.Now().UTC().Truncate(time.Second)
	userID := insertPreflightUser(t, tx, "access-source-owner@example.com")
	groupID := insertPreflightGroupForAccessSource(t, tx, "access-source-plan-group", "subscription", "active", false, true, false, false)
	standardGroupID := insertPreflightGroupForAccessSource(t, tx, "access-source-standard-group", "standard", "active", false, false, true, true)
	exclusiveGroupID := insertPreflightGroupForAccessSource(t, tx, "access-source-exclusive-group", "subscription", "active", true, true, false, true)
	inactiveGroupID := insertPreflightGroupForAccessSource(t, tx, "access-source-inactive-group", "subscription", "inactive", false, true, false, true)

	entitlementID := insertPreflightEntitlement(t, tx, userID, groupID, "active", now.Add(-time.Hour), now.Add(time.Hour), true, false)
	entitlementKeyID := insertPreflightAPIKeyWithAccessSource(t, tx, userID, groupID, sql.NullInt64{Int64: entitlementID, Valid: true}, "balance", "entitlement-source")
	balanceKeyID := insertPreflightAPIKeyWithAccessSource(t, tx, userID, standardGroupID, sql.NullInt64{}, "entitlement", "balance-source")

	runMigration155InTx(t, ctx, tx)
	runMigration155InTx(t, ctx, tx)

	require.Equal(t, "entitlement", preflightAPIKeyAccessSource(t, tx, entitlementKeyID))
	require.Equal(t, "balance", preflightAPIKeyAccessSource(t, tx, balanceKeyID))

	require.Equal(t, preflightGroupCapabilities{BalanceEnabled: false, SubscriptionEnabled: true, PlanAutoGrantEnabled: true}, preflightGroupCapabilityFlags(t, tx, groupID))
	require.Equal(t, preflightGroupCapabilities{BalanceEnabled: true, SubscriptionEnabled: false, PlanAutoGrantEnabled: false}, preflightGroupCapabilityFlags(t, tx, standardGroupID))
	require.Equal(t, preflightGroupCapabilities{BalanceEnabled: false, SubscriptionEnabled: true, PlanAutoGrantEnabled: false}, preflightGroupCapabilityFlags(t, tx, exclusiveGroupID))
	require.Equal(t, preflightGroupCapabilities{BalanceEnabled: false, SubscriptionEnabled: true, PlanAutoGrantEnabled: false}, preflightGroupCapabilityFlags(t, tx, inactiveGroupID))
}

func runMigration154InTx(t *testing.T, ctx context.Context, tx *sql.Tx) {
	t.Helper()
	path := filepath.Join("..", "..", "migrations", "154_api_key_entitlement_binding_preflight.sql")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err)
}

func runMigration155InTx(t *testing.T, ctx context.Context, tx *sql.Tx) {
	t.Helper()
	path := filepath.Join("..", "..", "migrations", "155_access_source_group_capabilities.sql")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err)
}

func insertPreflightUser(t *testing.T, tx *sql.Tx, email string) int64 {
	t.Helper()
	var id int64
	err := tx.QueryRowContext(context.Background(), `
		INSERT INTO users (email, password_hash, role, balance, concurrency, status, created_at, updated_at)
		VALUES ($1, 'hash', 'user', 0, 5, 'active', NOW(), NOW())
		RETURNING id
	`, email).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertPreflightGroup(t *testing.T, tx *sql.Tx, name string) int64 {
	t.Helper()
	var id int64
	err := tx.QueryRowContext(context.Background(), `
		INSERT INTO groups (name, status, platform, subscription_type, created_at, updated_at)
		VALUES ($1, 'active', 'openai', 'subscription', NOW(), NOW())
		RETURNING id
	`, name).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertPreflightGroupForAccessSource(
	t *testing.T,
	tx *sql.Tx,
	name string,
	subscriptionType string,
	status string,
	isExclusive bool,
	balanceEnabled bool,
	subscriptionEnabled bool,
	planAutoGrantEnabled bool,
) int64 {
	t.Helper()
	var id int64
	err := tx.QueryRowContext(context.Background(), `
		INSERT INTO groups (
			name, status, platform, subscription_type, is_exclusive,
			balance_enabled, subscription_enabled, plan_auto_grant_enabled,
			created_at, updated_at
		)
		VALUES ($1, $2, 'openai', $3, $4, $5, $6, $7, NOW(), NOW())
		RETURNING id
	`, name, status, subscriptionType, isExclusive, balanceEnabled, subscriptionEnabled, planAutoGrantEnabled).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertPreflightEntitlement(t *testing.T, tx *sql.Tx, userID, groupID int64, status string, startsAt, expiresAt time.Time, grantEnabled, deleted bool) int64 {
	t.Helper()
	var id int64
	var deletedAt any
	if deleted {
		deletedAt = time.Now().UTC()
	}
	err := tx.QueryRowContext(context.Background(), `
		INSERT INTO subscription_entitlements (
			user_id, primary_group_id, name, status, starts_at, expires_at,
			daily_usage_usd, weekly_usage_usd, monthly_usage_usd, overage_policy,
			created_at, updated_at, deleted_at
		)
		VALUES ($1, $2, 'preflight', $3, $4, $5, 0, 0, 0, 'block', NOW(), NOW(), $6)
		RETURNING id
	`, userID, groupID, status, startsAt, expiresAt, deletedAt).Scan(&id)
	require.NoError(t, err)

	_, err = tx.ExecContext(context.Background(), `
		INSERT INTO subscription_entitlement_groups (entitlement_id, group_id, sort_order, enabled, created_at, updated_at)
		VALUES ($1, $2, 0, $3, NOW(), NOW())
	`, id, groupID, grantEnabled)
	require.NoError(t, err)
	return id
}

func insertPreflightAPIKey(t *testing.T, tx *sql.Tx, userID, groupID, entitlementID int64, suffix string) int64 {
	t.Helper()
	var id int64
	err := tx.QueryRowContext(context.Background(), `
		INSERT INTO api_keys (user_id, key, name, group_id, status, subscription_entitlement_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'active', $5, NOW(), NOW())
		RETURNING id
	`, userID, "preflight-"+suffix, "preflight "+suffix, groupID, entitlementID).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertPreflightAPIKeyWithAccessSource(t *testing.T, tx *sql.Tx, userID, groupID int64, entitlementID sql.NullInt64, accessSource string, suffix string) int64 {
	t.Helper()
	var id int64
	err := tx.QueryRowContext(context.Background(), `
		INSERT INTO api_keys (user_id, key, name, group_id, status, subscription_entitlement_id, access_source, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'active', $5, $6, NOW(), NOW())
		RETURNING id
	`, userID, "preflight-"+suffix, "preflight "+suffix, groupID, entitlementID, accessSource).Scan(&id)
	require.NoError(t, err)
	return id
}

func preflightAPIKeyEntitlementID(t *testing.T, tx *sql.Tx, keyID int64) sql.NullInt64 {
	t.Helper()
	var entitlementID sql.NullInt64
	err := tx.QueryRowContext(context.Background(), `
		SELECT subscription_entitlement_id
		FROM api_keys
		WHERE id = $1
	`, keyID).Scan(&entitlementID)
	require.NoError(t, err)
	return entitlementID
}

func preflightAPIKeyAccessSource(t *testing.T, tx *sql.Tx, keyID int64) string {
	t.Helper()
	var accessSource string
	err := tx.QueryRowContext(context.Background(), `
		SELECT access_source
		FROM api_keys
		WHERE id = $1
	`, keyID).Scan(&accessSource)
	require.NoError(t, err)
	return accessSource
}

type preflightGroupCapabilities struct {
	BalanceEnabled       bool
	SubscriptionEnabled  bool
	PlanAutoGrantEnabled bool
}

func preflightGroupCapabilityFlags(t *testing.T, tx *sql.Tx, groupID int64) preflightGroupCapabilities {
	t.Helper()
	var caps preflightGroupCapabilities
	err := tx.QueryRowContext(context.Background(), `
		SELECT balance_enabled, subscription_enabled, plan_auto_grant_enabled
		FROM groups
		WHERE id = $1
	`, groupID).Scan(&caps.BalanceEnabled, &caps.SubscriptionEnabled, &caps.PlanAutoGrantEnabled)
	require.NoError(t, err)
	return caps
}
