SET LOCAL lock_timeout = '5s';

ALTER TABLE upstream_group_annotations
    ADD COLUMN IF NOT EXISTS excluded_auto_tags TEXT[] NOT NULL DEFAULT '{}';

-- Stable remote identities survive the discovery job's group-row replacement.
CREATE TABLE IF NOT EXISTS upstream_group_model_snapshots (
    connection_id BIGINT NOT NULL REFERENCES upstream_connections(id) ON DELETE CASCADE,
    remote_key TEXT NOT NULL,
    connection_version BIGINT NOT NULL,
    models TEXT[] NOT NULL DEFAULT '{}',
    auto_tags TEXT[] NOT NULL DEFAULT '{}',
    source TEXT NOT NULL DEFAULT '',
    coverage TEXT NOT NULL DEFAULT 'unknown',
    status TEXT NOT NULL DEFAULT 'unknown',
    error_code TEXT NOT NULL DEFAULT '',
    observed_at TIMESTAMPTZ,
    fresh_until TIMESTAMPTZ,
    next_sync_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_attempt_at TIMESTAMPTZ,
    failures INTEGER NOT NULL DEFAULT 0,
    lease_token TEXT NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ,
    PRIMARY KEY (connection_id, remote_key)
);
CREATE INDEX IF NOT EXISTS upstream_group_models_due ON upstream_group_model_snapshots(next_sync_at);
