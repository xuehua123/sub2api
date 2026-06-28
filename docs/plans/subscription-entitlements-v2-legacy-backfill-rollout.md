# Subscription Entitlements V2 Legacy Backfill Rollout

**Goal:** migrate existing legacy `user_subscriptions` users to subscription entitlement v2 without changing quota, usage, billing source, validity windows, API key behavior, or rollback safety.

**Scope:** this document is the production rollout design and dry-run contract. It does not execute production writes, enable flags, build Docker, or modify `sub2-payment-page`.

**Current production input, as provided by the read-only inventory:**

| Environment | Active legacy subscriptions | Groups | Legacy subscription groups | Active API keys | Observed account topology |
| --- | ---: | ---: | ---: | ---: | --- |
| wenrugou | 260 | 27 | 22 | 982 | OpenAI/Codex subscription groups mostly rate `2.2`, each bound to the same 8 accounts with 2 active schedulable accounts; Anthropic/Claude groups rate `1.0`, some without `account_groups` |
| pipixia | 813 | 18 | 15 | 2471 | OpenAI/Codex subscription groups mostly rate `1.5`, each bound to the same 14 accounts with 2 active schedulable accounts; Kiro/Anthropic groups rate `1.0`, some without `account_groups` |

## Product Model

The v2 model separates four concepts that legacy groups mixed together:

1. **Plan template:** what a user bought: price, validity, quota limits, overage policy, and historical package name.
2. **Runtime group:** where requests are routed: platform, account pool, model routing, rate multiplier, scheduling controls.
3. **Access source:** how an API key pays:
   - `balance`: virtual balance access source, not stored in `subscription_entitlements`.
   - `entitlement`: real subscription entitlement, stored in `subscription_entitlements`.
4. **Account pool:** `account_groups` bindings that route traffic for a runtime group.

Rules:

- API keys with `access_source='entitlement'` must have `subscription_entitlement_id`.
- API keys with `access_source='balance'` must have `subscription_entitlement_id=NULL`.
- An entitlement grants one or more runtime groups through `subscription_entitlement_groups`.
- Overage balance deduction is controlled only by `subscription_entitlements.overage_policy`.
- Legacy `user_subscriptions` remain untouched and usable for rollback.
- Never put an entitlement id into `usage_logs.subscription_id` or any legacy `subscription_id`.
- Never copy multiple `user_subscriptions` to simulate multi-group access.
- Never merge OpenAI/Codex and Anthropic/Kiro/Claude accounts into the same runtime group.
- Never automatically expose exclusive/private/test groups to all users.
- Legacy subscription groups may be `is_exclusive=true` and still be valid
  plan-template, quota, rate, and account-pool sources. The danger is opening
  the old exclusive group itself to everyone, not reading it as migration input.

## Codebase Facts Reviewed

- `subscription_entitlements` has a unique `legacy_subscription_id`, which is the correct idempotency anchor for one legacy subscription -> one entitlement.
- `subscription_entitlement_fulfillments` records source history and has unique source indexes. A backfill event must be inserted once per legacy subscription with `source_type='legacy_subscription_backfill'` and `source_id=user_subscriptions.id`.
- `api_keys.access_source` exists after migration 155 and is consumed by middleware/gateway routing.
- `groups` now has `balance_enabled`, `subscription_enabled`, and `plan_auto_grant_enabled`.
- v2 `/groups/available` returns source-aware `access_sources` only when `subscription_entitlements_v2_enabled=true`.
- `PaymentConfigService` wildcard plan scopes only auto-select groups with `subscription_enabled=true` and `plan_auto_grant_enabled=true`.
- `user.CanBindGroup` still protects exclusive groups through `user_allowed_groups`.

## Key Design Decision

Do not use legacy subscription groups as runtime groups after backfill.

Legacy subscription groups should become **plan-template anchors** only. They stay in `groups` for rollback and historical references, but v2 API keys should move to platform/rate-compatible runtime groups.

Because the current schema has no durable table for `old_group_id -> plan_id -> runtime_group_id`, the write implementation should add a small backfill mapping table before production writes:

```sql
CREATE TABLE subscription_legacy_backfill_mappings (
    legacy_group_id BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE RESTRICT,
    plan_id BIGINT NOT NULL REFERENCES subscription_plans(id) ON DELETE RESTRICT,
    runtime_group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
    runtime_group_key VARCHAR(128) NOT NULL,
    mapping_version VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

This avoids relying on names or heuristics during idempotent reruns and rollback.

## A. Data Model Mapping

### Legacy Groups To Plans

For each old active subscription group:

- Create or reuse one internal `subscription_plans` row.
- Suggested plan fields:
  - `name`: old group name, prefixed or tagged as legacy backfill if a duplicate would collide in UI.
  - `description`: "Legacy backfill plan for old group `<id>`".
  - `price`: reuse an existing `subscription_plans.price` for the old group if present; otherwise `0.00`.
  - `original_price`: preserve if an existing plan exists; otherwise `NULL`.
  - `validity_days`: old `groups.default_validity_days`.
  - `validity_unit`: `day`.
  - `access_scope`: `explicit`.
  - `daily_limit_usd`, `weekly_limit_usd`, `monthly_limit_usd`: copy from old group.
  - `overage_policy`: `block` unless a separate approved mapping says otherwise.
  - `for_sale`: `false` for backfill-created plans unless an admin explicitly enables sale later.
  - `group_id`: runtime group id for compatibility anchor, not the old plan-template group id.

Each plan gets exactly one enabled `subscription_plan_groups` grant to the runtime group for the initial migration. If later a package should cover multiple runtime groups, that must be an explicit admin/product decision.

### Legacy Groups To Runtime Groups

Create runtime groups by platform/ability/account topology/rate multiplier, not by legacy package price tier.

Recommended grouping key:

```text
runtime_group_key =
  normalized_platform_family + ":" +
  rate_multiplier + ":" +
  account_pool_fingerprint + ":" +
  routing_capability_fingerprint
```

Examples:

- `openai_codex:2.2:<same-account-pool>:<same-routing>` for wenrugou.
- `openai_codex:1.5:<same-account-pool>:<same-routing>` for pipixia.
- `anthropic_kiro_claude:1.0:<same-account-pool-or-manual>:<same-routing>` only if those requests are actually routed through Sub2API accounts.

Do not merge groups with different user-visible `rate_multiplier`. If a platform cluster has outlier rates, create a separate runtime group or put it in manual review.

Runtime group fields:

- `platform`: platform family that matches routing implementation.
- `rate_multiplier`: preserved from the old group cluster. If values differ, split runtime groups.
- `subscription_type`: `subscription` when `subscription_enabled=true` for legacy compatibility.
- `balance_enabled`: true if balance users should be able to use this runtime group.
- `subscription_enabled`: true for runtime groups usable by entitlements.
- `plan_auto_grant_enabled`: true only for active public runtime groups that may be included by future all/platform plans.
- `is_exclusive`: false for public unified runtime groups. Exclusive runtime groups must be created only for explicit private access and must not be auto-granted.
- `status`: `active`.

Legacy plan-template groups:

- Keep `status='active'` and `subscription_type='subscription'` during the rollback window so old `user_subscriptions` still work if API keys are restored.
- They may be `is_exclusive=true`; this is acceptable for reading old package
  configuration and account-pool fingerprints.
- Set capabilities to hide them from v2 source selection:
  - `balance_enabled=false`
  - `subscription_enabled=false`
  - `plan_auto_grant_enabled=false`
- Do not delete them.
- Do not make the old exclusive group public. Create or reuse a separate
  unified runtime group for v2 traffic.

### Account Groups

Runtime `account_groups` should be copied from the old groups in the same runtime cluster.

Rules:

- Copy only non-deleted accounts.
- Preserve account priority when all old groups agree.
- If the same account has conflicting priorities across old groups, dry-run must flag the runtime group as `account_priority_conflict`.
- If a legacy group has active subscriptions or active API keys but no schedulable accounts, dry-run must flag it as `no_schedulable_accounts`.
- Anthropic/Kiro/Claude groups with no `account_groups` require manual confirmation before becoming runtime groups.

## B. Legacy Subscription Migration

For every active legacy subscription:

```sql
user_subscriptions.deleted_at IS NULL
AND user_subscriptions.status = 'active'
AND user_subscriptions.starts_at <= NOW()
AND user_subscriptions.expires_at > NOW()
```

Create one `subscription_entitlements` row:

- `legacy_subscription_id = user_subscriptions.id`
- `user_id = user_subscriptions.user_id`
- `plan_id = subscription_legacy_backfill_mappings.plan_id`
- `primary_group_id = subscription_legacy_backfill_mappings.runtime_group_id`
- `name = plan.name`
- `source_type = 'legacy_subscription_backfill'`
- `source_id = user_subscriptions.id`
- `status = user_subscriptions.status`
- `starts_at = user_subscriptions.starts_at`
- `expires_at = user_subscriptions.expires_at`
- `daily_window_start = user_subscriptions.daily_window_start`
- `weekly_window_start = user_subscriptions.weekly_window_start`
- `monthly_window_start = user_subscriptions.monthly_window_start`
- `daily_usage_usd = user_subscriptions.daily_usage_usd`
- `weekly_usage_usd = user_subscriptions.weekly_usage_usd`
- `monthly_usage_usd = user_subscriptions.monthly_usage_usd`
- `daily_limit_usd = old group daily_limit_usd`
- `weekly_limit_usd = old group weekly_limit_usd`
- `monthly_limit_usd = old group monthly_limit_usd`
- `overage_policy = 'block'`
- `assigned_by = user_subscriptions.assigned_by`
- `assigned_at = user_subscriptions.assigned_at`
- `notes`: short backfill audit note, no sensitive source external ids
- `plan_snapshot`: JSON snapshot of old group id/name, runtime group id/name, limits, validity days, and mapping version

Create one enabled `subscription_entitlement_groups` row:

- `entitlement_id = subscription_entitlements.id`
- `group_id = runtime_group_id`
- `sort_order = 0`
- `enabled = true`

Create one fulfillment row:

- `entitlement_id = subscription_entitlements.id`
- `user_id = user_subscriptions.user_id`
- `plan_id = mapping.plan_id`
- `source_type = 'legacy_subscription_backfill'`
- `source_id = user_subscriptions.id`
- `validity_days = CEIL((expires_at - starts_at) / 86400)`
- `starts_at/expires_at` from the legacy subscription
- `assigned_by/assigned_at` from the legacy subscription
- `notes = 'legacy subscription backfill'`

All three writes must be in the same DB transaction.

## C. API Key Migration

Before writes, snapshot every affected API key:

```sql
api_key_id, user_id, old_group_id, old_access_source,
old_subscription_entitlement_id, old_updated_at
```

Suggested durable table:

```sql
CREATE TABLE api_key_legacy_backfill_snapshots (
    api_key_id BIGINT PRIMARY KEY REFERENCES api_keys(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL,
    old_group_id BIGINT,
    old_access_source VARCHAR(32),
    old_subscription_entitlement_id BIGINT,
    old_updated_at TIMESTAMPTZ,
    mapping_version VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Migration rules:

1. If `api_keys.group_id` is an old subscription group and exactly one active legacy subscription exists for the same `user_id + old_group_id`:
   - `access_source='entitlement'`
   - `subscription_entitlement_id = entitlement for that legacy_subscription_id`
   - `group_id = runtime_group_id`
2. If `api_keys.group_id` is an eligible balance runtime group:
   - `access_source='balance'`
   - `subscription_entitlement_id=NULL`
   - `group_id=runtime_group_id`
3. If a key cannot be uniquely mapped, do not update it. Put it in the ambiguous list.

Ambiguous reasons:

- no old group mapping
- no active legacy subscription for key user and group
- more than one active legacy subscription candidate
- old group is exclusive but is being proposed as a public balance/runtime group
  instead of a plan-template source
- old group is test/negative/deprecated
- old group has no runtime group
- old group has no schedulable account pool and no explicit product decision
- balance group is exclusive and user is not allowed
- current key is deleted/disabled
- existing `subscription_entitlement_id` conflicts with expected entitlement

## D. Required Dry-Run Output

Dry-run must output, without writing data:

- runtime groups that would be created
- plans that would be created or reused
- active subscriptions that would be backfilled
- expired/deleted subscriptions skipped
- API keys that would migrate automatically
- ambiguous API keys by reason
- account binding counts per runtime group
- account priority conflicts
- old exclusive subscription groups used as plan-template sources
- exclusive/private/test groups that would be excluded from public runtime or
  balance access
- old group inventory: active subscriptions, active API keys, limits, rate, platform, account count
- entitlement-vs-subscription reconciliation expectation
- post-write reconciliation SQL for `expires_at`, usage, windows, limits, and legacy id

The dry-run must never print emails, API key secrets, tokens, provider credentials, payment source ids, source external ids, or notes.

## E. Idempotency

Dry-run:

- can run before writes and after writes
- uses only `SELECT`
- stores no state

Write path:

- `subscription_entitlements.legacy_subscription_id` prevents duplicate entitlement creation.
- `subscription_entitlements(source_type, source_id)` with `source_type='legacy_subscription_backfill'` and `source_id=user_subscriptions.id` prevents duplicate source rows.
- `subscription_entitlement_fulfillments(source_type, source_id)` prevents duplicate fulfillment history.
- mapping table `legacy_group_id` prevents repeated plan/runtime remapping.
- API key snapshot table prevents repeated destructive rewrites and gives rollback input.
- API key update must be conditional:
  - only update if current `group_id/access_source/subscription_entitlement_id` still match the snapshot or expected pre-migration state
  - never override a user/admin change made after the snapshot

## F. Rollback

Primary rollback:

1. Set `subscription_entitlements_v2_enabled=false`.
2. Set `sub2_payment_page_legacy_mapping_enabled=false`.
3. Keep additive schema, entitlement rows, fulfillment history, plans, and runtime groups.
4. Restore API keys from `api_key_legacy_backfill_snapshots` if migrated keys need to run on legacy subscription groups again.
5. Leave `user_subscriptions` untouched and usable.

Do not:

- delete entitlement history
- delete plans created for backfill
- reuse entitlement ids as legacy subscription ids
- run destructive schema rollback

## G. Forbidden Actions

- Do not write production data during dry-run.
- Do not enable production flags during backfill preparation.
- Do not build Docker on production business hosts.
- Do not modify `sub2-payment-page`.
- Do not put entitlement ids into legacy `subscription_id`.
- Do not duplicate `user_subscriptions`.
- Do not merge OpenAI/Codex and Anthropic/Kiro/Claude into one runtime group.
- Do not auto-open exclusive/private/test groups.
- Do not print or commit secrets, evidence, DB dumps, tokens, API keys, provider credentials, or source external ids.

## H. Follow-Up Implementation Plan

### Task 19-B: Implement Backfill Dry-Run Command Or Script

Files likely involved:

- Create `tools/subscription_entitlements_v2_legacy_backfill_dry_run.ps1` or a Go admin command.
- Reuse SQL from `docs/plans/subscription-entitlements-v2-legacy-backfill-dry-run.sql`.
- Add parameters for DB connection through environment variables only.
- Default to dry-run and refuse to execute writes.
- Output redacted CSV/JSON evidence.

Tests:

- script help/dry-run output
- no secrets printed
- production URL/password absent from repo

### Task 19-C: Staging Dry-Run And Synthetic Write Rehearsal

- Run dry-run against staging synthetic data.
- Create runtime groups/plans/mappings in staging only.
- Execute write rehearsal on synthetic users.
- Validate entitlement reconciliation, API key migration, rollback snapshot restore.

### Task 19-D: Implement Real Backfill Write Path, Default Dry-Run

- Add mapping/snapshot tables through additive migration.
- Add write command guarded by `--execute` and environment confirmation.
- Transactionally create plan/mapping/runtime group/entitlement/grant/fulfillment/key updates.
- Add integration tests for idempotency and rollback restore.

### Task 19-E: Production Read-Only Dry-Run Evidence

- Deploy V2 schema with flags false.
- Run production dry-run only.
- Save redacted evidence outside the repo.
- Resolve every ambiguous API key before write execution.

### Task 19-F: Production Execution Window And Rollback Rehearsal

- Take API key migration snapshot.
- Execute backfill in a maintenance window.
- Validate reconciliation.
- Enable v2 only for a small cohort.
- Practice flag rollback and API key snapshot restore.

## Go/No-Go Rule For Production Backfill

Do not run the production write path unless all are true:

- production V2 schema is deployed
- both flags are false
- dry-run shows zero unresolved automatic-migration blockers
- every ambiguous API key has an approved handling decision
- runtime group mapping has been reviewed by platform/rate/account-pool
- API key snapshot table is populated
- rollback restore SQL has been rehearsed in staging
- on-call knows the flag rollback and key snapshot restore commands
