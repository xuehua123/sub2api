-- Recover subscription usage counters from authoritative usage_logs.
--
-- Intended for the 2026-05-20 subscription-window incident where the bad build
-- advanced legacy 00:00 usage windows to purchase-time anchors and reset
-- user_subscriptions daily/weekly/monthly counters.
--
-- Run with the application stopped. Review the dry-run output before COMMIT.

\set ON_ERROR_STOP on
\pset pager off

BEGIN;

SET LOCAL statement_timeout = '120s';

CREATE TEMP TABLE subscription_usage_repair_candidate ON COMMIT DROP AS
SELECT
    us.id,
    us.user_id,
    us.group_id,
    us.daily_window_start,
    us.weekly_window_start,
    us.monthly_window_start,
    us.daily_usage_usd,
    us.weekly_usage_usd,
    us.monthly_usage_usd
FROM user_subscriptions us
WHERE us.deleted_at IS NULL
  AND us.status = 'active'
  AND us.updated_at >= TIMESTAMPTZ '2026-05-20 14:00:00+08'
  AND us.created_at < us.updated_at - INTERVAL '5 minutes'
  AND (
      us.daily_window_start IS NOT NULL
      OR us.weekly_window_start IS NOT NULL
      OR us.monthly_window_start IS NOT NULL
  );

CREATE INDEX ON subscription_usage_repair_candidate (id);

CREATE TABLE IF NOT EXISTS subscription_usage_repair_backup_20260520 AS
SELECT
    now() AS repair_snapshot_at,
    us.*
FROM user_subscriptions us
JOIN subscription_usage_repair_candidate c ON c.id = us.id;

CREATE TEMP TABLE subscription_usage_repair_logs ON COMMIT DROP AS
SELECT
    ul.subscription_id,
    ul.created_at,
    ul.total_cost
FROM usage_logs ul
JOIN subscription_usage_repair_candidate c ON c.id = ul.subscription_id
WHERE ul.created_at >= TIMESTAMPTZ '2026-04-01 00:00:00+08';

CREATE INDEX ON subscription_usage_repair_logs (subscription_id, created_at);

CREATE TEMP TABLE subscription_usage_repair_computed ON COMMIT DROP AS
SELECT
    c.id,
    COALESCE(
        SUM(ul.total_cost) FILTER (
            WHERE c.daily_window_start IS NOT NULL
              AND ul.created_at >= c.daily_window_start
        ),
        0
    ) AS calc_daily_usage_usd,
    COALESCE(
        SUM(ul.total_cost) FILTER (
            WHERE c.weekly_window_start IS NOT NULL
              AND ul.created_at >= c.weekly_window_start
        ),
        0
    ) AS calc_weekly_usage_usd,
    COALESCE(
        SUM(ul.total_cost) FILTER (
            WHERE c.monthly_window_start IS NOT NULL
              AND ul.created_at >= c.monthly_window_start
        ),
        0
    ) AS calc_monthly_usage_usd
FROM subscription_usage_repair_candidate c
LEFT JOIN subscription_usage_repair_logs ul ON ul.subscription_id = c.id
GROUP BY c.id;

-- Dry-run summary. Deltas are "what this script will add back", never subtract.
SELECT
    count(*) AS candidate_count,
    count(*) FILTER (
        WHERE r.calc_daily_usage_usd > c.daily_usage_usd + 0.000001
           OR r.calc_weekly_usage_usd > c.weekly_usage_usd + 0.000001
           OR r.calc_monthly_usage_usd > c.monthly_usage_usd + 0.000001
    ) AS rows_to_repair,
    COALESCE(SUM(GREATEST(r.calc_daily_usage_usd - c.daily_usage_usd, 0)), 0) AS daily_delta_sum,
    COALESCE(SUM(GREATEST(r.calc_weekly_usage_usd - c.weekly_usage_usd, 0)), 0) AS weekly_delta_sum,
    COALESCE(SUM(GREATEST(r.calc_monthly_usage_usd - c.monthly_usage_usd, 0)), 0) AS monthly_delta_sum
FROM subscription_usage_repair_candidate c
JOIN subscription_usage_repair_computed r ON r.id = c.id;

WITH repaired AS (
    UPDATE user_subscriptions us
    SET
        daily_usage_usd = GREATEST(us.daily_usage_usd, r.calc_daily_usage_usd),
        weekly_usage_usd = GREATEST(us.weekly_usage_usd, r.calc_weekly_usage_usd),
        monthly_usage_usd = GREATEST(us.monthly_usage_usd, r.calc_monthly_usage_usd),
        updated_at = now()
    FROM subscription_usage_repair_computed r
    WHERE us.id = r.id
      AND (
          r.calc_daily_usage_usd > us.daily_usage_usd + 0.000001
          OR r.calc_weekly_usage_usd > us.weekly_usage_usd + 0.000001
          OR r.calc_monthly_usage_usd > us.monthly_usage_usd + 0.000001
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

-- Verify no repaired candidate is still under-counted.
SELECT
    count(*) AS remaining_under_count
FROM user_subscriptions us
JOIN subscription_usage_repair_computed r ON r.id = us.id
WHERE r.calc_daily_usage_usd > us.daily_usage_usd + 0.000001
   OR r.calc_weekly_usage_usd > us.weekly_usage_usd + 0.000001
   OR r.calc_monthly_usage_usd > us.monthly_usage_usd + 0.000001;

COMMIT;
