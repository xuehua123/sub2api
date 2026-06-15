-- Source history for subscription entitlement grants and renewals.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

CREATE TABLE IF NOT EXISTS subscription_entitlement_fulfillments (
    id                    BIGSERIAL PRIMARY KEY,
    entitlement_id        BIGINT NOT NULL REFERENCES subscription_entitlements(id) ON DELETE CASCADE,
    user_id               BIGINT NOT NULL,
    plan_id               BIGINT,
    source_type           VARCHAR(32) NOT NULL DEFAULT 'unknown',
    source_id             BIGINT,
    source_external_id    VARCHAR(128),
    source_redeem_code_id BIGINT REFERENCES redeem_codes(id) ON DELETE SET NULL,
    validity_days         INTEGER NOT NULL DEFAULT 0,
    starts_at             TIMESTAMPTZ NOT NULL,
    expires_at            TIMESTAMPTZ NOT NULL,
    assigned_by           BIGINT REFERENCES users(id) ON DELETE SET NULL,
    assigned_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes                 TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscription_entitlement_fulfillments_entitlement_id
    ON subscription_entitlement_fulfillments(entitlement_id);

CREATE INDEX IF NOT EXISTS idx_subscription_entitlement_fulfillments_user_plan
    ON subscription_entitlement_fulfillments(user_id, plan_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlement_fulfillments_source_redeem_unique
    ON subscription_entitlement_fulfillments(source_redeem_code_id)
    WHERE source_redeem_code_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlement_fulfillments_source_id_unique
    ON subscription_entitlement_fulfillments(source_type, source_id)
    WHERE source_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlement_fulfillments_source_external_unique
    ON subscription_entitlement_fulfillments(source_type, source_external_id)
    WHERE source_external_id IS NOT NULL;

INSERT INTO subscription_entitlement_fulfillments (
    entitlement_id, user_id, plan_id, source_type, source_id, source_external_id,
    source_redeem_code_id, validity_days, starts_at, expires_at,
    assigned_by, assigned_at, notes, created_at, updated_at
)
SELECT
    se.id, se.user_id, se.plan_id, se.source_type, se.source_id, se.source_external_id,
    se.source_redeem_code_id,
    GREATEST(0, CEIL(EXTRACT(EPOCH FROM (se.expires_at - se.starts_at)) / 86400.0)::INTEGER),
    se.starts_at, se.expires_at, se.assigned_by, se.assigned_at, se.notes, se.created_at, se.updated_at
FROM subscription_entitlements se
WHERE se.deleted_at IS NULL
  AND (
      se.source_id IS NOT NULL
      OR se.source_external_id IS NOT NULL
      OR se.source_redeem_code_id IS NOT NULL
  )
ON CONFLICT DO NOTHING;
