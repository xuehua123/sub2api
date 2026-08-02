-- Profit-control fields are part of the API-key auth snapshot and gate the
-- scheduling admission filter; the profit threshold D additionally depends on
-- group pricing and peak-window fields. Extend the durable invalidation
-- trigger so out-of-band group edits (direct SQL, crash between update and
-- app-level invalidation) cannot leave cached snapshots using stale auth,
-- routing, limit, or billing inputs. Normal admin saves already invalidate via
-- InvalidateAuthCacheByGroupID; this trigger is the durable backstop. Keep the
-- final function body cumulative with the fork extensions through migration 191.

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
       AND OLD.web_search_price_per_call IS NOT DISTINCT FROM NEW.web_search_price_per_call
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
