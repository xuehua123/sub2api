# Subscription Entitlements V2 Staging Execution Template

This template is for staging rollout preparation and evidence collection. Do
not commit filled evidence, real tokens, database passwords, production hosts,
database dumps, or payment credentials to the repository.

Related references:

- Rollout runbook: `docs/plans/subscription-entitlements-v2-staging-rollout-runbook.md`
- Preflight SQL: `docs/plans/subscription-entitlements-v2-preflight-sql.md`
- Smoke helper: `tools/subscription_entitlements_v2_staging_smoke.ps1`

## Staging Parameters

Fill this table outside source control or in a private staging evidence folder.
Token and connection-string values must stay redacted in shared reports.

| Item | Value / Location | Verification Notes |
| --- | --- | --- |
| Staging BaseUrl | `https://<staging-host>` | Must be a staging host, not production. |
| AdminToken acquisition method | `[REDACTED]` | Record who generated it and expiry time, not the token value. |
| AdminToken value | `[REDACTED]` | Never paste the token into this document or chat logs. |
| UserToken acquisition method | `[REDACTED]` | Use the staging test user only. |
| UserToken value | `[REDACTED]` | Never paste the token into this document or chat logs. |
| Staging DB connection method | `[REDACTED]` | Must point to staging DB only. |
| Staging DB connection secret | `[REDACTED]` | Do not save passwords or full DSNs in git. |
| Test user id | `<staging-user-id>` | Must belong to the UserToken subject. |
| Test user email | `<staging-user-email>` | Use a staging-only account. |
| Test API key id | `<staging-api-key-id>` | Use a key owned by the test user. |
| Test API key name | `<staging-api-key-name>` | Do not paste the API key secret. |
| Native subscription plan id | `<plan_id>` | Plan should have entitlement v2 configuration. |
| Entitlement id | `<entitlement_id>` | Created by native payment or redeem staging flow. |
| Legacy subscription id | `<legacy_subscription_id>` | Required only for `/subscriptions` alias validation. |
| Entitlement + legacy pair | `<entitlement_id> -> <legacy_subscription_id>` | Confirm alias uses the real legacy id and never the entitlement id. |
| Multi-group entitlement group ids | `<group_id_1>, <group_id_2>, ...` | Must include at least two authorized subscription groups. |
| Primary group id | `<primary_group_id>` | Used for legacy anchor checks. |
| Standard/balance group id | `<standard_or_balance_group_id>` | Used to verify non-entitlement behavior. |
| Fallback enough-balance user | `<user_id/email>` | Must have enough staging balance for fallback success. |
| Fallback insufficient-balance user | `<user_id/email>` | Must fail fallback without writing successful usage. |
| Safe staging payment method | `<staging-payment-method>` | Manual small-value staging-only payment if needed. |
| Safe staging amount / quota | `<amount-or-limit>` | Keep small and staging-only. |

## Pre-Execution Confirmation

Complete every row before running staging checks.

| Check | Owner | Result | Notes |
| --- | --- | --- | --- |
| Confirm target is not production |  |  | Host, DB, tokens, and payment method are staging-only. |
| Confirm `subscription_entitlements_v2_enabled=false` |  |  | Capture `flags-before.txt`. |
| Confirm `sub2_payment_page_legacy_mapping_enabled=false` |  |  | Capture `flags-before.txt`. |
| Confirm migrations reached 154 |  |  | Include migration status output. |
| Confirm preflight SQL output path |  |  | Example: `staging-evidence/YYYYMMDD-HHMM/preflight/`. |
| Confirm rollback owner |  |  | Person who can disable both flags. |
| Confirm rollback window |  |  | Include expected start/end time and on-call contact. |
| Confirm no production Docker build will run |  |  | Staging deploy should use the approved build path. |
| Confirm no evidence/token/db dump will be committed |  |  | Evidence stays outside git. |

## Evidence Directory

Do not commit the evidence directory. Use a local or secured shared location,
for example:

```text
staging-evidence/YYYYMMDD-HHMM/
  00-inputs-redacted.md
  01-flags-before.txt
  02-smoke-before.txt
  03-preflight/
    preflight-invalid-bindings.csv
    preflight-invalid-binding-details.csv
    preflight-future-expired-revoked.csv
    preflight-multi-active.csv
    preflight-alias-safety.csv
    preflight-billing-source-distribution.csv
    preflight-fallback-reconciliation.csv
  04-native-payment/
    native-payment-result.txt
    fulfillment-history-result.txt
  05-redeem/
    redeem-result.txt
    redeem-replay-conflict-result.txt
  06-api-key/
    group-selection-result.txt
    same-entitlement-switch-result.txt
  07-gateway/
    gateway-quota-result.txt
    quota-exceeded-result.txt
    fallback-result.txt
    fallback-insufficient-result.txt
  08-observability/
    admin-usage-entitlement-filter.txt
    admin-usage-billing-source.txt
    user-entitlements-redacted.json
    subscriptions-alias-redacted.json
  09-legacy-mapping/
    legacy-mapping-exact-match-result.txt
  10-rollback/
    flags-after-rollback.txt
    rollback-result.txt
```

Recommended evidence naming:

| Step | Evidence File | Contents |
| --- | --- | --- |
| Initial flags | `01-flags-before.txt` | Admin settings response with tokens removed. |
| Preflight invalid bindings | `preflight-invalid-bindings.csv` | Must be zero. |
| Preflight multi-active | `preflight-multi-active.csv` | Explain each non-zero row. |
| Initial smoke | `02-smoke-before.txt` | `-ExpectV2Enabled false -ExpectLegacyMappingEnabled false`. |
| V2 smoke | `smoke-after-v2.txt` | `-ExpectV2Enabled true -ExpectLegacyMappingEnabled false`. |
| Native payment | `native-payment-result.txt` | Payment order id, entitlement id, fulfillment id. |
| Redeem | `redeem-result.txt` | Redeem code id, entitlement id, replay result. |
| Gateway quota | `gateway-quota-result.txt` | Request id, entitlement usage delta, usage log id. |
| Fallback | `fallback-result.txt` | Request id, balance delta, `billing_source`. |
| Rollback | `rollback-result.txt` | Flag values and legacy baseline result after rollback. |

## Preflight SQL Plan

Run the SQL in `docs/plans/subscription-entitlements-v2-preflight-sql.md`
against staging only. Save raw output and a short interpretation.

Must be zero or empty before enabling v2 broadly:

- Invalid API key entitlement binding count.
- Invalid API key entitlement binding details.
- Future, expired, revoked, deleted, missing, owner-mismatched, or
  group-uncovered entitlements bound to API keys.
- Alias-eligible entitlement rows whose `legacy_subscription_id` does not point
  to a real legacy subscription.
- Invalid `entitlement_balance_fallback` usage rows where `entitlement_id` is
  missing, `actual_cost <= 0`, or `subscription_id` points to no real legacy
  subscription.

Requires human explanation, not automatic failure:

- Entitlement-only record count.
- Multiple active entitlements covering the same `user_id + group_id`.
- `billing_source='historical_null'` rows.
- Fallback balance reconciliation aggregates.

## Smoke Command Templates

Store sensitive values in environment variables for the shell session. Do not
paste real token values into this document or source control.

```powershell
$env:SUB2API_STAGING_BASE_URL = "https://<staging-host>"
$env:SUB2API_STAGING_ADMIN_TOKEN = "[REDACTED]"
$env:SUB2API_STAGING_USER_TOKEN = "[REDACTED]"
```

Initial v2-off check:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File tools/subscription_entitlements_v2_staging_smoke.ps1 `
  -BaseUrl $env:SUB2API_STAGING_BASE_URL `
  -AdminToken $env:SUB2API_STAGING_ADMIN_TOKEN `
  -ExpectV2Enabled false `
  -ExpectLegacyMappingEnabled false `
  *> staging-evidence/YYYYMMDD-HHMM/02-smoke-before.txt
```

V2-on, legacy mapping still off:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File tools/subscription_entitlements_v2_staging_smoke.ps1 `
  -BaseUrl $env:SUB2API_STAGING_BASE_URL `
  -AdminToken $env:SUB2API_STAGING_ADMIN_TOKEN `
  -UserToken $env:SUB2API_STAGING_USER_TOKEN `
  -EntitlementId <entitlement_id> `
  -ExpectedAliasLegacySubscriptionId <legacy_subscription_id> `
  -ExpectV2Enabled true `
  -ExpectLegacyMappingEnabled false `
  *> staging-evidence/YYYYMMDD-HHMM/smoke-after-v2.txt
```

Alias pair rule: `ExpectedAliasLegacySubscriptionId` must be the real legacy
subscription id linked to the entitlement. It must never be the entitlement id.

## Manual Validation Record

| Validation Item | Executor | Time | Result | Evidence File | Notes |
| --- | --- | --- | --- | --- | --- |
| Initial flags are both false |  |  |  | `01-flags-before.txt` |  |
| Preflight SQL complete |  |  |  | `03-preflight/` |  |
| Native payment entitlement fulfillment |  |  |  | `04-native-payment/native-payment-result.txt` |  |
| Native payment fulfillment history and replay |  |  |  | `04-native-payment/fulfillment-history-result.txt` |  |
| Redeem entitlement fulfillment |  |  |  | `05-redeem/redeem-result.txt` |  |
| Redeem replay/mismatch conflict |  |  |  | `05-redeem/redeem-replay-conflict-result.txt` |  |
| API key entitlement group selection |  |  |  | `06-api-key/group-selection-result.txt` |  |
| API key same-entitlement group switch |  |  |  | `06-api-key/same-entitlement-switch-result.txt` |  |
| Gateway quota success |  |  |  | `07-gateway/gateway-quota-result.txt` |  |
| Quota exceeded returns 429 and writes no success usage |  |  |  | `07-gateway/quota-exceeded-result.txt` |  |
| Balance fallback success |  |  |  | `07-gateway/fallback-result.txt` |  |
| Balance fallback insufficient balance rollback |  |  |  | `07-gateway/fallback-insufficient-result.txt` |  |
| Admin usage `entitlement_id` filter |  |  |  | `08-observability/admin-usage-entitlement-filter.txt` |  |
| Admin usage `billing_source` distinction |  |  |  | `08-observability/admin-usage-billing-source.txt` |  |
| User `/entitlements` safe DTO |  |  |  | `08-observability/user-entitlements-redacted.json` |  |
| `/subscriptions` alias uses real legacy id |  |  |  | `08-observability/subscriptions-alias-redacted.json` |  |
| Legacy mapping exact-match small staging check |  |  |  | `09-legacy-mapping/legacy-mapping-exact-match-result.txt` | Run only after native v2 passes. |
| Rollback closes both flags |  |  |  | `10-rollback/flags-after-rollback.txt` |  |
| Legacy baseline after rollback |  |  |  | `10-rollback/rollback-result.txt` |  |

## Rollback Steps

1. Set `subscription_entitlements_v2_enabled=false`.
2. Set `sub2_payment_page_legacy_mapping_enabled=false`.
3. Do not delete entitlement schema.
4. Do not roll back additive migrations.
5. If API key bindings are invalid, rerun migration
   `154_api_key_entitlement_binding_preflight.sql` or clear only invalid
   `api_keys.subscription_entitlement_id` rows using the same predicate.
6. Save flag values and legacy baseline evidence under `10-rollback/`.

## Prohibited Actions

- Do not log in to production.
- Do not enable production flags.
- Do not build Docker images on production business servers.
- Do not commit evidence, tokens, DB connection strings, database dumps,
  payment credentials, or user secrets.
- Do not modify `sub2-payment-page`.
- Do not run real payment or billing scripts without explicit staging-only
  human approval.
- Do not use production users, production API keys, production plans, or
  production payment methods.
