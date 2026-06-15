-- Introduce first-class API key access source and group capability flags for
-- subscription entitlements v2. This migration is additive and idempotent; it
-- does not rebalance or choose entitlements for users.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS access_source VARCHAR(32) NOT NULL DEFAULT 'balance';

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS balance_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS subscription_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS plan_auto_grant_enabled BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE api_keys
SET access_source = CASE
    WHEN subscription_entitlement_id IS NOT NULL THEN 'entitlement'
    ELSE 'balance'
END,
updated_at = NOW()
WHERE access_source IS NULL
   OR access_source = ''
   OR access_source NOT IN ('balance', 'entitlement')
   OR (subscription_entitlement_id IS NOT NULL AND access_source <> 'entitlement')
   OR (subscription_entitlement_id IS NULL AND access_source <> 'balance');

UPDATE groups
SET balance_enabled = CASE
        WHEN subscription_type = 'standard' THEN TRUE
        ELSE FALSE
    END,
    subscription_enabled = CASE
        WHEN subscription_type = 'subscription' THEN TRUE
        ELSE FALSE
    END,
    plan_auto_grant_enabled = CASE
        WHEN subscription_type = 'subscription'
             AND COALESCE(is_exclusive, FALSE) = FALSE
             AND status = 'active'
        THEN TRUE
        ELSE FALSE
    END,
    updated_at = NOW()
WHERE balance_enabled IS DISTINCT FROM CASE WHEN subscription_type = 'standard' THEN TRUE ELSE FALSE END
   OR subscription_enabled IS DISTINCT FROM CASE WHEN subscription_type = 'subscription' THEN TRUE ELSE FALSE END
   OR plan_auto_grant_enabled IS DISTINCT FROM CASE
        WHEN subscription_type = 'subscription'
             AND COALESCE(is_exclusive, FALSE) = FALSE
             AND status = 'active'
        THEN TRUE
        ELSE FALSE
    END;

CREATE INDEX IF NOT EXISTS idx_api_keys_access_source
    ON api_keys(access_source)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_groups_balance_enabled
    ON groups(balance_enabled)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_groups_subscription_capabilities
    ON groups(subscription_enabled, plan_auto_grant_enabled)
    WHERE deleted_at IS NULL;
