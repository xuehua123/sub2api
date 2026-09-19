# 上游 v0.2.7 合并审查记录

日期：2026-09-19。实际 origin 为 xuehua123/sub2api，保留现有 remote 与生产拓扑。

## 基线与版本

- 基线 origin/main：a4ed47bb5b218f623d77686d33cced1963b289a6。
- 上游标签 v0.2.7：aea725f2ea644d5592d0bbb1d63b607efa7e200a（标签对象 7484192016807acf55c6ef4f2827d371eb61ec1c）。
- 独立工作分支：codex/merge-upstream-v0.2.7；原工作区及其未跟踪文件保持原状。
- 上游标签声明 0.2.7，但 VERSION 实际为 0.2.5。先提交目标内容合并，再以独立窄提交同步 VERSION=0.2.7，不移动上游标签。
- 发布必须使用最终源提交对应的 CI 不可变 digest，并核对 fork 发布标签、OCI version/revision、可信 CI 标签及运行版本；本记录不构成生产发布成功证明。

## 冲突与适配

1. subscriptions.ts 保留本地权益请求代际、promise、加载状态及缓存清理；上游 loading=false 已包含。
2. ChannelMonitorView.grok.spec.ts 保留 PROVIDERS.length，匹配 fork 实际平台集合和分组展示。
3. RegisterView.spec.ts 保留本地邀请、返利与邮箱注册回归。将上游新测试更新到现有独立 RegisterView.upstream-v025.spec.ts，保留优惠码初始状态、防闪烁及确认密码覆盖。
4. Seedance 完成计费调用补传本地 SubscriptionEntitlement 和余额 fallback 参数，沿用既有用量、权益和异步任务去重路径。
5. 上游仅在 wire_gen.go 手写 SetAccountDirectory，重新生成会丢失。新增正式 ProvidePluginManager provider，先绑定账号目录，再由 Wire 生成调用；新增注入回归测试。属于执行本仓库生成流程所需适配。
6. 补回已声明的 github.com/google/subcommands v1.2.0 校验和，确保 go generate ./cmd/server 可重复执行；未升级该依赖。
7. 未更改部署工作流、DNS、Prompt Audit 默认状态；未改动纯上游 Seedance 协议和定价语义。

## 迁移与回滚边界

相对当前主分支，无新增/修改 backend/migrations 或 backend/ent/schema，无 Ent 生成变动；插件 KV 使用隔离 Redis 前缀。上线仍须比对生产迁移记录/checksum，确认无尚未执行的历史迁移。

涉及视频计费与插件宿主能力，按关键业务发布处理：稳如狗、Canada 各观察约 30 分钟。每站部署前记录镜像、容器、端口、Compose、Nginx 与迁移及健康基线；验证回滚路径。先稳如狗后 Canada。France 仅缓存镜像并维持副本，US 仅验证转发。历史 Canada 迁移记录不代表数据恢复能力，本次不得重复执行或反向修改既有迁移。

## 本地验证

- Go 1.27.0：全量 unit、integration（均 -p 2）通过；新增 DI 回归及 cleanup 回归通过。
- golangci-lint：0 issues。
- Wire 重新生成成功，生成文件纳入提交；不手改生成文件维持注入。
- 前端冻结锁安装、typecheck、lint:check、build 通过；366 文件、3206 测试通过。
- secret_scan 单测 10 项、完整扫描通过。
- 使用 Go 1.27 重新构建的 govulncheck v1.6.0：可达漏洞 0，11 个未调用模块级漏洞；不宣称依赖完全无漏洞。基线 gRPC 可达漏洞随上游升级到 v1.83.2 消除。
- pnpm audit 按既有 audit-exceptions.yml 验证通过，无新增例外。
- Compose security/gateway-env/runtime-resources 检查通过。生产部署 contract/cutover 与 Caddy 检查在本机隔离 Linux 容器通过；macOS apple-container 检查由分支 CI 完成。
- 普通及 embed 后端构建通过，两个本地二进制均显示 0.2.7；commit/build 为 unknown，仅 CI 镜像元数据可用于发布验收。
- 构建保留已有大 chunk 提示，无无关依赖升级；CRLF/LF 纯换行差异不纳入提交。

## 后续发布证据

分支 CI、安全扫描、最终主分支 CI、镜像元数据/运行版本与两站观察结果必须继续完成。最终提交及 digest、部署与观察日志存入本轮发布状态记录，不能以本地通过代替 CI 或生产验证。
