package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration112UsesIdempotentAddColumn(t *testing.T) {
	content, err := FS.ReadFile("112_add_payment_order_provider_key_snapshot.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS provider_key VARCHAR(30)")
	require.NotContains(t, sql, "ADD COLUMN provider_key VARCHAR(30);")
}

func TestMigration118DoesNotForceOverwriteAuthSourceGrantDefaults(t *testing.T) {
	content, err := FS.ReadFile("118_wechat_dual_mode_and_auth_source_defaults.sql")
	require.NoError(t, err)

	sql := string(content)
	require.NotContains(t, sql, "UPDATE settings")
	require.NotContains(t, sql, "SET value = 'false'")
	require.True(t, strings.Contains(sql, "ON CONFLICT (key) DO NOTHING"))
	require.Contains(t, sql, "THEN ''")
}

func TestAuthIdentityReportTypeWideningRunsBeforeLongReportWritersAndStillReconcilesAt121(t *testing.T) {
	preflightContent, err := FS.ReadFile("108a_widen_auth_identity_migration_report_type.sql")
	require.NoError(t, err)

	preflightSQL := string(preflightContent)
	require.Contains(t, preflightSQL, "ALTER TABLE auth_identity_migration_reports")
	require.Contains(t, preflightSQL, "ALTER COLUMN report_type TYPE VARCHAR(80)")

	content, err := FS.ReadFile("109_auth_identity_compat_backfill.sql")
	require.NoError(t, err)

	sql := string(content)
	require.NotContains(t, sql, "ALTER TABLE auth_identity_migration_reports")

	followupContent, err := FS.ReadFile("121_auth_identity_migration_report_type_widen.sql")
	require.NoError(t, err)

	followupSQL := string(followupContent)
	require.Contains(t, followupSQL, "ALTER TABLE auth_identity_migration_reports")
	require.Contains(t, followupSQL, "ALTER COLUMN report_type TYPE VARCHAR(80)")
}

func TestMigration119DefersPaymentIndexRolloutToOnlineFollowup(t *testing.T) {
	content, err := FS.ReadFile("119_enforce_payment_orders_out_trade_no_unique.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "120_enforce_payment_orders_out_trade_no_unique_notx.sql")
	require.Contains(t, sql, "NULL;")
	require.NotContains(t, sql, "CREATE UNIQUE INDEX")
	require.NotContains(t, sql, "DROP INDEX")

	followupContent, err := FS.ReadFile("120_enforce_payment_orders_out_trade_no_unique_notx.sql")
	require.NoError(t, err)

	followupSQL := string(followupContent)
	require.Contains(t, followupSQL, "explicit duplicate out_trade_no precheck")
	require.Contains(t, followupSQL, "stale invalid paymentorder_out_trade_no_unique index")
	require.Contains(t, followupSQL, "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS paymentorder_out_trade_no_unique")
	require.NotContains(t, followupSQL, "DROP INDEX CONCURRENTLY IF EXISTS paymentorder_out_trade_no_unique")
	require.Contains(t, followupSQL, "DROP INDEX CONCURRENTLY IF EXISTS paymentorder_out_trade_no")
	require.Contains(t, followupSQL, "WHERE out_trade_no <> ''")

	alignmentContent, err := FS.ReadFile("120a_align_payment_orders_out_trade_no_index_name.sql")
	require.NoError(t, err)

	alignmentSQL := string(alignmentContent)
	require.Contains(t, alignmentSQL, "paymentorder_out_trade_no_unique")
	require.Contains(t, alignmentSQL, "RENAME TO paymentorder_out_trade_no")
}

func TestMigration110SeedsAuthSourceSignupGrantsDisabledByDefault(t *testing.T) {
	content, err := FS.ReadFile("110_pending_auth_and_provider_default_grants.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "('auth_source_default_email_grant_on_signup', 'false')")
	require.Contains(t, sql, "('auth_source_default_linuxdo_grant_on_signup', 'false')")
	require.Contains(t, sql, "('auth_source_default_oidc_grant_on_signup', 'false')")
	require.Contains(t, sql, "('auth_source_default_wechat_grant_on_signup', 'false')")
	require.NotContains(t, sql, "('auth_source_default_email_grant_on_signup', 'true')")
}

func TestMigration122ScrubsPendingOAuthCompletionTokensAtRest(t *testing.T) {
	content, err := FS.ReadFile("122_pending_auth_completion_token_cleanup.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "UPDATE pending_auth_sessions")
	require.Contains(t, sql, "completion_response")
	require.Contains(t, sql, "access_token")
	require.Contains(t, sql, "refresh_token")
	require.Contains(t, sql, "expires_in")
	require.Contains(t, sql, "token_type")
}

func TestMigration123BackfillsLegacyAuthSourceGrantDefaultsSafely(t *testing.T) {
	content, err := FS.ReadFile("123_fix_legacy_auth_source_grant_on_signup_defaults.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "110_pending_auth_and_provider_default_grants.sql")
	require.Contains(t, sql, "schema_migrations")
	require.Contains(t, sql, "updated_at")
	require.Contains(t, sql, "'_grant_on_signup'")
	require.Contains(t, sql, "value = 'false'")
	require.Contains(t, sql, "auth_identity_migration_reports")
}

func TestMigration124BackfillsLegacyOIDCSecurityFlagsSafely(t *testing.T) {
	content, err := FS.ReadFile("124_backfill_legacy_oidc_security_flags.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "oidc_connect_use_pkce")
	require.Contains(t, sql, "oidc_connect_validate_id_token")
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
	require.Contains(t, sql, "oidc_connect_enabled")
	require.Contains(t, sql, "'false'")
}

func TestMigration134AddsAffiliateLedgerAuditFieldsWithoutJSONCast(t *testing.T) {
	content, err := FS.ReadFile("134_affiliate_ledger_audit_snapshots.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS source_order_id BIGINT")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS balance_after DECIMAL(20,8)")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS aff_quota_after DECIMAL(20,8)")
	require.Contains(t, sql, "substring(")
	require.Contains(t, sql, `"rebateAmount"`)
	require.Contains(t, sql, "COUNT(*) OVER (PARTITION BY ra.order_id) AS order_match_count")
	require.Contains(t, sql, "COUNT(*) OVER (PARTITION BY ual.id) AS ledger_match_count")
	require.NotContains(t, sql, "detail::jsonb")
}

func TestMigration135AllowsGitHubAndGoogleAuthProviders(t *testing.T) {
	content, err := FS.ReadFile("135_allow_email_oauth_provider_types.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "users_signup_source_check")
	require.Contains(t, sql, "auth_identities_provider_type_check")
	require.Contains(t, sql, "auth_identity_channels_provider_type_check")
	require.Contains(t, sql, "pending_auth_sessions_provider_type_check")
	require.Contains(t, sql, "'github'")
	require.Contains(t, sql, "'google'")
}

func TestMigration150AddsSubscriptionEntitlementsV2Additively(t *testing.T) {
	content, err := FS.ReadFile("150_subscription_entitlements_v2.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS access_scope VARCHAR(32) NOT NULL DEFAULT 'explicit'")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS allowed_platforms JSONB NOT NULL DEFAULT '[]'::jsonb")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS subscription_plan_groups")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS subscription_plan_external_mappings")
	require.Contains(t, sql, "legacy_value         DECIMAL(20, 8) NOT NULL")
	require.Contains(t, sql, "ON subscription_plan_external_mappings(source, legacy_group_id, legacy_validity_days, legacy_value)")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS plan_id BIGINT REFERENCES subscription_plans(id) ON DELETE SET NULL")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS subscription_entitlements")
	require.Contains(t, sql, "source_id             BIGINT")
	require.Contains(t, sql, "source_external_id    VARCHAR(128)")
	require.Contains(t, sql, "source_redeem_code_id BIGINT REFERENCES redeem_codes(id) ON DELETE SET NULL")
	require.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlements_legacy_subscription_id_unique")
	require.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlements_source_id_unique")
	require.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlements_source_external_unique")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_subscription_entitlements_user_id")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_subscription_entitlements_status")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_subscription_entitlements_expires_at")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_subscription_plan_external_mappings_legacy_group_id")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_subscription_plan_external_mappings_enabled")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS subscription_entitlement_groups")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS subscription_entitlement_id BIGINT REFERENCES subscription_entitlements(id) ON DELETE SET NULL")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS entitlement_id BIGINT REFERENCES subscription_entitlements(id) ON DELETE SET NULL")
	require.Contains(t, sql, "ON CONFLICT (legacy_subscription_id) DO NOTHING")
	require.Contains(t, sql, "ON CONFLICT (entitlement_id, group_id) DO NOTHING")
	require.Contains(t, sql, "Historical usage_logs entitlement attribution is intentionally not backfilled")
	require.NotContains(t, sql, "UPDATE usage_logs")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
}

func TestMigration149aNormalizesLegacyImageUsageRowsBeforeEntitlementBackfill(t *testing.T) {
	content, err := FS.ReadFile("149a_backfill_usage_log_image_billing_size.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "UPDATE usage_logs")
	require.Contains(t, sql, "image_size = 'mixed'")
	require.Contains(t, sql, "image_size_source = 'legacy'")
	require.Contains(t, sql, "COALESCE(image_count, 0) > 0")
	require.Contains(t, sql, "image_size NOT IN ('1K', '2K', '4K', 'mixed')")
	require.NotContains(t, sql, "DELETE FROM")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
}

func TestMigration152AddsRedeemCodeSubscriptionEntitlementIDAdditively(t *testing.T) {
	content, err := FS.ReadFile("152_redeem_codes_subscription_entitlement_id.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ALTER TABLE redeem_codes")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS subscription_entitlement_id BIGINT REFERENCES subscription_entitlements(id) ON DELETE SET NULL")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_redeem_codes_subscription_entitlement_id")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
}

func TestMigration153AddsUsageLogBillingSourceAdditively(t *testing.T) {
	content, err := FS.ReadFile("153_usage_logs_billing_source.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ALTER TABLE usage_logs")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS billing_source VARCHAR(50)")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_usage_logs_billing_source_created_at")
	require.Contains(t, sql, "usage_logs_billing_source_check")
	require.Contains(t, sql, "entitlement_balance_fallback")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
}

func TestMigration154ClearsInvalidAPIKeyEntitlementBindingsConservatively(t *testing.T) {
	content, err := FS.ReadFile("154_api_key_entitlement_binding_preflight.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "UPDATE api_keys ak")
	require.Contains(t, sql, "SET subscription_entitlement_id = NULL")
	require.Contains(t, sql, "ak.subscription_entitlement_id IS NOT NULL")
	require.Contains(t, sql, "AND NOT EXISTS")
	require.Contains(t, sql, "se.id = ak.subscription_entitlement_id")
	require.Contains(t, sql, "se.deleted_at IS NULL")
	require.Contains(t, sql, "se.user_id = ak.user_id")
	require.Contains(t, sql, "se.status = 'active'")
	require.Contains(t, sql, "se.starts_at <= NOW()")
	require.Contains(t, sql, "se.expires_at > NOW()")
	require.Contains(t, sql, "seg.group_id = ak.group_id")
	require.Contains(t, sql, "seg.enabled = TRUE")
	require.NotContains(t, sql, "SET subscription_entitlement_id = se.id")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
}

func TestMigration155AddsAccessSourceAndGroupCapabilitiesAdditively(t *testing.T) {
	content, err := FS.ReadFile("155_access_source_group_capabilities.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS access_source VARCHAR(32) NOT NULL DEFAULT 'balance'")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS balance_enabled BOOLEAN NOT NULL DEFAULT TRUE")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS subscription_enabled BOOLEAN NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS plan_auto_grant_enabled BOOLEAN NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "WHEN subscription_entitlement_id IS NOT NULL THEN 'entitlement'")
	require.Contains(t, sql, "ELSE 'balance'")
	require.Contains(t, sql, "subscription_type = 'standard'")
	require.Contains(t, sql, "subscription_type = 'subscription'")
	require.Contains(t, sql, "COALESCE(is_exclusive, FALSE) = FALSE")
	require.Contains(t, sql, "status = 'active'")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_api_keys_access_source")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_groups_subscription_capabilities")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
	require.NotContains(t, sql, "SET subscription_entitlement_id")
}

func TestMigration156AddsEntitlementCycleResetLogsAdditively(t *testing.T) {
	content, err := FS.ReadFile("156_subscription_entitlement_cycle_reset_logs.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS subscription_entitlement_cycle_reset_logs")
	require.Contains(t, sql, "id                                  BIGSERIAL PRIMARY KEY")
	require.Contains(t, sql, "user_id                             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE")
	require.Contains(t, sql, "entitlement_id                      BIGINT NOT NULL REFERENCES subscription_entitlements(id) ON DELETE CASCADE")
	require.Contains(t, sql, "plan_id                             BIGINT REFERENCES subscription_plans(id) ON DELETE SET NULL")
	require.Contains(t, sql, "previous_expires_at                 TIMESTAMPTZ NOT NULL")
	require.Contains(t, sql, "new_expires_at                      TIMESTAMPTZ NOT NULL")
	require.Contains(t, sql, "previous_monthly_usage_usd          DECIMAL(20, 10) NOT NULL DEFAULT 0")
	require.Contains(t, sql, "previous_monthly_window_start       TIMESTAMPTZ")
	require.Contains(t, sql, "new_monthly_window_start            TIMESTAMPTZ NOT NULL")
	require.Contains(t, sql, "deducted_days                       INTEGER NOT NULL")
	require.Contains(t, sql, "deducted_seconds                    BIGINT NOT NULL")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_subscription_entitlement_cycle_reset_logs_user_created")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_subscription_entitlement_cycle_reset_logs_entitlement_created")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
	require.NotContains(t, sql, "INSERT INTO subscription_cycle_reset_logs")
}

func TestMigration159AddsCycleResetAuditFieldsAdditively(t *testing.T) {
	content, err := FS.ReadFile("159_subscription_cycle_reset_audit_fields.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ALTER TABLE subscription_cycle_reset_logs")
	require.Contains(t, sql, "ALTER TABLE subscription_entitlement_cycle_reset_logs")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS mode VARCHAR(64) NOT NULL DEFAULT 'advance_next_cycle'")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT ''")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS admin_id BIGINT REFERENCES users(id) ON DELETE SET NULL")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_subscription_cycle_reset_logs_admin_created")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_subscription_entitlement_cycle_reset_logs_admin_created")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
}

func TestMigration160AddsCycleResetUsageChoiceAdditively(t *testing.T) {
	content, err := FS.ReadFile("160_subscription_cycle_reset_usage_choice.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ALTER TABLE subscription_cycle_reset_logs")
	require.Contains(t, sql, "ALTER TABLE subscription_entitlement_cycle_reset_logs")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS reset_monthly_usage BOOLEAN NOT NULL DEFAULT TRUE")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
}

func TestMigration157BackfillsActiveEntitlementWindowsFromStartsAt(t *testing.T) {
	content, err := FS.ReadFile("157_backfill_entitlement_window_starts.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "daily_limit_usd IS NOT NULL AND daily_limit_usd > 0")
	require.Contains(t, sql, "weekly_limit_usd IS NOT NULL AND weekly_limit_usd > 0")
	require.Contains(t, sql, "monthly_limit_usd IS NOT NULL AND monthly_limit_usd > 0")
	require.Contains(t, sql, "COALESCE(daily_window_start, starts_at)")
	require.Contains(t, sql, "COALESCE(weekly_window_start, starts_at)")
	require.Contains(t, sql, "COALESCE(monthly_window_start, starts_at)")
	require.Contains(t, sql, "status = 'active'")
	require.Contains(t, sql, "deleted_at IS NULL")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
}

func TestSubscriptionEntitlementsV2PreflightRunbookCoversSwitchAuditQueries(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "plans", "subscription-entitlements-v2-preflight-sql.md")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	doc := string(content)
	require.Contains(t, doc, "invalid_api_key_entitlement_bindings")
	require.Contains(t, doc, "owner_mismatch")
	require.Contains(t, doc, "future_start")
	require.Contains(t, doc, "expired")
	require.Contains(t, doc, "group_not_covered")
	require.Contains(t, doc, "HAVING COUNT(*) > 1")
	require.Contains(t, doc, "entitlement_only_records")
	require.Contains(t, doc, "billing_source")
	require.Contains(t, doc, "entitlement_balance_fallback")
	require.Contains(t, doc, "subscription_entitlements_v2_enabled=false")
	require.Contains(t, doc, "sub2_payment_page_legacy_mapping_enabled=false")
	require.Contains(t, doc, "Do not delete entitlement schema or history")
}

func TestMigration151AddsAccountAutoPauseExpiryPartialIndex(t *testing.T) {
	content, err := FS.ReadFile("151_account_autopause_expiry_index_notx.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_autopause_expiry_due")
	require.Contains(t, sql, "ON accounts (expires_at)")
	require.Contains(t, sql, "WHERE deleted_at IS NULL")
	require.Contains(t, sql, "schedulable = TRUE")
	require.Contains(t, sql, "auto_pause_on_expired = TRUE")
	require.Contains(t, sql, "expires_at IS NOT NULL")
}

func TestMigration158BackfillsGrokMediaGenerationGroups(t *testing.T) {
	content, err := FS.ReadFile("158_enable_grok_media_generation_groups.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "UPDATE groups")
	require.Contains(t, sql, "SET allow_image_generation = true")
	require.Contains(t, sql, "WHERE platform = 'grok'")
	require.Contains(t, sql, "AND allow_image_generation = false")
}

func TestMigration154AddsSparkShadowColumnsAndConstraintsWithoutHotIndexes(t *testing.T) {
	content, err := FS.ReadFile("154_account_spark_shadow.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS parent_account_id BIGINT")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS quota_dimension VARCHAR(20) NOT NULL DEFAULT 'global'")
	require.Contains(t, sql, "chk_accounts_parent_dimension")
	// 约束已放开为「影子 ⇒ 非 global 维度」（spark 不再写死进 parent 约束）
	require.Contains(t, sql, "parent_account_id IS NOT NULL AND quota_dimension <> 'global'")
	require.NotContains(t, sql, "parent_account_id IS NOT NULL AND quota_dimension = 'spark'")
	require.Contains(t, sql, "chk_accounts_parent_not_self")
	require.Contains(t, sql, "fk_accounts_parent_account_id")
	require.Contains(t, sql, "FOREIGN KEY (parent_account_id) REFERENCES accounts(id)")
	require.Contains(t, sql, "ON DELETE RESTRICT")
	require.Contains(t, sql, "NOT VALID")
	require.NotContains(t, sql, "CREATE INDEX")
	require.NotContains(t, sql, "CREATE UNIQUE INDEX")
	require.NotContains(t, sql, "CONCURRENTLY")
}

func TestMigration154aAddsSparkShadowIndexesConcurrently(t *testing.T) {
	content, err := FS.ReadFile("154a_account_spark_shadow_indexes_notx.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_parent_account_id")
	require.Contains(t, sql, "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_accounts_spark_shadow_per_parent")
	require.Contains(t, sql, "ON accounts (parent_account_id)")
	require.Contains(t, sql, "WHERE parent_account_id IS NOT NULL")
	require.Contains(t, sql, "quota_dimension = 'spark'")
	require.Contains(t, sql, "deleted_at IS NULL")
}

func TestMigration161AllowsCyberBlockedUsageRequestType(t *testing.T) {
	content, err := FS.ReadFile("161_allow_cyber_usage_request_type.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS usage_logs_request_type_check")
	require.Contains(t, sql, "ADD CONSTRAINT usage_logs_request_type_check")
	require.Contains(t, sql, "CHECK (request_type IN (0, 1, 2, 3, 4)) NOT VALID")
}
