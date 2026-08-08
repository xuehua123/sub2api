-- One payment order may issue at most one affiliate accrual. The migration
-- runner reports duplicate source orders before building this index and drops
-- a stale INVALID index before retrying a failed concurrent build.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_user_affiliate_ledger_accrue_order_uniq
    ON user_affiliate_ledger (source_order_id)
    WHERE action = 'accrue' AND source_order_id IS NOT NULL;
