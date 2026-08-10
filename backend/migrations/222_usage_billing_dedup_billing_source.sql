-- Preserve the final billing source with the idempotency key so retries can
-- emit the same accounting metadata without reapplying monetary effects.
-- Historical rows remain NULL because their final fallback path cannot be
-- reconstructed reliably from the dedup fingerprint alone.

ALTER TABLE usage_billing_dedup
    ADD COLUMN IF NOT EXISTS billing_source VARCHAR(40);

ALTER TABLE usage_billing_dedup_archive
    ADD COLUMN IF NOT EXISTS billing_source VARCHAR(40);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'usage_billing_dedup_billing_source_check'
          AND conrelid = 'usage_billing_dedup'::regclass
    ) THEN
        ALTER TABLE usage_billing_dedup
            ADD CONSTRAINT usage_billing_dedup_billing_source_check
            CHECK (
                billing_source IS NULL
                OR billing_source IN (
                    'balance',
                    'legacy_subscription',
                    'entitlement_quota',
                    'entitlement_balance_fallback'
                )
            ) NOT VALID;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'usage_billing_dedup_archive_billing_source_check'
          AND conrelid = 'usage_billing_dedup_archive'::regclass
    ) THEN
        ALTER TABLE usage_billing_dedup_archive
            ADD CONSTRAINT usage_billing_dedup_archive_billing_source_check
            CHECK (
                billing_source IS NULL
                OR billing_source IN (
                    'balance',
                    'legacy_subscription',
                    'entitlement_quota',
                    'entitlement_balance_fallback'
                )
            ) NOT VALID;
    END IF;
END $$;

COMMENT ON COLUMN usage_billing_dedup.billing_source IS
    'Final billing source for deterministic idempotent replay; NULL for historical rows.';

COMMENT ON COLUMN usage_billing_dedup_archive.billing_source IS
    'Archived final billing source for deterministic idempotent replay; NULL for historical rows.';
