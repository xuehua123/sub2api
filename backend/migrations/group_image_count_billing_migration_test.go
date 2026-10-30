package migrations

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration249ExtendsCumulativeGroupAuthCacheInvalidation(t *testing.T) {
	previous, err := FS.ReadFile("223_group_model_pricing_auth_cache_invalidation.sql")
	require.NoError(t, err)
	content, err := FS.ReadFile("249_group_image_count_billing_auth_cache_invalidation.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "SET LOCAL lock_timeout = '5s';")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION enqueue_group_auth_cache_invalidation()")
	require.Contains(t, sql, "INSERT INTO auth_cache_invalidation_outbox (cache_key)")
	require.Contains(t, sql, "WHERE k.group_id = target_group_id")
	for _, field := range []string{
		"image_count_billing",
		"force_openai_fast",
		"free_openai_fast",
		"max_reasoning_effort_over_limit",
		"model_allowlist",
		"codex_models_manifest_config",
	} {
		require.Contains(t, sql, "OLD."+field+" IS NOT DISTINCT FROM NEW."+field)
	}

	// Replacing a cumulative function must retain every previously watched
	// authorization and billing field, including the fork's entitlement fields.
	comparisons := regexp.MustCompile(`OLD\.[a-z0-9_]+ IS NOT DISTINCT FROM NEW\.[a-z0-9_]+`).FindAllString(string(previous), -1)
	require.NotEmpty(t, comparisons)
	for _, comparison := range comparisons {
		require.Contains(t, sql, comparison)
	}
}
