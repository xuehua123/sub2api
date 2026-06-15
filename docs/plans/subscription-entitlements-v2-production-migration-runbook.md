# Subscription Entitlements V2 Production Migration Runbook

本文件是套餐权益 V2 生产发布和迁移的最终执行手册。执行时以本文件为准，不从聊天记录里找命令。

目标：在不改变老用户额度、已用量、有效期、余额账本和 API Key 可用性的前提下，把旧 `user_subscriptions` 用户迁移到 V2 `subscription_entitlements`，并在可控维护窗口内完成切换。

## 最终原则

1. **先部署，后迁移，最后开 V2。** 不允许先开 V2 再迁移。
2. **只备份一次。** 进入维护窗口、冻结所有写入口之后，做一次生产 DB 备份。
3. **维护窗口内不重新设计方案。** 生产 dry-run 和 preflight 必须提前跑完，所有 blocker 必须提前解释。
4. **不清历史账。** 不清 `usage_logs`，不绕 guard，不删除 entitlement / fulfillment / plan / mapping。
5. **legacy mapping 单独上线。** 本轮只开 `subscription_entitlements_v2_enabled`，`sub2_payment_page_legacy_mapping_enabled` 继续保持 false。

## 当前迁移工具

主工具：

```bash
cd <repo-dir>/backend
go run -mod=readonly ./cmd/legacy-entitlement-backfill -h
```

目标 commit 必须支持：

- `dry-run`
- `snapshot`
- `apply`
- `resume-api-keys`
- `reconcile`
- `rollback`

如果 `-h` 里没有 `resume-api-keys`，停止上线。

## 绝对禁止

- 不在生产业务机 Docker build。
- 不清空或修改 `usage_logs` 来绕过 guard。
- 不手写 SQL 迁 API Keys 来绕过工具。
- 不把 `entitlement_id` 写进任何 legacy `subscription_id`。
- 不复制多条 `user_subscriptions` 来模拟多分组。
- 不开启 `sub2_payment_page_legacy_mapping_enabled`。
- 不提交 evidence、DB dump、token、API key、provider key、payment secret。

## 生产前置输入

| 项 | 值 |
| --- | --- |
| Target commit | `<commit>` |
| Production repo dir | `<repo-dir>` |
| Backend dir | `<repo-dir>/backend` |
| DATABASE_URL source | `<env var / secret manager>` |
| Mapping version | `production-legacy-backfill-YYYYMMDD-v1` |
| Evidence dir | `<secure-evidence-dir>` |
| DB backup file | `<backup-file.pgcustom>` |
| Rollback image/tag | `<image-or-artifact>` |
| Release owner | `<name>` |
| DB owner | `<name>` |
| Rollback owner | `<name>` |
| Maintenance window | `<time range>` |

## 维护窗口必须冻结的入口

进入维护窗口后，以下入口全部暂停。不要只停 gateway。

### 用量 / 余额 / 套餐扣费写入

- `/v1/messages`
- OpenAI compatible routes
- Gemini routes
- Anthropic routes
- Antigravity routes
- 所有会写 `usage_logs`、扣余额、扣套餐额度的请求

### 支付 / 购买 / 兑换

- 创建订单
- 支付回调 fulfillment
- native payment fulfillment
- 用户 redeem
- admin create-and-redeem
- negative redeem / refund-like redeem

### API Key 写操作

- 创建 API Key
- 编辑 API Key
- 删除 / 禁用 API Key
- 切换 group
- 切换 access source
- 绑定 / 解绑 entitlement

### 管理员订阅写操作

- 分配订阅
- 批量分配订阅
- 延长订阅
- 撤销订阅
- 删除订阅
- 提前使用下个月
- 重置额度
- 手动 reset quota
- refund 导致的缩短订阅

### 配置写操作

- 创建 / 编辑 / 删除 group
- 修改 group capability
- 修改 `balance_enabled`
- 修改 `subscription_enabled`
- 修改 `plan_auto_grant_enabled`
- 修改 plan
- 修改 plan groups
- 修改 account
- 修改 account_groups
- 修改 schedulable
- 修改 rate multiplier
- 修改 overage policy
- 修改平台路由相关配置

## 可以继续的入口

如果维护页/路由允许，这些只读入口可以继续；如果实现成本高，也可以全站维护。

- 登录
- 首页和静态资源
- 用户只读查询
- 管理员只读列表
- 查看余额
- 查看订阅
- 查看 API Key 列表

## 时间目标

推荐维护窗口：30 分钟。

实际停写时间取决于 DB 备份耗时：

- 如果备份 2-5 分钟：整体 10-20 分钟通常够。
- 如果备份超过 15 分钟：维护窗口应扩大到 60 分钟。

不要在不知道备份耗时的情况下承诺 5 分钟完成。

## 阶段 A：发布代码，不开 V2

这一步不进入维护窗口。

要求：

- 使用预构建 artifact / CI image。
- 生产业务机只允许 pull / restart / health check。
- 不在生产业务机 Docker build。

部署后确认：

```bash
curl -fsS https://xxxaicode.com/health
```

确认两个 flag 都是 false：

```sql
SELECT key, value
FROM settings
WHERE key IN (
  'subscription_entitlements_v2_enabled',
  'sub2_payment_page_legacy_mapping_enabled'
)
ORDER BY key;
```

期望：

```text
sub2_payment_page_legacy_mapping_enabled = false
subscription_entitlements_v2_enabled = false
```

Go / No-Go：

- health 不正常，回滚代码。
- 任一 flag 不是 false，恢复 false 后再继续。

## 阶段 B：生产 dry-run / preflight，不停机

这一步只读，不进入维护窗口。

设置变量：

```bash
export MAPPING_VERSION="production-legacy-backfill-YYYYMMDD-v1"
export EVIDENCE_DIR="<secure-evidence-dir>/${MAPPING_VERSION}"
export PGOPTIONS="-c default_transaction_read_only=on -c statement_timeout=300000"
mkdir -p "$EVIDENCE_DIR"
cd <repo-dir>/backend
```

执行 dry-run：

```bash
go run -mod=readonly ./cmd/legacy-entitlement-backfill \
  -mode dry-run \
  -env production \
  -database-url "$DATABASE_URL" \
  -mapping-version "$MAPPING_VERSION" \
  -output-dir "$EVIDENCE_DIR" \
  -timeout 5m
```

必须记录：

- `runtime_group_candidates`
- `plan_candidates`
- `active_legacy_subscriptions`
- `api_key_auto_migration_candidates`
- `ambiguous_api_keys`
- `review_reasons`
- 是否存在 no schedulable account pool

执行 preflight SQL：

- `docs/plans/subscription-entitlements-v2-preflight-sql.md`

必须为 0：

- invalid API key entitlement bindings
- alias-eligible entitlement missing legacy subscription
- invalid fallback usage rows

需要人工解释：

- multi-active entitlement coverage
- ambiguous API keys
- exclusive / private / test group review

Go / No-Go：

- 有无法解释的 blocker，不进入维护窗口。
- dry-run 结果必须保存到 evidence。

## 阶段 C：进入维护窗口，冻结所有写入口

从这里开始算维护时间。

执行：

1. 启用维护策略，冻结“维护窗口必须冻结的入口”。
2. 确认新 gateway 请求不会继续写 `usage_logs`。
3. 确认支付、兑换、API Key 写操作、管理员订阅写操作、配置写操作已暂停。
4. 确认后台没有明显长时间运行的 gateway 请求。
5. 再次确认两个 flag 仍为 false/false。

如果任一写入口无法暂停，停止迁移。

## 阶段 D：只备份一次生产 DB

维护窗口内，只做这一份迁移前备份。

示例：

```bash
pg_dump --format=custom --file "<backup-file.pgcustom>" "$DATABASE_URL"
```

备份完成后必须确认：

- 文件存在。
- 文件大小合理。
- DB owner 确认该备份可用于恢复。
- 备份路径写入 evidence。

Go / No-Go：

- 备份失败，不执行 snapshot/apply。

## 阶段 E：API Key snapshot

写模式前去掉只读 PGOPTIONS：

```bash
export PGOPTIONS="-c statement_timeout=300000"
```

执行：

```bash
go run -mod=readonly ./cmd/legacy-entitlement-backfill \
  -mode snapshot \
  -env production \
  -database-url "$DATABASE_URL" \
  -mapping-version "$MAPPING_VERSION" \
  -output-dir "$EVIDENCE_DIR" \
  -execute \
  -confirm-production CONFIRM_PRODUCTION_LEGACY_ENTITLEMENT_BACKFILL \
  -timeout 5m
```

必须满足：

- `missing_snapshot_api_keys = 0`
- `captured_api_keys + reused_existing_snapshots = covered_api_keys`

如果 snapshot coverage 不完整，停止并保持 V2 关闭。

## 阶段 F：执行迁移

优先执行 `apply`：

```bash
go run -mod=readonly ./cmd/legacy-entitlement-backfill \
  -mode apply \
  -env production \
  -database-url "$DATABASE_URL" \
  -mapping-version "$MAPPING_VERSION" \
  -output-dir "$EVIDENCE_DIR" \
  -execute \
  -confirm-production CONFIRM_PRODUCTION_LEGACY_ENTITLEMENT_BACKFILL \
  -timeout 10m
```

成功后记录：

- `created_runtime_groups`
- `created_plans`
- `created_mappings`
- `updated_entitlements`
- `updated_entitlement_grants`
- `upserted_fulfillments`
- `updated_api_keys`

如果 `apply` 因目标 entitlement 已有 usage log 停止：

1. 不清 `usage_logs`。
2. 不换 mapping version 强跑。
3. 不手写 SQL 切 API Keys。
4. 改用 `resume-api-keys`：

```bash
go run -mod=readonly ./cmd/legacy-entitlement-backfill \
  -mode resume-api-keys \
  -env production \
  -database-url "$DATABASE_URL" \
  -mapping-version "$MAPPING_VERSION" \
  -output-dir "$EVIDENCE_DIR" \
  -execute \
  -confirm-production CONFIRM_PRODUCTION_LEGACY_ENTITLEMENT_BACKFILL \
  -timeout 5m
```

`resume-api-keys` 必须只更新 `api_keys`，不能改 entitlement、grants、fulfillment、usage_logs。

如果工具输出 `restart_required=true`，下一阶段必须清 auth cache 或重启服务。

## 阶段 G：清 auth cache / 重启服务

目标：避免 API Key DB 已迁移，但运行时仍读旧缓存。

执行项目当前支持的 cache invalidation。如果没有独立 cache invalidation，就重启 Sub2API 服务。

确认：

```bash
curl -fsS https://xxxaicode.com/health
```

health 不通过，不允许开 V2。

## 阶段 H：reconcile / preflight

执行：

```bash
go run -mod=readonly ./cmd/legacy-entitlement-backfill \
  -mode reconcile \
  -env production \
  -database-url "$DATABASE_URL" \
  -mapping-version "$MAPPING_VERSION" \
  -output-dir "$EVIDENCE_DIR" \
  -timeout 5m
```

必须为 0：

- `entitlement_legacy_usage_mismatch`
- `missing_entitlement_runtime_grant`
- `missing_backfill_fulfillment`
- `api_keys_without_snapshot_on_runtime_group`

再跑 preflight SQL 关键项：

- invalid API key entitlement bindings = 0
- alias-eligible entitlement missing legacy subscription = 0
- invalid fallback usage rows = 0

Go / No-Go：

- 任一必须为 0 的项不为 0，不开 V2。
- 如需回滚，先执行 rollback，再恢复旧入口。

## 阶段 I：开启 V2

只开启：

```text
subscription_entitlements_v2_enabled=true
```

继续保持：

```text
sub2_payment_page_legacy_mapping_enabled=false
```

开启后立即查询确认：

```sql
SELECT key, value
FROM settings
WHERE key IN (
  'subscription_entitlements_v2_enabled',
  'sub2_payment_page_legacy_mapping_enabled'
)
ORDER BY key;
```

## 阶段 J：生产 smoke

生产 smoke 要少量、低成本，不打印 API key。

最小检查：

- 老订阅迁移用户 API key 请求成功，`billing_source='entitlement_quota'`。
- 余额 API key 请求成功，`billing_source='balance'`。
- fallback success 请求成功，`billing_source='entitlement_balance_fallback'`。
- fallback insufficient 返回 403，不写成功 usage log。
- quota exceeded 429 只在已有安全低额 fixture 时测；没有 fixture 不临时污染生产。
- `/entitlements` 不泄露 `source/notes/fulfillment`。
- `/subscriptions` alias 不伪造 legacy id。
- 管理员用户订阅能看到 plan/entitlement 关联，且不泄露 notes/source/fulfillment。

Smoke 不通过，立即关闭 V2。

## 阶段 K：恢复服务

恢复：

- gateway 请求
- 支付 / 购买
- 兑换
- API Key 创建 / 编辑 / 切组
- 管理员订阅写操作
- group / plan / account 配置写操作

继续保持：

```text
sub2_payment_page_legacy_mapping_enabled=false
```

观察至少 30-60 分钟：

- 5xx / error rate
- 429 rate
- `billing_source` 分布
- fallback balance deduction
- invalid API key entitlement binding
- 用户反馈

## 回滚

优先回滚路径：

1. 关闭 `subscription_entitlements_v2_enabled=false`。
2. 确认 `sub2_payment_page_legacy_mapping_enabled=false`。
3. 如需恢复 API keys 到旧 group，执行：

```bash
go run -mod=readonly ./cmd/legacy-entitlement-backfill \
  -mode rollback \
  -env production \
  -database-url "$DATABASE_URL" \
  -mapping-version "$MAPPING_VERSION" \
  -output-dir "$EVIDENCE_DIR" \
  -execute \
  -confirm-production CONFIRM_PRODUCTION_LEGACY_ENTITLEMENT_BACKFILL \
  -timeout 5m
```

4. 清 auth cache 或重启服务。
5. 恢复旧逻辑入口。

不要删除：

- `subscription_entitlements`
- entitlement grants
- fulfillment history
- generated plans
- runtime groups
- mapping/snapshot/control tables

只有数据库整体损坏或无法解释时，才走 DB restore。

## 最终 Go / No-Go

可以开 V2 的条件：

- 目标代码已部署。
- 两个 flag 在迁移前是 false/false。
- dry-run 无未解释 blocker。
- preflight 必须为 0 的项为 0。
- 维护窗口已开始，所有写入口已冻结。
- 生产 DB 备份成功且可恢复。
- snapshot coverage 完整。
- apply 或 resume-api-keys 通过。
- auth cache 已刷新。
- reconcile 必须为 0 的项全为 0。
- rollback owner 在线。

任何一项不满足，都不要开 V2。
