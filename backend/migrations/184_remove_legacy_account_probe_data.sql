-- Retire the account-local upstream monitoring implementation. Shared upstream
-- connections are now the only source for wallet, group, and key observations.

UPDATE accounts
SET extra = COALESCE(
        (
            SELECT jsonb_object_agg(entry.key, entry.value)
            FROM jsonb_each(COALESCE(accounts.extra, '{}'::jsonb)) AS entry(key, value)
            WHERE left(entry.key, length('balance_probe_')) <> 'balance_probe_'
              AND left(entry.key, length('upstream_billing_probe')) <> 'upstream_billing_probe'
              AND left(entry.key, length('upstream_rate_multiplier_sync_')) <> 'upstream_rate_multiplier_sync_'
        ),
        '{}'::jsonb
    ),
    updated_at = NOW()
WHERE EXISTS (
    SELECT 1
    FROM jsonb_object_keys(COALESCE(accounts.extra, '{}'::jsonb)) AS retired(key)
    WHERE left(retired.key, length('balance_probe_')) = 'balance_probe_'
       OR left(retired.key, length('upstream_billing_probe')) = 'upstream_billing_probe'
       OR left(retired.key, length('upstream_rate_multiplier_sync_')) = 'upstream_rate_multiplier_sync_'
);

UPDATE accounts
SET credentials = COALESCE(credentials, '{}'::jsonb)
        - 'upstream_management_auth'
        - 'upstream_management_base_url',
    updated_at = NOW()
WHERE COALESCE(credentials, '{}'::jsonb) ?| ARRAY[
    'upstream_management_auth',
    'upstream_management_base_url'
];

DELETE FROM settings
WHERE key = 'upstream_billing_probe_settings';

UPDATE settings
SET value = ((value::jsonb) - 'account_balance')::text,
    updated_at = NOW()
WHERE key = 'ops_alert_runtime_settings'
  AND jsonb_typeof(value::jsonb) = 'object'
  AND value::jsonb ? 'account_balance';

DROP INDEX IF EXISTS idx_upstream_connections_legacy_migration_key;

ALTER TABLE upstream_connections
    DROP COLUMN IF EXISTS legacy_migration_key;
