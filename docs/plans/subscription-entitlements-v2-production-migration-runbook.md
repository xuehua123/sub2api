# Subscription Entitlements V2 Production Migration Runbook

本文件是生产迁移当天的唯一执行索引。不要从聊天记录里找命令。

目标：在不改变老用户额度、已用量、有效期和余额账本的前提下，把旧 `user_subscriptions` 用户迁移到 V2 `subscription_entitlements`，再开启 V2。

## 一句话顺序

部署新代码但保持两个 flag 关闭 -> 备份生产 DB -> dry-run -> snapshot -> apply 或 resume API keys -> reconcile -> 抽查 -> 开 V2 flag -> smoke -> 保留回滚路径。

不要先开 V2 再迁移。

## 绝对禁止

- 不在生产业务机 Docker build。
- 不清空或修改 `usage_logs` 来绕过 guard。
- 不绕过 backfill 工具的 guard。
- 不把 `entitlement_id` 写进任何 legacy `subscription_id`。
- 不复制多条 `user_subscriptions` 来模拟多分组。
- 不开启 `sub2_payment_page_legacy_mapping_enabled`，除非 V2 native 迁移和 smoke 已通过。
- 不把 evidence、DB dump、token、API key、provider key、payment secret 提交到仓库。

## 生产前置输入

上线前把下面信息填好，敏感值只保存在安全通道或服务器环境变量中。

| 项 | 值 |
| --- | --- |
| Target commit | `<commit>` |
| Production repo dir | `<repo-dir>` |
| Backend dir | `<repo-dir>/backend` |
| Production DB backup path | `<backup-file.pgcustom>` |
| DATABASE_URL source | `<env var / secret manager>` |
| Mapping version | `production-legacy-backfill-YYYYMMDD-v1` |
| Evidence dir | `<secure-evidence-dir>` |
| Rollback owner | `<name>` |
| Maintenance window | `<time range>` |

## 使用的工具

主工具：

```bash
cd <repo-dir>/backend
go run -mod=readonly ./cmd/legacy-entitlement-backfill -h
```

当前已支持模式：

- `dry-run`
- `snapshot`
- `apply`
- `reconcile`
- `rollback`

如果生产遇到“目标 entitlement 已有 usage_logs，但 API keys 尚未迁移”的场景，需要使用 `resume-api-keys` 或等价安全模式。若目标 commit 的 `-h` 里没有该模式，必须停止，等后端补齐后再继续。

辅助只读盘点：

```bash
tools/subscription_entitlements_v2_legacy_backfill_dry_run.ps1
docs/plans/subscription-entitlements-v2-preflight-sql.md
```

## 0. 部署新代码，但不要开 V2

部署目标 commit 对应的预构建 artifact。生产机器只允许 pull / restart / health check，不允许 build。

部署后确认：

```bash
curl -fsS https://xxxaicode.com/health
```

确认运行版本和 commit 的方式以生产部署系统为准。不要只看公开版本号，因为版本号可能没有随 commit 变化。

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

## 1. 备份生产 DB

必须先备份，再执行任何写模式。

示例：

```bash
pg_dump --format=custom --file "<backup-file.pgcustom>" "$DATABASE_URL"
```

备份完成后确认文件存在、大小合理，并记录路径到 evidence。

## 2. 设置执行变量

示例：

```bash
export MAPPING_VERSION="production-legacy-backfill-YYYYMMDD-v1"
export EVIDENCE_DIR="<secure-evidence-dir>/${MAPPING_VERSION}"
export PGOPTIONS="-c statement_timeout=300000"
mkdir -p "$EVIDENCE_DIR"
cd <repo-dir>/backend
```

生产只读 dry-run 可以额外设置只读保护：

```bash
export PGOPTIONS="-c default_transaction_read_only=on -c statement_timeout=300000"
```

写模式前必须去掉 `default_transaction_read_only=on`。

## 3. 生产 dry-run

只读，不写 DB。

```bash
go run -mod=readonly ./cmd/legacy-entitlement-backfill \
  -mode dry-run \
  -env production \
  -database-url "$DATABASE_URL" \
  -mapping-version "$MAPPING_VERSION" \
  -output-dir "$EVIDENCE_DIR" \
  -timeout 5m
```

必须检查：

- `runtime_group_candidates`
- `plan_candidates`
- `active_legacy_subscriptions`
- `api_key_auto_migration_candidates`
- `ambiguous_api_keys`
- `review_reasons`
- 是否存在 no schedulable account pool

如果有无法解释的 blocker，停止。

## 4. API Key snapshot

写 snapshot 表。此时两个 flag 必须仍是 false。

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

必须检查：

- `covered_api_keys > 0`，除非生产确实没有待迁 API key。
- `missing_snapshot_api_keys = 0`
- `captured_api_keys + reused_existing_snapshots = covered_api_keys`

如果 snapshot coverage 不完整，停止。

## 5. apply 迁移

创建或复用 mapping、runtime group、plan、entitlement、grant、fulfillment，并迁移可自动迁移的 API keys。

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

如果 apply 因已有目标 entitlement usage log 停止：

1. 不清 `usage_logs`。
2. 不换 mapping version 强跑。
3. 不手写 SQL 临时迁 API keys。
4. 使用目标 commit 提供的 `resume-api-keys` 或等价安全模式。

`resume-api-keys` 要求：

- 只更新 `api_keys`。
- 不改 `subscription_entitlements`。
- 不改 entitlement grants。
- 不改 fulfillment history。
- 不改 `usage_logs`。
- 必须验证 snapshot coverage 完整。
- 必须处理 API key auth cache，或明确要求重启服务。

如果工具没有该模式，停止上线。

## 6. reconcile

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

再跑 `docs/plans/subscription-entitlements-v2-preflight-sql.md` 中的只读检查，至少确认：

- invalid API key entitlement bindings = 0
- alias-eligible entitlement missing legacy subscription = 0
- invalid fallback usage rows = 0
- multi-active entitlement 覆盖均可解释

## 7. 抽查

至少抽查：

- 一个老订阅用户：存在 linked entitlement。
- 一个已迁 API key：`access_source='entitlement'` 且 `subscription_entitlement_id` 正确。
- 一个余额 API key：`access_source='balance'` 且 `subscription_entitlement_id IS NULL`。
- 旧订阅的 `expires_at`、daily/weekly/monthly usage 和 entitlement 一致。
- `/entitlements` 不暴露 source notes / fulfillment / internal 字段。
- `/subscriptions` alias 不伪造 legacy subscription id。

## 8. 开启 V2 flag

只开：

```text
subscription_entitlements_v2_enabled=true
```

继续保持：

```text
sub2_payment_page_legacy_mapping_enabled=false
```

建议通过后台设置/API 操作，不建议手写 SQL。操作后立即查询 `settings` 确认。

## 9. smoke

最小 smoke：

- 老订阅用户 API key 请求成功，扣 entitlement quota。
- 余额 API key 请求成功，扣 balance。
- quota exceeded 返回 429，不写成功 usage log，不扣余额。
- fallback success 扣余额并写 `billing_source='entitlement_balance_fallback'`。
- fallback insufficient 返回 403，不写成功 usage log，余额不为负。
- 用户订阅页显示套餐权益和时间正确。
- 管理员订阅管理能看到 plan/entitlement 关联。

生产 smoke 请求要控制成本，API key 不得打印到终端或 evidence。

## 10. 回滚

优先回滚：

1. 关闭 `subscription_entitlements_v2_enabled=false`。
2. 关闭 `sub2_payment_page_legacy_mapping_enabled=false`。
3. 如 API keys 需要恢复旧 group，执行 rollback：

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

rollback 只恢复 API keys。不要删除 entitlement、fulfillment、plan、runtime group 或 schema。

如果数据库整体异常，再按生产 DB 备份恢复流程处理。

## Go / No-Go

可以开 V2 的条件：

- 新代码已部署。
- 两个 flag 在迁移前为 false/false。
- DB 已备份。
- dry-run 通过。
- snapshot coverage 完整。
- apply 或 resume-api-keys 通过。
- reconcile 全 0。
- preflight SQL 必须为 0 的项均为 0。
- smoke 准备完成。
- 回滚负责人在线。

任何一项不满足，都不要开 V2。
