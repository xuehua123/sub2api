ALTER TABLE batch_image_jobs
    ADD COLUMN IF NOT EXISTS group_id BIGINT,
    ADD COLUMN IF NOT EXISTS subscription_id BIGINT,
    ADD COLUMN IF NOT EXISTS entitlement_id BIGINT,
    ADD COLUMN IF NOT EXISTS billing_type SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS billing_source VARCHAR(32) NOT NULL DEFAULT 'balance',
    ADD COLUMN IF NOT EXISTS entitlement_balance_fallback BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS held_daily_window_start TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS held_weekly_window_start TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS held_monthly_window_start TIMESTAMPTZ;

ALTER TABLE batch_image_jobs
    DROP CONSTRAINT IF EXISTS batch_image_jobs_billing_type_check,
    ADD CONSTRAINT batch_image_jobs_billing_type_check
        CHECK (billing_type IN (0, 1)) NOT VALID;

ALTER TABLE batch_image_jobs
    DROP CONSTRAINT IF EXISTS batch_image_jobs_billing_source_check,
    ADD CONSTRAINT batch_image_jobs_billing_source_check
        CHECK (billing_source IN ('balance', 'legacy_subscription', 'entitlement_quota', 'entitlement_balance_fallback')) NOT VALID;

CREATE INDEX IF NOT EXISTS batch_image_jobs_subscription_id_idx
    ON batch_image_jobs (subscription_id) WHERE subscription_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS batch_image_jobs_entitlement_id_idx
    ON batch_image_jobs (entitlement_id) WHERE entitlement_id IS NOT NULL;

COMMENT ON COLUMN batch_image_jobs.group_id IS 'Resolved API key group used for Batch Image billing';
COMMENT ON COLUMN batch_image_jobs.subscription_id IS 'Legacy subscription selected when the Batch Image hold was created';
COMMENT ON COLUMN batch_image_jobs.entitlement_id IS 'Entitlement selected when the Batch Image hold was created';
COMMENT ON COLUMN batch_image_jobs.billing_type IS 'Billing family snapshot: 0 balance, 1 subscription';
COMMENT ON COLUMN batch_image_jobs.billing_source IS 'Effective hold source, including entitlement wallet fallback';
COMMENT ON COLUMN batch_image_jobs.entitlement_balance_fallback IS 'Whether entitlement quota exhaustion may fall back to wallet balance';
COMMENT ON COLUMN batch_image_jobs.held_daily_window_start IS 'Daily quota window reserved by the Batch Image hold';
COMMENT ON COLUMN batch_image_jobs.held_weekly_window_start IS 'Weekly quota window reserved by the Batch Image hold';
COMMENT ON COLUMN batch_image_jobs.held_monthly_window_start IS 'Monthly quota window reserved by the Batch Image hold';
