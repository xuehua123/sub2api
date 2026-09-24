SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

ALTER TABLE subscription_entitlements
    ADD COLUMN IF NOT EXISTS auto_advance_monthly BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS last_seen_event_id BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS subscription_entitlement_events (
    id BIGSERIAL PRIMARY KEY,
    entitlement_id BIGINT NOT NULL REFERENCES subscription_entitlements(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind VARCHAR(32) NOT NULL,
    source_type VARCHAR(32) NOT NULL DEFAULT '',
    previous_expires_at TIMESTAMPTZ,
    new_expires_at TIMESTAMPTZ NOT NULL,
    validity_seconds BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS subscriptionentitlementevent_user_id_entitlement_id_id
    ON subscription_entitlement_events(user_id, entitlement_id, id);
