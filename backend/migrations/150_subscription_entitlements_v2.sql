-- Subscription entitlements v2 additive schema and legacy backfill.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS access_scope VARCHAR(32) NOT NULL DEFAULT 'explicit',
    ADD COLUMN IF NOT EXISTS allowed_platforms JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS daily_limit_usd DECIMAL(20, 8),
    ADD COLUMN IF NOT EXISTS weekly_limit_usd DECIMAL(20, 8),
    ADD COLUMN IF NOT EXISTS monthly_limit_usd DECIMAL(20, 8),
    ADD COLUMN IF NOT EXISTS overage_policy VARCHAR(32) NOT NULL DEFAULT 'block';

CREATE TABLE IF NOT EXISTS subscription_plan_groups (
    plan_id    BIGINT NOT NULL REFERENCES subscription_plans(id) ON DELETE CASCADE,
    group_id   BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (plan_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_subscription_plan_groups_group_enabled
    ON subscription_plan_groups(group_id, enabled);

CREATE TABLE IF NOT EXISTS subscription_plan_external_mappings (
    id                   BIGSERIAL PRIMARY KEY,
    source               VARCHAR(64) NOT NULL DEFAULT 'sub2-payment-page',
    legacy_group_id      BIGINT NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
    legacy_validity_days INTEGER NOT NULL,
    legacy_value         DECIMAL(20, 8) NOT NULL,
    plan_id              BIGINT NOT NULL REFERENCES subscription_plans(id) ON DELETE CASCADE,
    enabled              BOOLEAN NOT NULL DEFAULT TRUE,
    priority             INTEGER NOT NULL DEFAULT 0,
    notes                TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at           TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_plan_external_mappings_unique
    ON subscription_plan_external_mappings(source, legacy_group_id, legacy_validity_days, legacy_value)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_subscription_plan_external_mappings_plan_id
    ON subscription_plan_external_mappings(plan_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_subscription_plan_external_mappings_legacy_group_id
    ON subscription_plan_external_mappings(legacy_group_id);

CREATE INDEX IF NOT EXISTS idx_subscription_plan_external_mappings_enabled
    ON subscription_plan_external_mappings(enabled);

ALTER TABLE redeem_codes
    ADD COLUMN IF NOT EXISTS plan_id BIGINT REFERENCES subscription_plans(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_redeem_codes_plan_id
    ON redeem_codes(plan_id);

CREATE TABLE IF NOT EXISTS subscription_entitlements (
    id                    BIGSERIAL PRIMARY KEY,
    user_id               BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_id               BIGINT REFERENCES subscription_plans(id) ON DELETE SET NULL,
    legacy_subscription_id BIGINT REFERENCES user_subscriptions(id) ON DELETE SET NULL,
    primary_group_id      BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    name                  VARCHAR(120) NOT NULL DEFAULT '',
    source_type           VARCHAR(32) NOT NULL DEFAULT 'unknown',
    status                VARCHAR(20) NOT NULL DEFAULT 'active',
    starts_at             TIMESTAMPTZ NOT NULL,
    expires_at            TIMESTAMPTZ NOT NULL,
    daily_window_start    TIMESTAMPTZ,
    weekly_window_start   TIMESTAMPTZ,
    monthly_window_start  TIMESTAMPTZ,
    daily_limit_usd       DECIMAL(20, 8),
    weekly_limit_usd      DECIMAL(20, 8),
    monthly_limit_usd     DECIMAL(20, 8),
    daily_usage_usd       DECIMAL(20, 10) NOT NULL DEFAULT 0,
    weekly_usage_usd      DECIMAL(20, 10) NOT NULL DEFAULT 0,
    monthly_usage_usd     DECIMAL(20, 10) NOT NULL DEFAULT 0,
    overage_policy        VARCHAR(32) NOT NULL DEFAULT 'block',
    plan_snapshot         JSONB NOT NULL DEFAULT '{}'::jsonb,
    source_id             BIGINT,
    source_external_id    VARCHAR(128),
    source_redeem_code_id BIGINT REFERENCES redeem_codes(id) ON DELETE SET NULL,
    assigned_by           BIGINT REFERENCES users(id) ON DELETE SET NULL,
    assigned_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes                 TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at            TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_subscription_entitlements_user_status_expires
    ON subscription_entitlements(user_id, status, expires_at)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_subscription_entitlements_plan_id
    ON subscription_entitlements(plan_id)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlements_legacy_subscription_id_unique
    ON subscription_entitlements(legacy_subscription_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlements_source_redeem_unique
    ON subscription_entitlements(source_redeem_code_id)
    WHERE source_redeem_code_id IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlements_source_id_unique
    ON subscription_entitlements(source_type, source_id)
    WHERE source_id IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlements_source_external_unique
    ON subscription_entitlements(source_type, source_external_id)
    WHERE source_external_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_subscription_entitlements_user_id
    ON subscription_entitlements(user_id);

CREATE INDEX IF NOT EXISTS idx_subscription_entitlements_status
    ON subscription_entitlements(status);

CREATE INDEX IF NOT EXISTS idx_subscription_entitlements_expires_at
    ON subscription_entitlements(expires_at);

CREATE TABLE IF NOT EXISTS subscription_entitlement_groups (
    entitlement_id BIGINT NOT NULL REFERENCES subscription_entitlements(id) ON DELETE CASCADE,
    group_id       BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    sort_order     INTEGER NOT NULL DEFAULT 0,
    enabled        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (entitlement_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_subscription_entitlement_groups_group_enabled
    ON subscription_entitlement_groups(group_id, enabled);

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS subscription_entitlement_id BIGINT REFERENCES subscription_entitlements(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_api_keys_subscription_entitlement_id
    ON api_keys(subscription_entitlement_id);

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS entitlement_id BIGINT REFERENCES subscription_entitlements(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_usage_logs_entitlement_id
    ON usage_logs(entitlement_id);

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_entitlement_id BIGINT REFERENCES subscription_entitlements(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_payment_orders_subscription_entitlement_id
    ON payment_orders(subscription_entitlement_id);

INSERT INTO subscription_plan_groups (plan_id, group_id, sort_order, enabled, created_at, updated_at)
SELECT id, group_id, 0, TRUE, NOW(), NOW()
FROM subscription_plans
WHERE group_id IS NOT NULL
ON CONFLICT (plan_id, group_id) DO NOTHING;

UPDATE subscription_plans sp
SET
    daily_limit_usd = COALESCE(sp.daily_limit_usd, g.daily_limit_usd),
    weekly_limit_usd = COALESCE(sp.weekly_limit_usd, g.weekly_limit_usd),
    monthly_limit_usd = COALESCE(sp.monthly_limit_usd, g.monthly_limit_usd),
    access_scope = COALESCE(NULLIF(sp.access_scope, ''), 'explicit')
FROM groups g
WHERE sp.group_id = g.id;

INSERT INTO subscription_entitlements (
    user_id, legacy_subscription_id, primary_group_id, name, source_type,
    status, starts_at, expires_at,
    daily_window_start, weekly_window_start, monthly_window_start,
    daily_limit_usd, weekly_limit_usd, monthly_limit_usd,
    daily_usage_usd, weekly_usage_usd, monthly_usage_usd,
    overage_policy, notes, assigned_by, assigned_at, created_at, updated_at
)
SELECT
    us.user_id, us.id, us.group_id, COALESCE(g.name, 'Legacy Subscription'), 'legacy_migration',
    us.status, us.starts_at, us.expires_at,
    us.daily_window_start, us.weekly_window_start, us.monthly_window_start,
    g.daily_limit_usd, g.weekly_limit_usd, g.monthly_limit_usd,
    us.daily_usage_usd, us.weekly_usage_usd, us.monthly_usage_usd,
    'block', us.notes, us.assigned_by, us.assigned_at, us.created_at, us.updated_at
FROM user_subscriptions us
JOIN groups g ON g.id = us.group_id
WHERE us.deleted_at IS NULL
ON CONFLICT (legacy_subscription_id) DO NOTHING;

INSERT INTO subscription_entitlement_groups (entitlement_id, group_id, sort_order, enabled, created_at, updated_at)
SELECT se.id, se.primary_group_id, 0, TRUE, NOW(), NOW()
FROM subscription_entitlements se
WHERE se.primary_group_id IS NOT NULL
ON CONFLICT (entitlement_id, group_id) DO NOTHING;

UPDATE api_keys ak
SET subscription_entitlement_id = se.id
FROM subscription_entitlements se
WHERE ak.user_id = se.user_id
  AND ak.group_id = se.primary_group_id
  AND ak.subscription_entitlement_id IS NULL
  AND se.deleted_at IS NULL;

-- Historical usage_logs entitlement attribution is intentionally not backfilled
-- during application startup. Large production installations can have millions
-- of usage rows, and doing that UPDATE here blocks the server from becoming
-- healthy during deploy. Run the legacy-entitlement-backfill tool's
-- backfill-usage-logs mode after deployment while both v2 flags remain off.
