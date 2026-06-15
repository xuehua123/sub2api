-- Legacy subscription entitlement backfill control tables.
--
-- This migration is intentionally additive. It does not migrate data by itself;
-- the dry-run/apply helper uses these tables to keep legacy group mappings and
-- API key rollback snapshots idempotent and auditable.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS subscription_legacy_backfill_mappings (
    legacy_group_id    BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE RESTRICT,
    plan_id            BIGINT NOT NULL REFERENCES subscription_plans(id) ON DELETE RESTRICT,
    runtime_group_id   BIGINT NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
    runtime_group_key  VARCHAR(128) NOT NULL,
    mapping_version    VARCHAR(64) NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscription_legacy_backfill_mappings_plan_id
    ON subscription_legacy_backfill_mappings(plan_id);

CREATE INDEX IF NOT EXISTS idx_subscription_legacy_backfill_mappings_runtime_group_id
    ON subscription_legacy_backfill_mappings(runtime_group_id);

CREATE INDEX IF NOT EXISTS idx_subscription_legacy_backfill_mappings_runtime_key
    ON subscription_legacy_backfill_mappings(runtime_group_key);

CREATE TABLE IF NOT EXISTS api_key_legacy_backfill_snapshots (
    api_key_id                       BIGINT PRIMARY KEY REFERENCES api_keys(id) ON DELETE RESTRICT,
    user_id                          BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    old_group_id                     BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    old_access_source                VARCHAR(32),
    old_subscription_entitlement_id  BIGINT REFERENCES subscription_entitlements(id) ON DELETE SET NULL,
    old_updated_at                   TIMESTAMPTZ,
    mapping_version                  VARCHAR(64) NOT NULL,
    created_at                       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_api_key_legacy_backfill_snapshots_user_id
    ON api_key_legacy_backfill_snapshots(user_id);

CREATE INDEX IF NOT EXISTS idx_api_key_legacy_backfill_snapshots_mapping_version
    ON api_key_legacy_backfill_snapshots(mapping_version);
