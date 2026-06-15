-- Add a direct audit pointer from redeem codes to subscription entitlement v2.
-- The fulfillment history remains the replay source of truth; this column makes
-- admin/user history traceable from the redeem code row.

ALTER TABLE redeem_codes
    ADD COLUMN IF NOT EXISTS subscription_entitlement_id BIGINT REFERENCES subscription_entitlements(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_redeem_codes_subscription_entitlement_id
    ON redeem_codes(subscription_entitlement_id);
