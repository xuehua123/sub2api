# Subscription Entitlements V2 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace "one user subscription per group" with "one shared package entitlement that can authorize multiple groups while keeping one quota and reset window."

**Architecture:** Keep wallet balance as cash balance. Introduce a subscription entitlement layer between plans and groups: plans define quota and access scope, entitlements snapshot that quota/scope at purchase time, API keys bind to one entitlement plus one current group, and billing increments the entitlement usage regardless of which authorized group handled the request.

**Tech Stack:** Go 1.26.1, Gin, Ent, PostgreSQL/SQLite migrations, Google Wire, Vue 3, Vite, TypeScript, Pinia, pnpm.

---

## 1. Product Decision

This is not a balance rename and not a "monthly balance" hack.

Final product language:

- `账户余额`: cash wallet, pay-as-you-go, refundable/rechargeable, does not auto-reset.
- `套餐额度`: package quota, reset by entitlement windows, may expire.
- `套餐通行证`: package entitlement, grants access to one or more groups.
- `当前分组`: API key routing group currently used for upstream/model/channel behavior.

Target user story:

1. User buys "Pro 月卡".
2. The purchase creates one entitlement with one quota ledger and one reset schedule.
3. The entitlement authorizes multiple subscription groups.
4. The user's API key can switch among authorized groups.
5. Every request deducts the same entitlement usage ledger.
6. When entitlement quota is exhausted, group switching does not create more quota.
7. Optional policy: block, or fall back to wallet balance.

Non-goals:

- Do not merge wallet balance and package quota.
- Do not make group limits the source of truth for new packages.
- Do not drop legacy `user_subscriptions` in the first release.
- Do not rebuild Docker images on production hosts.

---

## 2. Key Invariants

The implementation must preserve these invariants:

1. A package entitlement owns quota and reset windows.
2. A group owns routing, model capability, platform, rate multiplier, and upstream selection.
3. Billing cost is computed using the actual current group/model/channel, then applied to the selected entitlement.
4. A user may bind or switch an API key to a subscription group only when an active entitlement covers that group.
5. An API key with `subscription_entitlement_id` may only use groups covered by that entitlement.
6. If an entitlement covers many groups, those groups share the same daily/weekly/monthly usage counters.
7. Existing subscriptions migrate to single-group entitlements with identical behavior.
8. Existing balance behavior remains unchanged.
9. `sub2-payment-page` is treated as an unmodifiable external cashier: it keeps calling the old `group_id + validity_days` fulfillment contract, and all upgrades happen inside the main service.
10. Old external inputs may interpret `group_id` as a legacy package anchor, then map it to a v2 multi-group entitlement; if no mapping matches, the main service must fall back to old single-group behavior.
11. All new service code respects depguard layering: `service` must not import repository, GORM, Redis, or direct DB packages except existing allowed exceptions.

---

## 3. Target Data Model

### 3.1 Keep Existing Tables For Compatibility

Keep these tables and columns for at least one release:

- `subscription_plans.group_id`
- `user_subscriptions`
- `usage_logs.subscription_id`
- `payment_orders.subscription_group_id`
- `payment_orders.subscription_days`

They become compatibility fields. Do not delete them in this rollout.

### 3.2 Extend `subscription_plans`

Add package-level quota and access policy fields:

```sql
ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS access_scope VARCHAR(32) NOT NULL DEFAULT 'explicit',
    ADD COLUMN IF NOT EXISTS allowed_platforms JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS daily_limit_usd DECIMAL(20, 8),
    ADD COLUMN IF NOT EXISTS weekly_limit_usd DECIMAL(20, 8),
    ADD COLUMN IF NOT EXISTS monthly_limit_usd DECIMAL(20, 8),
    ADD COLUMN IF NOT EXISTS overage_policy VARCHAR(32) NOT NULL DEFAULT 'block';
```

`access_scope` values:

- `explicit`: use `subscription_plan_groups`.
- `all_subscription_groups`: all active groups where `subscription_type = 'subscription'`.
- `platform_subscription_groups`: all active subscription groups whose platform is in `allowed_platforms`.

`overage_policy` values:

- `block`: return quota exhausted.
- `balance_fallback`: after package quota exhaustion, charge wallet balance.

For backward compatibility, every legacy plan should get `access_scope = 'explicit'` and one row in `subscription_plan_groups` for its current `group_id`.

### 3.3 Create `subscription_plan_groups`

```sql
CREATE TABLE IF NOT EXISTS subscription_plan_groups (
    plan_id    BIGINT NOT NULL REFERENCES subscription_plans(id) ON DELETE CASCADE,
    group_id   BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (plan_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_subscription_plan_groups_group_enabled
    ON subscription_plan_groups(group_id, enabled);
```

### 3.3.1 Create `subscription_plan_external_mappings`

This table exists because `sub2-payment-page` cannot be changed. The external cashier still sends the legacy payload:

```json
{
  "type": "subscription",
  "value": 29.90,
  "group_id": 5,
  "validity_days": 30,
  "user_id": 123
}
```

The main service maps `source + legacy_group_id + legacy_validity_days + legacy_value` to a real v2 plan. Do not map by `group_id + validity_days` alone; two legacy packages may share the same group and term but differ in price and entitlement scope.

```sql
CREATE TABLE IF NOT EXISTS subscription_plan_external_mappings (
    id                    BIGSERIAL PRIMARY KEY,
    source                VARCHAR(64) NOT NULL DEFAULT 'sub2-payment-page',
    legacy_group_id        BIGINT NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
    legacy_validity_days   INTEGER NOT NULL,
    legacy_value           DECIMAL(20, 8) NOT NULL,
    plan_id               BIGINT NOT NULL REFERENCES subscription_plans(id) ON DELETE CASCADE,
    enabled               BOOLEAN NOT NULL DEFAULT TRUE,
    priority              INTEGER NOT NULL DEFAULT 0,
    notes                 TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at            TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_plan_external_mappings_unique
    ON subscription_plan_external_mappings(source, legacy_group_id, legacy_validity_days, legacy_value)
    WHERE deleted_at IS NULL;
```

Matching rules:

- `source` is derived by the main service, never trusted from request JSON.
- `legacy_value` is normalized from request `value`; compare canonical money values, not fuzzy tolerances.
- If the mapping misses, use legacy single-group behavior or reject according to the caller path; never guess a plan.

### 3.3.2 Extend `redeem_codes`

Plan-based redeem codes require a nullable `plan_id`.

```sql
ALTER TABLE redeem_codes
    ADD COLUMN IF NOT EXISTS plan_id BIGINT REFERENCES subscription_plans(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_redeem_codes_plan_id
    ON redeem_codes(plan_id);
```

Compatibility:

- `sub2-payment-page` still sends only `group_id`; the main service resolves `plan_id` internally through the external mapping.
- Normal admin-created subscription redeem codes must use `plan_id XOR group_id`.

### 3.4 Create `subscription_entitlements`

This is the new source of truth for package quota.

```sql
CREATE TABLE IF NOT EXISTS subscription_entitlements (
    id                         BIGSERIAL PRIMARY KEY,
    user_id                    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_id                    BIGINT REFERENCES subscription_plans(id) ON DELETE SET NULL,
    legacy_subscription_id      BIGINT UNIQUE REFERENCES user_subscriptions(id) ON DELETE SET NULL,
    primary_group_id            BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    name                       VARCHAR(120) NOT NULL DEFAULT '',
    source_type                VARCHAR(32) NOT NULL DEFAULT 'unknown',
    status                     VARCHAR(20) NOT NULL DEFAULT 'active',
    starts_at                  TIMESTAMPTZ NOT NULL,
    expires_at                 TIMESTAMPTZ NOT NULL,
    daily_window_start          TIMESTAMPTZ,
    weekly_window_start         TIMESTAMPTZ,
    monthly_window_start        TIMESTAMPTZ,
    daily_limit_usd             DECIMAL(20, 8),
    weekly_limit_usd            DECIMAL(20, 8),
    monthly_limit_usd           DECIMAL(20, 8),
    daily_usage_usd             DECIMAL(20, 10) NOT NULL DEFAULT 0,
    weekly_usage_usd            DECIMAL(20, 10) NOT NULL DEFAULT 0,
    monthly_usage_usd           DECIMAL(20, 10) NOT NULL DEFAULT 0,
    overage_policy              VARCHAR(32) NOT NULL DEFAULT 'block',
    plan_snapshot               JSONB NOT NULL DEFAULT '{}'::jsonb,
    source_id                   BIGINT,
    source_external_id          VARCHAR(128),
    source_redeem_code_id        BIGINT REFERENCES redeem_codes(id) ON DELETE SET NULL,
    assigned_by                 BIGINT REFERENCES users(id) ON DELETE SET NULL,
    assigned_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes                       TEXT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at                  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_subscription_entitlements_user_status_expires
    ON subscription_entitlements(user_id, status, expires_at)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_subscription_entitlements_plan_id
    ON subscription_entitlements(plan_id)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlements_source_redeem_unique
    ON subscription_entitlements(source_redeem_code_id)
    WHERE source_redeem_code_id IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlements_source_id_unique
    ON subscription_entitlements(source_type, source_id)
    WHERE source_id IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlements_source_external_unique
    ON subscription_entitlements(source_type, source_external_id)
    WHERE source_external_id IS NOT NULL AND deleted_at IS NULL;
```

Source fields:

- `source_id`: internal source ID such as a payment order ID.
- `source_external_id`: external order ID such as legacy cashier `out_trade_no`.
- `source_redeem_code_id`: redeem code ID; required for redeem-code-based entitlement grants to prevent duplicate fulfillment.

### 3.5 Create `subscription_entitlement_fulfillments`

The `subscription_entitlements.source_*` fields can only describe the current or most recent source for one entitlement. After a renewal, those single-row pointers may be overwritten by the new source. If an old payment callback, external order ID, or redeem code is replayed later, checking only the current entitlement row is not enough to know whether that older source was already fulfilled, so it can accidentally extend the entitlement again.

Keep a history row for every grant or renewal event:

```sql
CREATE TABLE IF NOT EXISTS subscription_entitlement_fulfillments (
    id                    BIGSERIAL PRIMARY KEY,
    entitlement_id        BIGINT NOT NULL REFERENCES subscription_entitlements(id) ON DELETE CASCADE,
    user_id               BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_id               BIGINT REFERENCES subscription_plans(id) ON DELETE SET NULL,
    source_type           VARCHAR(32) NOT NULL DEFAULT 'unknown',
    source_id             BIGINT,
    source_external_id    VARCHAR(128),
    source_redeem_code_id BIGINT REFERENCES redeem_codes(id) ON DELETE SET NULL,
    validity_days         INTEGER NOT NULL DEFAULT 0,
    starts_at             TIMESTAMPTZ NOT NULL,
    expires_at            TIMESTAMPTZ NOT NULL,
    assigned_by           BIGINT REFERENCES users(id) ON DELETE SET NULL,
    assigned_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes                 TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscription_entitlement_fulfillments_entitlement
    ON subscription_entitlement_fulfillments(entitlement_id);

CREATE INDEX IF NOT EXISTS idx_subscription_entitlement_fulfillments_user_plan
    ON subscription_entitlement_fulfillments(user_id, plan_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlement_fulfillments_source_redeem_unique
    ON subscription_entitlement_fulfillments(source_redeem_code_id)
    WHERE source_redeem_code_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlement_fulfillments_source_id_unique
    ON subscription_entitlement_fulfillments(source_type, source_id)
    WHERE source_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_entitlement_fulfillments_source_external_unique
    ON subscription_entitlement_fulfillments(source_type, source_external_id)
    WHERE source_external_id IS NOT NULL;
```

Rules:

- Creating or extending an entitlement must write the fulfillment event in the same database transaction; if either step fails, both must roll back.
- Idempotency checks must query `subscription_entitlement_fulfillments` first. A hit returns the existing result and must not extend the entitlement again.
- Future payment/redeem integration must prefer fulfillment history before falling back to legacy entitlement `source_*` fields.
- Entitlement `source_*` fields are compatibility and latest-source snapshots, not the complete idempotency history.

### 3.6 Create `subscription_entitlement_groups`

```sql
CREATE TABLE IF NOT EXISTS subscription_entitlement_groups (
    entitlement_id BIGINT NOT NULL REFERENCES subscription_entitlements(id) ON DELETE CASCADE,
    group_id       BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    sort_order     INTEGER NOT NULL DEFAULT 0,
    enabled        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (entitlement_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_subscription_entitlement_groups_group_enabled
    ON subscription_entitlement_groups(group_id, enabled);
```

### 3.7 Add Foreign Keys To Hot Tables

```sql
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS subscription_entitlement_id BIGINT REFERENCES subscription_entitlements(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_api_keys_subscription_entitlement_id
    ON api_keys(subscription_entitlement_id);

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS entitlement_id BIGINT REFERENCES subscription_entitlements(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_usage_logs_entitlement_id
    ON usage_logs(entitlement_id);

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_entitlement_id BIGINT REFERENCES subscription_entitlements(id) ON DELETE SET NULL;
```

Do not remove `group_id`. API keys still need `group_id` as current routing group.

---

## 4. Migration Strategy

Migration should be idempotent and safe to run on existing environments.

Use next available migration number, likely:

- Create: `backend/migrations/150_subscription_entitlements_v2.sql`

Backfill steps:

1. Insert legacy plan groups:

```sql
INSERT INTO subscription_plan_groups (plan_id, group_id, sort_order, enabled, created_at, updated_at)
SELECT id, group_id, 0, TRUE, NOW(), NOW()
FROM subscription_plans
WHERE group_id IS NOT NULL
ON CONFLICT (plan_id, group_id) DO NOTHING;
```

2. Copy legacy group quotas into plans only when plan quota is unset:

```sql
UPDATE subscription_plans sp
SET
    daily_limit_usd = COALESCE(sp.daily_limit_usd, g.daily_limit_usd),
    weekly_limit_usd = COALESCE(sp.weekly_limit_usd, g.weekly_limit_usd),
    monthly_limit_usd = COALESCE(sp.monthly_limit_usd, g.monthly_limit_usd),
    access_scope = COALESCE(NULLIF(sp.access_scope, ''), 'explicit')
FROM groups g
WHERE sp.group_id = g.id;
```

3. Backfill one entitlement per legacy `user_subscriptions` row:

```sql
INSERT INTO subscription_entitlements (
    user_id, legacy_subscription_id, primary_group_id, name, source_type,
    status, starts_at, expires_at,
    daily_window_start, weekly_window_start, monthly_window_start,
    daily_limit_usd, weekly_limit_usd, monthly_limit_usd,
    daily_usage_usd, weekly_usage_usd, monthly_usage_usd,
    overage_policy, notes, assigned_by, assigned_at, created_at, updated_at
)
SELECT
    us.user_id, us.id, us.group_id, COALESCE(g.name, 'Legacy Subscription'), 'legacy_migration',
    us.status, us.starts_at, us.expires_at,
    us.daily_window_start, us.weekly_window_start, us.monthly_window_start,
    g.daily_limit_usd, g.weekly_limit_usd, g.monthly_limit_usd,
    us.daily_usage_usd, us.weekly_usage_usd, us.monthly_usage_usd,
    'block', us.notes, us.assigned_by, us.assigned_at, us.created_at, us.updated_at
FROM user_subscriptions us
JOIN groups g ON g.id = us.group_id
WHERE us.deleted_at IS NULL
ON CONFLICT (legacy_subscription_id) DO NOTHING;
```

4. Backfill entitlement groups:

```sql
INSERT INTO subscription_entitlement_groups (entitlement_id, group_id, sort_order, enabled, created_at, updated_at)
SELECT se.id, se.primary_group_id, 0, TRUE, NOW(), NOW()
FROM subscription_entitlements se
WHERE se.primary_group_id IS NOT NULL
ON CONFLICT (entitlement_id, group_id) DO NOTHING;
```

5. Backfill API keys:

```sql
UPDATE api_keys ak
SET subscription_entitlement_id = se.id
FROM subscription_entitlements se
WHERE ak.user_id = se.user_id
  AND ak.group_id = se.primary_group_id
  AND ak.subscription_entitlement_id IS NULL
  AND se.deleted_at IS NULL;
```

6. Backfill usage logs:

```sql
UPDATE usage_logs ul
SET entitlement_id = se.id
FROM subscription_entitlements se
WHERE ul.subscription_id = se.legacy_subscription_id
  AND ul.entitlement_id IS NULL;
```

Rollback posture:

- New columns/tables are additive.
- Old `user_subscriptions` remains untouched.
- If rollout fails, disable v2 via setting and continue legacy reads.

---

## 5. Backend Design

### 5.1 New Domain Models

Create:

- `backend/internal/service/subscription_entitlement.go`
- `backend/internal/service/subscription_entitlement_port.go`
- `backend/internal/repository/subscription_entitlement_repo.go`

Core service structs:

```go
type SubscriptionEntitlement struct {
    ID int64
    UserID int64
    PlanID *int64
    LegacySubscriptionID *int64
    PrimaryGroupID *int64
    Name string
    SourceType string
    Status string
    StartsAt time.Time
    ExpiresAt time.Time
    DailyWindowStart *time.Time
    WeeklyWindowStart *time.Time
    MonthlyWindowStart *time.Time
    DailyLimitUSD *float64
    WeeklyLimitUSD *float64
    MonthlyLimitUSD *float64
    DailyUsageUSD float64
    WeeklyUsageUSD float64
    MonthlyUsageUSD float64
    OveragePolicy string
    PlanSnapshot map[string]any
    SourceID *int64
    SourceExternalID *string
    SourceRedeemCodeID *int64
    Groups []Group
}

type EntitlementResolution struct {
    Entitlement *SubscriptionEntitlement
    Group *Group
    FromGroupID int64
    ToGroupID int64
    Switched bool
    Reason string
    UseBalanceFallback bool
}
```

Repository port:

```go
type SubscriptionEntitlementRepository interface {
    Create(ctx context.Context, ent *SubscriptionEntitlement, groupIDs []int64) error
    CreateTx(ctx context.Context, ent *SubscriptionEntitlement, groupIDs []int64) error
    GetByID(ctx context.Context, id int64) (*SubscriptionEntitlement, error)
    GetBySourceID(ctx context.Context, sourceType string, sourceID int64) (*SubscriptionEntitlement, error)
    GetBySourceExternalID(ctx context.Context, sourceType, sourceExternalID string) (*SubscriptionEntitlement, error)
    GetBySourceRedeemCodeID(ctx context.Context, redeemCodeID int64) (*SubscriptionEntitlement, error)
    GetActiveCoveringGroup(ctx context.Context, userID, groupID int64) ([]SubscriptionEntitlement, error)
    ListActiveByUserID(ctx context.Context, userID int64) ([]SubscriptionEntitlement, error)
    ListActiveCoveringGroupForUser(ctx context.Context, userID, groupID int64) ([]SubscriptionEntitlement, error)
    UpdateTerm(ctx context.Context, id int64, startsAt, expiresAt time.Time, status, notes string) error
    ResetUsage(ctx context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, windowStart time.Time) error
    ApplyEntitlementUsage(ctx context.Context, id int64, costUSD float64, now time.Time) (*EntitlementUsageApplyResult, error)
    ReplaceGroups(ctx context.Context, id int64, groupIDs []int64) error
}

type EntitlementUsageApplyResult struct {
    UpdatedAt time.Time
    DailyUsageUSD float64
    WeeklyUsageUSD float64
    MonthlyUsageUSD float64
    DailyWindowStart *time.Time
    WeeklyWindowStart *time.Time
    MonthlyWindowStart *time.Time
}

type SubscriptionPlanExternalMappingRepository interface {
    FindEnabled(ctx context.Context, source string, legacyGroupID int64, legacyValidityDays int, legacyValue float64) (*SubscriptionPlanExternalMapping, error)
}
```

### 5.2 Ent Schemas

Create:

- `backend/ent/schema/subscription_plan_group.go`
- `backend/ent/schema/subscription_entitlement.go`
- `backend/ent/schema/subscription_entitlement_group.go`

Modify:

- `backend/ent/schema/subscription_plan.go`
- `backend/ent/schema/api_key.go`
- `backend/ent/schema/usage_log.go`
- `backend/ent/schema/payment_order.go`

After schema changes:

```bash
cd backend
go generate ./ent
go generate ./cmd/server
```

### 5.3 Service Layer

Add:

- `backend/internal/service/subscription_entitlement_service.go`
- `backend/internal/service/subscription_entitlement_resolver.go`
- `backend/internal/service/subscription_entitlement_progress.go`
- `backend/internal/service/subscription_entitlement_maintenance.go`

Keep `SubscriptionService` as a compatibility facade initially:

- Legacy public methods may delegate to entitlement service when v2 is enabled.
- Old methods such as `GetActiveSubscription(userID, groupID)` should resolve the active entitlement covering group and return a compatibility `UserSubscription` when old callers still require it.

Decision rule when multiple active entitlements cover a group:

1. If API key has `subscription_entitlement_id`, use that entitlement only.
2. Else prefer entitlement with earliest `expires_at`.
3. Then prefer entitlement with lowest group sort order in `subscription_entitlement_groups`.
4. Then lowest entitlement id.

### 5.4 API Key Binding

Modify:

- `backend/internal/service/api_key_service.go`
- `backend/internal/handler/api_key_handler.go`
- `backend/internal/repository/api_key_repo.go`
- `backend/ent/schema/api_key.go`

Request changes:

```go
type CreateAPIKeyRequest struct {
    GroupID *int64 `json:"group_id"`
    SubscriptionEntitlementID *int64 `json:"subscription_entitlement_id"`
}

type UpdateAPIKeyRequest struct {
    GroupID *int64 `json:"group_id"`
    SubscriptionEntitlementID *int64 `json:"subscription_entitlement_id"`
}
```

Validation:

- Standard group: current `AllowedGroups` / `IsExclusive` behavior.
- Subscription group with explicit entitlement id: entitlement must be active, belong to user, and cover group.
- Subscription group without explicit entitlement id: select default entitlement using resolver rule.
- Switching `group_id` must preserve or re-resolve `subscription_entitlement_id`.

User available groups endpoint:

- `GET /api/v1/groups/available` should return groups covered by active entitlements.
- Add entitlement context for UI:

```json
{
  "id": 12,
  "name": "OpenAI Fast",
  "subscription_type": "subscription",
  "entitlements": [
    { "id": 44, "name": "Pro Monthly", "expires_at": "..." }
  ]
}
```

### 5.5 Middleware Resolution

Modify:

- `backend/internal/server/middleware/api_key_auth.go`
- `backend/internal/server/middleware/api_key_auth_google.go`

New flow:

1. Validate API key status/quota/expiry as today.
2. Load current `apiKey.Group`.
3. If group is standard, use balance flow as today.
4. If group is subscription:
   - Resolve entitlement by `apiKey.SubscriptionEntitlementID`.
   - Verify entitlement covers current group.
   - Run endpoint eligibility against current group.
   - Validate shared entitlement quota.
   - Set context keys for entitlement, current group, and compatibility subscription id if present.
5. If current group is not eligible and auto-switch is enabled:
   - Switch only within groups covered by the same entitlement.
   - If entitlement quota is exhausted, do not switch groups for quota reasons.
   - If overage policy is `balance_fallback`, set `UseBalanceFallback`.

Add a new context key:

- `ContextKeySubscriptionEntitlement`

Keep `ContextKeySubscription` temporarily for code paths that still expect it.

### 5.6 Billing

Modify:

- `backend/internal/service/usage_billing.go`
- `backend/internal/repository/usage_billing_repo.go`
- gateway usage recording files that construct `UsageBillingCommand`

Extend command:

```go
type UsageBillingCommand struct {
    SubscriptionID *int64
    EntitlementID *int64
    SubscriptionCost float64
}
```

Apply rule:

- If `EntitlementID != nil`, increment `subscription_entitlements`.
- Else if `SubscriptionID != nil`, use legacy `user_subscriptions`.
- Store returned entitlement version for cache invalidation.

v2 atomicity:

- Do not implement entitlement usage as "read remaining quota, then increment later".
- Repository must provide one transactional method such as `ApplyEntitlementUsage(ctx, entitlementID, cost, now)`.
- That method locks the entitlement row, resets expired windows if needed, checks status/expiry/deleted state, checks daily/weekly/monthly limits, increments usage, and returns the new version in one transaction.
- Concurrent requests against the same entitlement must allow only the transactions that truly fit within the remaining quota.
- `balance_fallback` must be represented in the same `UsageBillingCommand`; do not swallow an entitlement limit error in handlers and start a separate balance deduction.

`usage_logs` creation:

- Set `entitlement_id` for v2 billing.
- Keep `subscription_id` if the entitlement came from migrated legacy subscription.
- Continue setting `group_id` to actual current group.

### 5.7 Billing Cache

Modify:

- `backend/internal/service/billing_cache_service.go`
- any cache key helpers under `backend/internal/service/*billing*`

Required cache shapes:

- Entitlement status by `entitlement_id`.
- Entitlement coverage by `user_id + group_id` for fallback/default resolution.
- Active entitlements by `user_id`.

Cache invalidation:

- Invalidate entitlement by id after usage increment.
- Invalidate user active entitlements after purchase, refund, admin assign, revoke, group replacement, or expiry maintenance.
- Keep old invalidation methods as wrappers during compatibility.

### 5.8 Payment Fulfillment

Modify:

- `backend/internal/service/payment_order.go`
- `backend/internal/service/payment_fulfillment.go`
- `backend/internal/service/payment_refund.go`
- `backend/internal/service/payment_config_plans.go`
- `backend/internal/handler/admin/payment_handler.go`

New order creation:

- `plan_id` remains required for subscription order.
- `PaymentService.validateSubOrder` must stop requiring `plan.GroupID` to point to a single subscription group. It should validate that the plan is for sale and resolves to at least one active authorized subscription group.
- Snapshot:
  - plan name
  - validity days/unit
  - quota limits
  - overage policy
  - access scope
  - explicit groups resolved at purchase time

Payment order should store `plan_id` as today and after fulfillment set `subscription_entitlement_id`.

Compatibility fields:

- Keep `payment_orders.subscription_group_id` in v1.
- For v2 orders, it may store `primary_group_id` or the first authorized group for old reports, emails, and refund fallback.
- It must not be the source of truth for fulfillment.
- `payment_fulfillment.go` must prefer `plan_id -> entitlementService.AssignOrExtendFromPlan`.
- Fall back to old `subscription_group_id + subscription_days` only when v2 is disabled, `plan_id` is absent, or legacy data requires it.

Fulfillment:

```go
entitlement, reused, err := entitlementService.AssignOrExtendFromPlan(ctx, AssignEntitlementFromPlanInput{
    UserID: o.UserID,
    PlanID: *o.PlanID,
    OrderID: o.ID,
    AssignedBy: 0,
})
```

Reuse rule:

- If user already has active entitlement for same plan and same group scope, extend it.
- If expired, renew and reset usage windows.
- If plan scope changed since previous purchase, preserve old entitlement scope unless this is a renewal from same order flow and product explicitly says "renew with latest plan scope".

Recommendation: purchase snapshots scope, renewal from same plan updates scope only if `plan_scope_update_policy = latest`.

Refund:

- Existing subscription refund should find by `subscription_entitlement_id` first.
- Legacy orders fall back to `subscription_group_id` and legacy subscription lookup.
- Proportional day deduction applies to entitlement term.
- If full refund makes entitlement invalid, revoke entitlement and clear API keys pointing to it or re-resolve them.

### 5.9 Redeem Codes And Default Grants

Modify:

- `backend/internal/service/redeem_service.go`
- `backend/internal/handler/admin/redeem_handler.go`
- `backend/ent/schema/redeem_code.go`
- `backend/internal/service/setting_service.go`
- `backend/internal/handler/admin/setting_handler.go`
- add external mapping service/repository

Compatibility:

- `sub2-payment-page` remains unchanged and keeps calling `/api/v1/admin/redeem-codes/create-and-redeem`.
- Its payload `{type:"subscription", value, group_id, validity_days}` is treated as an external fulfillment request only when all strict guards pass:
  - `sub2_payment_page_legacy_mapping_enabled = true`
  - `Idempotency-Key` format is `s2p_*`
  - `code` format is `auto_*`
  - the suffix after `s2p_` equals the suffix after `auto_`
  - `type == "subscription"`
  - `group_id` exists
  - `validity_days > 0`
  - `value > 0`
  - external mapping matches exactly by `source + group_id + validity_days + value`
- `notes` is audit-only and must not be used to identify the source.
- If the strict mapping misses, fall back to legacy single-group behavior or reject according to the caller path.

Transaction and idempotency:

- Marking the redeem code as used and granting entitlement must happen in the same `txCtx`.
- Entitlement service exposes a transaction-aware method such as `AssignOrExtendFromPlanTx(txCtx, input)`.
- Grants from redeem codes must write `source_redeem_code_id`.
- Legacy cashier grants also write `source_type='sub2-payment-page'` and `source_external_id=out_trade_no`.
- Unique source constraints prevent replay from extending twice.

New preferred input:

```json
{
  "type": "subscription",
  "plan_id": 123,
  "validity_days": 30
}
```

To support `plan_id`, update `CreateAndRedeemCodeRequest`, service `RedeemCode`, Ent schema, repository models, and DTO conversion. Subscription redeem validation becomes `plan_id XOR group_id` for normal admin-created codes; the old cashier still sends `group_id` and resolves `plan_id` through the mapping table.

Default signup grants:

- Keep existing `group_id + validity_days`.
- Add optional `plan_id`.
- If both are present, reject.
- New UI should prefer plan-based defaults.

### 5.10 Admin Subscription Management

Modify:

- `backend/internal/handler/admin/subscription_handler.go`
- `backend/internal/service/subscription_service.go`
- add entitlement admin service methods

New admin endpoints:

- `GET /api/v1/admin/entitlements`
- `GET /api/v1/admin/entitlements/:id`
- `POST /api/v1/admin/entitlements/assign`
- `POST /api/v1/admin/entitlements/:id/extend`
- `POST /api/v1/admin/entitlements/:id/reset-quota`
- `PUT /api/v1/admin/entitlements/:id/groups`
- `DELETE /api/v1/admin/entitlements/:id`

Keep old `/admin/subscriptions` routes as compatibility wrappers for one release.

---

## 6. Frontend Design

### 6.1 Types And API Clients

Modify:

- `frontend/src/types/payment.ts`
- `frontend/src/types/index.ts`
- `frontend/src/api/payment.ts`
- `frontend/src/api/admin/payment.ts`
- `frontend/src/api/subscriptions.ts`
- `frontend/src/api/keys.ts`
- create `frontend/src/api/admin/entitlements.ts`

New types:

```ts
export interface SubscriptionEntitlement {
  id: number
  name: string
  status: string
  starts_at: string
  expires_at: string
  daily_limit_usd?: number | null
  weekly_limit_usd?: number | null
  monthly_limit_usd?: number | null
  daily_usage_usd: number
  weekly_usage_usd: number
  monthly_usage_usd: number
  overage_policy: 'block' | 'balance_fallback'
  groups: Group[]
}

export interface SubscriptionPlan {
  id: number
  group_id: number
  access_scope: 'explicit' | 'all_subscription_groups' | 'platform_subscription_groups'
  group_ids: number[]
  daily_limit_usd?: number | null
  weekly_limit_usd?: number | null
  monthly_limit_usd?: number | null
  overage_policy: 'block' | 'balance_fallback'
}
```

### 6.2 Admin Plan Editor

Modify:

- `frontend/src/views/admin/orders/PlanEditDialog.vue`
- `frontend/src/views/admin/orders/AdminPaymentPlansView.vue`

UI:

- Keep plan name, price, validity.
- Add quota fields:
  - daily limit
  - weekly limit
  - monthly limit
- Add access scope segmented control:
  - selected groups
  - all subscription groups
  - all subscription groups by platform
- Add group multi-select for explicit scope.
- Add overage policy select:
  - block
  - wallet fallback

Validation:

- At least one quota limit or "unlimited package" confirmation.
- Explicit scope requires at least one group.
- Platform scope requires at least one platform.

### 6.3 User Purchase Page

Modify:

- `frontend/src/views/user/PaymentView.vue`
- `frontend/src/components/payment/SubscriptionPlanCard.vue`

Display:

- Plan quota: `$x/day`, `$y/month`, or unlimited.
- Access scope: "可用 8 个分组" or "全部 OpenAI 订阅分组".
- Existing active entitlement for plan with expiry and usage summary.

Purchase behavior:

- Still sends `order_type = subscription`, `plan_id`.
- No group selection is needed for multi-group plan purchase.

### 6.4 User Entitlements Page

Modify:

- `frontend/src/views/user/SubscriptionsView.vue`
- `frontend/src/stores/subscriptions.ts`

Rename user-facing page language to "套餐额度" while keeping route `/subscriptions` for compatibility.

Cards should show:

- entitlement name
- status
- expiry
- daily/weekly/monthly usage progress
- allowed groups list
- reset time
- overage policy

Switch priority UI changes:

- Current priority list should become "同一套餐内自动切组顺序".
- Preferences are stored by entitlement id and group id, not only by group id.

### 6.5 API Key Page

Modify:

- `frontend/src/views/user/KeysView.vue`
- `frontend/src/api/keys.ts`

Create/update API key UI:

- Select package entitlement first when multiple active entitlements exist.
- Then select current group from that entitlement's allowed groups.
- If only one entitlement covers selected group, auto-select entitlement.
- Show a warning when switching group changes upstream/platform but not package quota.

Payload:

```ts
{
  group_id: selectedGroupId,
  subscription_entitlement_id: selectedEntitlementId
}
```

---

## 7. API Compatibility Contract

Existing endpoints stay available:

- `GET /api/v1/subscriptions`
- `GET /api/v1/subscriptions/active`
- `GET /api/v1/subscriptions/progress`
- `POST /api/v1/subscriptions/:id/advance-monthly-cycle`
- `GET /api/v1/groups/available`
- `POST /api/v1/payment/orders`

Compatibility behavior:

- `subscriptions` endpoints return entitlement data with legacy fields filled:
  - `group_id`: primary group or first enabled group.
  - `group`: primary group object.
  - `groups`: new list.
  - `entitlement_id`: new explicit id.
- Old clients can still display one group.
- New clients use `groups` and `entitlement_id`.

New endpoints:

- `GET /api/v1/entitlements`
- `GET /api/v1/entitlements/active`
- `GET /api/v1/entitlements/:id/progress`
- `GET /api/v1/entitlements/:id/group-preferences`
- `PUT /api/v1/entitlements/:id/group-preferences`

Recommendation:

- Implement new endpoints.
- Keep old endpoints as aliases for one release.

---

## 8. Rollout Plan

### Phase A: Additive Schema And Backfill

1. Add Ent schemas and migration.
2. Run `go generate ./ent`.
3. Backfill plan groups and entitlements.
4. Do not change runtime behavior yet.
5. Add a runtime setting:
   - `subscription_entitlements_v2_enabled`
   - `sub2_payment_page_legacy_mapping_enabled`
   - default `false`.

### Phase B: Dual Read

1. Entitlement service can read v2 tables.
2. Existing subscription endpoints include v2 fields.
3. API key available groups can still use legacy until flag is on.
4. Admin can inspect entitlement records.

### Phase C: Dual Write

1. New purchases create entitlement.
2. Legacy `user_subscriptions` is also written for single-group compatibility only when needed.
3. Usage logs include `entitlement_id`.
4. Billing still supports old subscription id.
5. Any legacy subscriptions created after Phase A backfill and before Phase C dual-write must be filled by a catch-up backfill.
6. Record catch-up row counts; before runtime switch they must be zero or explicitly expected.

### Phase D: Runtime Switch

1. Run the final catch-up backfill.
2. Verify every active legacy subscription has an entitlement and entitlement group.
3. Verify every legacy cashier `group_id + validity_days + value` tuple has an enabled external mapping.
4. Enable `subscription_entitlements_v2_enabled`.
5. Middleware resolves entitlements.
6. API key group binding uses entitlement coverage.
7. Shared quota becomes active.
8. Enable `sub2_payment_page_legacy_mapping_enabled` only after external mapping verification passes.

### Phase E: Cleanup Later

Not in first release:

- Drop old `user_subscriptions`.
- Remove old subscription routes.
- Remove `payment_orders.subscription_group_id`.
- Remove `usage_logs.subscription_id`.

---

## 9. Detailed Tasks

### Task 1: Entitlement Feature Flag

**Files:**

- Modify: `backend/internal/service/setting.go`
- Modify: `backend/internal/service/setting_service.go`
- Modify: `backend/internal/handler/admin/setting_handler.go`
- Test: `backend/internal/service/setting_service_get_all_test.go`
- Test: `backend/internal/handler/admin/setting_handler_auth_source_defaults_test.go`

**Steps:**

1. Add setting key `subscription_entitlements_v2_enabled`.
2. Add setting key `sub2_payment_page_legacy_mapping_enabled`, default `false`.
3. Add runtime getters on `SettingService`.
4. Add both admin settings DTO fields.
5. Write tests for default false and update true.
6. Run:

```bash
cd backend
go test -tags=unit ./internal/service -run Setting
go test -tags=unit ./internal/handler/admin -run Setting
```

### Task 2: Ent Schemas And SQL Migration

**Files:**

- Create: `backend/ent/schema/subscription_plan_group.go`
- Create: `backend/ent/schema/subscription_plan_external_mapping.go`
- Create: `backend/ent/schema/subscription_entitlement.go`
- Create: `backend/ent/schema/subscription_entitlement_group.go`
- Modify: `backend/ent/schema/subscription_plan.go`
- Modify: `backend/ent/schema/redeem_code.go`
- Modify: `backend/ent/schema/api_key.go`
- Modify: `backend/ent/schema/usage_log.go`
- Modify: `backend/ent/schema/payment_order.go`
- Create: `backend/migrations/150_subscription_entitlements_v2.sql`
- Test: `backend/migrations/auth_identity_payment_migrations_regression_test.go`

**Steps:**

1. Write the migration as additive and idempotent.
2. Add Ent schemas and edges.
3. Verify the migration includes:
   - entitlement core tables
   - `subscription_plan_external_mappings` with unique key `source + legacy_group_id + legacy_validity_days + legacy_value`
   - `redeem_codes.plan_id`
   - entitlement source fields and unique constraints
   - `api_keys.subscription_entitlement_id`, `usage_logs.entitlement_id`, `payment_orders.subscription_entitlement_id`
4. Run:

```bash
cd backend
go generate ./ent
go test -tags=unit ./migrations
```

5. Verify generated code under `backend/ent/` is included.

### Task 3: Repository Port And Ent Repository

**Files:**

- Create: `backend/internal/service/subscription_entitlement.go`
- Create: `backend/internal/service/subscription_entitlement_port.go`
- Create: `backend/internal/service/subscription_plan_external_mapping.go`
- Create: `backend/internal/service/subscription_plan_external_mapping_port.go`
- Create: `backend/internal/repository/subscription_entitlement_repo.go`
- Create: `backend/internal/repository/subscription_plan_external_mapping_repo.go`
- Modify: `backend/internal/repository/wire.go`
- Test: `backend/internal/repository/subscription_entitlement_repo_integration_test.go`
- Test: `backend/internal/repository/subscription_plan_external_mapping_repo_integration_test.go`

**Steps:**

1. Define service-layer model and repository interface.
2. Implement Ent repository in repository layer.
3. Cover:
   - create entitlement with groups
   - list active by user
   - list active covering group
   - replace groups
   - apply entitlement usage atomically
   - exact external mapping lookup by `source + group_id + validity_days + value`
   - amount mismatch does not match
4. Expose transaction-aware repository ports needed for grant and billing flows.
5. Run:

```bash
cd backend
go test -tags=integration ./internal/repository -run SubscriptionEntitlement
```

### Task 4: Entitlement Service Core

**Files:**

- Create: `backend/internal/service/subscription_entitlement_service.go`
- Create: `backend/internal/service/subscription_entitlement_resolver.go`
- Create: `backend/internal/service/subscription_entitlement_maintenance.go`
- Test: `backend/internal/service/subscription_entitlement_service_test.go`
- Test: `backend/internal/service/subscription_entitlement_resolver_test.go`

**Steps:**

1. Implement assign/extend from plan.
2. Implement active entitlement resolution by API key and group.
3. Implement quota checks using entitlement limits.
4. Implement daily/weekly/monthly window maintenance.
5. Implement transaction-aware grant method such as `AssignOrExtendFromPlanTx(txCtx, input)` for redeem/payment flows.
6. Implement source idempotency:
   - existing `source_redeem_code_id` returns existing entitlement, no double extension
   - existing `source_type + source_external_id` returns existing entitlement, no double extension
7. Write tests:
   - one entitlement covers two groups and shares monthly usage
   - exhausted entitlement does not switch to another group for extra quota
   - explicit API key entitlement id wins
   - missing coverage returns `GROUP_NOT_ALLOWED`
   - same source redeem code does not extend twice
   - same source external id does not extend twice
8. Run:

```bash
cd backend
go test -tags=unit ./internal/service -run SubscriptionEntitlement
```

### Task 5: Wire Dependency Injection

**Files:**

- Modify: `backend/internal/service/wire.go`
- Modify: `backend/cmd/server/wire.go`
- Generated: `backend/cmd/server/wire_gen.go`
- Generated test: `backend/cmd/server/wire_gen_test.go`

**Steps:**

1. Add entitlement repository provider.
2. Add external mapping repository provider.
3. Add entitlement service provider.
4. Add external mapping service provider.
5. Inject entitlement service into payment, API key, middleware paths, admin handlers, and redeem service.
6. Inject external mapping service into redeem service.
7. Run:

```bash
cd backend
go generate ./cmd/server
go test -tags=unit ./cmd/server
```

### Task 6: Payment Plan Configuration

**Files:**

- Modify: `backend/internal/service/payment_config_plans.go`
- Modify: `backend/internal/handler/admin/payment_handler.go`
- Modify: `frontend/src/types/payment.ts`
- Modify: `frontend/src/api/admin/payment.ts`
- Modify: `frontend/src/views/admin/orders/PlanEditDialog.vue`
- Modify: `frontend/src/views/admin/orders/AdminPaymentPlansView.vue`
- Test: `backend/internal/service/payment_config_plans_validation_test.go`
- Test: frontend admin payment tests if present.

**Steps:**

1. Extend create/update plan request with quota fields, access scope, group ids, allowed platforms, overage policy.
2. Validate scope and group ids.
3. Persist `subscription_plan_groups`.
4. Update admin UI to edit quotas and access scope.
5. Run:

```bash
cd backend
go test -tags=unit ./internal/service -run PaymentConfig
cd ../frontend
pnpm run typecheck
```

### Task 7: Payment Fulfillment And Refund

**Files:**

- Modify: `backend/internal/service/payment_order.go`
- Modify: `backend/internal/service/payment_fulfillment.go`
- Modify: `backend/internal/service/payment_refund.go`
- Modify: `backend/internal/handler/admin/redeem_handler.go`
- Test: `backend/internal/service/payment_fulfillment_test.go`
- Test: `backend/internal/service/payment_refund_test.go`

**Steps:**

1. Create entitlement from plan on successful payment.
2. Set `payment_orders.subscription_entitlement_id`.
3. Keep legacy fields populated for compatibility.
4. Refund by entitlement id first, legacy group fallback second.
5. External cashier create-and-redeem referral sync must reuse the new source detection result; do not keep relying on `code` starting with `s2p_` or notes containing `sub2apipay`.
6. Add tests:
   - plan with two groups creates one entitlement with two group grants
   - renewal extends same entitlement and preserves shared usage unless expired
   - payment provider webhook refund shortens entitlement
   - admin manual refund shortens entitlement
   - old order with only subscription_group_id still refunds
   - external cashier subscription order with `Idempotency-Key=s2p_*`, `code=auto_*`, and `notes=Sub:*` still syncs referral audit correctly
7. Run:

```bash
cd backend
go test -tags=unit ./internal/service -run 'PaymentFulfillment|PaymentRefund'
```

### Task 8: API Key Binding And Available Groups

**Files:**

- Modify: `backend/internal/service/api_key_service.go`
- Modify: `backend/internal/repository/api_key_repo.go`
- Modify: `backend/internal/handler/api_key_handler.go`
- Modify: `backend/internal/handler/available_channel_handler.go`
- Modify: `frontend/src/api/keys.ts`
- Modify: `frontend/src/views/user/KeysView.vue`
- Test: `backend/internal/service/api_key_service_test.go`
- Test: `backend/internal/handler/available_channel_handler_test.go`

**Steps:**

1. Add `subscription_entitlement_id` to API key create/update DTOs.
2. Validate entitlement coverage on create/update.
3. Return entitlement-aware available groups.
4. Update user key UI to choose entitlement and current group.
5. Run:

```bash
cd backend
go test -tags=unit ./internal/service -run APIKey
go test -tags=unit ./internal/handler -run AvailableChannel
cd ../frontend
pnpm run typecheck
```

### Task 9: Middleware And Auto-Switch

**Files:**

- Modify: `backend/internal/server/middleware/api_key_auth.go`
- Modify: `backend/internal/server/middleware/api_key_auth_google.go`
- Test: `backend/internal/server/middleware/api_key_auth_test.go`
- Test: `backend/internal/server/middleware/api_key_auth_google_test.go`

**Steps:**

1. Resolve entitlement for subscription groups when v2 flag is enabled.
2. Set entitlement context key.
3. Switch groups only within the same entitlement.
4. Do not switch for quota exhaustion unless there is a different active entitlement selected by explicit policy.
5. Support `balance_fallback`.
6. Run:

```bash
cd backend
go test -tags=unit ./internal/server/middleware -run APIKeyAuth
```

### Task 10: Billing And Usage Logs

**Files:**

- Modify: `backend/internal/service/usage_billing.go`
- Modify: `backend/internal/repository/usage_billing_repo.go`
- Modify: `backend/internal/service/gateway_service.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/repository/usage_log_repo.go`
- Modify: `backend/internal/service/usage_log.go`
- Test: `backend/internal/repository/usage_billing_repo_integration_test.go`
- Test: gateway usage tests under `backend/internal/service/*gateway*test.go`

**Steps:**

1. Add entitlement id to usage billing command.
2. Increment entitlement usage atomically when present.
3. Persist `usage_logs.entitlement_id`.
4. Keep `subscription_id` compatibility.
5. Add tests:
   - two different groups increment same entitlement
   - group id in usage log remains actual group
   - entitlement id is persisted
   - concurrent requests cannot exceed entitlement limit
   - `balance_fallback` happens in the same billing command
6. Run:

```bash
cd backend
go test -tags=integration ./internal/repository -run UsageBilling
go test -tags=unit ./internal/service -run Gateway
```

### Task 11: User Entitlement APIs

**Files:**

- Create: `backend/internal/handler/entitlement_handler.go`
- Modify: `backend/internal/handler/wire.go`
- Modify: `backend/internal/server/routes/user.go`
- Modify: `frontend/src/api/subscriptions.ts`
- Modify: `frontend/src/stores/subscriptions.ts`
- Modify: `frontend/src/views/user/SubscriptionsView.vue`
- Test: `backend/internal/handler/entitlement_handler_test.go`

**Steps:**

1. Add new `/entitlements` routes.
2. Keep `/subscriptions` routes as compatibility aliases.
3. Return groups list and shared quota progress.
4. Update user page to show package entitlement cards.
5. Run:

```bash
cd backend
go test -tags=unit ./internal/handler -run Entitlement
cd ../frontend
pnpm run typecheck
pnpm run test:run
```

### Task 12: Admin Entitlement Management

**Files:**

- Create: `backend/internal/handler/admin/entitlement_handler.go`
- Modify: `backend/internal/handler/wire.go`
- Modify: `backend/internal/server/routes/admin.go`
- Create: `frontend/src/api/admin/entitlements.ts`
- Create or modify admin entitlement view under `frontend/src/views/admin/`
- Test: admin handler tests.

**Steps:**

1. List/filter entitlements by user, group, plan, status.
2. Assign entitlement from plan.
3. Extend/revoke/reset quota.
4. Replace allowed groups.
5. Update admin UI.
6. Run:

```bash
cd backend
go test -tags=unit ./internal/handler/admin -run Entitlement
cd ../frontend
pnpm run typecheck
```

### Task 13: Redeem Codes And Default Grants

**Files:**

- Modify: `backend/internal/service/redeem_service.go`
- Modify: `backend/internal/handler/admin/redeem_handler.go`
- Modify: `backend/internal/service/setting_service.go`
- Modify: `backend/internal/handler/admin/setting_handler.go`
- Modify: redeem code service model, repository model, DTO conversion.
- Create/modify: external mapping repository/service.
- Modify: `frontend/src/views/admin/SettingsView.vue`
- Modify: `frontend/src/views/user/RedeemView.vue`
- Test: redeem and settings tests.

**Steps:**

1. Test legacy cashier request requires `Idempotency-Key: s2p_xxx`, `code=auto_xxx`, matching suffixes, `type=subscription`, `value>0`, `group_id`, `validity_days>0`, enabled flag, and exact mapping before creating multi-group entitlement.
2. Test amount mismatch does not match.
3. Test `notes` alone never identifies the source.
4. Test replay returns existing entitlement and does not extend twice.
5. Test missing mapping keeps old single-group compatibility.
6. Test ordinary admin redeem codes are not affected by legacy cashier mapping.
7. Test legacy cashier subscription requests still create referral/recharge audit records, with `external_order_id` derived from the common `s2p_`/`auto_` suffix.
8. Add `PlanID *int64` to `CreateAndRedeemCodeRequest`; normal subscription redeem validation is `plan_id XOR group_id`.
9. Read `Idempotency-Key` in handler and pass source context into service; do not trust request body source.
10. In redeem service, apply strict source detection, exact mapping, and transaction-aware entitlement grant.
11. Add `plan_id` support for subscription redeem codes.
12. Add plan-based default subscription settings.
13. Update settings UI to prefer plan picker and expose legacy cashier mapping flag.
14. Run:

```bash
cd backend
go test -tags=unit ./internal/service -run 'Redeem|Setting'
go test -tags=unit ./internal/handler/admin -run 'Redeem|Setting'
cd ../frontend
pnpm run typecheck
```

### Task 14: Reporting And Admin Usage Filters

**Files:**

- Modify: `backend/internal/repository/usage_log_repo.go`
- Modify: `backend/internal/handler/admin/usage_handler.go`
- Modify: `frontend/src/api/admin/usage.ts`
- Modify: `frontend/src/views/admin/UsageView.vue`

**Steps:**

1. Add optional `entitlement_id` filters.
2. Keep `subscription_id` filters.
3. Show both actual group and package entitlement in usage details.
4. Run:

```bash
cd backend
go test -tags=unit ./internal/handler/admin -run Usage
cd ../frontend
pnpm run typecheck
```

### Task 15: End-To-End Regression

**Files:**

- Add or modify integration tests under `backend/internal/integration/`
- Add frontend integration tests under `frontend/src/__tests__/integration/` if existing patterns fit.

**Scenarios:**

1. User buys multi-group plan.
2. API key binds entitlement + group A.
3. Request through group A increments entitlement usage.
4. User switches same API key to group B.
5. Request through group B increments same entitlement usage.
6. Quota exhausted on group B blocks group A too.
7. If overage policy is balance fallback, next request deducts wallet.
8. Legacy single-group subscription still works.

Run:

```bash
cd backend
go test -tags=unit ./...
go test -tags=integration ./...
golangci-lint run ./...
cd ../frontend
pnpm run lint:check
pnpm run typecheck
pnpm run test:run
pnpm run build
```

### Task 16: Documentation And Operator Runbook

**Files:**

- Modify: `docs/PAYMENT.md`
- Modify: `docs/PAYMENT_CN.md`
- Modify: `README_CN.md` if payment/package behavior is documented there.
- Create: `docs/SUBSCRIPTION_ENTITLEMENTS_V2_RUNBOOK.md`

Runbook must include:

- How to create a multi-group plan.
- How to migrate legacy plans.
- How to inventory legacy cashier `group_id + validity_days + value` package tuples.
- How to create and verify `subscription_plan_external_mappings`.
- How to run the final pre-switch catch-up backfill.
- How to enable v2 flag.
- How to enable/disable the legacy cashier mapping flag independently.
- How to verify backfill counts.
- How to roll back runtime flag.
- How to inspect one user's entitlement and usage logs.

---

## 10. Testing Matrix

Backend unit:

- entitlement resolver
- quota reset
- payment fulfillment
- refund
- API key binding
- middleware
- setting flag
- redeem compatibility
- legacy cashier source detection
- referral/recharge audit sync for legacy cashier orders

Backend integration:

- Ent repositories
- usage billing atomic increments
- migration idempotency
- catch-up backfill idempotency
- purchase to entitlement creation
- webhook refund and admin refund both deduct entitlement term
- external mapping miss does not grant multi-group entitlement

Frontend:

- plan editor validation
- purchase page renders quotas and scope
- key page entitlement/group selection
- subscriptions page shared quota display

Manual QA:

1. Create two subscription groups with different route behavior.
2. Create one plan covering both groups with monthly limit 1.00 USD.
3. Buy plan as user.
4. Create API key bound to group A.
5. Send request; verify `usage_logs.group_id = A` and `entitlement_id = E`.
6. Switch key to group B.
7. Send request; verify same `entitlement_id = E`.
8. Exhaust monthly limit.
9. Verify both groups are blocked or wallet fallback is used based on policy.

---

## 11. Production Rollout Checklist

Pre-deploy:

- `go test -tags=unit ./...` passes.
- `go test -tags=integration ./...` passes.
- `golangci-lint run ./...` clean.
- `pnpm run lint:check` passes.
- `pnpm run typecheck` passes.
- `pnpm run test:run` passes.
- `pnpm run build` passes.
- Migration tested on a staging copy.
- Backfill row counts recorded.
- Inventory actual legacy cashier package tuples from current `sub2-payment-page` config or production orders: `group_id + validity_days + value`.
- Create `subscription_plan_external_mappings` for every legacy cashier tuple and record the target `plan_id`.
- On staging, verify every tuple matches exactly and amount mismatch does not match.

Deploy:

1. Deploy code with v2 flag off.
2. Run migrations.
3. Run initial backfill.
4. Enable dual-write paths while keeping `subscription_entitlements_v2_enabled=false`.
5. Wait through at least one low-risk payment/redeem window and verify new purchases also write entitlements.
6. Run final catch-up backfill before switching.
7. Verify:
   - number of `subscription_entitlements` >= active legacy subscriptions.
   - every active legacy subscription has one entitlement group.
   - active API keys with subscription groups have entitlement ids.
   - final catch-up inserted zero rows, or every inserted row is explained.
   - every legacy cashier tuple has an enabled external mapping.
8. Enable v2 for one test user if per-user flag exists, otherwise staging first.
9. Enable `subscription_entitlements_v2_enabled` during low traffic.
10. After a small legacy cashier test order verifies external mapping, enable `sub2_payment_page_legacy_mapping_enabled`.
11. Watch:
   - 403/429 rates
   - payment fulfillment errors
   - usage billing errors
   - API key auth conflict errors
   - cache invalidation errors
   - external mapping miss count
   - entitlement source duplicate/conflict count

Rollback:

1. Disable `subscription_entitlements_v2_enabled`.
2. Disable `sub2_payment_page_legacy_mapping_enabled`.
3. Keep new tables.
4. Legacy `user_subscriptions` and balance billing continue to work.
5. Do not drop columns.

---

## 12. Recommended Commit Breakdown

1. `feat(subscription): add entitlement schemas and migration`
2. `feat(subscription): add entitlement repository`
3. `feat(subscription): add entitlement service and resolver`
4. `feat(payment): configure multi-group subscription plans`
5. `feat(payment): fulfill purchases as entitlements`
6. `feat(apikey): bind keys to entitlement scoped groups`
7. `feat(gateway): bill shared entitlement quota`
8. `feat(subscription): add entitlement user and admin APIs`
9. `feat(frontend): support multi-group subscription plans`
10. `docs(subscription): add entitlement v2 runbook`

Each commit should compile and pass the narrow tests for its area.

---

## 13. Final Architecture Summary

After this refactor:

- `subscription_plans` describe sellable products.
- `subscription_plan_groups` describes what groups a plan can authorize.
- `subscription_entitlements` is the user's actual package quota ledger.
- `subscription_entitlement_groups` is the user's actual access grant snapshot.
- `api_keys.group_id` remains the current routing group.
- `api_keys.subscription_entitlement_id` points to the package quota ledger.
- `usage_logs.group_id` records the actual group used.
- `usage_logs.entitlement_id` records the package quota charged.
- `users.balance` remains cash wallet only.

This is the clean one-step destination while still allowing safe production rollout.
