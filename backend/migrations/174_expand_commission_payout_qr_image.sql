-- Allow the payout-account form to persist uploaded QR-code data URLs.
-- The frontend accepts images up to 2 MiB, so VARCHAR(512) is insufficient.
ALTER TABLE commission_payout_accounts
    ALTER COLUMN qr_image_url TYPE TEXT;
