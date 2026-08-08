-- Track how much of an accrual has already been reversed without mutating the
-- original ledger amount. The default lets older versions keep inserting rows,
-- but older thaw/transfer code still releases gross amounts. Do not write a
-- non-zero reversed_amount until every old application slot has stopped (or a
-- separately reviewed feature gate has completed the expand/contract rollout).
ALTER TABLE user_affiliate_ledger
    ADD COLUMN IF NOT EXISTS reversed_amount DECIMAL(20,8) NOT NULL DEFAULT 0;

COMMENT ON COLUMN user_affiliate_ledger.reversed_amount IS
    'Cumulative amount reversed from an accrue entry; zero for other ledger actions';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_user_affiliate_ledger_accrue_reversed_amount'
          AND conrelid = 'user_affiliate_ledger'::regclass
    ) THEN
        ALTER TABLE user_affiliate_ledger
            ADD CONSTRAINT chk_user_affiliate_ledger_accrue_reversed_amount
            CHECK (
                reversed_amount >= 0
                AND (
                    (action = 'accrue' AND reversed_amount <= amount)
                    OR (action <> 'accrue' AND reversed_amount = 0)
                )
            ) NOT VALID;
    END IF;
END $$;

-- NOT VALID avoids a strong table lock while the constraint is installed;
-- validation scans existing rows while allowing normal reads and writes.
ALTER TABLE user_affiliate_ledger
    VALIDATE CONSTRAINT chk_user_affiliate_ledger_accrue_reversed_amount;
