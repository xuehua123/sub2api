SET LOCAL lock_timeout = '5s';
CREATE TABLE IF NOT EXISTS user_business_daily_fx (
    rate_date DATE PRIMARY KEY,
    usd_cny NUMERIC(12,6) NOT NULL CHECK (usd_cny > 0 AND usd_cny <= 100),
    source TEXT NOT NULL CHECK (length(trim(source)) BETWEEN 1 AND 200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
COMMENT ON TABLE user_business_daily_fx IS 'Explicit daily CNY/USD reporting rates; never backfilled from a current rate.';
