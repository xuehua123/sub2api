SET LOCAL lock_timeout = '5s';

ALTER TABLE upstream_group_model_snapshots
    ADD COLUMN IF NOT EXISTS refresh_generation BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS refresh_active BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE upstream_group_model_account_snapshots
    ADD COLUMN IF NOT EXISTS refresh_generation BIGINT NOT NULL DEFAULT 0;
