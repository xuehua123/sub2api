-- Media pricing fields added by migrations 217-219 are part of the API-key
-- auth snapshot and billing inputs. Extend the cumulative durable invalidation
-- trigger so direct SQL updates cannot leave cached snapshots using stale
-- prices. Keep this function cumulative with the fork extensions through
-- migration 193.

-- Migration 220 documented this snapshot as disposable after operator review.
-- Recreate an empty compatible table when it was deliberately dropped so the
-- guarded recovery and invalidation backfill below remain upgrade-safe.
CREATE TABLE IF NOT EXISTS groups_video_price_backup_220 (
    group_id BIGINT,
    platform TEXT,
    video_price_480p DECIMAL(20,8),
    video_price_720p DECIMAL(20,8),
    video_price_1080p DECIMAL(20,8),
    video_model_prices JSONB,
    backed_up_at TIMESTAMPTZ
);

CREATE OR REPLACE FUNCTION enqueue_group_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_group_id BIGINT;
BEGIN
    target_group_id := OLD.id;
    IF TG_OP = 'UPDATE'
       AND OLD.status IS NOT DISTINCT FROM NEW.status
       AND OLD.is_exclusive IS NOT DISTINCT FROM NEW.is_exclusive
       AND OLD.platform IS NOT DISTINCT FROM NEW.platform
       AND OLD.subscription_type IS NOT DISTINCT FROM NEW.subscription_type
       AND OLD.balance_enabled IS NOT DISTINCT FROM NEW.balance_enabled
       AND OLD.subscription_enabled IS NOT DISTINCT FROM NEW.subscription_enabled
       AND OLD.plan_auto_grant_enabled IS NOT DISTINCT FROM NEW.plan_auto_grant_enabled
       AND OLD.rate_multiplier IS NOT DISTINCT FROM NEW.rate_multiplier
       AND OLD.daily_limit_usd IS NOT DISTINCT FROM NEW.daily_limit_usd
       AND OLD.weekly_limit_usd IS NOT DISTINCT FROM NEW.weekly_limit_usd
       AND OLD.monthly_limit_usd IS NOT DISTINCT FROM NEW.monthly_limit_usd
       AND OLD.allow_image_generation IS NOT DISTINCT FROM NEW.allow_image_generation
       AND OLD.allow_batch_image_generation IS NOT DISTINCT FROM NEW.allow_batch_image_generation
       AND OLD.image_rate_independent IS NOT DISTINCT FROM NEW.image_rate_independent
       AND OLD.image_rate_multiplier IS NOT DISTINCT FROM NEW.image_rate_multiplier
       AND OLD.image_price_1k IS NOT DISTINCT FROM NEW.image_price_1k
       AND OLD.image_price_2k IS NOT DISTINCT FROM NEW.image_price_2k
       AND OLD.image_price_4k IS NOT DISTINCT FROM NEW.image_price_4k
       AND OLD.video_rate_independent IS NOT DISTINCT FROM NEW.video_rate_independent
       AND OLD.video_rate_multiplier IS NOT DISTINCT FROM NEW.video_rate_multiplier
       AND OLD.video_price_480p IS NOT DISTINCT FROM NEW.video_price_480p
       AND OLD.video_price_720p IS NOT DISTINCT FROM NEW.video_price_720p
       AND OLD.video_price_1080p IS NOT DISTINCT FROM NEW.video_price_1080p
       AND OLD.video_model_prices IS NOT DISTINCT FROM NEW.video_model_prices
       AND OLD.web_search_price_per_call IS NOT DISTINCT FROM NEW.web_search_price_per_call
       AND OLD.search_price_per_1k IS NOT DISTINCT FROM NEW.search_price_per_1k
       AND OLD.audio_realtime_price_per_min IS NOT DISTINCT FROM NEW.audio_realtime_price_per_min
       AND OLD.audio_tts_price_per_million_chars IS NOT DISTINCT FROM NEW.audio_tts_price_per_million_chars
       AND OLD.audio_stt_price_per_hour IS NOT DISTINCT FROM NEW.audio_stt_price_per_hour
       AND OLD.claude_code_only IS NOT DISTINCT FROM NEW.claude_code_only
       AND OLD.fallback_group_id IS NOT DISTINCT FROM NEW.fallback_group_id
       AND OLD.fallback_group_id_on_invalid_request IS NOT DISTINCT FROM NEW.fallback_group_id_on_invalid_request
       AND OLD.model_routing IS NOT DISTINCT FROM NEW.model_routing
       AND OLD.model_routing_enabled IS NOT DISTINCT FROM NEW.model_routing_enabled
       AND OLD.mcp_xml_inject IS NOT DISTINCT FROM NEW.mcp_xml_inject
       AND OLD.supported_model_scopes IS NOT DISTINCT FROM NEW.supported_model_scopes
       AND OLD.allow_messages_dispatch IS NOT DISTINCT FROM NEW.allow_messages_dispatch
       AND OLD.allow_live IS NOT DISTINCT FROM NEW.allow_live
       AND OLD.default_mapped_model IS NOT DISTINCT FROM NEW.default_mapped_model
       AND OLD.messages_dispatch_model_config IS NOT DISTINCT FROM NEW.messages_dispatch_model_config
       AND OLD.models_list_config IS NOT DISTINCT FROM NEW.models_list_config
       AND OLD.rpm_limit IS NOT DISTINCT FROM NEW.rpm_limit
       AND OLD.max_reasoning_effort IS NOT DISTINCT FROM NEW.max_reasoning_effort
       AND OLD.reasoning_effort_mappings IS NOT DISTINCT FROM NEW.reasoning_effort_mappings
       AND OLD.peak_rate_enabled IS NOT DISTINCT FROM NEW.peak_rate_enabled
       AND OLD.peak_start IS NOT DISTINCT FROM NEW.peak_start
       AND OLD.peak_end IS NOT DISTINCT FROM NEW.peak_end
       AND OLD.peak_rate_multiplier IS NOT DISTINCT FROM NEW.peak_rate_multiplier
       AND OLD.profit_control_enabled IS NOT DISTINCT FROM NEW.profit_control_enabled
       AND OLD.profit_min_margin IS NOT DISTINCT FROM NEW.profit_min_margin
       AND OLD.profit_safety_buffer IS NOT DISTINCT FROM NEW.profit_safety_buffer
       AND OLD.deleted_at IS NOT DISTINCT FROM NEW.deleted_at THEN
        RETURN NEW;
    END IF;

    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT encode(sha256(convert_to(k.key, 'UTF8')), 'hex')
    FROM api_keys AS k
    WHERE k.group_id = target_group_id
      AND k.deleted_at IS NULL
      AND k.key <> '';
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

-- An early fork release candidate ran migration 220 before composite groups
-- were excluded, so their video pricing was snapshotted and then cleared. The
-- migration runner stores SHA-256 over TrimSpace(content); restore only when
-- that exact legacy checksum is recorded (or its known raw-file equivalent),
-- the group is still composite, every video price remains empty, and no later
-- application update touched the group. This avoids overwriting operator
-- changes while repairing rows whose empty state is attributable to old 220.
UPDATE groups AS g
SET video_price_480p = backup.video_price_480p,
    video_price_720p = backup.video_price_720p,
    video_price_1080p = backup.video_price_1080p,
    video_model_prices = backup.video_model_prices,
    updated_at = NOW()
FROM groups_video_price_backup_220 AS backup
WHERE g.id = backup.group_id
  AND g.platform = 'composite'
  AND backup.platform = 'composite'
  AND g.video_price_480p IS NULL
  AND g.video_price_720p IS NULL
  AND g.video_price_1080p IS NULL
  AND g.video_model_prices IS NULL
  AND (
      backup.video_price_480p IS NOT NULL
      OR backup.video_price_720p IS NOT NULL
      OR backup.video_price_1080p IS NOT NULL
      OR backup.video_model_prices IS NOT NULL
  )
  AND g.updated_at <= backup.backed_up_at
  AND EXISTS (
      SELECT 1
      FROM schema_migrations AS applied
      WHERE applied.filename = '220_clear_non_grok_video_generation_config.sql'
        AND applied.checksum IN (
            '353c8e8e1805f2a6fd61311e03118e7dd8388f264cfd9af9e0cabe2a696388c4',
            '3da48c8fdffe6390325f43d08b8e353e0a365df43d44a78dbbe655d0deb18402'
        )
  );

-- A release candidate may have applied migration 220 before this trigger knew
-- about video_model_prices. In that window, clearing only that JSON field did
-- not enqueue invalidation. Backfill both the 220 snapshot population and any
-- group that currently carries pricing added by migrations 217-219. Exclude
-- keys already waiting in the outbox so applying this idempotent SQL again does
-- not amplify pending work.
WITH affected_group_ids AS (
    SELECT group_id
    FROM groups_video_price_backup_220

    UNION

    SELECT id
    FROM groups
    WHERE video_model_prices IS NOT NULL
       OR search_price_per_1k IS NOT NULL
       OR audio_realtime_price_per_min IS NOT NULL
       OR audio_tts_price_per_million_chars IS NOT NULL
       OR audio_stt_price_per_hour IS NOT NULL
), affected_cache_keys AS (
    SELECT DISTINCT encode(sha256(convert_to(k.key, 'UTF8')), 'hex') AS cache_key
    FROM api_keys AS k
    INNER JOIN affected_group_ids AS affected ON affected.group_id = k.group_id
    WHERE k.deleted_at IS NULL
      AND k.key <> ''
)
INSERT INTO auth_cache_invalidation_outbox (cache_key)
SELECT affected.cache_key
FROM affected_cache_keys AS affected
WHERE NOT EXISTS (
    SELECT 1
    FROM auth_cache_invalidation_outbox AS pending
    WHERE pending.cache_key = affected.cache_key
);
