ALTER TABLE subscription_cycle_reset_logs
    ADD COLUMN IF NOT EXISTS mode VARCHAR(64) NOT NULL DEFAULT 'advance_next_cycle',
    ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS admin_id BIGINT REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE subscription_entitlement_cycle_reset_logs
    ADD COLUMN IF NOT EXISTS mode VARCHAR(64) NOT NULL DEFAULT 'advance_next_cycle',
    ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS admin_id BIGINT REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_subscription_cycle_reset_logs_admin_created
    ON subscription_cycle_reset_logs(admin_id, created_at DESC)
    WHERE admin_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_subscription_entitlement_cycle_reset_logs_admin_created
    ON subscription_entitlement_cycle_reset_logs(admin_id, created_at DESC)
    WHERE admin_id IS NOT NULL;
