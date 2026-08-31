-- Restrict public (non-exclusive) groups to the user's explicit allowlist.
-- This is independent from restrict_to_allowed_groups, which keeps the fork's
-- exclusive-group-only policy. Both default to false for existing users.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS restrict_public_groups BOOLEAN NOT NULL DEFAULT FALSE;
