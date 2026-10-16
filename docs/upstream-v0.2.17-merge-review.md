# 上游 v0.2.17 合并评审

## 来源与版本

- 基线：origin/main 2062efa1a180348d1512b9a19966a962f8e8954a，版本 0.2.16。
- 上游标签：Wei-Shaw/sub2api v0.2.17，精确提交 060d0160417782cd962a0c7f460f6cd4565d4916。
- 上游标签内 VERSION 仍为 0.2.16；先合并标签，再合并上游 b0a02a3304a0a78cdd003ddda1dd23ba02c03084，同步 VERSION 为 0.2.17。
- fork 内容合并提交 d05d4c3fd；版本同步合并提交 572c3675d。

## 冲突处理与兼容性

8 个冲突文件逐段整合：支付宝、支付履约、下单校验、退款与测试、退款计划结构、网关导入、账号管理页。

- 待定退款保留已扣余额/订阅天数；成功不重复扣减，确认失败时在状态、回退和必需审计的同一事务内处理。回退失败会在另一事务中原子标记失败与 REFUND_ROLLBACK_FAILED。
- 保留 fork 的退款认领、金额与币种换算、外部退款/拒付组件、推荐与分销同步、订阅权益和旧审计格式兼容。
- 在订单快照中持久化已扣权益、原订阅/权益快照和请求时间；服务重建后仍可恢复。读取或解析待定状态失败时返回错误，不误判为已归还。
- 保留外部成功回调优先：查询失败与成功回调竞争时，不把成功退款改为失败，也不归还已经完成退款的扣减。
- 账号页保留 fork 的可见分组、上游连接及流量范围提示；订单类型白名单保留 fork 的套餐访问返回值。
- 适配新增 Responses created_at 测试的 fork WebSocket 参数，保留已有协议扩展。
- GPT-5.5 退役筛选按上游账号和实际映射目标执行；管理员清单测试更新为新行为，API Key 兼容性仍由上游专用测试覆盖。

新增 4 个权益退款跨待定状态/服务重建场景，覆盖普通缩短和撤销，以及最终成功/失败；另增加外部成功回调与失败终态竞争场景。

## 数据库与生成文件

本次无 SQL 迁移、Ent schema、依赖清单或锁文件变更。生产迁移应始终为 345 行，SHA256 b04ac056e2352b4fb566a2ba19aeeb61c3b14611302d463a9286c1868825536a。Wire 重新生成后无差异。Ent 生成检查遇到 Windows 文件映射锁后中断；本次生成改写的文件均恢复至已提交字节，Ent 源码与基线无差异。

## 验证记录

- Go 1.27.2；golangci-lint 2.14.0：0 issues。
- 支付/退款专项通过；新增权益恢复及并发回调场景通过。
- 完整 unit 首轮发现旧清单数量断言，已按退役行为修正。第二轮仅未改动的全局堆采样用例超过 8 MiB 上限；该用例和相关回归连续 3 次通过，保留原阈值，第三轮只有未改动的 Grok 限流重置时间断言比 1 秒边界多约 0.3 毫秒；完整 suite 以 go test -tags=unit -parallel=1 ./... 执行通过，未放宽阈值或改动这些用例。CI 全部门禁仍必须在合并 main 前核实。
- 前端冻结安装、lint、typecheck、build 通过；454 文件 / 3933 测试通过。门户 13 项测试通过。
- 嵌入前端的 CGO_ENABLED=0 后端构建通过，版本入口显示 0.2.17。
- govulncheck：无被调用漏洞；另有 7 项仅出现在依赖模块中的未调用漏洞。
- 前端安全审计匹配原有有效例外，未新增例外。
- 秘密扫描和 10 项故障行为测试通过。只为上游插入/删除行导致的既有文档占位符、支付宝假密钥调整 6 条精确当前行号指纹，没有关闭规则。
- Windows 本机无 Docker 引擎；容器集成、macOS/Unix shell 与 release helper 检查必须由最终提交 CI 全部门禁确认。
- 新工作树有 14 个基线文件的 CRLF 与 text/eol 属性冲突；已验证磁盘原始字节与 Git 索引完全相同，未将这些换行噪声纳入提交。

## 发布约束

用户授权按完整流程完成发布。本文件是合并评审，不表示生产已发布。必须等待分支和 main 的配置 CI 门禁，通过后核验 fork tag、VERSION、OCI version/revision/source/trusted-ci 与不可变 digest。

先稳如狗蓝绿与约 30 分钟关键业务观察，通过后加拿大蓝绿与约 30 分钟观察。发布前分别记录回滚状态、迁移及公开健康基线。美国仅核验公开中继，不部署或传递应用/数据库秘密；法国仅缓存同一 digest，保持 PostgreSQL/Redis 副本与应用停止。Prompt Audit 保持关闭，不修改 DNS 或数据库角色。

## GitHub Actions 外部阻塞

截至北京时间 2026-10-17，GitHub 页面明确显示：Workflows are not currently being run on this fork due to the scale of GitHub Actions usage. A repository maintainer can choose to re-enable these. API dispatch 返回 HTTP 422 / Actions has been disabled for this repository。普通 Actions 权限写回 enabled=true 与逐工作流 enable 均不能解除该 fork 级状态。

推送、PR 重开、原生 API 引用更新都没有产生 Actions 运行。临时尝试的 workflow_dispatch 入口已经撤回，既有 workflow 内容保持原样。浏览器会话未登录，需要维护者在 GitHub Actions 页面登录并重新启用。恢复后重触发最新分支完整门禁，再按流程合并 main、等待精确 main CI 和不可变镜像、稳如狗及加拿大各观察约 30 分钟。没有绕过门禁、合并 main 或变更生产。

## Actions 恢复

北京时间 2026-10-17，维护者登录后的 Actions 页面已显示 Actions Enabled。重新推送本验证记录以触发升级分支的完整 CI 与 Security Scan；实际运行和所有作业成功仍须逐项核验，然后合并 main 并验证该精确提交的可信镜像。
