# 上游 v0.2.14 合并审查

日期：2026-10-07。

## 来源与版本

- fork 基线：`c05d2b0ceeeb109a67ace1d645ba59d22c2840ac`（v0.2.13）。
- 上游 v0.2.14 注释标签对象：`1400a7b482974d98db5b284a8b2afbe3eaf9aaef`；标签提交：`0363b8cdba8cec3e2ba4b2dbd49c4481143fa55d`。
- 合并后上游版本同步提交：`3f1a2ea0a760730e3bc528105c00b4ee4f23e469`。最终 `backend/cmd/server/VERSION` 为 `0.2.14`。
- 保留 fork 定制；发布标签须在全部 CI 通过后指向最终 `main` 提交，不移动上游标签。

## 合并内容与风险

- 合入 EasyPay 回调参数白名单、签名防重放及 `return_url` 查询参数清理。
- 加固首次安装管理员账户生成和凭据校验，避免可猜测默认凭据。
- 加入 Codex API Key 模式远程模型发现能力。
- 更新 Vue 与 source-map-js 依赖审计修复。
- 修复 fork 的 pnpm 锁文件孤立解析，并显式声明测试运行所需的 `@vue/compiler-dom` 与 `@vue/server-renderer`；全新冻结安装后测试套件可完整收集。
- 相对 v0.2.13 无数据库迁移、Ent schema 或 Wire 注入变更；生产迁移数量应保持 340。
- EasyPay 回调属于支付安全边界；管理员初始化属于账户安全边界。按关键业务发布执行，稳如狗与 Canada 均需约 30 分钟观察。

## 本地验证

- 后端：`go test -tags=unit ./...`、`go test -tags=integration ./...`、`golangci-lint run ./...` 通过。
- 前端：`pnpm install --frozen-lockfile --ignore-scripts` 通过；完整 Vitest 420 个 suite、3643 个测试通过。最终依赖提交上的 lint、typecheck、build 正在复核。
- 密钥扫描、GitHub CI、安全扫描及镜像构建需在最终 `main` 提交上通过后，方可部署。
- 部署前核对 `VERSION`、fork 发布标签、OCI version/revision、trusted-ci 标签和镜像 digest；只部署该精确提交 CI 生成的不可变镜像。

## 渐进发布门禁

1. 稳如狗先蓝绿部署并验证健康、运行时版本、迁移与支付回调行为；观察约 30 分钟。
2. 加拿大发布前确认其为可写 PostgreSQL 主库和 Redis master，法国复制追平，美国转发正常，且 Canada 仅有一个活跃应用槽。实时核对槽位后选择 inactive slot，记录镜像、Compose、Nginx、迁移状态和健康基线，再蓝绿切换并观察约 30 分钟。
3. 失败时依记录的旧镜像和 Nginx upstream 回滚应用；常规回滚不改 DNS 或数据库角色。
4. 法国只缓存同一镜像并维持数据库/Redis 副本、应用停止；美国仅核验中继，不部署应用或下发秘密。继续跟踪美国主机内核验限制，未获得可信登录前不尝试绕过主机密钥或认证。
5. Prompt Audit 保持默认关闭；不在生产主机构建镜像，不改 DNS。

## 发布状态

审查分支尚待推送、合入最新 `origin/main` 并等待全部 CI 和不可变镜像构建。生产发布尚未开始；每个生产槽位、运行镜像 digest、观察指标和最终结果应追加至对应发布记录。
