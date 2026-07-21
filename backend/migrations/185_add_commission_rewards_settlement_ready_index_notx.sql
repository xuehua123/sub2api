-- Settlement runner batch path:
--   WHERE status IN (...) AND available_at <= $readyAt
--         AND (available_at, id) > ($afterAvailableAt, $afterID)
--   ORDER BY available_at ASC, id ASC
--   LIMIT n FOR UPDATE SKIP LOCKED
-- Index helps filter ready rows by status + available_at (+ id keyset).
-- status IN may still merge per-status index ranges (not a guaranteed zero-sort
-- plan). CONCURRENTLY for hot production DBs; INVALID leftovers are dropped in
-- prepareNonTransactionalMigration before retry.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_commission_rewards_status_available_at_id
    ON commission_rewards (status, available_at, id);
