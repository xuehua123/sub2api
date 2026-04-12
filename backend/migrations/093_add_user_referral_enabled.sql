-- Migration: 093_add_user_referral_enabled
-- Per-user referral override: allows admin to enable referral for specific users
-- even when the global referral switch is off.

ALTER TABLE users ADD COLUMN IF NOT EXISTS referral_enabled BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_users_referral_enabled ON users(referral_enabled);
