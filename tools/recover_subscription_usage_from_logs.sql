-- Recover subscription usage counters from authoritative usage_logs.
--
-- Intended for the 2026-05-20 subscription-window incident where the bad build
-- advanced legacy 00:00 usage windows to purchase-time anchors and reset
-- user_subscriptions daily/weekly/monthly counters.
--
-- Subscription quota is charged with usage_logs.actual_cost, not total_cost.
-- This script also safely corrects rows touched by the first total_cost-based
-- repair: only rows present in subscription_usage_repair_backup_20260520 may be
-- lowered, and never below max(first-repair backup value, actual_cost sum).
--
-- Run with the application stopped. Review the dry-run output before COMMIT.

\set ON_ERROR_STOP on
\pset pager off

BEGIN;

SET LOCAL statement_timeout = '300s';
SET LOCAL lock_timeout = '15s';

CREATE TEMP TABLE subscription_usage_repair_candidate ON COMMIT DROP AS
SELECT
    us.id,
    us.user_id,
    us.group_id,
    us.daily_window_start,
    us.weekly_window_start,
    us.monthly_window_start,
    us.daily_usage_usd::numeric AS daily_usage_usd,
    us.weekly_usage_usd::numeric AS weekly_usage_usd,
    us.monthly_usage_usd::numeric AS monthly_usage_usd,
    LEAST(
        COALESCE(us.daily_window_start, TIMESTAMPTZ 'infinity'),
        COALESCE(us.weekly_window_start, TIMESTAMPTZ 'infinity'),
        COALESCE(us.monthly_window_start, TIMESTAMPTZ 'infinity')
    ) AS min_window_start
FROM user_subscriptions us
WHERE us.deleted_at IS NULL
  AND us.status = 'active'
  AND now() < us.expires_at
  AND (
      us.daily_window_start IS NOT NULL
      OR us.weekly_window_start IS NOT NULL
      OR us.monthly_window_start IS NOT NULL
  );

CREATE INDEX ON subscription_usage_repair_candidate (id);

CREATE TABLE IF NOT EXISTS subscription_usage_repair_backup_20260520_actual_v2 AS
SELECT
    now() AS repair_snapshot_at,
    us.*
FROM user_subscriptions us
WHERE false;

INSERT INTO subscription_usage_repair_backup_20260520_actual_v2
SELECT
    now() AS repair_snapshot_at,
    us.*
FROM user_subscriptions us
JOIN subscription_usage_repair_candidate c ON c.id = us.id;

CREATE TEMP TABLE subscription_usage_repair_logs ON COMMIT DROP AS
SELECT
    ul.subscription_id,
    ul.created_at,
    ul.actual_cost::numeric AS actual_cost
FROM usage_logs ul
JOIN subscription_usage_repair_candidate c ON c.id = ul.subscription_id
WHERE ul.created_at >= c.min_window_start;

CREATE INDEX ON subscription_usage_repair_logs (subscription_id, created_at);

CREATE TEMP TABLE subscription_usage_repair_computed ON COMMIT DROP AS
SELECT
    c.id,
    COALESCE(
        SUM(ul.actual_cost) FILTER (
            WHERE c.daily_window_start IS NOT NULL
              AND ul.created_at >= c.daily_window_start
        ),
        0
    ) AS calc_daily_usage_usd,
    COALESCE(
        SUM(ul.actual_cost) FILTER (
            WHERE c.weekly_window_start IS NOT NULL
              AND ul.created_at >= c.weekly_window_start
        ),
        0
    ) AS calc_weekly_usage_usd,
    COALESCE(
        SUM(ul.actual_cost) FILTER (
            WHERE c.monthly_window_start IS NOT NULL
              AND ul.created_at >= c.monthly_window_start
        ),
        0
    ) AS calc_monthly_usage_usd
FROM subscription_usage_repair_candidate c
LEFT JOIN subscription_usage_repair_logs ul ON ul.subscription_id = c.id
GROUP BY c.id;

CREATE TEMP TABLE first_repair_backup (
    id bigint PRIMARY KEY,
    daily_usage_usd numeric,
    weekly_usage_usd numeric,
    monthly_usage_usd numeric,
    repair_snapshot_at timestamptz
) ON COMMIT DROP;

DO $$
BEGIN
    IF to_regclass('public.subscription_usage_repair_backup_20260520') IS NOT NULL THEN
        INSERT INTO first_repair_backup(
            id,
            daily_usage_usd,
            weekly_usage_usd,
            monthly_usage_usd,
            repair_snapshot_at
        )
        SELECT DISTINCT ON (id)
            id,
            daily_usage_usd::numeric,
            weekly_usage_usd::numeric,
            monthly_usage_usd::numeric,
            repair_snapshot_at
        FROM subscription_usage_repair_backup_20260520
        ORDER BY id, repair_snapshot_at DESC;
    END IF;
END $$;

CREATE TEMP TABLE subscription_usage_repair_targets ON COMMIT DROP AS
SELECT
    c.id,
    c.user_id,
    c.group_id,
    c.daily_usage_usd AS old_daily_usage_usd,
    c.weekly_usage_usd AS old_weekly_usage_usd,
    c.monthly_usage_usd AS old_monthly_usage_usd,
    r.calc_daily_usage_usd,
    r.calc_weekly_usage_usd,
    r.calc_monthly_usage_usd,
    CASE
        WHEN b.id IS NOT NULL THEN GREATEST(b.daily_usage_usd, r.calc_daily_usage_usd)
        ELSE GREATEST(c.daily_usage_usd, r.calc_daily_usage_usd)
    END AS target_daily_usage_usd,
    CASE
        WHEN b.id IS NOT NULL THEN GREATEST(b.weekly_usage_usd, r.calc_weekly_usage_usd)
        ELSE GREATEST(c.weekly_usage_usd, r.calc_weekly_usage_usd)
    END AS target_weekly_usage_usd,
    CASE
        WHEN b.id IS NOT NULL THEN GREATEST(b.monthly_usage_usd, r.calc_monthly_usage_usd)
        ELSE GREATEST(c.monthly_usage_usd, r.calc_monthly_usage_usd)
    END AS target_monthly_usage_usd
FROM subscription_usage_repair_candidate c
JOIN subscription_usage_repair_computed r ON r.id = c.id
LEFT JOIN first_repair_backup b ON b.id = c.id;

-- Dry-run summary. Raises repair reset counters. Lowers only first-repair rows
-- that were incorrectly inflated by the total_cost-based repair.
SELECT
    count(*) AS candidate_count,
    count(*) FILTER (
        WHERE abs(target_daily_usage_usd - old_daily_usage_usd) > 0.000001
           OR abs(target_weekly_usage_usd - old_weekly_usage_usd) > 0.000001
           OR abs(target_monthly_usage_usd - old_monthly_usage_usd) > 0.000001
    ) AS rows_to_update,
    count(*) FILTER (WHERE target_monthly_usage_usd > old_monthly_usage_usd + 0.000001) AS monthly_raise_count,
    COALESCE(SUM(target_monthly_usage_usd - old_monthly_usage_usd) FILTER (
        WHERE target_monthly_usage_usd > old_monthly_usage_usd + 0.000001
    ), 0) AS monthly_raise_sum,
    count(*) FILTER (WHERE old_monthly_usage_usd > target_monthly_usage_usd + 0.000001) AS monthly_lower_count,
    COALESCE(SUM(old_monthly_usage_usd - target_monthly_usage_usd) FILTER (
        WHERE old_monthly_usage_usd > target_monthly_usage_usd + 0.000001
    ), 0) AS monthly_lower_sum
FROM subscription_usage_repair_targets;

WITH repaired AS (
    UPDATE user_subscriptions us
    SET
        daily_usage_usd = t.target_daily_usage_usd,
        weekly_usage_usd = t.target_weekly_usage_usd,
        monthly_usage_usd = t.target_monthly_usage_usd,
        updated_at = now()
    FROM subscription_usage_repair_targets t
    WHERE us.id = t.id
      AND (
          abs(t.target_daily_usage_usd - us.daily_usage_usd::numeric) > 0.000001
          OR abs(t.target_weekly_usage_usd - us.weekly_usage_usd::numeric) > 0.000001
          OR abs(t.target_monthly_usage_usd - us.monthly_usage_usd::numeric) > 0.000001
      )
    RETURNING
        us.id,
        us.user_id,
        us.group_id,
        us.daily_usage_usd,
        us.weekly_usage_usd,
        us.monthly_usage_usd
)
SELECT count(*) AS repaired_rows FROM repaired;

-- Verify no active candidate is still under-counted after the repair.
SELECT
    count(*) AS remaining_under_count
FROM user_subscriptions us
JOIN subscription_usage_repair_computed r ON r.id = us.id
WHERE r.calc_daily_usage_usd > us.daily_usage_usd::numeric + 0.000001
   OR r.calc_weekly_usage_usd > us.weekly_usage_usd::numeric + 0.000001
   OR r.calc_monthly_usage_usd > us.monthly_usage_usd::numeric + 0.000001;

COMMIT;
