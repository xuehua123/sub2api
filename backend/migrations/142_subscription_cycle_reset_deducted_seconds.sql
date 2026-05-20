ALTER TABLE subscription_cycle_reset_logs
    ADD COLUMN IF NOT EXISTS deducted_seconds BIGINT NOT NULL DEFAULT 0;

UPDATE subscription_cycle_reset_logs
SET deducted_seconds = deducted_days::BIGINT * 86400
WHERE deducted_seconds = 0;
