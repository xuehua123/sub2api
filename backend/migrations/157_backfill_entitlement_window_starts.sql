-- Backfill V2 entitlement quota windows so purchase activation and quota cycles align.
-- Earlier entitlement rows could be active with NULL window starts, which made the UI
-- show "waiting for first use" even though the card was already valid.

UPDATE subscription_entitlements
SET
    daily_window_start = CASE
        WHEN daily_limit_usd IS NOT NULL AND daily_limit_usd > 0 THEN COALESCE(daily_window_start, starts_at)
        ELSE daily_window_start
    END,
    weekly_window_start = CASE
        WHEN weekly_limit_usd IS NOT NULL AND weekly_limit_usd > 0 THEN COALESCE(weekly_window_start, starts_at)
        ELSE weekly_window_start
    END,
    monthly_window_start = CASE
        WHEN monthly_limit_usd IS NOT NULL AND monthly_limit_usd > 0 THEN COALESCE(monthly_window_start, starts_at)
        ELSE monthly_window_start
    END,
    updated_at = NOW()
WHERE deleted_at IS NULL
  AND status = 'active'
  AND (
      (daily_limit_usd IS NOT NULL AND daily_limit_usd > 0 AND daily_window_start IS NULL)
      OR (weekly_limit_usd IS NOT NULL AND weekly_limit_usd > 0 AND weekly_window_start IS NULL)
      OR (monthly_limit_usd IS NOT NULL AND monthly_limit_usd > 0 AND monthly_window_start IS NULL)
  );
