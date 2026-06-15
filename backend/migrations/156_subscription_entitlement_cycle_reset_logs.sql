CREATE TABLE IF NOT EXISTS subscription_entitlement_cycle_reset_logs (
    id                                  BIGSERIAL PRIMARY KEY,
    user_id                             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entitlement_id                      BIGINT NOT NULL REFERENCES subscription_entitlements(id) ON DELETE CASCADE,
    plan_id                             BIGINT REFERENCES subscription_plans(id) ON DELETE SET NULL,
    previous_expires_at                 TIMESTAMPTZ NOT NULL,
    new_expires_at                      TIMESTAMPTZ NOT NULL,
    previous_monthly_usage_usd          DECIMAL(20, 10) NOT NULL DEFAULT 0,
    previous_monthly_window_start       TIMESTAMPTZ,
    new_monthly_window_start            TIMESTAMPTZ NOT NULL,
    deducted_days                       INTEGER NOT NULL,
    deducted_seconds                    BIGINT NOT NULL,
    created_at                          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscription_entitlement_cycle_reset_logs_user_created
    ON subscription_entitlement_cycle_reset_logs(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_subscription_entitlement_cycle_reset_logs_entitlement_created
    ON subscription_entitlement_cycle_reset_logs(entitlement_id, created_at DESC);
