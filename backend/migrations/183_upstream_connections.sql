-- Shared upstream management connections (V2).
-- This migration is additive: existing account credentials and billing fields
-- remain untouched for compatibility and rollback.

CREATE TABLE IF NOT EXISTS upstream_connections (
    id                     BIGSERIAL PRIMARY KEY,
    name                   VARCHAR(100) NOT NULL,
    provider               VARCHAR(32) NOT NULL DEFAULT 'auto',
    auth_mode              VARCHAR(32) NOT NULL,
    management_base_url    VARCHAR(500) NOT NULL,
    forwarding_base_url    VARCHAR(500) NOT NULL DEFAULT '',
    credential_encrypted   TEXT NOT NULL,
    credential_fingerprint VARCHAR(128) NOT NULL DEFAULT '',
    legacy_migration_key   VARCHAR(128),
    credential_hint        VARCHAR(100) NOT NULL DEFAULT '',
    remote_user_id         VARCHAR(128) NOT NULL DEFAULT '',
    proxy_id               BIGINT REFERENCES proxies(id) ON DELETE SET NULL,
    capabilities           JSONB NOT NULL DEFAULT '{}'::jsonb,
    status                 VARCHAR(32) NOT NULL DEFAULT 'pending',
    last_error             TEXT NOT NULL DEFAULT '',
    sync_enabled           BOOLEAN NOT NULL DEFAULT TRUE,
    sync_interval_seconds  INT NOT NULL DEFAULT 300,
    sync_failures          INT NOT NULL DEFAULT 0,
    version                BIGINT NOT NULL DEFAULT 1,
    wallet_amount          DECIMAL(20,8),
    wallet_currency        VARCHAR(16) NOT NULL DEFAULT '',
    wallet_usd             DECIMAL(20,8),
    wallet_unlimited       BOOLEAN NOT NULL DEFAULT FALSE,
    wallet_source          VARCHAR(64) NOT NULL DEFAULT '',
    wallet_reliability     VARCHAR(32) NOT NULL DEFAULT 'unknown',
    wallet_raw             JSONB NOT NULL DEFAULT '{}'::jsonb,
    wallet_observed_at     TIMESTAMPTZ,
    last_discovered_at     TIMESTAMPTZ,
    last_synced_at         TIMESTAMPTZ,
    next_sync_at           TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_connections_sync_interval_check
        CHECK (sync_interval_seconds BETWEEN 30 AND 86400),
    CONSTRAINT upstream_connections_counters_check
        CHECK (sync_failures >= 0 AND version >= 1),
    CONSTRAINT upstream_connections_capabilities_json_check
        CHECK (jsonb_typeof(capabilities) = 'object'),
    CONSTRAINT upstream_connections_wallet_raw_json_check
        CHECK (jsonb_typeof(wallet_raw) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_upstream_connections_sync_due
    ON upstream_connections (sync_enabled, next_sync_at, id);
CREATE INDEX IF NOT EXISTS idx_upstream_connections_status
    ON upstream_connections (status);
CREATE INDEX IF NOT EXISTS idx_upstream_connections_management_url
    ON upstream_connections (management_base_url);
CREATE INDEX IF NOT EXISTS idx_upstream_connections_forwarding_url
    ON upstream_connections (forwarding_base_url);
CREATE INDEX IF NOT EXISTS idx_upstream_connections_provider
    ON upstream_connections (provider);
CREATE INDEX IF NOT EXISTS idx_upstream_connections_remote_user
    ON upstream_connections (remote_user_id);
CREATE INDEX IF NOT EXISTS idx_upstream_connections_proxy
    ON upstream_connections (proxy_id);
ALTER TABLE upstream_connections
    ADD COLUMN IF NOT EXISTS legacy_migration_key VARCHAR(128);
CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_connections_legacy_migration_key
    ON upstream_connections (legacy_migration_key)
    WHERE legacy_migration_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS upstream_groups (
    id              BIGSERIAL PRIMARY KEY,
    connection_id   BIGINT NOT NULL REFERENCES upstream_connections(id) ON DELETE CASCADE,
    remote_id       VARCHAR(128) NOT NULL DEFAULT '',
    name            VARCHAR(128) NOT NULL,
    rate_multiplier DECIMAL(20,8),
    source          VARCHAR(64) NOT NULL DEFAULT '',
    confidence      VARCHAR(32) NOT NULL DEFAULT 'unknown',
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    observed_at     TIMESTAMPTZ,
    fresh_until     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_groups_multiplier_check
        CHECK (rate_multiplier IS NULL OR rate_multiplier >= 0),
    CONSTRAINT upstream_groups_metadata_json_check
        CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT upstream_groups_connection_name_unique
        UNIQUE (connection_id, name)
);

CREATE INDEX IF NOT EXISTS idx_upstream_groups_connection_remote
    ON upstream_groups (connection_id, remote_id);
CREATE INDEX IF NOT EXISTS idx_upstream_groups_observed
    ON upstream_groups (observed_at);
CREATE INDEX IF NOT EXISTS idx_upstream_groups_fresh_until
    ON upstream_groups (fresh_until);

CREATE TABLE IF NOT EXISTS upstream_account_bindings (
    id                  BIGSERIAL PRIMARY KEY,
    account_id          BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    connection_id       BIGINT NOT NULL REFERENCES upstream_connections(id) ON DELETE CASCADE,
    key_fingerprint     VARCHAR(128) NOT NULL DEFAULT '',
    remote_token_id     VARCHAR(128) NOT NULL DEFAULT '',
    remote_token_name   VARCHAR(255) NOT NULL DEFAULT '',
    resolution_kind     VARCHAR(32) NOT NULL DEFAULT 'unresolved',
    remote_group_id     VARCHAR(128) NOT NULL DEFAULT '',
    remote_group_name   VARCHAR(128) NOT NULL DEFAULT '',
    fallback_groups     JSONB NOT NULL DEFAULT '[]'::jsonb,
    observed_multiplier DECIMAL(20,8),
    confidence          VARCHAR(32) NOT NULL DEFAULT 'unknown',
    source              VARCHAR(64) NOT NULL DEFAULT '',
    apply_policy        VARCHAR(32) NOT NULL DEFAULT 'observe_only',
    status              VARCHAR(32) NOT NULL DEFAULT 'pending',
    sync_failures       INTEGER NOT NULL DEFAULT 0,
    last_error          TEXT NOT NULL DEFAULT '',
    resolution_details  JSONB NOT NULL DEFAULT '{}'::jsonb,
    observed_at         TIMESTAMPTZ,
    fresh_until         TIMESTAMPTZ,
    next_sync_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_account_bindings_account_unique UNIQUE (account_id),
    CONSTRAINT upstream_account_bindings_multiplier_check
        CHECK (observed_multiplier IS NULL OR observed_multiplier >= 0),
    CONSTRAINT upstream_account_bindings_sync_failures_check
        CHECK (sync_failures >= 0),
    CONSTRAINT upstream_account_bindings_fallback_json_check
        CHECK (jsonb_typeof(fallback_groups) = 'array'),
    CONSTRAINT upstream_account_bindings_details_json_check
        CHECK (jsonb_typeof(resolution_details) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_upstream_account_bindings_sync_due
    ON upstream_account_bindings (connection_id, next_sync_at, id);
CREATE INDEX IF NOT EXISTS idx_upstream_account_bindings_status
    ON upstream_account_bindings (status);
CREATE INDEX IF NOT EXISTS idx_upstream_account_bindings_remote_token
    ON upstream_account_bindings (connection_id, remote_token_id);
CREATE INDEX IF NOT EXISTS idx_upstream_account_bindings_resolution
    ON upstream_account_bindings (resolution_kind);
CREATE INDEX IF NOT EXISTS idx_upstream_account_bindings_fresh_until
    ON upstream_account_bindings (fresh_until);
