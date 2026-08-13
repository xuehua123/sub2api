package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration223AddsGroupPricingAndExtendsAuthCacheInvalidation(t *testing.T) {
	content, err := FS.ReadFile("223_group_model_pricing_auth_cache_invalidation.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "SET LOCAL lock_timeout = '5s';")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS long_context_pricing_enabled BOOLEAN NOT NULL DEFAULT TRUE")
	require.Contains(t, sql, "ALTER COLUMN long_context_pricing_enabled SET DEFAULT TRUE")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS model_pricing JSONB")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION enqueue_group_auth_cache_invalidation()")
	require.Contains(t, sql, "OLD.long_context_pricing_enabled IS NOT DISTINCT FROM NEW.long_context_pricing_enabled")
	require.Contains(t, sql, "OLD.model_pricing IS NOT DISTINCT FROM NEW.model_pricing")
	require.Contains(t, sql, "SET long_context_pricing_enabled = TRUE")
}

func TestUpstreamGroupPricingMigrationWasRenumberedAfterAppliedForkMigrations(t *testing.T) {
	_, err := FS.ReadFile("221_group_model_pricing.sql")
	require.Error(t, err, "upstream migration 221 must not collide with the fork's already-applied migration 221")

	_, err = FS.ReadFile("221_group_media_pricing_auth_cache_invalidation.sql")
	require.NoError(t, err)
	_, err = FS.ReadFile("222_usage_billing_dedup_billing_source.sql")
	require.NoError(t, err)
	_, err = FS.ReadFile("223_group_model_pricing_auth_cache_invalidation.sql")
	require.NoError(t, err)
}
