-- Add stable usage billing source attribution for admin observability.

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS billing_source VARCHAR(50);

CREATE INDEX IF NOT EXISTS idx_usage_logs_billing_source_created_at
    ON usage_logs(billing_source, created_at);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'usage_logs_billing_source_check'
          AND conrelid = 'usage_logs'::regclass
    ) THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_billing_source_check
            CHECK (
                billing_source IS NULL
                OR billing_source IN (
                    'balance',
                    'legacy_subscription',
                    'entitlement_quota',
                    'entitlement_balance_fallback'
                )
            );
    END IF;
END $$;

COMMENT ON COLUMN usage_logs.billing_source IS
    'Stable billing source: balance, legacy_subscription, entitlement_quota, or entitlement_balance_fallback.';
