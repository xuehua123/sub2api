# Subscription Entitlements V2 Staging Rollout Runbook

This runbook is the required staging validation package before enabling
subscription entitlements v2 in production. It is intentionally conservative:
do not run it on production as an enablement script, do not build Docker images
on production business servers, and do not put real domains, credentials,
tokens, account ids, or payment amounts into source control.

Related references:

- Implementation plan: `docs/plans/2026-06-11-subscription-entitlements-v2-cn.md`
- Preflight SQL: `docs/plans/subscription-entitlements-v2-preflight-sql.md`
- Optional smoke helper: `tools/subscription_entitlements_v2_staging_smoke.ps1`

## Scope And Safety

This rollout validates:

- Native payment entitlement fulfillment.
- Redeem and create-and-redeem entitlement fulfillment.
- API key entitlement group selection and same-entitlement group switching.
- Middleware auth context and gateway usage billing.
- Entitlement quota, quota exceeded, and balance fallback behavior.
- Admin usage observability through `entitlement_id` and `billing_source`.
- User read-only `/entitlements` and minimal `/subscriptions` alias safety.
- Legacy behavior when flags are disabled.

This rollout does not validate by editing `sub2-payment-page`, logging into
production, enabling production flags, building Docker on production, deleting
schema, or rolling back additive migrations.

## Required Inputs

Prepare these values outside the repository:

- Staging base URL.
- Admin token for staging.
- User token for a staging test user.
- User API key for a staging test user.
- Native subscription plan id and payment method suitable for staging.
- Redeem code or redeem plan fixture for staging.
- Subscription group ids covered by the test entitlement.
- A standard or balance group id for legacy/balance comparison.
- Test amounts and quota limits that are safe for staging.
- Database read access for running preflight SQL.

Do not commit any of these values.

## Phase 0: Deploy With Both Flags Disabled

After deploying the build to staging, both flags must start disabled:

- `subscription_entitlements_v2_enabled=false`
- `sub2_payment_page_legacy_mapping_enabled=false`

Use the admin settings page or API to verify the values. The smoke helper can
also check this read-only:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File tools/subscription_entitlements_v2_staging_smoke.ps1 `
  -BaseUrl "https://staging.example.com" `
  -AdminToken "<admin-token>" `
  -ExpectV2Enabled false `
  -ExpectLegacyMappingEnabled false
```

## Phase 1: Migration And Preflight

1. Confirm all migrations have run, including
   `154_api_key_entitlement_binding_preflight.sql`.
2. Run the SQL in
   `docs/plans/subscription-entitlements-v2-preflight-sql.md`.
3. Save the SQL output with the staging evidence.

The following preflight results must be zero before enabling v2 broadly:

- Invalid API key entitlement binding count.
- Invalid API key entitlement binding details.
- Future, expired, revoked, deleted, missing, owner-mismatched, or
  group-uncovered entitlements bound to API keys.
- Alias-eligible entitlement rows whose `legacy_subscription_id` does not point
  to a real legacy subscription.
- Invalid `entitlement_balance_fallback` usage rows where `entitlement_id` is
  missing, `actual_cost <= 0`, or `subscription_id` points to no real legacy
  subscription.

The following preflight results require human explanation, not automatic
failure:

- Entitlement-only record count. These must remain visible through
  `/entitlements` and excluded from the `/subscriptions` alias.
- Multiple active entitlements covering the same `user_id + group_id`.
- `billing_source='historical_null'` rows. Historical rows may predate the new
  attribution field.
- Fallback balance reconciliation aggregates. Compare these with staging
  balance-cache and balance-deduction evidence.

If multiple active entitlements cover the same `user_id + group_id`, validate
that the user API key UI forces explicit entitlement selection. Do not enable a
broad rollout if users can be silently assigned to an arbitrary entitlement.

## Phase 2: Legacy Baseline With V2 Disabled

With both flags still disabled:

1. Create or use a legacy subscription API key.
2. Call a supported model through the gateway and verify the request succeeds.
3. Verify legacy subscription usage and legacy balance billing still behave as
   they did before this branch.
4. Verify admin usage can still list legacy balance and legacy subscription
   records.
5. Verify no entitlement-specific field is required by the legacy API key flow.

Expected result: no user-visible behavior changes from the legacy system.

## Phase 3: Enable Entitlements V2 Only

Enable only:

- `subscription_entitlements_v2_enabled=true`

Keep disabled:

- `sub2_payment_page_legacy_mapping_enabled=false`

Do not enable legacy payment-page mapping until native v2 flows pass.

## Phase 4: Native V2 Validation

Run these checks in staging:

1. Native payment plan fulfillment:
   - Buy a staging subscription plan using a safe staging payment method.
   - Verify a `subscription_entitlements` row exists.
   - Verify a fulfillment history row exists for the payment source.
   - Verify replaying the same payment source does not extend twice.
   - Verify `payment_orders.subscription_entitlement_id` is set.

2. Redeem fulfillment:
   - Redeem a subscription code that maps to a v2 plan.
   - Verify the entitlement is created or extended once.
   - Verify `redeem_codes.subscription_entitlement_id` and `plan_id` are set.
   - Verify replay with mismatched value, group, validity days, type, or source
     suffix fails with conflict and does not issue entitlement.

3. API key creation and group selection:
   - Open the user API key create/edit UI.
   - Select a subscription group covered by the entitlement.
   - If only one entitlement covers the group, verify it is selected.
   - If multiple entitlements cover the group, verify the UI requires explicit
     entitlement choice.
   - Verify standard and balance groups do not send
     `subscription_entitlement_id`.

4. Same-entitlement group switching:
   - Switch an API key to another group covered by the same entitlement.
   - Verify the API key keeps the same entitlement binding.
   - Verify switching to a group covered only by another entitlement requires
     an explicit edit and does not silently search for new quota.

5. Gateway usage billing:
   - Send a successful billable request through an entitlement-bound API key.
   - Verify entitlement usage increments once.
   - Verify `usage_logs.entitlement_id` is set.
   - Verify `usage_logs.billing_source='entitlement_quota'`.

6. Quota exceeded:
   - Exhaust or temporarily lower an entitlement quota in staging.
   - Send a request that exceeds quota.
   - Verify HTTP 429.
   - Verify no successful usage log is written for the failed request.
   - Verify entitlement usage and balance are unchanged.

7. Balance fallback success:
   - Use an entitlement with `overage_policy='balance_fallback'`.
   - Exceed entitlement quota with enough user balance.
   - Verify balance is deducted.
   - Verify balance cache and low-balance notification behavior are consistent.
   - Verify `usage_logs.billing_source='entitlement_balance_fallback'`.
   - Verify entitlement usage is not incremented for fallback usage.

8. Balance fallback insufficient balance:
   - Use the same policy with insufficient balance.
   - Verify the request fails.
   - Verify usage log, billing dedup, entitlement usage, and balance changes are
     rolled back.

9. Admin usage observability:
   - Filter admin usage by `entitlement_id`.
   - Confirm `billing_source` distinguishes `entitlement_quota` and
     `entitlement_balance_fallback`.
   - Confirm legacy balance and legacy subscription rows still display
     correctly.

10. User read-only APIs:
    - `GET /api/v1/entitlements` returns only the current user's entitlements.
    - The response does not include `source_id`, `source_external_id`,
      `source_redeem_code_id`, `assigned_by`, `notes`, `plan_snapshot`, or
      fulfillment history.
    - `GET /api/v1/subscriptions` returns only entitlement aliases with a real
      `legacy_subscription_id`.
    - Entitlement-only rows are not returned through `/subscriptions` and no
      entitlement id is used as a fake legacy subscription id.

Use the smoke helper to run read-only API checks where possible:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File tools/subscription_entitlements_v2_staging_smoke.ps1 `
  -BaseUrl "https://staging.example.com" `
  -AdminToken "<admin-token>" `
  -UserToken "<user-token>" `
  -EntitlementId 123 `
  -ExpectedAliasLegacySubscriptionId 456 `
  -ExpectV2Enabled true `
  -ExpectLegacyMappingEnabled false
```

When checking `/subscriptions` alias safety, pass
`-ExpectedAliasLegacySubscriptionId` for a known entitlement that is linked to a
real legacy subscription. This lets the helper assert that the alias row uses
the real legacy id instead of an entitlement id.

## Phase 5: Legacy Payment-Page Mapping Exact Match

Only after native payment, redeem, API key, and gateway checks pass:

1. Keep the mapping flag disabled and verify external mapping rows are present.
2. Verify amount, group, validity days, and source suffix are exact matches.
3. Enable `sub2_payment_page_legacy_mapping_enabled=true` in staging only.
4. Run one small-value legacy cashier order with a test payment flow.
5. Verify the exact mapping produced one entitlement fulfillment.
6. Verify amount mismatch or tuple mismatch does not issue v2 entitlement.
7. Disable the mapping flag again unless continuing with an approved staging
   canary.

Do not modify `sub2-payment-page` for this validation.

## Phase 6: Rollback And Closeout

Rollback path:

1. Set `subscription_entitlements_v2_enabled=false`.
2. Set `sub2_payment_page_legacy_mapping_enabled=false`.
3. Do not delete entitlement schema.
4. Do not roll back additive migrations.
5. If API key bindings are invalid, rerun migration
   `154_api_key_entitlement_binding_preflight.sql` or clear only invalid
   `api_keys.subscription_entitlement_id` rows using the same predicate.
6. Keep fulfillment and usage history for audit.

After disabling flags, repeat the legacy baseline smoke from Phase 2.

## Evidence To Capture

For each staging run, capture:

- Deployed commit SHA.
- Migration status.
- Preflight SQL output.
- Flag values before and after each phase.
- Native payment order id and entitlement id.
- Redeem code id and entitlement id.
- API key id, selected group id, and selected entitlement id.
- Gateway request id for quota success, quota exceeded, fallback success, and
  fallback insufficient balance.
- Admin usage screenshots or JSON showing `entitlement_id` and `billing_source`.
- Rollback verification if flags are disabled.

## Production Gate

Do not enable production v2 flags until staging evidence shows:

- All must-be-zero preflight checks are zero.
- Native v2 flows pass.
- Legacy mapping exact-match flow passes in staging with a small-value test.
- Rollback has been tested by disabling both flags.
- On-call and operators know to prefer disabling flags over deleting schema.
