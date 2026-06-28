# Subscription Entitlements V2 Preflight SQL

Run these checks on staging first, then on production before enabling
`subscription_entitlements_v2_enabled`. Keep
`sub2_payment_page_legacy_mapping_enabled=false` until the legacy cashier
mapping checks pass with real test orders.

## API Key Entitlement Bindings

Invalid binding count. These rows should be zero after migration
`154_api_key_entitlement_binding_preflight.sql` has run.

```sql
SELECT COUNT(*) AS invalid_api_key_entitlement_bindings
FROM api_keys ak
LEFT JOIN subscription_entitlements se
  ON se.id = ak.subscription_entitlement_id
LEFT JOIN subscription_entitlement_groups seg
  ON seg.entitlement_id = se.id
 AND seg.group_id = ak.group_id
 AND seg.enabled = TRUE
WHERE ak.subscription_entitlement_id IS NOT NULL
  AND (
      se.id IS NULL
      OR se.deleted_at IS NOT NULL
      OR se.user_id <> ak.user_id
      OR se.status <> 'active'
      OR se.starts_at > NOW()
      OR se.expires_at <= NOW()
      OR seg.entitlement_id IS NULL
  );
```

Invalid binding details.

```sql
SELECT
    ak.id AS api_key_id,
    ak.user_id AS api_key_user_id,
    ak.group_id AS api_key_group_id,
    ak.subscription_entitlement_id,
    se.user_id AS entitlement_user_id,
    se.status AS entitlement_status,
    se.starts_at,
    se.expires_at,
    se.deleted_at,
    CASE
        WHEN se.id IS NULL THEN 'missing_entitlement'
        WHEN se.deleted_at IS NOT NULL THEN 'deleted_entitlement'
        WHEN se.user_id <> ak.user_id THEN 'owner_mismatch'
        WHEN se.status <> 'active' THEN 'inactive_status'
        WHEN se.starts_at > NOW() THEN 'future_start'
        WHEN se.expires_at <= NOW() THEN 'expired'
        WHEN seg.entitlement_id IS NULL THEN 'group_not_covered'
        ELSE 'unknown'
    END AS reason
FROM api_keys ak
LEFT JOIN subscription_entitlements se
  ON se.id = ak.subscription_entitlement_id
LEFT JOIN subscription_entitlement_groups seg
  ON seg.entitlement_id = se.id
 AND seg.group_id = ak.group_id
 AND seg.enabled = TRUE
WHERE ak.subscription_entitlement_id IS NOT NULL
  AND (
      se.id IS NULL
      OR se.deleted_at IS NOT NULL
      OR se.user_id <> ak.user_id
      OR se.status <> 'active'
      OR se.starts_at > NOW()
      OR se.expires_at <= NOW()
      OR seg.entitlement_id IS NULL
  )
ORDER BY reason, ak.user_id, ak.group_id, ak.id;
```

Future, expired, or revoked entitlements currently bound to API keys.

```sql
SELECT
    se.status,
    COUNT(*) AS bound_api_keys
FROM api_keys ak
JOIN subscription_entitlements se ON se.id = ak.subscription_entitlement_id
WHERE ak.subscription_entitlement_id IS NOT NULL
  AND (
      se.status <> 'active'
      OR se.starts_at > NOW()
      OR se.expires_at <= NOW()
      OR se.deleted_at IS NOT NULL
  )
GROUP BY se.status
ORDER BY se.status;
```

Multiple active entitlements covering the same user and group. These are valid
but ambiguous for default selection; API key forms should require explicit
choice when they are visible to users.

```sql
SELECT
    se.user_id,
    seg.group_id,
    COUNT(*) AS active_entitlement_count,
    ARRAY_AGG(se.id ORDER BY se.expires_at ASC, seg.sort_order ASC, se.id ASC) AS entitlement_ids
FROM subscription_entitlements se
JOIN subscription_entitlement_groups seg
  ON seg.entitlement_id = se.id
 AND seg.enabled = TRUE
WHERE se.deleted_at IS NULL
  AND se.status = 'active'
  AND se.starts_at <= NOW()
  AND se.expires_at > NOW()
GROUP BY se.user_id, seg.group_id
HAVING COUNT(*) > 1
ORDER BY active_entitlement_count DESC, se.user_id, seg.group_id;
```

## `/subscriptions` Alias Safety

Entitlement-only records should not be returned through the legacy
`/subscriptions` alias. They remain visible through `/entitlements`.

```sql
SELECT
    COUNT(*) AS entitlement_only_records
FROM subscription_entitlements
WHERE deleted_at IS NULL
  AND legacy_subscription_id IS NULL;
```

Alias-eligible records must have a real legacy subscription id.

```sql
SELECT
    se.id AS entitlement_id,
    se.legacy_subscription_id,
    us.id AS legacy_subscription_exists
FROM subscription_entitlements se
LEFT JOIN user_subscriptions us ON us.id = se.legacy_subscription_id
WHERE se.deleted_at IS NULL
  AND se.legacy_subscription_id IS NOT NULL
  AND us.id IS NULL
ORDER BY se.id;
```

## Usage Attribution

Billing source distribution.

```sql
SELECT
    COALESCE(billing_source, 'historical_null') AS billing_source,
    COUNT(*) AS rows,
    SUM(actual_cost) AS actual_cost_sum
FROM usage_logs
WHERE created_at >= NOW() - INTERVAL '7 days'
GROUP BY COALESCE(billing_source, 'historical_null')
ORDER BY rows DESC;
```

Fallback rows should have an entitlement id, no fake legacy subscription id
unless a real legacy subscription also exists, and a positive actual cost.

```sql
SELECT
    ul.id,
    ul.user_id,
    ul.api_key_id,
    ul.group_id,
    ul.subscription_id,
    ul.entitlement_id,
    ul.actual_cost,
    ul.created_at
FROM usage_logs ul
LEFT JOIN user_subscriptions us
  ON us.id = ul.subscription_id
WHERE ul.billing_source = 'entitlement_balance_fallback'
  AND (
      ul.entitlement_id IS NULL
      OR ul.actual_cost <= 0
      OR (ul.subscription_id IS NOT NULL AND us.id IS NULL)
  )
ORDER BY ul.created_at DESC
LIMIT 100;
```

Fallback balance reconciliation sample. This query compares fallback cost with
the usage rows that should have triggered balance deduction; use it with
application balance-cache logs during staging.

```sql
SELECT
    user_id,
    COUNT(*) AS fallback_requests,
    SUM(actual_cost) AS fallback_balance_cost
FROM usage_logs
WHERE billing_source = 'entitlement_balance_fallback'
  AND created_at >= NOW() - INTERVAL '24 hours'
GROUP BY user_id
ORDER BY fallback_balance_cost DESC;
```

## Production Handling

1. Keep both entitlement flags disabled until the invalid binding count is 0.
2. If invalid bindings are found after deployment, rerun migration
   `154_api_key_entitlement_binding_preflight.sql` or clear only the invalid
   rows with the same predicate. Do not delete entitlement schema or history.
3. Prefer clearing invalid `api_keys.subscription_entitlement_id` to `NULL`.
   Runtime default resolution will select an active entitlement that covers the
   key's current group.
4. If multi-active coverage is common, verify the user API key UI forces an
   explicit entitlement choice before enabling v2 broadly.
5. Rollback path: set `subscription_entitlements_v2_enabled=false` and
   `sub2_payment_page_legacy_mapping_enabled=false`. Leave additive schema in
   place.
