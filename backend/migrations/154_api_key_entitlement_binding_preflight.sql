-- Preflight hardening for API key entitlement bindings before enabling
-- subscription entitlements v2. Migration 150 performed a broad legacy
-- backfill; this corrective pass only clears invalid explicit bindings and
-- deliberately does not choose a replacement entitlement.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

UPDATE api_keys ak
SET subscription_entitlement_id = NULL,
    updated_at = NOW()
WHERE ak.subscription_entitlement_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1
      FROM subscription_entitlements se
      JOIN subscription_entitlement_groups seg
        ON seg.entitlement_id = se.id
       AND seg.group_id = ak.group_id
       AND seg.enabled = TRUE
      WHERE se.id = ak.subscription_entitlement_id
        AND se.deleted_at IS NULL
        AND se.user_id = ak.user_id
        AND se.status = 'active'
        AND se.starts_at <= NOW()
        AND se.expires_at > NOW()
  );
