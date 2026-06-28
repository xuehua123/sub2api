# 套餐权益 V2 改造落地方案

> **给执行 Agent 的要求：** 如果用 Claude/Codex 执行本方案，请按任务逐项执行，每个任务完成后先跑对应测试，再进入下一项。

**目标：** 将现有“用户订阅某一个分组”的模型，升级为“用户持有一张共享额度的套餐权益，这张权益可以授权多个分组”。

**架构：** 账户余额继续作为现金钱包，不参与月卡语义。新增套餐权益层：套餐定义额度和可访问范围，用户购买后生成一张权益快照，API Key 绑定“套餐权益 + 当前分组”，无论用户切到哪个被授权分组，请求都扣同一份套餐权益额度。

**技术栈：** Go 1.26.1、Gin、Ent、PostgreSQL/SQLite migrations、Google Wire、Vue 3、Vite、TypeScript、Pinia、pnpm。

---

## 1. 产品决策

这次改造不是“给余额换个名字”，也不是“把余额改造成月卡”。

最终产品语义：

- `账户余额`：现金钱包，用于充值、按量付费、余额兜底；不自动重置。
- `套餐额度`：套餐/月卡内的可用额度，按权益窗口重置，可过期。
- `套餐通行证`：用户购买后得到的一张套餐权益，用于授权一个或多个分组。
- `当前分组`：API Key 当前使用的路由分组，决定模型、渠道、倍率、调度能力。

目标用户故事：

1. 用户购买 “Pro 月卡”。
2. 系统创建一张套餐权益，里面有一份额度账本和一套重置周期。
3. 这张套餐权益授权多个订阅型分组。
4. 用户的 API Key 可以在这些授权分组之间切换。
5. 每次请求都扣同一张套餐权益的用量。
6. 当套餐额度耗尽时，切换分组不会产生新的额度。
7. 可选策略：额度耗尽后直接拦截，或自动改用账户余额兜底。

明确不做：

- 不合并账户余额和套餐额度。
- 不再让分组额度作为新套餐的额度真相源。
- 第一版不删除旧 `user_subscriptions` 表。
- 不在生产业务服务器上构建 Docker 镜像。

---

## 2. 核心不变量

实现必须始终满足这些不变量：

1. 套餐权益负责额度、有效期和重置窗口。
2. 分组负责路由、模型能力、平台、倍率和上游选择。
3. 请求费用按照实际使用的当前分组、模型、渠道计算，再写入选中的套餐权益。
4. 用户只有在存在覆盖目标分组的有效套餐权益时，才能把 API Key 绑定或切换到该订阅型分组。
5. 如果 API Key 显式绑定了 `subscription_entitlement_id`，它只能使用这张权益授权的分组。
6. 一张权益覆盖多个分组时，这些分组共享同一组 daily/weekly/monthly 用量计数。
7. 现有订阅迁移为“只覆盖原分组”的单分组套餐权益，行为保持不变。
8. 现有余额行为保持不变。
9. `sub2-payment-page` 视为不可修改的外部收银台：它继续调用旧的 `group_id + validity_days` 履约协议，所有升级都必须在主服务内完成。
10. 旧外部输入中的 `group_id` 在 v2 下可以被主服务解释为“外部套餐锚点”，再映射到新的多分组套餐权益；没有命中映射时必须回退到旧单分组订阅行为。
11. 新增 service 代码必须遵守项目分层规则：`service` 不直接 import repository、GORM、Redis 或直接数据库实现，除非属于现有 depguard 白名单例外。

---

## 3. 目标数据模型

### 3.1 保留旧表作为兼容层

第一版继续保留以下表和字段：

- `subscription_plans.group_id`
- `user_subscriptions`
- `usage_logs.subscription_id`
- `payment_orders.subscription_group_id`
- `payment_orders.subscription_days`

这些字段变成兼容字段。第一阶段不要删除。

### 3.2 扩展 `subscription_plans`

新增套餐级额度和访问范围字段：

```sql
ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS access_scope VARCHAR(32) NOT NULL DEFAULT 'explicit',
    ADD COLUMN IF NOT EXISTS allowed_platforms JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS daily_limit_usd DECIMAL(20, 8),
    ADD COLUMN IF NOT EXISTS weekly_limit_usd DECIMAL(20, 8),
    ADD COLUMN IF NOT EXISTS monthly_limit_usd DECIMAL(20, 8),
    ADD COLUMN IF NOT EXISTS overage_policy VARCHAR(32) NOT NULL DEFAULT 'block';
```

`access_scope` 可选值：

- `explicit`：使用 `subscription_plan_groups` 显式配置分组。
- `all_subscription_groups`：授权所有 active 且 `subscription_type = 'subscription'` 的分组。
- `platform_subscription_groups`：授权指定平台下所有 active 订阅型分组，平台来自 `allowed_platforms`。

`overage_policy` 可选值：

- `block`：套餐额度耗尽后拦截请求。
- `balance_fallback`：套餐额度耗尽后改用账户余额兜底。

兼容要求：

- 所有旧套餐默认设置为 `access_scope = 'explicit'`。
- 每个旧套餐在 `subscription_plan_groups` 中插入一条它当前 `group_id` 对应的授权记录。

### 3.3 新建 `subscription_plan_groups`

该表表示“一个套餐可以授权哪些分组”。

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

### 3.3.1 新建 `subscription_plan_external_mappings`

该表专门解决 `sub2-payment-page` 不能改的问题。支付页仍然发送旧字段：

```json
{
  "type": "subscription",
  "value": 29.90,
  "group_id": 5,
  "validity_days": 30,
  "user_id": 123
}
```

主服务收到后不要求支付页知道新的 `plan_id`。它先用 `source + legacy_group_id + legacy_validity_days + legacy_value` 查找映射；命中后开通新的多分组套餐权益；未命中时继续走旧单分组订阅逻辑。

`legacy_value` 来自旧支付页传入的 `value`，也就是支付页套餐配置里的价格。不能只用 `group_id + validity_days` 匹配，否则未来出现“同分组、同天数、不同价格/权益”的套餐时会开错权益。

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

初始映射示例：

```text
sub2-payment-page + group_id=4 + validity_days=7  + value=9.90  -> 体验套餐权益
sub2-payment-page + group_id=5 + validity_days=30 + value=29.90 -> 月度标准套餐权益
sub2-payment-page + group_id=6 + validity_days=30 + value=59.90 -> 月度高级套餐权益
```

这里的 `group_id` 只是旧支付页传来的“套餐锚点”，不是新权益最终只能使用的分组。最终可用分组来自 `subscription_plan_groups`。

匹配规则：

- `source` 必须是主服务识别出的固定来源，不接受请求体自报。
- `legacy_group_id` 必须等于旧请求里的 `group_id`。
- `legacy_validity_days` 必须等于旧请求里的 `validity_days`。
- `legacy_value` 使用主服务把旧请求 `value` 规范化后的金额，建议按人民币分保留两位小数后再比较。
- 不做金额容差匹配；金额不一致就回退旧逻辑或拒绝，避免误开高等级套餐。

### 3.3.2 扩展 `redeem_codes`

为了让主服务后台和未来 API 可以直接发放 plan-based 套餐权益，兑换码需要支持 `plan_id`。

```sql
ALTER TABLE redeem_codes
    ADD COLUMN IF NOT EXISTS plan_id BIGINT REFERENCES subscription_plans(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_redeem_codes_plan_id
    ON redeem_codes(plan_id);
```

兼容规则：

- 旧支付页仍然只发送 `group_id`，不发送 `plan_id`。
- 旧支付页请求如果命中外部映射，由主服务内部解析出 `plan_id` 并发放 entitlement；不要求回写到旧请求体。
- 普通后台创建订阅兑换码时，`plan_id` 和 `group_id` 必须二选一。
- 同时传 `plan_id` 和 `group_id` 的普通请求必须拒绝，避免一张码同时表达两种权益。

### 3.4 新建 `subscription_entitlements`

该表是新套餐额度的真相源。它表示“用户实际持有的一张套餐权益”。

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

字段说明：

- `legacy_subscription_id`：兼容旧 `user_subscriptions`。
- `primary_group_id`：旧接口返回 `group_id` 时使用的默认/主分组。
- `daily_limit_usd`、`weekly_limit_usd`、`monthly_limit_usd`：购买时从套餐快照写入，后续套餐修改不影响已购买权益。
- `plan_snapshot`：保存购买时套餐名、价格、有效期、额度、授权范围等快照，方便退款和审计。
- `source_id`：内置支付订单 ID、管理员操作 ID 等内部来源 ID。
- `source_external_id`：外部订单号，例如旧支付页 `out_trade_no`。用于支付页兼容履约幂等。
- `source_redeem_code_id`：兑换码 ID。通过兑换码发放权益时必须写入，保证同一兑换码不会重复发放权益。

### 3.5 新建 `subscription_entitlement_fulfillments`

`subscription_entitlements.source_*` 只能描述这张权益当前或最近一次来源。续费发生后，单行 source 指针会被新来源覆盖；如果旧支付回调、旧外部订单号或旧兑换码再次 replay，仅依赖 entitlement 当前指针无法判断它是否已经被处理过，存在重复续期风险。

因此必须保留每一次创建/续期事件的历史记录：

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

规则：
- 创建或续期 entitlement 时，必须在同一数据库事务内写入 fulfillment event；任一步失败都要整体回滚。
- 幂等检查必须优先查询 `subscription_entitlement_fulfillments`，命中后直接返回既有结果，不再次延长有效期。
- 后续 payment/redeem 接入时，必须先查 fulfillment history，再考虑兼容旧 entitlement `source_*` 指针。
- entitlement 上的 `source_*` 仅作为兼容和最近来源快照，不再作为完整幂等历史。

### 3.6 新建 `subscription_entitlement_groups`

该表表示“用户这张权益实际授权了哪些分组”。

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

### 3.7 给热点表补充外键

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

不要删除 `api_keys.group_id`。它仍然是 API Key 当前正在使用的路由分组。

---

## 4. 数据迁移策略

迁移必须幂等，并且可以在已有环境安全运行。

建议使用下一个迁移编号：

- 新建：`backend/migrations/150_subscription_entitlements_v2.sql`

### 4.1 回填套餐分组关系

```sql
INSERT INTO subscription_plan_groups (plan_id, group_id, sort_order, enabled, created_at, updated_at)
SELECT id, group_id, 0, TRUE, NOW(), NOW()
FROM subscription_plans
WHERE group_id IS NOT NULL
ON CONFLICT (plan_id, group_id) DO NOTHING;
```

### 4.2 从旧 group 额度回填 plan 额度

仅当 plan 自己还没有设置额度时回填：

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

### 4.3 每条旧订阅回填一张套餐权益

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

### 4.4 回填权益授权分组

```sql
INSERT INTO subscription_entitlement_groups (entitlement_id, group_id, sort_order, enabled, created_at, updated_at)
SELECT se.id, se.primary_group_id, 0, TRUE, NOW(), NOW()
FROM subscription_entitlements se
WHERE se.primary_group_id IS NOT NULL
ON CONFLICT (entitlement_id, group_id) DO NOTHING;
```

### 4.5 回填 API Key 的权益 ID

```sql
UPDATE api_keys ak
SET subscription_entitlement_id = se.id
FROM subscription_entitlements se
WHERE ak.user_id = se.user_id
  AND ak.group_id = se.primary_group_id
  AND ak.subscription_entitlement_id IS NULL
  AND se.deleted_at IS NULL;
```

### 4.6 回填 usage_logs 的 entitlement_id

```sql
UPDATE usage_logs ul
SET entitlement_id = se.id
FROM subscription_entitlements se
WHERE ul.subscription_id = se.legacy_subscription_id
  AND ul.entitlement_id IS NULL;
```

### 4.7 回滚姿态

- 所有表和字段都是新增，不破坏旧数据。
- 旧 `user_subscriptions` 不修改、不删除。
- 如果上线异常，关闭 v2 开关，恢复旧订阅读取路径。

---

## 5. 后端设计

### 5.1 新增领域模型

新增文件：

- `backend/internal/service/subscription_entitlement.go`
- `backend/internal/service/subscription_entitlement_port.go`
- `backend/internal/repository/subscription_entitlement_repo.go`

核心 service 结构：

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

Repository port：

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

### 5.2 Ent Schema

新增：

- `backend/ent/schema/subscription_plan_group.go`
- `backend/ent/schema/subscription_entitlement.go`
- `backend/ent/schema/subscription_entitlement_group.go`

修改：

- `backend/ent/schema/subscription_plan.go`
- `backend/ent/schema/api_key.go`
- `backend/ent/schema/usage_log.go`
- `backend/ent/schema/payment_order.go`

Schema 修改后运行：

```bash
cd backend
go generate ./ent
go generate ./cmd/server
```

### 5.3 Service 层

新增：

- `backend/internal/service/subscription_entitlement_service.go`
- `backend/internal/service/subscription_entitlement_resolver.go`
- `backend/internal/service/subscription_entitlement_progress.go`
- `backend/internal/service/subscription_entitlement_maintenance.go`

`SubscriptionService` 第一版保留为兼容 facade：

- v2 开启时，旧公开方法内部委托到 entitlement service。
- 旧方法如 `GetActiveSubscription(userID, groupID)` 可通过“查找覆盖该 group 的有效权益”返回兼容版 `UserSubscription`。

当多个有效权益同时覆盖同一个分组时，选择规则：

1. 如果 API Key 显式绑定了 `subscription_entitlement_id`，只使用这张权益。
2. 否则优先选择 `expires_at` 最早的权益。
3. 再按 `subscription_entitlement_groups.sort_order` 排序。
4. 最后按 entitlement id 从小到大。

### 5.4 API Key 绑定

修改：

- `backend/internal/service/api_key_service.go`
- `backend/internal/handler/api_key_handler.go`
- `backend/internal/repository/api_key_repo.go`
- `backend/ent/schema/api_key.go`

请求结构新增：

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

校验规则：

- 标准分组：保持当前 `AllowedGroups` / `IsExclusive` 逻辑。
- 订阅分组 + 显式 entitlement id：权益必须有效、属于该用户，并覆盖目标分组。
- 订阅分组 + 未传 entitlement id：按默认解析规则选择一张覆盖目标分组的权益。
- 切换 `group_id` 时必须保留或重新解析 `subscription_entitlement_id`。

用户可用分组接口：

- `GET /api/v1/groups/available` 返回当前用户有效权益覆盖的分组。
- 返回值中补充 entitlement 上下文，供前端选择：

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

### 5.5 认证中间件解析

修改：

- `backend/internal/server/middleware/api_key_auth.go`
- `backend/internal/server/middleware/api_key_auth_google.go`

新流程：

1. 按现有逻辑校验 API Key 状态、额度、过期时间。
2. 加载当前 `apiKey.Group`。
3. 如果当前 group 是标准分组，继续走余额逻辑。
4. 如果当前 group 是订阅分组：
   - 根据 `apiKey.SubscriptionEntitlementID` 解析权益。
   - 校验权益覆盖当前 group。
   - 使用当前 group 做 endpoint 能力校验。
   - 校验共享套餐额度。
   - 设置 context：entitlement、当前 group、兼容 subscription id。
5. 如果当前 group 不可用且启用了自动切组：
   - 只在同一张权益授权的分组内切换。
   - 如果只是套餐额度耗尽，不通过切组寻找新额度。
   - 如果 overage policy 是 `balance_fallback`，设置余额兜底标记。

新增 context key：

- `ContextKeySubscriptionEntitlement`

临时保留 `ContextKeySubscription`，兼容仍然读取旧 subscription 的代码路径。

### 5.6 计费扣量

修改：

- `backend/internal/service/usage_billing.go`
- `backend/internal/repository/usage_billing_repo.go`
- 所有构造 `UsageBillingCommand` 的 gateway 相关文件。

扩展命令：

```go
type UsageBillingCommand struct {
    SubscriptionID *int64
    EntitlementID *int64
    SubscriptionCost float64
}
```

扣量规则：

- 如果 `EntitlementID != nil`，更新 `subscription_entitlements` 用量。
- 否则如果 `SubscriptionID != nil`，走旧 `user_subscriptions`。
- 返回 entitlement version，用于缓存失效。

v2 原子性要求：

- `subscription_entitlements` 扣量不能只做“先查余额、后累加”。
- repository 层必须提供一个事务内原子方法，例如 `ApplyEntitlementUsage(ctx, entitlementID, cost, now)`。
- 该方法必须在同一 SQL/事务中完成：
  - 锁定目标 entitlement 行。
  - 判断 status、expires_at、deleted_at。
  - 判断 daily/weekly/monthly 窗口是否需要重置。
  - 判断扣量后是否超过对应 limit。
  - 更新 daily/weekly/monthly usage 和 window_start。
  - 返回新的 usage、window_start、updated_at/version。
- 并发请求命中同一张 entitlement 时，只允许一个事务看到可用额度并成功扣量；超额事务返回 `ErrEntitlementLimitExceeded`。
- `balance_fallback` 策略必须在同一个 `UsageBillingCommand` 中明确表达：套餐扣量失败且策略允许时，才转为扣余额；不能在 handler 层吞掉 entitlement 超额错误后另起一次独立扣款。

写 usage log：

- v2 扣量时写入 `usage_logs.entitlement_id`。
- 如果是从旧 subscription 迁移过来的 entitlement，同时保留 `subscription_id`。
- `usage_logs.group_id` 始终记录实际使用的当前分组。

### 5.7 Billing Cache

修改：

- `backend/internal/service/billing_cache_service.go`
- `backend/internal/service/*billing*` 下相关 cache key helper。

需要支持的缓存：

- 按 `entitlement_id` 缓存权益状态。
- 按 `user_id + group_id` 缓存可覆盖该分组的权益。
- 按 `user_id` 缓存所有 active entitlements。

缓存失效：

- 每次权益用量递增后，失效该 entitlement。
- 购买、退款、管理员分配、撤销、替换授权分组、过期维护后，失效用户 active entitlement 列表。
- 旧的 subscription invalidation 方法先保留为 wrapper。

### 5.8 支付履约

修改：

- `backend/internal/service/payment_order.go`
- `backend/internal/service/payment_fulfillment.go`
- `backend/internal/service/payment_refund.go`
- `backend/internal/service/payment_config_plans.go`
- `backend/internal/handler/admin/payment_handler.go`

创建订单：

- 订阅订单仍然要求 `plan_id`。
- `PaymentService.validateSubOrder` 不能再要求 `plan.GroupID` 对应单个订阅分组；应改为校验 plan 本身可售，并且 plan 能解析出至少一个有效授权分组。
- 订单中保存套餐快照：
  - 套餐名
  - 有效期 days/unit
  - 额度限制
  - overage policy
  - access scope
  - 购买时解析出的授权分组

订单继续保存 `plan_id`，履约完成后写入 `subscription_entitlement_id`。

兼容字段处理：

- `payment_orders.subscription_group_id` 第一版继续保留。
- 创建 v2 订阅订单时可以写入 `primary_group_id` 或第一个授权分组，供旧报表、邮件和退款 fallback 使用。
- 它不能再作为履约真相源。
- `payment_fulfillment.go` 中 `ExecuteSubscriptionFulfillment` 和 `doSub` 必须优先使用 `plan_id -> entitlementService.AssignOrExtendFromPlan`。
- 只有当 v2 开关关闭、订单没有 `plan_id`、或兼容数据缺失时，才 fallback 到旧 `subscription_group_id + subscription_days` 逻辑。

履约核心调用：

```go
entitlement, reused, err := entitlementService.AssignOrExtendFromPlan(ctx, AssignEntitlementFromPlanInput{
    UserID: o.UserID,
    PlanID: *o.PlanID,
    OrderID: o.ID,
    AssignedBy: 0,
})
```

复用规则：

- 如果用户已有同 plan 且同 scope 的 active entitlement，则续期该权益。
- 如果已过期，则重新激活并重置用量窗口。
- 如果套餐配置在用户续费前改变，默认保留旧权益 scope；除非产品明确设置“续费使用最新套餐范围”。

推荐新增策略字段：

- `plan_scope_update_policy = snapshot | latest`

退款：

- 优先通过 `subscription_entitlement_id` 查找权益。
- 旧订单 fallback 到 `subscription_group_id` 和旧 subscription。
- 按退款比例扣减权益有效天数。
- 如果全额退款导致权益失效，撤销权益，并清理或重新解析指向它的 API Key。

### 5.9 兑换码和默认赠送

修改：

- `backend/internal/service/redeem_service.go`
- `backend/internal/handler/admin/redeem_handler.go`
- `backend/ent/schema/redeem_code.go`
- `backend/internal/service/setting_service.go`
- `backend/internal/handler/admin/setting_handler.go`
- 新增：`backend/internal/service/subscription_plan_external_mapping_service.go`
- 新增：对应 repository 和 Ent schema。

兼容旧输入：

- `sub2-payment-page` 不改，仍然调用 `/api/v1/admin/redeem-codes/create-and-redeem`。
- 它发送的 `{type:"subscription", value, group_id, validity_days}` 先被主服务识别为外部支付页履约请求。
- 主服务用 `source='sub2-payment-page' + legacy_group_id + legacy_validity_days + legacy_value` 查 `subscription_plan_external_mappings`。
- 命中映射：按映射到的 `plan_id` 创建或续期新的多分组套餐权益。
- 未命中映射：回退为旧逻辑，创建或续期该 `group_id` 的单分组订阅/权益。
- 手动创建兑换码如果没有支付页特征，不应该被意外映射，避免管理员临时发码时行为突变。

支付页请求来源识别：

- 不允许请求体自报 source。
- 只在 `sub2_payment_page_legacy_mapping_enabled = true` 时启用。
- 必须同时满足以下条件，才允许进入 `sub2-payment-page` 映射：
  - `Idempotency-Key` 存在且格式为 `s2p_*`。
  - `code` 存在且格式为 `auto_*`。
  - `Idempotency-Key` 去掉 `s2p_` 后的订单号必须等于 `code` 去掉 `auto_` 后的订单号。
  - `type == "subscription"`。
  - `group_id` 存在。
  - `validity_days > 0`。
  - `value > 0`。
  - `subscription_plan_external_mappings` 精确命中。
- `notes` 只能作为审计信息，不能作为来源识别条件。
- 任何条件不满足，都不要使用外部映射，继续走普通管理员兑换码/旧兼容逻辑。

事务和幂等：

- `Redeem` 中“标记兑换码已使用”和“发放 entitlement”必须在同一个 `txCtx` 内完成。
- entitlement service 需要暴露事务内方法，例如 `AssignOrExtendFromPlanTx(txCtx, input)`，不得在 redeem 流程里另起事务。
- 通过兑换码发放 entitlement 时必须写入 `source_redeem_code_id`。
- 通过旧支付页映射发放 entitlement 时同时写入：
  - `source_type = 'sub2-payment-page'`
  - `source_external_id = out_trade_no`，从 `Idempotency-Key` 和 `code` 的共同后缀解析。
  - `source_redeem_code_id = redeem_codes.id`
- `source_redeem_code_id` 和 `source_type + source_external_id` 都有唯一约束，防止重复续期。

新增推荐输入：

```json
{
  "type": "subscription",
  "plan_id": 123,
  "validity_days": 30
}
```

注意：这个新增输入是给主服务内置购买页、后台、未来 API 用的；不是要求 `sub2-payment-page` 改造。

为支持 `plan_id`，必须同步修改：

- `CreateAndRedeemCodeRequest` 增加 `PlanID *int64`。
- service 层 `RedeemCode` 模型增加 `PlanID *int64`。
- `redeem_codes` Ent schema 和迁移增加 `plan_id`。
- repository 创建、查询、DTO 转换都要带上 `plan_id`。
- 订阅兑换码校验改为：
  - 普通兑换：`plan_id XOR group_id`。
  - 旧支付页映射：请求仍然只有 `group_id`，命中映射后由主服务得到 `plan_id`。
  - `validity_days == 0` 只有普通 `plan_id` 兑换可以默认用 plan 有效期；旧支付页映射必须使用请求里的正数天数参与匹配。

注册默认赠送：

- 保留旧 `group_id + validity_days`。
- 新增可选 `plan_id`。
- 如果同时传 `group_id` 和 `plan_id`，直接拒绝。
- 新 UI 优先使用 plan-based default grants。

### 5.10 管理员订阅管理

修改：

- `backend/internal/handler/admin/subscription_handler.go`
- `backend/internal/service/subscription_service.go`
- 新增 entitlement admin service 方法。

新增管理员接口：

- `GET /api/v1/admin/entitlements`
- `GET /api/v1/admin/entitlements/:id`
- `POST /api/v1/admin/entitlements/assign`
- `POST /api/v1/admin/entitlements/:id/extend`
- `POST /api/v1/admin/entitlements/:id/reset-quota`
- `PUT /api/v1/admin/entitlements/:id/groups`
- `DELETE /api/v1/admin/entitlements/:id`

旧 `/admin/subscriptions` 路由保留一版，作为兼容 wrapper。

---

## 6. 前端设计

### 6.1 类型和 API Client

修改：

- `frontend/src/types/payment.ts`
- `frontend/src/types/index.ts`
- `frontend/src/api/payment.ts`
- `frontend/src/api/admin/payment.ts`
- `frontend/src/api/subscriptions.ts`
- `frontend/src/api/keys.ts`
- 新建 `frontend/src/api/admin/entitlements.ts`

新增类型：

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

### 6.2 管理端套餐编辑器

修改：

- `frontend/src/views/admin/orders/PlanEditDialog.vue`
- `frontend/src/views/admin/orders/AdminPaymentPlansView.vue`

UI 改造：

- 保留套餐名称、价格、有效期。
- 新增额度字段：
  - 每日额度
  - 每周额度
  - 每月额度
- 新增访问范围分段控件：
  - 指定分组
  - 全部订阅型分组
  - 指定平台下全部订阅型分组
- 显式范围下使用 group 多选。
- 新增超额策略选择：
  - 拦截
  - 账户余额兜底

校验：

- 至少配置一个额度，或明确确认“无限额度套餐”。
- `explicit` 范围必须至少选择一个分组。
- `platform_subscription_groups` 范围必须至少选择一个平台。

### 6.3 用户购买页

修改：

- `frontend/src/views/user/PaymentView.vue`
- `frontend/src/components/payment/SubscriptionPlanCard.vue`

展示内容：

- 套餐额度：`$x/日`、`$y/月` 或不限量。
- 可访问范围：例如“可用 8 个分组”或“全部 OpenAI 订阅分组”。
- 如果用户已有同套餐 active entitlement，展示到期时间和用量摘要。

购买行为：

- 仍发送 `order_type = subscription` 和 `plan_id`。
- 多分组套餐购买时不需要用户选 group。

### 6.4 用户套餐权益页

修改：

- `frontend/src/views/user/SubscriptionsView.vue`
- `frontend/src/stores/subscriptions.ts`

用户侧文案改为“套餐额度”，但路由 `/subscriptions` 可以继续保留。

卡片展示：

- 权益名称
- 状态
- 到期时间
- daily/weekly/monthly 用量进度
- 可用分组列表
- 下次重置时间
- 超额策略

自动切组优先级 UI：

- 当前的“订阅分组优先级”改为“同一套餐内自动切组顺序”。
- 偏好设置维度从单纯 `group_id` 改为 `entitlement_id + group_id`。

### 6.5 API Key 页面

修改：

- `frontend/src/views/user/KeysView.vue`
- `frontend/src/api/keys.ts`

创建/编辑 API Key：

- 如果用户有多个 active entitlement，先选套餐权益。
- 再从该权益授权的分组中选择当前分组。
- 如果只有一张权益覆盖选中分组，自动选择权益。
- 切组时提示：切换的是路由/上游能力，不会增加套餐额度。

Payload：

```ts
{
  group_id: selectedGroupId,
  subscription_entitlement_id: selectedEntitlementId
}
```

---

## 7. API 兼容契约

继续保留现有接口：

- `GET /api/v1/subscriptions`
- `GET /api/v1/subscriptions/active`
- `GET /api/v1/subscriptions/progress`
- `POST /api/v1/subscriptions/:id/advance-monthly-cycle`
- `GET /api/v1/groups/available`
- `POST /api/v1/payment/orders`

兼容返回：

- `/subscriptions` 接口返回 entitlement 数据，同时填充旧字段：
  - `group_id`：primary group 或第一个 enabled group。
  - `group`：primary group 对象。
  - `groups`：新字段，完整授权分组列表。
  - `entitlement_id`：新字段，权益 ID。
- 旧客户端仍然可以按单 group 展示。
- 新客户端使用 `groups` 和 `entitlement_id`。

新增接口：

- `GET /api/v1/entitlements`
- `GET /api/v1/entitlements/active`
- `GET /api/v1/entitlements/:id/progress`
- `GET /api/v1/entitlements/:id/group-preferences`
- `PUT /api/v1/entitlements/:id/group-preferences`

建议：

- 新增接口正式承载 v2。
- 旧接口作为 alias 至少保留一个版本。

---

## 8. 上线阶段

### Phase A：加表加字段并回填

1. 增加 Ent schema 和 migration。
2. 运行 `go generate ./ent`。
3. 回填 plan groups 和 entitlements。
4. 暂不改变运行时行为。
5. 新增运行时设置：
   - `subscription_entitlements_v2_enabled`
   - `sub2_payment_page_legacy_mapping_enabled`
   - 默认 `false`。

### Phase B：双读

1. entitlement service 可以读取 v2 表。
2. 现有订阅接口返回 v2 字段。
3. v2 开关关闭时，API Key 可用分组仍按旧逻辑。
4. 管理端可以查看 entitlement 记录。

### Phase C：双写

1. 新购买创建 entitlement。
2. 兼容需要时，单分组场景仍写 legacy `user_subscriptions`。
3. usage log 开始写入 `entitlement_id`。
4. billing 同时支持旧 subscription id 和新 entitlement id。
5. 在 Phase A 回填之后、Phase C 双写之前产生的新增旧订阅，必须通过 catch-up backfill 补齐 entitlement。
6. 记录 catch-up backfill 影响行数，后续切换前必须为 0 或符合预期。

### Phase D：运行时切换

1. 最后一次执行 catch-up backfill。
2. 验证所有 active legacy subscription 都存在 entitlement 和 entitlement group。
3. 验证旧支付页 `group_id + validity_days + value` 全部存在 enabled external mapping。
4. 先开启 `subscription_entitlements_v2_enabled`。
5. 中间件改用 entitlement 解析。
6. API Key 绑定校验改用 entitlement coverage。
7. 共享额度正式生效。
8. 旧支付页映射验证通过后，再单独开启 `sub2_payment_page_legacy_mapping_enabled`。

### Phase E：后续清理

第一版不做：

- 删除旧 `user_subscriptions`。
- 删除旧 subscription routes。
- 删除 `payment_orders.subscription_group_id`。
- 删除 `usage_logs.subscription_id`。

---

## 9. 详细任务拆解

### Task 1：权益 v2 开关

**文件：**

- 修改：`backend/internal/service/setting.go`
- 修改：`backend/internal/service/setting_service.go`
- 修改：`backend/internal/handler/admin/setting_handler.go`
- 测试：`backend/internal/service/setting_service_get_all_test.go`
- 测试：`backend/internal/handler/admin/setting_handler_auth_source_defaults_test.go`

**步骤：**

1. 新增设置 key：`subscription_entitlements_v2_enabled`。
2. 新增设置 key：`sub2_payment_page_legacy_mapping_enabled`，默认 `false`。
3. 在 `SettingService` 增加 runtime getter。
4. 在管理员设置 DTO 中加入这两个字段。
5. 测试默认 false 和更新 true。
6. 运行：

```bash
cd backend
go test -tags=unit ./internal/service -run Setting
go test -tags=unit ./internal/handler/admin -run Setting
```

### Task 2：Ent Schema 和 SQL Migration

**文件：**

- 新建：`backend/ent/schema/subscription_plan_group.go`
- 新建：`backend/ent/schema/subscription_plan_external_mapping.go`
- 新建：`backend/ent/schema/subscription_entitlement.go`
- 新建：`backend/ent/schema/subscription_entitlement_group.go`
- 修改：`backend/ent/schema/subscription_plan.go`
- 修改：`backend/ent/schema/redeem_code.go`
- 修改：`backend/ent/schema/api_key.go`
- 修改：`backend/ent/schema/usage_log.go`
- 修改：`backend/ent/schema/payment_order.go`
- 新建：`backend/migrations/150_subscription_entitlements_v2.sql`
- 测试：`backend/migrations/auth_identity_payment_migrations_regression_test.go`

**步骤：**

1. 写 additive、idempotent 的迁移。
2. 增加 Ent schemas 和 edges。
3. 确认迁移包含：
   - entitlement 三张核心表。
   - `subscription_plan_external_mappings`，唯一键包含 `source + legacy_group_id + legacy_validity_days + legacy_value`。
   - `redeem_codes.plan_id`。
   - `subscription_entitlements.source_id/source_external_id/source_redeem_code_id` 以及唯一约束。
   - `api_keys.subscription_entitlement_id`、`usage_logs.entitlement_id`、`payment_orders.subscription_entitlement_id`。
4. 运行：

```bash
cd backend
go generate ./ent
go test -tags=unit ./migrations
```

5. 确认 `backend/ent/` 生成代码被提交。

### Task 3：Repository Port 和 Ent Repository

**文件：**

- 新建：`backend/internal/service/subscription_entitlement.go`
- 新建：`backend/internal/service/subscription_entitlement_port.go`
- 新建：`backend/internal/service/subscription_plan_external_mapping.go`
- 新建：`backend/internal/service/subscription_plan_external_mapping_port.go`
- 新建：`backend/internal/repository/subscription_entitlement_repo.go`
- 新建：`backend/internal/repository/subscription_plan_external_mapping_repo.go`
- 修改：`backend/internal/repository/wire.go`
- 测试：`backend/internal/repository/subscription_entitlement_repo_integration_test.go`
- 测试：`backend/internal/repository/subscription_plan_external_mapping_repo_integration_test.go`

**步骤：**

1. 定义 service 层模型和 repository interface。
2. 在 repository 层实现 Ent repository。
3. 覆盖测试：
   - 创建 entitlement 并绑定多个 groups。
   - 按 user 列出 active entitlements。
   - 查询覆盖某 group 的 active entitlements。
   - 替换授权 groups。
   - 原子检查并递增用量。
   - 按 `source + group_id + validity_days + value` 精确查询外部映射。
   - 金额不一致不命中映射。
4. entitlement repository 提供事务内发放/扣量所需端口，避免 service 自己直接依赖 Ent client。
5. 运行：

```bash
cd backend
go test -tags=integration ./internal/repository -run SubscriptionEntitlement
```

### Task 4：权益 Service 核心

**文件：**

- 新建：`backend/internal/service/subscription_entitlement_service.go`
- 新建：`backend/internal/service/subscription_entitlement_resolver.go`
- 新建：`backend/internal/service/subscription_entitlement_maintenance.go`
- 测试：`backend/internal/service/subscription_entitlement_service_test.go`
- 测试：`backend/internal/service/subscription_entitlement_resolver_test.go`

**步骤：**

1. 实现从 plan 分配/续期 entitlement。
2. 实现按 API Key 和 group 解析 active entitlement。
3. 实现基于 entitlement limits 的额度校验。
4. 实现 daily/weekly/monthly 窗口维护。
5. 实现事务内发放方法，例如 `AssignOrExtendFromPlanTx(txCtx, input)`，供 redeem/payment fulfillment 在已有事务中调用。
6. 实现来源幂等：
   - `source_redeem_code_id` 已存在时返回已有 entitlement，不重复续期。
   - `source_type + source_external_id` 已存在时返回已有 entitlement，不重复续期。
   - 同一支付订单/兑换码重试必须是 replay，不是二次发放。
7. 写测试：
   - 一张 entitlement 覆盖两个 group，monthly usage 共享。
   - entitlement 耗尽后不会通过切换 group 得到新额度。
   - API Key 显式 entitlement id 优先。
   - 权益未覆盖目标 group 时返回 `GROUP_NOT_ALLOWED`。
   - 同一 `source_redeem_code_id` 重试不重复续期。
   - 同一 `source_external_id` 重试不重复续期。
8. 运行：

```bash
cd backend
go test -tags=unit ./internal/service -run SubscriptionEntitlement
```

### Task 5：Wire 依赖注入

**文件：**

- 修改：`backend/internal/service/wire.go`
- 修改：`backend/cmd/server/wire.go`
- 生成：`backend/cmd/server/wire_gen.go`
- 生成测试：`backend/cmd/server/wire_gen_test.go`

**步骤：**

1. 增加 entitlement repository provider。
2. 增加 external mapping repository provider。
3. 增加 entitlement service provider。
4. 增加 external mapping service provider。
5. 将 entitlement service 注入 payment、API key、中间件路径、admin handlers、redeem service。
6. 将 external mapping service 注入 redeem service。
7. 运行：

```bash
cd backend
go generate ./cmd/server
go test -tags=unit ./cmd/server
```

### Task 6：支付套餐配置

**文件：**

- 修改：`backend/internal/service/payment_config_plans.go`
- 修改：`backend/internal/handler/admin/payment_handler.go`
- 修改：`frontend/src/types/payment.ts`
- 修改：`frontend/src/api/admin/payment.ts`
- 修改：`frontend/src/views/admin/orders/PlanEditDialog.vue`
- 修改：`frontend/src/views/admin/orders/AdminPaymentPlansView.vue`
- 测试：`backend/internal/service/payment_config_plans_validation_test.go`

**步骤：**

1. 扩展 create/update plan request：额度字段、access scope、group ids、allowed platforms、overage policy。
2. 校验 scope 和 group ids。
3. 持久化 `subscription_plan_groups`。
4. 更新管理端套餐编辑 UI。
5. 运行：

```bash
cd backend
go test -tags=unit ./internal/service -run PaymentConfig
cd ../frontend
pnpm run typecheck
```

### Task 7：支付履约和退款

**文件：**

- 修改：`backend/internal/service/payment_order.go`
- 修改：`backend/internal/service/payment_fulfillment.go`
- 修改：`backend/internal/service/payment_refund.go`
- 修改：`backend/internal/handler/admin/redeem_handler.go`
- 测试：`backend/internal/service/payment_fulfillment_test.go`
- 测试：`backend/internal/service/payment_refund_test.go`

**步骤：**

1. 支付成功后根据 plan 创建 entitlement。
2. 设置 `payment_orders.subscription_entitlement_id`。
3. 继续填充 legacy 字段保证兼容。
4. 退款优先按 entitlement id，旧 group fallback。
5. 外部支付页 create-and-redeem 的返利同步复用新的 source detection 结果；不要继续依赖 `code` 以 `s2p_` 开头或 notes 包含 `sub2apipay`。
6. 新增测试：
   - 覆盖两个 group 的 plan 创建一张 entitlement 且授权两个 group。
   - 续费同一张 entitlement，并按规则保留共享用量。
   - 支付平台 webhook 退款缩短 entitlement 有效期。
   - 管理员后台手动退款缩短 entitlement 有效期。
   - 只有 `subscription_group_id` 的旧订单仍可退款。
   - 外部支付页 `Idempotency-Key=s2p_*`、`code=auto_*`、`notes=Sub:*` 的订阅订单仍能正确同步返利审计。
7. 运行：

```bash
cd backend
go test -tags=unit ./internal/service -run 'PaymentFulfillment|PaymentRefund'
```

### Task 8：API Key 绑定和可用分组

**文件：**

- 修改：`backend/internal/service/api_key_service.go`
- 修改：`backend/internal/repository/api_key_repo.go`
- 修改：`backend/internal/handler/api_key_handler.go`
- 修改：`backend/internal/handler/available_channel_handler.go`
- 修改：`frontend/src/api/keys.ts`
- 修改：`frontend/src/views/user/KeysView.vue`
- 测试：`backend/internal/service/api_key_service_test.go`
- 测试：`backend/internal/handler/available_channel_handler_test.go`

**步骤：**

1. API Key 创建/更新 DTO 增加 `subscription_entitlement_id`。
2. 创建/更新时校验 entitlement coverage。
3. 返回 entitlement-aware 的可用分组列表。
4. 更新用户 API Key 页面，先选 entitlement，再选当前 group。
5. 运行：

```bash
cd backend
go test -tags=unit ./internal/service -run APIKey
go test -tags=unit ./internal/handler -run AvailableChannel
cd ../frontend
pnpm run typecheck
```

### Task 9：中间件和自动切组

**文件：**

- 修改：`backend/internal/server/middleware/api_key_auth.go`
- 修改：`backend/internal/server/middleware/api_key_auth_google.go`
- 测试：`backend/internal/server/middleware/api_key_auth_test.go`
- 测试：`backend/internal/server/middleware/api_key_auth_google_test.go`

**步骤：**

1. v2 开启时，订阅分组使用 entitlement 解析。
2. 设置 entitlement context key。
3. 自动切组只在同一张 entitlement 内切换。
4. 额度耗尽时不为寻找新额度而切组。
5. 支持 `balance_fallback`。
6. 运行：

```bash
cd backend
go test -tags=unit ./internal/server/middleware -run APIKeyAuth
```

### Task 10：计费和用量日志

**文件：**

- 修改：`backend/internal/service/usage_billing.go`
- 修改：`backend/internal/repository/usage_billing_repo.go`
- 修改：`backend/internal/service/gateway_service.go`
- 修改：`backend/internal/service/openai_gateway_service.go`
- 修改：`backend/internal/repository/usage_log_repo.go`
- 修改：`backend/internal/service/usage_log.go`
- 测试：`backend/internal/repository/usage_billing_repo_integration_test.go`
- 测试：gateway usage 相关测试。

**步骤：**

1. 给 usage billing command 增加 entitlement id。
2. entitlement id 存在时递增 entitlement 用量。
3. 持久化 `usage_logs.entitlement_id`。
4. 保留 `subscription_id` 兼容。
5. 新增测试：
   - 两个不同 group 递增同一张 entitlement。
   - usage log 的 group id 是实际 group。
   - entitlement id 正确落库。
6. 运行：

```bash
cd backend
go test -tags=integration ./internal/repository -run UsageBilling
go test -tags=unit ./internal/service -run Gateway
```

### Task 11：用户套餐权益 API

**文件：**

- 新建：`backend/internal/handler/entitlement_handler.go`
- 修改：`backend/internal/handler/wire.go`
- 修改：`backend/internal/server/routes/user.go`
- 修改：`frontend/src/api/subscriptions.ts`
- 修改：`frontend/src/stores/subscriptions.ts`
- 修改：`frontend/src/views/user/SubscriptionsView.vue`
- 测试：`backend/internal/handler/entitlement_handler_test.go`

**步骤：**

1. 新增 `/entitlements` 用户接口。
2. 保留 `/subscriptions` 作为兼容 alias。
3. 返回 groups 列表和共享额度进度。
4. 更新用户订阅页为套餐权益卡片。
5. 运行：

```bash
cd backend
go test -tags=unit ./internal/handler -run Entitlement
cd ../frontend
pnpm run typecheck
pnpm run test:run
```

### Task 12：管理员套餐权益管理

**文件：**

- 新建：`backend/internal/handler/admin/entitlement_handler.go`
- 修改：`backend/internal/handler/wire.go`
- 修改：`backend/internal/server/routes/admin.go`
- 新建：`frontend/src/api/admin/entitlements.ts`
- 新建或修改：`frontend/src/views/admin/` 下管理端权益页面。
- 测试：admin handler tests。

**步骤：**

1. 支持按 user、group、plan、status 筛选 entitlement。
2. 支持从 plan 分配 entitlement。
3. 支持续期、撤销、重置额度。
4. 支持替换授权分组。
5. 更新管理端 UI。
6. 运行：

```bash
cd backend
go test -tags=unit ./internal/handler/admin -run Entitlement
cd ../frontend
pnpm run typecheck
```

### Task 13：兑换码和默认赠送

**文件：**

- 修改：`backend/internal/service/redeem_service.go`
- 修改：`backend/internal/handler/admin/redeem_handler.go`
- 修改：`backend/internal/service/setting_service.go`
- 修改：`backend/internal/handler/admin/setting_handler.go`
- 新建：`backend/internal/service/subscription_plan_external_mapping_service.go`
- 新建或修改：对应 repository 接口与实现。
- 修改：redeem code service model、repository model、DTO 转换。
- 修改：`frontend/src/views/admin/SettingsView.vue`
- 修改：`frontend/src/views/user/RedeemView.vue`
- 测试：redeem 和 settings 相关测试。

**步骤：**

1. 写测试：`sub2-payment-page` 风格请求必须同时带 `Idempotency-Key: s2p_xxx`、`code=auto_xxx`、二者订单号后缀相同、`type=subscription`、`value>0`、`group_id=5`、`validity_days=30`，且开关开启、映射精确命中时，创建多分组 entitlement。
2. 写测试：金额不一致时不命中映射。
3. 写测试：只有 `notes: Sub: xxx`、但缺少 `Idempotency-Key` 或 `auto_` code 时，不命中映射。
4. 写测试：同样的旧支付页请求重放时，返回已有 entitlement，不重复续期。
5. 写测试：同样的 `group_id + validity_days + value` 没有命中映射时，继续走旧单分组兼容逻辑。
6. 写测试：普通管理员手动兑换码不带支付页强匹配特征时，不被 `sub2-payment-page` 映射误伤。
7. 写测试：旧支付页订阅请求仍能生成 referral/recharge 审计记录，`external_order_id` 来自 `s2p_` 和 `auto_` 的共同后缀。
8. 在 `CreateAndRedeemCodeRequest` 增加 `PlanID *int64`，并修改 handler 校验：
   - `type=subscription` 时允许 `plan_id` 或 `group_id` 二选一。
   - 普通请求同时传 `plan_id` 和 `group_id` 直接拒绝。
   - 旧支付页映射请求仍然只接收 `group_id`，由映射表解析出 `plan_id`。
9. 在 `CreateAndRedeem` handler 中读取 `Idempotency-Key`，将来源识别上下文传入 service；不要把 source 放进请求体。
10. 在 redeem service 中先做严格来源判断，再解析外部映射；命中后调用 entitlement service 的事务内方法，未命中才调用 legacy subscription facade。
11. 订阅型兑换码支持 `plan_id`，用于主服务后台和未来 API。
12. 默认订阅赠送支持基于 plan。
13. 设置 UI 优先展示 plan picker，并增加旧支付页映射开关。
14. 运行：

```bash
cd backend
go generate ./ent
go test -tags=unit ./internal/service -run 'Redeem|Setting'
go test -tags=unit ./internal/handler/admin -run 'Redeem|Setting'
cd ../frontend
pnpm run typecheck
```

### Task 14：报表和管理员用量筛选

**文件：**

- 修改：`backend/internal/repository/usage_log_repo.go`
- 修改：`backend/internal/handler/admin/usage_handler.go`
- 修改：`frontend/src/api/admin/usage.ts`
- 修改：`frontend/src/views/admin/UsageView.vue`

**步骤：**

1. 增加可选 `entitlement_id` 筛选。
2. 保留 `subscription_id` 筛选。
3. 用量详情中同时展示实际 group 和套餐权益。
4. 运行：

```bash
cd backend
go test -tags=unit ./internal/handler/admin -run Usage
cd ../frontend
pnpm run typecheck
```

### Task 15：端到端回归

**文件：**

- 新增或修改：`backend/internal/integration/` 下集成测试。
- 如现有模式合适，新增：`frontend/src/__tests__/integration/` 前端集成测试。

**场景：**

1. 用户购买多分组套餐。
2. API Key 绑定 entitlement + group A。
3. 通过 group A 请求，entitlement usage 增加。
4. 用户切换同一个 API Key 到 group B。
5. 通过 group B 请求，仍然增加同一张 entitlement 的 usage。
6. 套餐额度耗尽后，group A 和 group B 都被拦截。
7. 如果 overage policy 是 balance fallback，则下一次请求扣账户余额。
8. 旧单分组订阅仍然正常工作。

运行：

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

### Task 16：文档和运维手册

**文件：**

- 修改：`docs/PAYMENT.md`
- 修改：`docs/PAYMENT_CN.md`
- 如 README 中有相关说明，修改：`README_CN.md`
- 新建：`docs/SUBSCRIPTION_ENTITLEMENTS_V2_RUNBOOK.md`

Runbook 必须包含：

- 如何创建多分组套餐。
- 如何迁移旧套餐。
- 如何盘点旧支付页 `group_id + validity_days + value` 套餐元组。
- 如何创建和验证 `subscription_plan_external_mappings`。
- 如何执行切换前最后一次 catch-up backfill。
- 如何启用 v2 开关。
- 如何单独启用/关闭旧支付页映射开关。
- 如何验证回填数量。
- 如何回滚运行时开关。
- 如何排查某个用户的权益和 usage logs。

---

## 10. 测试矩阵

后端 unit：

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

后端 integration：

- Ent repositories
- usage billing 原子递增
- migration 幂等性
- catch-up backfill 幂等性
- purchase 到 entitlement 创建完整链路
- webhook 退款和后台退款都能扣减 entitlement
- external mapping miss 不会误开多分组权益

前端：

- plan editor validation
- purchase page 展示额度和授权范围
- key page entitlement/group 选择
- subscriptions page 展示共享额度

手动 QA：

1. 创建两个订阅型分组，配置不同 route 行为。
2. 创建一个覆盖这两个分组的套餐，monthly limit 设置为 1.00 USD。
3. 用户购买套餐。
4. 创建 API Key 绑定 group A。
5. 发送请求，确认 `usage_logs.group_id = A` 且 `entitlement_id = E`。
6. 切换 API Key 到 group B。
7. 发送请求，确认仍然是同一个 `entitlement_id = E`。
8. 消耗完 monthly limit。
9. 按配置验证：两个分组都拦截，或进入账户余额兜底。

---

## 11. 生产上线 Checklist

上线前：

- `go test -tags=unit ./...` 通过。
- `go test -tags=integration ./...` 通过。
- `golangci-lint run ./...` 通过。
- `pnpm run lint:check` 通过。
- `pnpm run typecheck` 通过。
- `pnpm run test:run` 通过。
- `pnpm run build` 通过。
- 在 staging 数据库副本验证 migration。
- 记录回填行数。
- 从当前 `sub2-payment-page` 配置或线上订单中盘点实际售卖套餐元组：`group_id + validity_days + value`。
- 为每个外部支付页套餐元组创建 `subscription_plan_external_mappings`，并记录映射到的 `plan_id`。
- 在 staging 验证：每个外部支付页套餐元组都能精确命中映射，金额不一致不会命中。

上线：

1. 部署代码，v2 开关保持关闭。
2. 执行 migrations。
3. 执行初始 backfill。
4. 启用双写路径，但保持 `subscription_entitlements_v2_enabled=false`。
5. 等待至少一个支付/兑换低风险窗口，确认新增购买同时写入 entitlement。
6. 切换前执行最后一次 catch-up backfill。
7. 验证：
   - `subscription_entitlements` 数量 >= active legacy subscriptions。
   - 每个 active legacy subscription 都有对应 entitlement group。
   - 订阅型 group 的 active API Key 已回填 entitlement id。
   - catch-up backfill 本次新增行数为 0，或全部能解释。
   - 每个旧支付页套餐元组都有 enabled external mapping。
8. 如果支持 per-user flag，先给测试用户开启；否则先在 staging 验证。
9. 低峰期开启 `subscription_entitlements_v2_enabled`。
10. 验证旧支付页小额测试订单命中 external mapping 后，再开启 `sub2_payment_page_legacy_mapping_enabled`。
11. 观察：
   - 403/429 比例
   - payment fulfillment errors
   - usage billing errors
   - API key auth conflict errors
   - cache invalidation errors
   - external mapping miss count
   - entitlement source duplicate/conflict count

回滚：

1. 关闭 `subscription_entitlements_v2_enabled`。
2. 关闭 `sub2_payment_page_legacy_mapping_enabled`。
3. 保留新表和新字段。
4. 旧 `user_subscriptions` 和 balance billing 继续工作。
5. 不删除列，不执行破坏性回滚。

---

## 12. 推荐提交拆分

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

每个提交都应能编译，并通过对应模块的窄范围测试。

---

## 13. 最终架构总结

改造完成后：

- `subscription_plans` 描述可售卖的套餐商品。
- `subscription_plan_groups` 描述套餐可以授权哪些分组。
- `subscription_entitlements` 是用户实际持有的套餐额度账本。
- `subscription_entitlement_groups` 是用户这张权益的实际授权分组快照。
- `api_keys.group_id` 继续表示当前路由分组。
- `api_keys.subscription_entitlement_id` 指向套餐额度账本。
- `usage_logs.group_id` 记录实际使用的分组。
- `usage_logs.entitlement_id` 记录实际被扣量的套餐权益。
- `users.balance` 只表示现金账户余额。

这就是“一步到位”的目标架构，同时仍然允许安全灰度、兼容旧数据、快速回滚。
