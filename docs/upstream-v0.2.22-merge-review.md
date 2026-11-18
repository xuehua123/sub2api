# 上游 v0.2.22 合并审查记录

审查日期：2026-11-18（Asia/Singapore）。本记录覆盖整个合并差异、冲突处理以及复核补丁；范围是本地 fork 问题与合并回归。纯上游问题仅记录，不扩大修复范围。当前结论：已发现的本地兼容问题均已修复，未发现新的未修复生产缺陷；完整集成测试仍有上游原有测试不稳定，不能宣称全部发布门禁通过。

版本与工作区：

- 工作树：`C:/Users/72774/AppData/Local/Temp/sub2api-v0.2.22-review`；分支：`codex/update-v0.2.22`。
- 本地 HEAD / 本次核验的远端 main：`8bb8c4a2ac27cc5f471c7d04d285888822ef5b2b`。
- 上游 tag 对象：`b11573c9d55d39b6780e1e9f6e193ba2e6ca2321`；tag 对应提交：`b0fa9f3e6eeab1968a53e1289b69daa4bb40ca2b`。
- 合并目标：`15871f904056f548dd874a1d069ffc6cdf777b78`，为 tag 的直接子提交，仅将 VERSION 从 0.2.21 同步到 0.2.22。当前 VERSION 为 `0.2.22`，后端工具链为 Go 1.27.4。
- 合并尚未提交、推送或部署，没有本次合并提交 SHA 与 CI 不可变镜像 digest；本记录不是发布成功证明。原始工作区的已跟踪文件未改动。

审查覆盖与方法：

原始暂存合并差异为 341 个文件；本轮另补充原有仓储单测文件，使代码及测试差异共 342 个文件。本记录另外占 1 个文件。上游与 fork 同时修改的文件有 137 个，手工冲突涉及 27 个文件。第一遍按模块核对合并差异及行为，第二遍比较两侧来源、检查本地特有逻辑是否丢失，并复核修复后的调用链和验证结果。

| 范围 | 文件数 | 重点 |
| --- | ---: | --- |
| 服务层 | 95 | 权益预检/结算、计费倍率、缓存版本、调度快照、协议转换、流式失败与用量 |
| 仓储层 | 16 | 单笔与批量语义、幂等和归档、锁顺序、汇总/清理、用户分组费率 |
| 处理层 | 13 | 请求策略、流式提交与重试、分组字段、用户统计成本隔离 |
| 协议与公共包 | 13 | Anthropic/Responses/Chat 转换、工具与服务端搜索、Gemini schema |
| 前端实现与配置 | 21 | 透传白名单、新开关完整往返、额度未知状态、端点成本、既有 fork 开关 |
| Ent 生成代码 | 8 | 新字段与现有本地字段同时保留，生成与编译一致 |
| 测试 | 152 | 新增场景与已有 fork 回归、断言和接口参数适配 |
| 其他 | 24 | Ent schema、两项迁移、版本、Wire、依赖、发布配置、模型资源和文档 |

其中 8 个文件只有 CRLF/LF 差异，已用忽略行尾空白的差异确认无内容变更：auth 下 3 份 Markdown、两个 tsconfig、gateway README、admin skill 及其 CLI reference。依赖只将已存在的 jsonschema-go 从 indirect 改为直接依赖；没有新增前端依赖或锁文件变化。三个 Dockerfile 与 GitHub workflows 相对 main 无差异，未引入拓扑或部署范围变更。

已修复发现：

| 严重程度 | 来源 | 问题与处理 |
| --- | --- | --- |
| P1 | 合并回归 | 上游批量计费未处理 fork 的 EntitlementID，会绕过权益扣减、窗口检查、余额回退和旧订阅别名同步。用户通道发现权益命令时按序调用已有 Apply；仓储校验在事务开始前拒绝直接批量结算权益，避免旁路调用静默漏扣。 |
| P2 | 合并回归；本轮补齐 | 新批量路径没有持久化或返回 fork 的最终 BillingSource，重复和归档重放也不读取首次来源。现于同一事务批量写回新结算来源，重放复用单笔路径的来源读取；发生写入错误或行数不符则回滚。 |
| P2 | 合并适配 | 旧订阅累加函数在 fork 返回 updated_at；新批量路径需传播 SubscriptionVersion，否则异步缓存失效门禁失去版本依据。现为每条订阅命令返回本次事务的微秒版本。缓存 Lua 允许同版本的多笔有效增量，不会将一批中后续增量去重丢弃。 |
| P2 | 两侧字段演进冲突 | 两侧均使用 auth snapshot v28，但字段集合不同。已提升到 v29，同时保留 image_count_billing 与 force_openai_ultrafast，防止复用不完整快照。 |
| P2 | 本地校验与上游覆盖逻辑冲突 | 上游新增 DeepSeek 显式价格覆盖与 fork 已有负单价拒绝逻辑组合后，负覆盖会丢弃整个模型条目。合并覆盖时忽略对应无效负值，保留合法目录价；上游 negative_input 场景及本地目录校验均通过。这不是单独修补纯上游代码。 |
| 测试适配 | 合并回归 | EndpointStats 测试调用遗漏 fork 的 entitlementID 参数，已补 0；序列/批量对照测试保留各自事务时间差，不再直接比较独立事务的版本值。 |

关键位置：`backend/internal/service/usage_billing_lane.go`、`backend/internal/repository/usage_billing_batch_repo.go`、`backend/internal/service/api_key_auth_cache_impl.go`、`backend/internal/service/pricing_service.go`。新增回归覆盖混合权益队列、仓储事务前拒绝、实际来源落库、在线与归档重放；已有事务回滚和 300 笔同用户并发用例共同验证批量路径。

迁移与新旧版本共存：

- 两个 `251_` 文件按完整文件名排序与记账，不会因数字相同相互覆盖。
- Ultrafast 为默认 false 的新增列。汇总迁移新增列并替换同签名触发函数，未删旧列；锁等待设为 5 秒。
- 真实 PostgreSQL/Redis 的专项集成测试通过，共 27 个顶层测试，包含写入不阻塞、迟到记录、删除/保留窗口、时区、重建中断恢复、账号锁顺序以及批量计费。
- 为核实旧版兼容，临时引入 HEAD 的原始汇总代码，针对迁移后数据库执行旧版重建、旧版发布穿插新版登记/计算/发布、历史记录晚到、新版完成后切回旧版重建，均与明细逐日核对通过。临时测试已移出提交范围，源码和执行日志保存在本机审查目录。
- 上述验证消除了此前仅凭锁变化提出的“可能不兼容”猜测；不等于证明所有生产并发时序。旧版在短暂共存期间仍会持有原有长事务锁，发布必须按项目流程控制重叠并观察。

验证结果：

| 检查 | 结果 |
| --- | --- |
| `go test -tags=unit ./...` | 本轮通过，含新增仓储保护测试 |
| `go test -tags=integration ./...` | 首轮仓储全部通过；完整 suite 的 service 包存在下述原有计时测试失败 |
| `go test -tags=integration -parallel=1 ./...` | 仍有下述两项原有测试不稳定，未按全绿记录 |
| 核心仓储/迁移专项，`CI=true`、`-count=1` | 27 个顶层测试通过；Docker 缺失会直接失败，未靠跳过集成获得通过 |
| TTFT 测试单独重复 3 次 | 通过；不替代完整 suite 的失败记录 |
| Windows/Linux 嵌入前端构建 | 本轮均通过；本地二进制显示 0.2.22，commit/build 为 unknown，不能替代 CI 镜像元数据核验 |
| golangci-lint | 本轮 0 issues |
| govulncheck v1.6.0 | 被调用代码 0 漏洞；另有 6 项仅依赖模块级、未被代码调用的提示 |
| 前端冻结安装、lint、typecheck、build | 前轮通过；本轮未改前端逻辑 |
| 前端 Vitest | 前轮 469 文件 / 4227 测试通过 |
| 前端生产依赖 audit | 本轮按已有 exceptions 验证通过，未新增例外 |
| 门户/静态文档测试 | 13 项通过 |
| 发布矩阵脚本测试 | WSL/Linux 下 10 项通过；Windows 的缺依赖/编码/bash 路径问题未当成产品缺陷修补 |
| Apple/部署/镜像脚本语法、Compose simple-mode 检查 | 通过 |
| Secret scan 与故障行为测试 | 扫描通过；10 项测试通过 |
| Ent、Wire | 前轮重新生成并验证；本轮未再改 schema/DI |
| 工作树与差异检查 | 无未解决冲突；diff whitespace 检查通过；原工作区已跟踪文件未改 |

保留的上游测试问题（不修改）：

1. `TestOpenAIWSPassthroughFirstTokenFollowsTTFTMode/semantic` 在完整集成运行测得 499ms，而固定事件延迟断言要求至少 500ms；单独重复 3 次通过。测试及其计时实现相关文件与本次基线的差异已核对，失败测试文件与上游及 HEAD 完全一致。
2. `TestOpsOutputTPSDistribution` 用 `time.Now().UnixNano()` 生成请求 ID，Windows 连续插入偶发重复，触发 `idx_usage_logs_request_id_api_key_unique`；同一仓储完整 suite 首轮通过、第二轮出现。该文件在 HEAD、上游和合并结果完全一致。

两者均不属于本次本地或合并修复范围，不放宽断言、不添加跳过规则，也不以专项通过替代完整门禁。后续精确提交仍须通过项目配置的 Linux CI；若 CI 同样失败，记录上游来源并由操作者决定处理，不擅自扩大范围。

审查资料位于 `C:/Users/72774/AppData/Local/Temp/sub2api-v022-audit/`：包括全部改动文件清单、unit/integration/lint/security 日志、迁移兼容临时测试与日志。本地构建只验证版本与编译，不是生产可部署镜像。进入发布前仍须取得最终合并提交、全部 CI 结果及与 VERSION/revision 一致的不可变 digest，并按稳如狗 → Canada 的生产流程执行；美国保持中继、法国保持副本与应用停止。
