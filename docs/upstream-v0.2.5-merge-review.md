# 上游 v0.2.5 合并审查记录

日期：2026-09-15。范围：本地隔离合并、冲突处理、回归和发布前审查；未授权推送主分支或操作生产。

## 基线与版本

- 实际 origin：xuehua123/sub2api。AGENTS.md 中旧 fork 名称不用于改写 remote。
- 基线 origin/main：09e77c65cdeded881f7dcdafe8e2a8c141cbcc4f；本轮结束前远程只读复核仍为该提交。
- 目标：Wei-Shaw/sub2api 的 v0.2.5，精确提交 86f93c28ee34cc74b629dafb748bd5ac5ca8c5ea。
- 工作分支：codex/merge-upstream-v0.2.5。
- 隔离工作目录：D:/IdeaProjects/xxxaicode/sub2api-merge-v0.2.5。
- 原目录 D:/IdeaProjects/xxxaicode/sub2api 未切换分支，原有未跟踪目录不动。
- 上游标签内 backend/cmd/server/VERSION 实际为 0.2.4。完成目标内容合并、解决索引冲突后，单独将该文件同步为 0.2.5；不是提前升级未合并分支。
- 本地当前仍是未提交的合并候选；没有最终 merge commit、CI 镜像 digest 或生产运行版本证明。不得据此宣称已发布。

## 冲突与兼容决策

初始 45 个冲突路径：43 个内容冲突已解决，2 个修改/删除冲突保持 fork 删除。完整暂存差异应与本报告一同复核。

1. 保留本地设置、模型价格可见性、返利和 LobeHub 字段；合入上游订阅显示和余额支付开关。
2. 保留套餐 PlanID、权益别名、权益独立订阅和事务流程。批量分配复用单项套餐流程，而不是直接调用上游按分组分配的旧路径；同步前后端 plan_id。
3. 批量延长、重置、撤销、恢复支持负数权益合成 ID，沿用权益服务自己的事务和缓存失效，不把权益 ID 交给旧订阅仓库。新增成功生命周期、重复 ID、套餐关联及重放回归。
4. 权益独立分配的复用统计使用实际结果。继承原有续期规则：新的业务分配可续期；HTTP 幂等键与业务“续期”不是同一概念。
5. 保留本地价格阶梯、长上下文缓存倍率、用量成本展示，合入图片缓存 token 计费及上游模型计价变更。
6. 保留 WS compaction 和账号/API Key 范围的 turn-state 来源校验；合入上游 execution scope/pool 变更。独立会话不复用握手来源状态。
7. 新 Images 直连路径继续经过本地受信图片 URL 归一化；同步模型映射和 JSON 载荷测试。
8. 不恢复已被本地集中监控取代的 upstream_billing_probe.go 及对应旧多平台测试。新 Ollama 测试改用现存仓库测试夹具。
9. 保留用户用量页本地布局、错误标签页、人民币/实际成本、compaction 筛选；接入模型、分组、请求类型和计费筛选，CSV 导出固定查询快照。模型选项来自选定范围的模型统计，不局限当前分页；允许手动输入并保留选中值。
10. KeysView 保留权益可访问分组和自定义创建流程；PaymentView 保留本地套餐卡片，正确处理订阅/余额支付禁用。
11. 分离保存上游注册/密钥测试，避免覆盖本地测试。Ent/Wire 已重新生成，生成文件纳入候选。
12. Ollama 过期回调测试改用上一代 reset + 1 秒建立明确不同的代际，避免 Windows 时钟精度碰撞；没有放宽生产 CAS 或删除断言，重复运行 50 次通过。

## Review 来源归因与修复边界

按用户要求，仅修复合并或 fork 本地修改引入的问题；纯上游问题只记录，不扩展修复范围。归因对照合并前 HEAD 与上游 v0.2.5 精确提交。

- **已修复，本次合并适配引入：批量套餐分配重试可能重复续期。** 上游及合并前本地 BulkAssign 均直接走旧分组分配路径，本次接入 PlanID 后才会进入本地单项分配的不同套餐 entitlement-only fallback。已有 legacy 订阅关联套餐 10，再批量分配套餐 9，重复业务调用会增加 30 天；同批重复用户也会重复续期。修复仅作用于带 plan_id 的批量操作：服务层用户去重；HTTP 层强制有效幂等键、无 coordinator 时拒绝执行、复用现有持久化结果重放与脱离客户端取消的 2 分钟执行上下文；前端规范化用户顺序、保存请求对应的会话幂等键，网络/错误响应时保留，成功后清除。不同套餐、参数、管理员与已完成后的新业务操作使用独立键。仅该新接口选择完整幂等结果保存，覆盖 64KB 以上结果重放；其他接口仍保留原有截断策略，不顺带修复上游通用实现。旧分组批量分配及单项续期语义不变。
- **纯上游，保持不改：图片缓存渠道定价覆盖。** v0.2.5 本身已包含 ImageCacheReadPricePerToken 计算及渠道覆盖遗漏；10 个图片缓存 token、渠道 cache_read=0.1、继承图片缓存=0.5 的复现为 5 而非 1。不是本次合并或本地代码独有问题，本轮不更改计费实现，保留已知风险记录。
- **纯上游结构建议，不属于已证实故障：** v0.2.5 已将 image_cache_read_tokens 放进 image_size_breakdown。前端尺寸 formatter 不展示 token 键不足以证明计费或数据丢失；撤销此前 P2 阻断定性，不添加字段/迁移、不修改实现。
- 幂等保障沿用现有存储、有效期和故障恢复边界；不宣称数据库业务写入与幂等记录跨事务具备崩溃情况下的 exactly-once。超过保存有效期或持久化故障后的结果不确定情形需人工核对，不能盲目换键重发。

## 数据库迁移审查

新增两份 SQL 均使用 238 前缀；迁移器按完整文件名记录 checksum 并排序，因此不重命名已发布迁移，也不修改既有 SQL。

- 238_opencode_go_platform.sql：扩展配额、复合路由和监控 provider CHECK，保留既有 MiniMax 等值。无删列/重命名；DDL 仍可能获取强锁并扫描现有行。上线前必须评估表规模、长事务和锁等待，不能把蓝绿当作无锁迁移。旧槽观察期间暂不创建旧代码无法识别的 OpenCode 配置。
- 238_purge_unlimited_user_platform_quotas.sql：仅删除三档限额均为 NULL 的行，不删除任何一档配置有限额的记录。虽无限额语义等同缺失行，但删除会丢失这些行的存量字段，应用回滚不会还原数据。上线前统计候选行、备份候选行/数据库、确认不依赖其历史字段；大表需预先评估分批清理或迁移超时和 WAL/复制延迟。
- 本地集成测试验证不等于生产数据演练；生产快照、锁评估、旧新版本并行读写与回滚记录尚未执行。本轮未访问生产数据库。
- 涉及迁移、计费、订阅、权益，归类为关键业务发布，稳如狗和 Canada 的观察窗口均按约 30 分钟。

## 验证记录

- pnpm 冻结锁安装通过；前端全量 Vitest：351 个文件、3134 个测试通过。
- 前端最新 typecheck、lint、build 全部通过；构建保留已有大 chunk 与 browserslist 数据陈旧提示，未为消除提示进行无关依赖升级。
- 后端 Go 1.27.0 单元测试：最新完整代码全量通过（go test -p 2 -tags=unit ./...），包括新增套餐与权益生命周期回归。
- 后端最新全量 integration（go test -p 2 -tags=integration ./...）、golangci-lint 2.13（0 issues）、普通构建与 embed 构建全部通过。
- 两个本地二进制 -version 均为 0.2.5；本地未注入 commit/build 元数据，显示 unknown，不能作为 CI 生产镜像的版本一致性证明。
- 集成默认高并发曾遇到 Windows 内存不足；改用 -p 2 重跑，不修改或跳过测试。
- Ent/Wire 生成通过。
- secret_scan 单测 10 项通过；修正测试假密钥后完整 secret_scan 通过。最终暂存内容复扫通过。
- 使用 Go 1.27 构建的 govulncheck v1.6.0：可达漏洞 0；仍报告 13 个未被调用的模块级漏洞，不表述为依赖完全无漏洞。
- pnpm audit 原始退出码为 1；按项目现有 .github/audit-exceptions.yml 校验通过，未新增例外。
- Windows Git Bash：两份脚本语法、Compose security/gateway-env/runtime-resources、Caddy 缓存策略测试通过。
- production-deployment-policy-test.sh：Windows mock 环境失败后，在本机隔离 Linux 容器（不挂载 Docker socket、不连接生产）通过，包括 stable contract 和 nginx blue-green cutover 行为测试。
- apple-container-test.sh 使用 macOS stat/plutil 接口，Windows 不能完成其权限断言；保留失败记录，不修改测试绕过。必须等待配置的 macos-15 CI 作业通过。
- 33 个仅 CRLF/LF 规范化差异未暂存，不进行无关全仓换行修改。测试日志、audit 原始数据、Python 缓存不纳入提交。

## 本地问题修复后的最终复验

- 后端完整 unit、integration 均通过（Go 1.27.0，-p 2）；golangci-lint 0 issues。对应 review-local-fix-unit-final.log、review-local-fix-integration-final.log、review-local-fix-lint-final.log。
- 后端普通与 embed 构建通过，两个二进制 -version 都为 0.2.5；本地 commit/build 仍为 unknown，不能代替 CI 镜像元数据校验。
- 套餐回归验证重复用户只处理一次、相同操作重放不续期、新业务操作可正常续期；HTTP 回归验证缺失/空白键、不可用 coordinator、客户端取消、参数冲突，以及超过 64KB 的批量结果完整重放。
- 前端全量 351 文件、3136 测试通过；typecheck、lint:check、build 通过。新增网络错误/504 后稳定重试、成功后新键及参数/管理员隔离测试；旧批量操作键测试保持通过。
- 最终业务修复暂存后 worktree/index 秘密扫描通过；无未解决合并索引项，diff --check 通过。日志和原有无关换行差异不纳入提交。
- 本轮未更改图片缓存计费、图片用量 JSON、迁移、依赖、Ent/Wire；没有通过改写上游业务逻辑来消除审查记录中的上游问题。

## 下一阶段门禁（需要用户确认）

1. 复核暂存树、版本同步及本报告，形成中文合并提交；可将版本同步拆成紧随合并提交的窄提交。不得遗漏当前暂存外的最终修正。
2. 再核对最新 origin/main，完成审阅后合入并推送；本轮未执行。远程 v0.2.5 发布标签本轮复核未存在；发布时须再次核对，禁止移动已有不同提交的标签。
3. 等所有配置 CI（尤其 macOS shell）和安全门禁；仅使用此确切提交的 CI 不可变镜像 digest。核对发布标签、VERSION=0.2.5、OCI version/revision、digest 与运行版本。
4. 发布前审查迁移并保存容器、镜像、端口、Compose、Nginx、数据库迁移及公开健康基线，验证回滚路径。
5. 稳如狗蓝绿先发，约 30 分钟观察后，确认 Canada 主库/Redis master、France 复制追平，再对 Canada 使用同一 digest 蓝绿，观察约 30 分钟。每站最终只保留一个运行槽。
6. US 只验证 relay/WireGuard 和旧域名转发；不部署应用、不下发秘密。France 只缓存镜像，保持数据库副本、Redis replica、应用停止，不启动迁移。
7. 不修改 DNS、不在生产构建、不启用 Prompt Audit 或原始提示词存储；故障先阻断后续发布，普通应用回滚不改数据库角色。

结论：已按来源归因收窄修复范围，批量套餐适配问题已补修复及定向回归；纯上游问题与未证实的结构建议不改。修复后的本地完整回归已通过。仍需 macOS CI、最终提交、镜像一致性及生产迁移/回滚验证；本轮未提交、推送或操作生产。
