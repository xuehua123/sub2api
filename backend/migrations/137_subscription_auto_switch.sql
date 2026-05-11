-- 137: Subscription auto-switch support.

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS auto_switch_group_enabled BOOLEAN NOT NULL DEFAULT TRUE;

COMMENT ON COLUMN api_keys.auto_switch_group_enabled
    IS 'Whether this API key may automatically move to another usable subscription group when the current subscription quota is exhausted.';

CREATE TABLE IF NOT EXISTS user_subscription_group_preferences (
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id    BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_user_subscription_group_preferences_user_enabled
    ON user_subscription_group_preferences(user_id, enabled, sort_order, group_id);

CREATE TABLE IF NOT EXISTS api_key_auto_switch_logs (
    id                    BIGSERIAL PRIMARY KEY,
    api_key_id            BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    user_id               BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    from_group_id         BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    to_group_id           BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    from_subscription_id  BIGINT REFERENCES user_subscriptions(id) ON DELETE SET NULL,
    to_subscription_id    BIGINT NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
    reason                VARCHAR(64) NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_api_key_auto_switch_logs_api_key_created
    ON api_key_auto_switch_logs(api_key_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_api_key_auto_switch_logs_user_created
    ON api_key_auto_switch_logs(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS subscription_cycle_reset_logs (
    id                   BIGSERIAL PRIMARY KEY,
    user_id              BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subscription_id      BIGINT NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
    group_id             BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    previous_expires_at  TIMESTAMPTZ NOT NULL,
    new_expires_at       TIMESTAMPTZ NOT NULL,
    previous_monthly_usage_usd DECIMAL(20, 10) NOT NULL DEFAULT 0,
    previous_monthly_window_start TIMESTAMPTZ,
    new_monthly_window_start TIMESTAMPTZ NOT NULL,
    deducted_days        INTEGER NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscription_cycle_reset_logs_user_created
    ON subscription_cycle_reset_logs(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_subscription_cycle_reset_logs_subscription_created
    ON subscription_cycle_reset_logs(subscription_id, created_at DESC);
