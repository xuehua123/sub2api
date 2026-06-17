ALTER TABLE subscription_cycle_reset_logs
    ADD COLUMN IF NOT EXISTS reset_monthly_usage BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE subscription_entitlement_cycle_reset_logs
    ADD COLUMN IF NOT EXISTS reset_monthly_usage BOOLEAN NOT NULL DEFAULT TRUE;
