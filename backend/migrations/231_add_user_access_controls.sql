-- Per-user access controls for exclusive-group-only access and payment denial.
-- Both default to false to preserve all existing user behavior after upgrade.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS restrict_to_allowed_groups BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS payment_disabled BOOLEAN NOT NULL DEFAULT FALSE;
