//go:build integration

package repository

// migration 193 回归：groups 触发器的 durable 失效监视清单必须覆盖认证快照中
// 所有会影响授权、调度、限额或计费的字段（正常后台保存走 InvalidateAuthCacheByGroupID
// 即时失效，触发器兜底直改 SQL / 更新与失效之间崩溃等 out-of-band 场景）。

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestAuthCacheInvalidationTrigger_AuthSnapshotColumns(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	group := mustCreateGroup(t, integrationEntClient, &service.Group{
		Name: fmt.Sprintf("profit-trigger-group-%d", suffix), Platform: service.PlatformOpenAI, RateMultiplier: 1,
	})
	user := mustCreateUser(t, integrationEntClient, &service.User{
		Email: fmt.Sprintf("profit-trigger-%d@example.com", suffix), Concurrency: 5,
	})
	groupID := group.ID
	keyValue := fmt.Sprintf("sk-profit-trigger-%d", suffix)
	apiKeyRepo := NewAPIKeyRepository(integrationEntClient, integrationDB)
	key := &service.APIKey{UserID: user.ID, GroupID: &groupID, Key: keyValue, Name: "profit-trigger", Status: service.StatusActive}
	require.NoError(t, apiKeyRepo.Create(ctx, key))

	sum := sha256.Sum256([]byte(keyValue))
	cacheKey := hex.EncodeToString(sum[:])
	clear := func() {
		_, err := integrationDB.ExecContext(ctx, "DELETE FROM auth_cache_invalidation_outbox WHERE cache_key = $1", cacheKey)
		require.NoError(t, err)
	}
	count := func() int {
		var value int
		require.NoError(t, integrationDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM auth_cache_invalidation_outbox WHERE cache_key = $1", cacheKey).Scan(&value))
		return value
	}
	clear()
	t.Cleanup(clear)
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, "DELETE FROM api_keys WHERE id = $1", key.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id = $1", group.ID)
		require.NoError(t, err)
	})

	_, err := integrationDB.ExecContext(ctx, "UPDATE groups SET name = name || '-cosmetic' WHERE id = $1", group.ID)
	require.NoError(t, err)
	require.Zero(t, count(), "cosmetic 更新不得入队（既有语义回归）")

	_, err = integrationDB.ExecContext(ctx, "UPDATE groups SET profit_control_enabled = NOT profit_control_enabled WHERE id = $1", group.ID)
	require.NoError(t, err)
	require.Equal(t, 1, count(), "profit_control_enabled 变更必须入队")
	clear()

	_, err = integrationDB.ExecContext(ctx, "UPDATE groups SET profit_min_margin = 0.3 WHERE id = $1", group.ID)
	require.NoError(t, err)
	require.Equal(t, 1, count(), "profit_min_margin 变更必须入队")
	clear()

	_, err = integrationDB.ExecContext(ctx, "UPDATE groups SET profit_safety_buffer = 0.02 WHERE id = $1", group.ID)
	require.NoError(t, err)
	require.Equal(t, 1, count(), "profit_safety_buffer 变更必须入队")
	clear()

	_, err = integrationDB.ExecContext(ctx, "UPDATE groups SET profit_min_margin = profit_min_margin WHERE id = $1", group.ID)
	require.NoError(t, err)
	require.Zero(t, count(), "利润字段无实际变化的 UPDATE 不得入队")

	for name, update := range map[string]string{
		"status":                               "status = 'disabled'",
		"is_exclusive":                         "is_exclusive = true",
		"platform":                             "platform = 'anthropic'",
		"subscription_type":                    "subscription_type = 'subscription'",
		"balance_enabled":                      "balance_enabled = false",
		"subscription_enabled":                 "subscription_enabled = true",
		"plan_auto_grant_enabled":              "plan_auto_grant_enabled = true",
		"rate_multiplier":                      "rate_multiplier = 0.9",
		"daily_limit_usd":                      "daily_limit_usd = 1.1",
		"weekly_limit_usd":                     "weekly_limit_usd = 2.2",
		"monthly_limit_usd":                    "monthly_limit_usd = 3.3",
		"allow_image_generation":               "allow_image_generation = true",
		"allow_batch_image_generation":         "allow_batch_image_generation = true",
		"image_rate_independent":               "image_rate_independent = true",
		"image_rate_multiplier":                "image_rate_multiplier = 0.8",
		"image_price_1k":                       "image_price_1k = 0.01",
		"image_price_2k":                       "image_price_2k = 0.02",
		"image_price_4k":                       "image_price_4k = 0.04",
		"video_rate_independent":               "video_rate_independent = true",
		"video_rate_multiplier":                "video_rate_multiplier = 0.7",
		"video_price_480p":                     "video_price_480p = 0.10",
		"video_price_720p":                     "video_price_720p = 0.20",
		"video_price_1080p":                    "video_price_1080p = 0.30",
		"video_model_prices":                   `video_model_prices = '{"grok-imagine-video": {"720p": 0.40}}'::jsonb`,
		"web_search_price_per_call":            "web_search_price_per_call = 0.05",
		"search_price_per_1k":                  "search_price_per_1k = 0.06",
		"audio_realtime_price_per_min":         "audio_realtime_price_per_min = 0.07",
		"audio_tts_price_per_million_chars":    "audio_tts_price_per_million_chars = 0.08",
		"audio_stt_price_per_hour":             "audio_stt_price_per_hour = 0.09",
		"claude_code_only":                     "claude_code_only = true",
		"fallback_group_id":                    "fallback_group_id = $1",
		"fallback_group_id_on_invalid_request": "fallback_group_id_on_invalid_request = $1",
		"model_routing":                        `model_routing = '{"gpt-*": [1]}'::jsonb`,
		"model_routing_enabled":                "model_routing_enabled = true",
		"mcp_xml_inject":                       "mcp_xml_inject = false",
		"supported_model_scopes":               `supported_model_scopes = '["claude"]'::jsonb`,
		"allow_messages_dispatch":              "allow_messages_dispatch = true",
		"allow_live":                           "allow_live = true",
		"default_mapped_model":                 "default_mapped_model = 'gpt-5'",
		"messages_dispatch_model_config":       `messages_dispatch_model_config = '{"default_model": "gpt-5"}'::jsonb`,
		"models_list_config":                   `models_list_config = '{"models": ["gpt-5"]}'::jsonb`,
		"rpm_limit":                            "rpm_limit = 123",
		"max_reasoning_effort":                 "max_reasoning_effort = 'high'",
		"reasoning_effort_mappings":            `reasoning_effort_mappings = '[{"from": "high", "to": "medium"}]'::jsonb`,
		"peak_rate_enabled":                    "peak_rate_enabled = true",
		"peak_start":                           "peak_start = '08:00'",
		"peak_end":                             "peak_end = '09:00'",
		"peak_rate_multiplier":                 "peak_rate_multiplier = 1.2",
	} {
		t.Run(name, func(t *testing.T) {
			clear()
			_, err := integrationDB.ExecContext(ctx, "UPDATE groups SET "+update+" WHERE id = $1", group.ID)
			require.NoError(t, err)
			require.Equal(t, 1, count(), name+" 变更必须入队")
		})
	}
}

func TestMigration221BackfillsMediaPricingAuthCacheInvalidations(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	user := mustCreateUser(t, integrationEntClient, &service.User{
		Email: fmt.Sprintf("media-pricing-backfill-%d@example.com", suffix), Concurrency: 5,
	})

	backupGroup := mustCreateGroup(t, integrationEntClient, &service.Group{
		Name: fmt.Sprintf("media-pricing-backup-%d", suffix), Platform: service.PlatformOpenAI, RateMultiplier: 1,
	})
	currentGroup := mustCreateGroup(t, integrationEntClient, &service.Group{
		Name: fmt.Sprintf("media-pricing-current-%d", suffix), Platform: service.PlatformOpenAI, RateMultiplier: 1,
	})
	unaffectedGroup := mustCreateGroup(t, integrationEntClient, &service.Group{
		Name: fmt.Sprintf("media-pricing-unaffected-%d", suffix), Platform: service.PlatformOpenAI, RateMultiplier: 1,
	})

	apiKeyRepo := NewAPIKeyRepository(integrationEntClient, integrationDB)
	type keyedGroup struct {
		group *service.Group
		key   *service.APIKey
		hash  string
	}
	keyedGroups := make([]keyedGroup, 0, 3)
	for label, group := range map[string]*service.Group{
		"backup": backupGroup, "current": currentGroup, "unaffected": unaffectedGroup,
	} {
		groupID := group.ID
		keyValue := fmt.Sprintf("sk-media-pricing-%s-%d", label, suffix)
		key := &service.APIKey{
			UserID: user.ID, GroupID: &groupID, Key: keyValue, Name: "media-pricing-" + label, Status: service.StatusActive,
		}
		require.NoError(t, apiKeyRepo.Create(ctx, key))
		sum := sha256.Sum256([]byte(keyValue))
		keyedGroups = append(keyedGroups, keyedGroup{group: group, key: key, hash: hex.EncodeToString(sum[:])})
	}

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM groups_video_price_backup_220 WHERE group_id IN ($1, $2, $3)", backupGroup.ID, currentGroup.ID, unaffectedGroup.ID)
		for _, item := range keyedGroups {
			_, _ = integrationDB.ExecContext(ctx, "DELETE FROM api_keys WHERE id = $1", item.key.ID)
		}
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id IN ($1, $2, $3)", backupGroup.ID, currentGroup.ID, unaffectedGroup.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
		for _, item := range keyedGroups {
			_, _ = integrationDB.ExecContext(ctx, "DELETE FROM auth_cache_invalidation_outbox WHERE cache_key = $1", item.hash)
		}
	})

	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO groups_video_price_backup_220 (
    group_id, platform, video_price_480p, video_price_720p,
    video_price_1080p, video_model_prices, backed_up_at
) VALUES ($1, 'openai', NULL, NULL, NULL, '{"grok-imagine-video":{"720p":0.40}}'::jsonb, now())
`, backupGroup.ID)
	require.NoError(t, err)

	_, err = integrationDB.ExecContext(ctx, `
UPDATE groups
SET video_model_prices = '{"grok-imagine-video":{"720p":0.40}}'::jsonb,
    search_price_per_1k = 0.06,
    audio_realtime_price_per_min = 0.07,
    audio_tts_price_per_million_chars = 0.08,
    audio_stt_price_per_hour = 0.09
WHERE id = $1
`, currentGroup.ID)
	require.NoError(t, err)

	for _, item := range keyedGroups {
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM auth_cache_invalidation_outbox WHERE cache_key = $1", item.hash)
		require.NoError(t, err)
	}

	migrationSQL, err := dbmigrations.FS.ReadFile("221_group_media_pricing_auth_cache_invalidation.sql")
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	count := func(cacheKey string) int {
		var value int
		require.NoError(t, integrationDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM auth_cache_invalidation_outbox WHERE cache_key = $1", cacheKey).Scan(&value))
		return value
	}
	var hashesByGroup = make(map[int64]string, len(keyedGroups))
	for _, item := range keyedGroups {
		hashesByGroup[item.group.ID] = item.hash
	}
	require.Equal(t, 1, count(hashesByGroup[backupGroup.ID]), "220 备份涉及的分组必须补投")
	require.Equal(t, 1, count(hashesByGroup[currentGroup.ID]), "当前持有新媒体价格的分组必须补投")
	require.Zero(t, count(hashesByGroup[unaffectedGroup.ID]), "无关分组不得补投")

	_, err = integrationDB.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	require.Equal(t, 1, count(hashesByGroup[backupGroup.ID]), "migration 重跑不得重复补投待办")
	require.Equal(t, 1, count(hashesByGroup[currentGroup.ID]), "migration 重跑不得重复补投待办")
}
