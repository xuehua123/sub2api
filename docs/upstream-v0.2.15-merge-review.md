# 上游 v0.2.15 合并审查

日期：2026-10-09。

## 来源与版本完整性

- fork 基线：`a3c949642d09d7c97a2743a847c5fb9bad0f0c74`（v0.2.14）。
- 上游 v0.2.15 注释标签对象：`86a80c13dcba86f52f9ca815b5a471cecc236227`；实际标签提交：`f2669c8cf62555cd92389b3f55920e9e6e7c6ff2`。
- 合并提交：`8658c0b97c4077f8f7ba65a17ffe07f7c1ed84f3`，第二父提交为上游实际标签提交。
- 上游标签内 `backend/cmd/server/VERSION` 仍为 `0.2.14`。先提交完整标签内容，再以独立提交 `56fa024621aea3854c6fb529a3c775db88aae6ab` 同步为 `0.2.15`，没有在未合入标签的分支提前升版。
- Go 随上游升级为 `1.27.2`；fork 的 integration CI、安全扫描、release 检查与 README/AGENTS 已同步。golangci-lint 为 `v2.14.0`。
- 最终 fork 发布标签、VERSION、CI OCI version/revision/trusted-ci、不可变 digest 与运行时必须作为同一发布单元核验；任一不符不得部署。

## 最终实现与合并处理

- 合入上游平台清单、Command Code/Cline provider、协议转换与 web_search history 修复、图片 token 解析、管理界面和交互修复。
- 保留 fork shared upstream connections、签名监控、严格供应商余额解析、TypeSafe、权益准入/余额回退/超额记账、native compaction、payload hash、用户计费与导出等定制。
- 旧账号主动 billing probe 继续保持退役状态，不恢复后台 runner、旧 API 或 DI。新增 provider 测试仅移除对旧 probe 的断言，保留 provider 功能覆盖。
- WebSocket 每轮刷新同 Key/同 Group 的分组价格和利润门，保留连接调度身份与权益快照。独立审查发现 cyber 异常回合仍按建连价格计费，已统一使用该回合计费快照；回归覆盖 passthrough/ctx_pool、3.0→0.3 调价、真实 usage 与恰好一次记账。
- 用户 UsageView 使用 fork 独立表格，自动合并未带入上游 TPS。已补充共享 TPS formatter，并增加回归。
- 新 ping 回归仅对实际 CC chunk 和 heartbeat 等待立即 flush，兼容 fork 原有 TTFC 记录；状态事件无输出时继续读取。Command Code 目录测试避免 Windows 同 tick 时间戳碰撞，以独立用例键隔离缓存。两项均保留生产行为。
- 继承自 fork 的计费边界：分组倍率 resolver 默认缓存约 30 秒；cyber 异常路径的峰谷价格沿用落账时刻。本次未扩大修改范围。

## 数据库迁移与回滚

- 新增 `242_drop_platform_check_constraints.sql`，仅删除 quota platform 与 composite route target platform 的固定 CHECK，不删列、不改类型、不重写既有数据、不改变 monitor CHECK。
- 对新迁移增加 `SET LOCAL lock_timeout = '5s'`，限定共享数据库 DDL 排队，避免候选启动长时间阻塞旧槽。
- 所有已登记的 340 个迁移文件/checksum 应保持原样；升级后必须恰好增加这一文件，总数 341。
- 新迁移 checksum 按 runner 的 TrimSpace 规则：`462aa8fee507b0ae9265c7f75e21d8f7c01c6c8efcca0c1bd7079cceb6d58efa`。
- 旧应用对既有平台仍兼容。旧槽排空停止前不得配置新 Command Code/Cline；镜像降级前须检查没有新平台配置。迁移失败只阻止候选启动，不通过停旧槽或更改角色处理。

## 验证证据

- 前端 frozen install、lint、typecheck、完整 Vitest（444 files / 3853 tests）、production build 通过；UsageView 最终 15 项回归通过。
- Go 1.27.2 原位 Ent 生成受 Windows mmap 文件锁阻止；使用同 Ent 版本、int64 ID、四项 features 与原 package 隔离生成成功，446 文件与暂存生成代码零漂移。DI 未改，无需 Wire 再生。
- WS 正常/异常调价与利润门定向回归通过；secret scanner 10 项单测、gateway 13 项测试与文档一致性、release helper 10 项测试、部署契约/切换/安全/资源/simple-mode/Caddy 检查通过。
- govulncheck：实际调用漏洞 0。pnpm audit：既有 xlsx high 2、critical 0，精确例外校验通过。
- `.gitleaksignore` 仅增加 deterministic golden test 的 `case_keys_sha256` 第 4 行精确 current/index/history 指纹；该值是测试用例摘要，不是凭据，没有放宽整条规则或路径。
- 完整 backend `go test -p 2 -tags=unit ./...`（最终复跑）、`go test -p 2 -tags=integration ./...`、golangci-lint（0 issues）、embed build 通过；构建二进制显示 0.2.15。合并后的完整历史密钥扫描通过；最终审查提交再次扫描后推送。
- GitHub CI/安全门禁须在审查分支和最终 main 精确提交上全部通过。macOS Apple-container 检查以配置的 macos-15 CI 为准，本地 Linux 无法代替 BSD stat 检查。
- 隔离工作树存在 33 个既有 CRLF/LF 状态提示，已确认工作文件与 index 字节相同；没有把无关换行归一化加入发布。

## 渐进发布

1. 审查分支全部检查通过后合入最新 origin/main，等待该精确提交全部 CI/安全门禁及 CI 镜像。
2. 稳如狗先蓝绿发布、验证 341 迁移、运行版本与业务指标，观察约 30 分钟。
3. Canada 确认可写 PG primary / Redis master、France 复制追平、公共 relay 健康；本轮 green/28080 v0.2.14 → blue/18080 v0.2.15。记录新 rollback state，预热候选、原子切换专用 Nginx include、自然排空旧 worker/SSE/WS，再停止旧槽，观察约 30 分钟。
4. 稳如狗线上脚本已有安全自然排空修复，不覆盖为仓库旧版。Canada 发布/回滚均在停止槽之前自然排空，不能以 Docker stop 600 秒代替应用自身约 5 秒 shutdown deadline。
5. France 只缓存同一 immutable digest，维持 PostgreSQL/Redis 副本且零运行应用；不运行 candidate 验证或 startup migration。
6. 不向 US 部署、不下发秘密；依已有用户授权跳过内部 SSH，继续核验公开 relay 路径。常规发布不改 DNS/数据库角色，Prompt Audit 保持关闭。

生产结果、最终 CI 提交和 image digest 记录在本轮部署报告；本文件不以本地构建通过代替生产完成。
