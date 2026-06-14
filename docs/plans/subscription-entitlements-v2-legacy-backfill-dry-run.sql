-- Subscription Entitlements V2 legacy backfill dry-run queries.
--
-- Safety:
-- - Read-only: this file intentionally contains no INSERT, UPDATE, DELETE,
--   ALTER, CREATE, DROP, or TRUNCATE statements.
-- - Run on staging first.
-- - On production, run only through a read-only connection and save redacted
--   output outside the repository.
-- - Do not print API key secrets, provider credentials, emails, payment source
--   ids, source_external_id values, or notes.
--
-- Assumptions:
-- - Before schema deployment, run sections that only reference legacy tables.
-- - After V2 schema deployment, run all sections while both flags remain false.
-- - Fill/manual-review runtime group mapping before any future write path.

-- ============================================================================
-- 1. Legacy inventory: old groups as plan templates and runtime candidates.
-- ============================================================================

WITH old_groups AS (
    SELECT
        g.id AS old_group_id,
        g.name AS old_group_name,
        g.platform,
        g.subscription_type,
        g.status,
        g.deleted_at,
        g.is_exclusive,
        g.rate_multiplier,
        g.daily_limit_usd,
        g.weekly_limit_usd,
        g.monthly_limit_usd,
        g.default_validity_days,
        COALESCE(subs.active_subscription_count, 0) AS active_subscription_count,
        COALESCE(keys.active_api_key_count, 0) AS active_api_key_count,
        COALESCE(accts.account_count, 0) AS account_count,
        COALESCE(accts.active_schedulable_account_count, 0) AS active_schedulable_account_count,
        COALESCE(accts.account_ids, ARRAY[]::BIGINT[]) AS account_ids,
        CASE
            WHEN g.deleted_at IS NOT NULL THEN 'deleted_group'
            WHEN g.status <> 'active' THEN 'inactive_group'
            WHEN COALESCE(g.is_exclusive, FALSE) AND g.subscription_type = 'subscription'
                 THEN 'legacy_exclusive_group_used_as_plan_template_source'
            WHEN COALESCE(g.is_exclusive, FALSE)
                 THEN 'exclusive_balance_or_runtime_manual_review'
            WHEN g.name ~* '(test|测试|negative|负向|deprecated|停用)' THEN 'test_or_negative_manual_review'
            WHEN g.subscription_type = 'subscription'
                 AND COALESCE(accts.active_schedulable_account_count, 0) = 0
                 THEN 'subscription_group_no_schedulable_accounts'
            ELSE 'ok'
        END AS group_review_reason,
        CASE
            WHEN lower(g.platform) IN ('openai', 'codex')
                 OR g.name ~* '(openai|codex|gpt)'
                 THEN 'openai_codex'
            WHEN lower(g.platform) IN ('anthropic', 'claude', 'kiro')
                 OR g.name ~* '(anthropic|claude|kiro)'
                 THEN 'anthropic_kiro_claude'
            ELSE 'manual_platform_review'
        END AS proposed_platform_family
    FROM groups g
    LEFT JOIN (
        SELECT group_id, COUNT(*) AS active_subscription_count
        FROM user_subscriptions
        WHERE deleted_at IS NULL
          AND status = 'active'
          AND starts_at <= NOW()
          AND expires_at > NOW()
        GROUP BY group_id
    ) subs ON subs.group_id = g.id
    LEFT JOIN (
        SELECT group_id, COUNT(*) AS active_api_key_count
        FROM api_keys
        WHERE deleted_at IS NULL
          AND status = 'active'
        GROUP BY group_id
    ) keys ON keys.group_id = g.id
    LEFT JOIN (
        SELECT
            ag.group_id,
            COUNT(DISTINCT ag.account_id) AS account_count,
            COUNT(DISTINCT ag.account_id) FILTER (
                WHERE a.deleted_at IS NULL
                  AND a.status = 'active'
                  AND COALESCE(a.schedulable, TRUE) = TRUE
            ) AS active_schedulable_account_count,
            ARRAY_AGG(DISTINCT ag.account_id ORDER BY ag.account_id) AS account_ids
        FROM account_groups ag
        JOIN accounts a ON a.id = ag.account_id
        GROUP BY ag.group_id
    ) accts ON accts.group_id = g.id
    WHERE g.deleted_at IS NULL
)
SELECT
    old_group_id,
    old_group_name,
    platform,
    subscription_type,
    status,
    is_exclusive,
    rate_multiplier,
    daily_limit_usd,
    weekly_limit_usd,
    monthly_limit_usd,
    default_validity_days,
    active_subscription_count,
    active_api_key_count,
    account_count,
    active_schedulable_account_count,
    proposed_platform_family,
    proposed_platform_family || ':' || rate_multiplier::TEXT AS proposed_runtime_group_key,
    group_review_reason
FROM old_groups
ORDER BY
    subscription_type DESC,
    proposed_platform_family,
    rate_multiplier,
    old_group_id;

-- ============================================================================
-- 2. Runtime group candidates and account-pool consistency.
-- ============================================================================

WITH old_groups AS (
    SELECT
        g.id AS old_group_id,
        g.name AS old_group_name,
        g.platform,
        g.subscription_type,
        g.status,
        g.deleted_at,
        g.is_exclusive,
        g.rate_multiplier,
        CASE
            WHEN lower(g.platform) IN ('openai', 'codex')
                 OR g.name ~* '(openai|codex|gpt)'
                 THEN 'openai_codex'
            WHEN lower(g.platform) IN ('anthropic', 'claude', 'kiro')
                 OR g.name ~* '(anthropic|claude|kiro)'
                 THEN 'anthropic_kiro_claude'
            ELSE 'manual_platform_review'
        END AS proposed_platform_family,
        COALESCE(accts.account_ids, ARRAY[]::BIGINT[]) AS account_ids,
        COALESCE(accts.account_priorities, ARRAY[]::TEXT[]) AS account_priorities,
        COALESCE(accts.account_count, 0) AS account_count,
        COALESCE(accts.active_schedulable_account_count, 0) AS active_schedulable_account_count
    FROM groups g
    LEFT JOIN (
        SELECT
            ag.group_id,
            ARRAY_AGG(DISTINCT ag.account_id ORDER BY ag.account_id) AS account_ids,
            ARRAY_AGG((ag.account_id::TEXT || ':' || ag.priority::TEXT) ORDER BY ag.account_id, ag.priority) AS account_priorities,
            COUNT(DISTINCT ag.account_id) AS account_count,
            COUNT(DISTINCT ag.account_id) FILTER (
                WHERE a.deleted_at IS NULL
                  AND a.status = 'active'
                  AND COALESCE(a.schedulable, TRUE) = TRUE
            ) AS active_schedulable_account_count
        FROM account_groups ag
        JOIN accounts a ON a.id = ag.account_id
        GROUP BY ag.group_id
    ) accts ON accts.group_id = g.id
    WHERE g.deleted_at IS NULL
      AND g.subscription_type = 'subscription'
      AND g.status = 'active'
)
SELECT
    proposed_platform_family || ':' || rate_multiplier::TEXT AS proposed_runtime_group_key,
    proposed_platform_family,
    rate_multiplier,
    COUNT(*) AS legacy_group_count,
    ARRAY_AGG(old_group_id ORDER BY old_group_id) AS legacy_group_ids,
    ARRAY_AGG(old_group_name ORDER BY old_group_id) AS legacy_group_names,
    COUNT(*) FILTER (WHERE COALESCE(is_exclusive, FALSE)) AS legacy_exclusive_source_group_count,
    COUNT(DISTINCT account_ids) AS distinct_account_pool_count,
    COUNT(DISTINCT account_priorities) AS distinct_account_priority_count,
    SUM(account_count) AS summed_account_bindings,
    MAX(active_schedulable_account_count) AS max_active_schedulable_accounts,
    CASE
        WHEN COUNT(*) FILTER (WHERE COALESCE(is_exclusive, FALSE)) > 0
            THEN 'legacy_exclusive_group_used_as_plan_template_source'
        ELSE 'public_legacy_group_source'
    END AS source_group_review_note,
    CASE
        WHEN proposed_platform_family = 'manual_platform_review' THEN 'manual_platform_review'
        WHEN COUNT(DISTINCT account_ids) > 1 THEN 'account_pool_conflict'
        WHEN COUNT(DISTINCT account_priorities) > 1 THEN 'account_priority_conflict'
        WHEN MAX(active_schedulable_account_count) = 0 THEN 'no_schedulable_accounts'
        ELSE 'ok'
    END AS runtime_group_review_reason
FROM old_groups
GROUP BY proposed_platform_family, rate_multiplier
ORDER BY proposed_platform_family, rate_multiplier;

-- ============================================================================
-- 3. Plan candidates: one plan-template per old subscription group.
-- ============================================================================

WITH legacy_plan_candidates AS (
    SELECT
        g.id AS old_group_id,
        g.name AS old_group_name,
        g.platform,
        g.rate_multiplier,
        g.default_validity_days,
        g.daily_limit_usd,
        g.weekly_limit_usd,
        g.monthly_limit_usd,
        CASE
            WHEN lower(g.platform) IN ('openai', 'codex')
                 OR g.name ~* '(openai|codex|gpt)'
                 THEN 'openai_codex'
            WHEN lower(g.platform) IN ('anthropic', 'claude', 'kiro')
                 OR g.name ~* '(anthropic|claude|kiro)'
                 THEN 'anthropic_kiro_claude'
            ELSE 'manual_platform_review'
        END || ':' || g.rate_multiplier::TEXT AS proposed_runtime_group_key
    FROM groups g
    WHERE g.deleted_at IS NULL
      AND g.subscription_type = 'subscription'
)
SELECT
    old_group_id,
    old_group_name,
    proposed_runtime_group_key,
    default_validity_days,
    daily_limit_usd,
    weekly_limit_usd,
    monthly_limit_usd,
    'create_or_reuse_internal_backfill_plan_after_mapping_review' AS plan_action
FROM legacy_plan_candidates
ORDER BY proposed_runtime_group_key, old_group_id;

-- ============================================================================
-- 4. Subscription backfill dry-run counts.
-- ============================================================================

WITH subs AS (
    SELECT
        us.id AS legacy_subscription_id,
        us.user_id,
        us.group_id AS old_group_id,
        us.status,
        us.starts_at,
        us.expires_at,
        us.deleted_at,
        g.name AS old_group_name,
        g.subscription_type,
        g.daily_limit_usd,
        g.weekly_limit_usd,
        g.monthly_limit_usd
    FROM user_subscriptions us
    JOIN groups g ON g.id = us.group_id
)
SELECT
    CASE
        WHEN deleted_at IS NOT NULL THEN 'skip_deleted'
        WHEN status <> 'active' THEN 'skip_not_active'
        WHEN starts_at > NOW() THEN 'skip_future'
        WHEN expires_at <= NOW() THEN 'skip_expired'
        WHEN subscription_type <> 'subscription' THEN 'skip_non_subscription_group'
        ELSE 'would_backfill_active'
    END AS action,
    COUNT(*) AS rows
FROM subs
GROUP BY action
ORDER BY action;

-- ============================================================================
-- 5. API key migration dry-run and ambiguous reasons.
-- ============================================================================

WITH active_subs AS (
    SELECT
        us.id AS legacy_subscription_id,
        us.user_id,
        us.group_id AS old_group_id,
        COUNT(*) OVER (PARTITION BY us.user_id, us.group_id) AS active_sub_count
    FROM user_subscriptions us
    WHERE us.deleted_at IS NULL
      AND us.status = 'active'
      AND us.starts_at <= NOW()
      AND us.expires_at > NOW()
),
group_inventory AS (
    SELECT
        g.id AS old_group_id,
        g.name AS old_group_name,
        g.platform,
        g.subscription_type,
        g.status,
        g.deleted_at,
        g.is_exclusive,
        g.rate_multiplier,
        COALESCE(accts.active_schedulable_account_count, 0) AS active_schedulable_account_count,
        CASE
            WHEN lower(g.platform) IN ('openai', 'codex')
                 OR g.name ~* '(openai|codex|gpt)'
                 THEN 'openai_codex'
            WHEN lower(g.platform) IN ('anthropic', 'claude', 'kiro')
                 OR g.name ~* '(anthropic|claude|kiro)'
                 THEN 'anthropic_kiro_claude'
            ELSE 'manual_platform_review'
        END || ':' || g.rate_multiplier::TEXT AS proposed_runtime_group_key
    FROM groups g
    LEFT JOIN (
        SELECT
            ag.group_id,
            COUNT(DISTINCT ag.account_id) FILTER (
                WHERE a.deleted_at IS NULL
                  AND a.status = 'active'
                  AND COALESCE(a.schedulable, TRUE) = TRUE
            ) AS active_schedulable_account_count
        FROM account_groups ag
        JOIN accounts a ON a.id = ag.account_id
        GROUP BY ag.group_id
    ) accts ON accts.group_id = g.id
),
key_candidates AS (
    SELECT
        ak.id AS api_key_id,
        ak.user_id,
        ak.group_id AS old_group_id,
        gi.old_group_name,
        gi.subscription_type,
        gi.status AS group_status,
        gi.deleted_at AS group_deleted_at,
        gi.is_exclusive,
        gi.proposed_runtime_group_key,
        gi.active_schedulable_account_count,
        s.legacy_subscription_id,
        s.active_sub_count,
        CASE
            WHEN ak.deleted_at IS NOT NULL THEN 'skip_deleted_key'
            WHEN ak.status <> 'active' THEN 'skip_inactive_key'
            WHEN ak.group_id IS NULL THEN 'ambiguous_no_group'
            WHEN gi.old_group_id IS NULL THEN 'ambiguous_missing_group'
            WHEN gi.deleted_at IS NOT NULL THEN 'ambiguous_deleted_group'
            WHEN gi.status <> 'active' THEN 'ambiguous_inactive_group'
            WHEN gi.old_group_name ~* '(test|测试|negative|负向|deprecated|停用)' THEN 'ambiguous_test_or_negative_group'
            WHEN gi.subscription_type = 'subscription' AND s.legacy_subscription_id IS NULL THEN 'ambiguous_no_active_subscription'
            WHEN gi.subscription_type = 'subscription' AND s.active_sub_count <> 1 THEN 'ambiguous_multiple_active_subscriptions'
            WHEN gi.subscription_type = 'subscription' AND gi.active_schedulable_account_count = 0 THEN 'ambiguous_no_schedulable_accounts'
            WHEN gi.subscription_type = 'subscription' THEN 'would_migrate_to_entitlement'
            WHEN COALESCE(gi.is_exclusive, FALSE) THEN 'ambiguous_exclusive_balance_group'
            WHEN gi.subscription_type = 'standard' THEN 'would_keep_or_move_balance_source'
            ELSE 'ambiguous_unknown_group_type'
        END AS migration_action
    FROM api_keys ak
    LEFT JOIN group_inventory gi ON gi.old_group_id = ak.group_id
    LEFT JOIN active_subs s ON s.user_id = ak.user_id AND s.old_group_id = ak.group_id
)
SELECT
    migration_action,
    COUNT(*) AS api_key_count
FROM key_candidates
GROUP BY migration_action
ORDER BY migration_action;

-- Ambiguous API key details. Redact user identifying data outside this query.
WITH active_subs AS (
    SELECT
        us.id AS legacy_subscription_id,
        us.user_id,
        us.group_id AS old_group_id,
        COUNT(*) OVER (PARTITION BY us.user_id, us.group_id) AS active_sub_count
    FROM user_subscriptions us
    WHERE us.deleted_at IS NULL
      AND us.status = 'active'
      AND us.starts_at <= NOW()
      AND us.expires_at > NOW()
),
group_inventory AS (
    SELECT
        g.id AS old_group_id,
        g.name AS old_group_name,
        g.platform,
        g.subscription_type,
        g.status,
        g.deleted_at,
        g.is_exclusive,
        g.rate_multiplier,
        COALESCE(accts.active_schedulable_account_count, 0) AS active_schedulable_account_count,
        CASE
            WHEN lower(g.platform) IN ('openai', 'codex')
                 OR g.name ~* '(openai|codex|gpt)'
                 THEN 'openai_codex'
            WHEN lower(g.platform) IN ('anthropic', 'claude', 'kiro')
                 OR g.name ~* '(anthropic|claude|kiro)'
                 THEN 'anthropic_kiro_claude'
            ELSE 'manual_platform_review'
        END || ':' || g.rate_multiplier::TEXT AS proposed_runtime_group_key
    FROM groups g
    LEFT JOIN (
        SELECT
            ag.group_id,
            COUNT(DISTINCT ag.account_id) FILTER (
                WHERE a.deleted_at IS NULL
                  AND a.status = 'active'
                  AND COALESCE(a.schedulable, TRUE) = TRUE
            ) AS active_schedulable_account_count
        FROM account_groups ag
        JOIN accounts a ON a.id = ag.account_id
        GROUP BY ag.group_id
    ) accts ON accts.group_id = g.id
),
key_candidates AS (
    SELECT
        ak.id AS api_key_id,
        ak.user_id,
        ak.group_id AS old_group_id,
        gi.old_group_name,
        gi.platform,
        gi.subscription_type,
        gi.proposed_runtime_group_key,
        s.legacy_subscription_id,
        s.active_sub_count,
        CASE
            WHEN ak.deleted_at IS NOT NULL THEN 'skip_deleted_key'
            WHEN ak.status <> 'active' THEN 'skip_inactive_key'
            WHEN ak.group_id IS NULL THEN 'ambiguous_no_group'
            WHEN gi.old_group_id IS NULL THEN 'ambiguous_missing_group'
            WHEN gi.deleted_at IS NOT NULL THEN 'ambiguous_deleted_group'
            WHEN gi.status <> 'active' THEN 'ambiguous_inactive_group'
            WHEN gi.old_group_name ~* '(test|测试|negative|负向|deprecated|停用)' THEN 'ambiguous_test_or_negative_group'
            WHEN gi.subscription_type = 'subscription' AND s.legacy_subscription_id IS NULL THEN 'ambiguous_no_active_subscription'
            WHEN gi.subscription_type = 'subscription' AND s.active_sub_count <> 1 THEN 'ambiguous_multiple_active_subscriptions'
            WHEN gi.subscription_type = 'subscription' AND gi.active_schedulable_account_count = 0 THEN 'ambiguous_no_schedulable_accounts'
            WHEN gi.subscription_type = 'subscription' THEN 'would_migrate_to_entitlement'
            WHEN COALESCE(gi.is_exclusive, FALSE) THEN 'ambiguous_exclusive_balance_group'
            WHEN gi.subscription_type = 'standard' THEN 'would_keep_or_move_balance_source'
            ELSE 'ambiguous_unknown_group_type'
        END AS migration_action
    FROM api_keys ak
    LEFT JOIN group_inventory gi ON gi.old_group_id = ak.group_id
    LEFT JOIN active_subs s ON s.user_id = ak.user_id AND s.old_group_id = ak.group_id
)
SELECT
    api_key_id,
    user_id,
    old_group_id,
    old_group_name,
    platform,
    subscription_type,
    proposed_runtime_group_key,
    legacy_subscription_id,
    migration_action
FROM key_candidates
WHERE migration_action LIKE 'ambiguous_%'
ORDER BY migration_action, old_group_id, api_key_id
LIMIT 500;

-- ============================================================================
-- 6. Exclusive group exposure check.
-- ============================================================================

SELECT
    g.id AS group_id,
    g.name AS group_name,
    g.subscription_type,
    g.status,
    g.is_exclusive,
    COUNT(DISTINCT us.id) FILTER (
        WHERE us.deleted_at IS NULL
          AND us.status = 'active'
          AND us.starts_at <= NOW()
          AND us.expires_at > NOW()
    ) AS active_subscription_count,
    COUNT(DISTINCT ak.id) FILTER (
        WHERE ak.deleted_at IS NULL
          AND ak.status = 'active'
    ) AS active_api_key_count,
    COUNT(DISTINCT uag.user_id) AS explicitly_allowed_users
FROM groups g
LEFT JOIN user_subscriptions us ON us.group_id = g.id
LEFT JOIN api_keys ak ON ak.group_id = g.id
LEFT JOIN user_allowed_groups uag ON uag.group_id = g.id
WHERE g.deleted_at IS NULL
  AND COALESCE(g.is_exclusive, FALSE) = TRUE
GROUP BY g.id, g.name, g.subscription_type, g.status, g.is_exclusive
ORDER BY active_api_key_count DESC, active_subscription_count DESC, g.id;

-- ============================================================================
-- 7. Post-write reconciliation queries. Run after a future write rehearsal.
-- ============================================================================

-- Entitlement vs legacy subscription exact state check.
SELECT
    COUNT(*) AS mismatched_entitlement_rows
FROM subscription_entitlements se
JOIN user_subscriptions us ON us.id = se.legacy_subscription_id
WHERE se.source_type = 'legacy_subscription_backfill'
  AND se.deleted_at IS NULL
  AND (
      se.user_id <> us.user_id
      OR se.status <> us.status
      OR se.starts_at <> us.starts_at
      OR se.expires_at <> us.expires_at
      OR se.daily_window_start IS DISTINCT FROM us.daily_window_start
      OR se.weekly_window_start IS DISTINCT FROM us.weekly_window_start
      OR se.monthly_window_start IS DISTINCT FROM us.monthly_window_start
      OR se.daily_usage_usd <> us.daily_usage_usd
      OR se.weekly_usage_usd <> us.weekly_usage_usd
      OR se.monthly_usage_usd <> us.monthly_usage_usd
  );

-- Backfilled entitlement rows without grant.
SELECT
    se.id AS entitlement_id,
    se.legacy_subscription_id,
    se.primary_group_id
FROM subscription_entitlements se
LEFT JOIN subscription_entitlement_groups seg
  ON seg.entitlement_id = se.id
 AND seg.group_id = se.primary_group_id
 AND seg.enabled = TRUE
WHERE se.source_type = 'legacy_subscription_backfill'
  AND se.deleted_at IS NULL
  AND seg.entitlement_id IS NULL
ORDER BY se.id
LIMIT 500;

-- Backfilled entitlement rows without fulfillment history.
SELECT
    se.id AS entitlement_id,
    se.legacy_subscription_id
FROM subscription_entitlements se
LEFT JOIN subscription_entitlement_fulfillments f
  ON f.entitlement_id = se.id
 AND f.source_type = 'legacy_subscription_backfill'
 AND f.source_id = se.legacy_subscription_id
WHERE se.source_type = 'legacy_subscription_backfill'
  AND se.deleted_at IS NULL
  AND f.id IS NULL
ORDER BY se.id
LIMIT 500;

-- API keys migrated to entitlement source but not bound to the expected user or group coverage.
SELECT
    ak.id AS api_key_id,
    ak.user_id,
    ak.group_id,
    ak.access_source,
    ak.subscription_entitlement_id,
    se.user_id AS entitlement_user_id,
    seg.group_id AS covered_group_id
FROM api_keys ak
LEFT JOIN subscription_entitlements se
  ON se.id = ak.subscription_entitlement_id
LEFT JOIN subscription_entitlement_groups seg
  ON seg.entitlement_id = se.id
 AND seg.group_id = ak.group_id
 AND seg.enabled = TRUE
WHERE ak.deleted_at IS NULL
  AND ak.access_source = 'entitlement'
  AND (
      ak.subscription_entitlement_id IS NULL
      OR se.id IS NULL
      OR se.deleted_at IS NOT NULL
      OR se.user_id <> ak.user_id
      OR se.status <> 'active'
      OR se.starts_at > NOW()
      OR se.expires_at <= NOW()
      OR seg.entitlement_id IS NULL
  )
ORDER BY ak.id
LIMIT 500;
