# API 端点智能连接检测设计方案（无持久化 MVP）

## 1. 文档信息

- 文档状态：MVP 实施方案定稿（v7）
- 修订日期：2026-08-03
- 适用项目：Sub2API Fork
- 目标页面：用户端 API 密钥页面
- 核心对象：管理员公开给用户选择的 API URL

### 1.1 v7 修订重点

1. 将“纯前端 MVP”更名为“无持久化 MVP”。本期仍需新增后端探针路由和公开设置，只有测量与评分在浏览器本地完成。
2. 明确本期不建设管理端诊断数据层。
3. 取消单独的 `client-info` 路由，由每个真实 API URL 的探针按严格可信代理规则返回可选的用户出口 IP。
4. 本期地区解析改为**本地 GeoIP MMDB 方案**（`connectivity.geoip_database_path`），仍不向 `ip-api.com` 或其他第三方发送用户 IP；数据库缺失/损坏时地区功能 fail closed，仅停用地区、不影响探针与 IP。
5. 浏览器评分只使用可稳定获得的成功率、总耗时、P95 和抖动；DNS、TCP、TLS 等阶段耗时仅作本地尽力采集，不参与评分。
6. 增加 URL 安全校验、固定探针路径、禁止重定向、跨域要求、限流、日志降噪和配置边界。

### 1.2 v8 修订重点

1. **展示“典型延迟”**：四档等级仍是核心，但每行补充成功样本 `medianMs` 的整数毫秒值（“典型延迟 {ms} ms”），无成功样本时显示“暂无可用延迟”。
2. **展示已验证出口 IP 与粗粒度地区**：沿用严格可信代理链解析的出口 IP；地区由本地 GeoIP MMDB 提供（国家/一级行政区/城市），文案使用“估算”，地区不可用时显示“地区未知”。
3. **探针响应扩展 `client_location`**：`{"ok":true,"client_ip":...,"client_location":{country_code,country,region,city}}`；响应体硬上限从 128 字节提高到 1 KiB，前端严格校验新字段（超长/错误类型/异常控制字符判 protocol_error）。
4. **不持久化**：出口 IP、地区、原始样本、P95/MAD 均不进入任何缓存或日志；浏览器 30 分钟缓存只新增 `median_ms`（向后兼容旧记录）。
5. **管理后台只读状态**：在 IP 开关旁显示 GeoIP 就绪/未配置/数据不可用，不暴露数据库路径。

> **MVP 能力边界：本期不满足管理员诊断需求。**
>
> 管理员无法查看某个用户的测试详情、完整 IP、逐次样本、历史记录、地区分布或 7 天数据。原始延迟样本和浏览器评分不会上传或形成产品级数据集；普通 Nginx/边缘访问日志仍按既有运维策略处理。管理员诊断、完整数据保存 7 天和匿名聚合属于二期。

## 2. 背景

当前 API 密钥页面通过 `EndpointPopover.vue` 展示默认 API 地址和自定义端点，并把测速操作跳转到第三方网站。该方式存在以下问题：

1. 用户离开当前页面，体验割裂。
2. 第三方页面不能复现用户访问真实 API URL 的 DNS、证书和入口路由。
3. 用户容易把一次测试误解为服务器或线路的整体质量。
4. 测试结果无法直接帮助用户在现有 API URL 中作出选择。

本方案在 API 密钥页面内增加“检测连接”。用户浏览器直接访问每个真实 API URL 的固定无副作用探针，并在本地计算“优秀、良好、一般、不推荐”四档结果。

## 3. MVP 范围

### 3.1 本期交付

1. 在 API 密钥页面内完成检测，不打开第三方页面。
2. 使用用户当前设备、浏览器和网络环境访问真实 API URL。
3. 用户选择和复制的对象始终是 URL，不是物理服务器。
4. 用户以四档连接结果和推荐结论为核心，并看到每个端点的“典型延迟”（成功样本 `medianMs` 整数毫秒）。
5. 可选展示已验证出口 IP 与本地 GeoIP 估算地区（受 `connectivity_client_ip_enabled` 与 GeoIP 就绪状态双重约束）。
6. 浏览器本地完成采样、评分和 30 分钟会话级缓存（缓存含 `median_ms`，不含 IP/地区/样本）。
7. 后端提供一个固定的无鉴权、无业务副作用探针。
8. 功能开关、阈值和采样参数通过现有系统设置管理。
9. 功能默认关闭，按端点完成验收后逐步开启。

### 3.2 本期明确不交付

1. 不保存测试会话、逐次样本、评分结果或用户测试历史。
2. 不提供管理员诊断页面、搜索、导出或测试编号。
3. 不提供完整 IP 的 7 天保存。
4. 不提供地区、运营商、ASN 聚合。
5. 不做服务端评分和防篡改。
6. 不做真实 API Key 鉴权链路测试。
7. 不调用 AI 上游模型，不测试模型可用性。
8. 不自动修改用户第三方客户端中的 API URL。

### 3.3 不影响的业务链路

探针不得进入以下任何链路：

- API Key 鉴权
- 分组解析和账号调度
- 订阅权益和额度
- 余额扣费
- UsageLog
- 渠道监控和渠道降级
- 账号封禁、临时不可调度和错误分类
- AI 上游请求

## 4. 核心产品边界

### 4.1 用户选择的是 URL

用户界面的主体是公开 API URL，例如：

```text
https://api.example-a.com
https://api.example-b.com
https://api.example-c.com
```

管理员可以配置“默认端点”“备用端点”等辅助名称，但界面不得出现“加拿大服务器”“美国服务器”“法国服务器”等内部名称，也不得展示服务器 IP、物理位置、主备角色或转发拓扑。

一个 URL 背后的服务器、路由或地区发生变化时，用户仍然只感知该 URL。

本方案保证的是“界面和探针响应不额外返回我方内部节点、源站和中转 IP”。公开 API URL 的 DNS 解析结果是互联网访问必需信息，本来就可被用户查询，不属于本功能能够隐藏的范围；检测功能不得在此基础上新增任何内部地址暴露。

### 4.2 结果只描述本次连接表现

页面固定展示：

> 检测结果仅代表测试时，您当前设备和当前网络访问各 API URL 的连接表现。结果可能受运营商、地区、Wi-Fi、代理/VPN及测试时间影响，不代表该端点对其他用户或其他网络环境的表现。

禁止使用：

- 此线路很差
- 服务器网络不好
- 某地区线路故障
- 某服务器不稳定
- 该节点质量差

允许使用：

- 您当前网络访问此 API URL 的表现为“良好”。
- 在您当前网络环境下，建议选择其他 API URL。
- 本次检测未完成，请稍后重试。

### 4.3 本期检测能力

本期检测的是：

```text
用户浏览器 -> API URL 的 DNS/TLS/公开入口 -> 中转链路 -> Sub2API 固定探针
```

本期不证明以下能力：

- AI 模型可用
- API Key 有效
- 某个分组有可用账号
- POST 大请求稳定
- SSE/WebSocket 长连接稳定
- 上游首字延迟或生成速度

因此产品名称使用“连接检测”，不得使用“模型测速”“线路质量认证”或“真实调用测试”。

## 5. 用户体验

### 5.1 入口

移除当前跳转 `tcptest.cn` 的测速链接，在 API 端点区域提供“检测连接”操作。点击后在当前页面打开抽屉或对话框，不打开新标签页。

功能开关关闭或设置加载失败时，不渲染检测入口，不影响 URL 展示和复制。

### 5.2 检测流程

1. 用户点击“开始检测”。
2. 前端强制刷新一次公开检测设置。
3. 前端只使用后端返回的已校验可测试端点列表。
4. 浏览器交错访问各 URL 的固定探针。
5. 浏览器在本地计算等级和推荐项。
6. 用户可以复制某个 URL 或重新开始一轮检测。

### 5.3 展示内容

所有 URL 使用同一出口 IP 时，顶部展示一次网络出口：

```text
当前设备网络
公网出口：113.110.12.34
IP 归属地：中国 · 广东 · 深圳（估算）

默认端点                                  优秀
https://api.psydo.top
典型延迟 86 ms                            推荐使用

CF 优选端点                               良好
https://api.ppx-ai.com
典型延迟 142 ms
```

| 状态 | 颜色 | 用户文案 |
| --- | --- | --- |
| 优秀 | 绿色 | 您当前网络访问此 API URL 表现优秀，推荐使用 |
| 良好 | 蓝色 | 您当前网络访问此 API URL 表现良好，可以使用 |
| 一般 | 黄色 | 您当前网络可以访问此 API URL，体验可能存在波动 |
| 不推荐 | 红色 | 在您当前网络环境下，建议选择其他 API URL |
| 测试中/未测试/未完成 | 灰色 | 正在检测/尚未检测/本次检测未完成 |

颜色不能作为唯一信息载体，必须同时显示文字和状态图标。

- 每行展示“典型延迟 {medianMs 四舍五入} ms”；无成功样本时显示“暂无可用延迟”。文案必须叫“典型延迟”，不得叫 Ping、网络延迟或模型响应时间。
- 不向普通用户展示 P95、MAD、成功率、原始样本、DNS/TLS/HTTP 状态、评分版本或内部错误。
- 底部固定说明：延迟不是模型响应速度，IP 归属地也不是服务器所在地区。

### 5.4 用户出口 IP 与估算地区

出口 IP 是网络环境信息，不是第五种评分参数。地区是可选增强（依赖本地 GeoIP 就绪），同样不参与评级和推荐。

1. `connectivity_client_ip_enabled=false` 时出口 IP 与地区完全不展示。
2. 只有探针通过严格可信代理链确认最终用户公网 IP 时才返回 IP；IP 不可用时地区一并隐藏。
3. 所有 URL 返回相同出口 IP 时，在对话框顶部显示一次 IP 与估算地区。
4. 不同 URL 返回不同出口 IP 时，提示“不同 API URL 使用了不同网络出口，可能存在代理/VPN 分流”，并在对应 URL 下显示其自身出口 IP 与估算地区。
5. 任意 URL 无法安全确认时显示“当前网络出口无法识别”，不得猜测。
6. 地区仅为粗粒度估算，文案必须使用“估算”并允许“地区未知”；同一 IP 的多端点多地区不一致时隐藏地区、保留 IP。不显示运营商、ASN、坐标、邮编、时区、我方节点 IP 或内部地址。
7. IP 与地区均不参与端点推荐和评级；未完成检测时不展示误导性的 IP、地区或延迟。

固定提示：

> 出口 IP 仅表示该 API URL 在本次检测中看到的您的公网出口。若与日常网络不符，可能正在使用代理、VPN 或分流规则。

### 5.5 浏览器缓存

最近一次等级和测试时间最多在当前标签页或 `sessionStorage` 保存 30 分钟：

- 保存 URL、等级、`median_ms`（典型延迟）、测试时间和 `grading_version`
- 不保存逐次样本、错误详情、出口 IP、地区、P95/MAD 或代理链
- 旧缓存记录（无 `median_ms`）仍可读取，仅缺失典型延迟；新缓存字段必须校验有限、非负、有合理上限
- 页面关闭或超过 30 分钟后失效
- 重新检测时立即清除旧结果

## 6. 总体架构

```text
用户浏览器
  ├─ GET /api/v1/settings/public
  │    └─ 获取开关、阈值、采样参数和已校验端点列表
  ├─ GET {probe_url}?nonce={随机值}
  │    └─ 每个真实 API URL 的固定无副作用探针
  └─ performance.now() + 可选 Resource Timing
       └─ 本地计算四档等级，不上报样本

Sub2API
  ├─ 公开设置：返回检测配置和安全过滤后的端点 DTO
  └─ /.well-known/sub2api/edge-probe
       ├─ 无鉴权
       ├─ 无数据库、Redis、计费和调度依赖
       ├─ 固定小响应
       └─ 可选返回经严格验证的用户出口 IP
```

### 6.1 架构不变量

1. 浏览器不能提交任意探测目标给后端。
2. 后端探针不能访问任何外部 URL。
3. 探针成功不读取数据库和 Redis。
4. 探针失败不得影响普通 API URL 的可用状态。
5. 评分结果不得作为服务端调度、封禁、计费或风控依据。
6. 测试功能可以整体关闭，且关闭后普通 API 请求行为完全不变。

## 7. 端点来源与安全资格

### 7.1 端点来源

MVP 不建端点表，来源仍为现有设置：

- `api_base_url`
- `custom_endpoints`

后端在公开设置响应中新增 `connectivity_test_endpoints`，由上述设置生成安全 DTO：

```json
[
  {
    "name": "默认端点",
    "api_url": "https://api.example-a.com",
    "probe_url": "https://api.example-a.com/.well-known/sub2api/edge-probe",
    "is_default": true
  }
]
```

前端不得自行把任意 `custom_endpoints` 拼成探测目标，只能使用后端生成的 `probe_url`。

### 7.2 URL 标准化

后端使用标准 URL 解析器完成：

1. scheme 和 host 转为规范形式。
2. 移除普通尾部斜杠。
3. 拒绝 URL userinfo、query 和 fragment。
4. 生产环境仅允许 HTTPS。
5. `probe_url` 必须与 `api_url` 同 origin。
6. 探针路径固定为 `/.well-known/sub2api/edge-probe`，不能由浏览器或查询参数改变。
7. 相同 origin 的多个 API URL 共享一次探测结果，但仍以各自 URL 展示。

### 7.3 安全资格

以下 URL 不进入 `connectivity_test_endpoints`：

- HTTP URL（仅本地开发环境可显式放行 localhost）
- 包含用户名或密码
- 字面量为回环、私网、链路本地、组播、未指定或保留 IP
- `localhost`、单标签主机名，以及 `.localhost`、`.local`、`.internal`、`.home.arpa` 等本地名称
- 纯数字、十六进制、八进制或其他可能被浏览器解释成另类 IP 的主机名
- hostname 为空或格式非法
- 不在管理员明确启用的 `connectivity_probe_allowed_origins` 中
- 探针 origin 与展示 API URL origin 不一致

浏览器请求还必须使用 `redirect: "error"`，防止公开 URL 把探针重定向到第三方或用户局域网。

`connectivity_probe_allowed_origins` 是管理员设置，不作为任意请求目标使用；后端只把它与现有公开 API URL 求交集。该列表只能包含平台实际控制的精确域名，不接受通配符或用户提交的域名。

保存配置、应用启动和周期复核时，后端解析允许 origin 的 A/AAAA 记录；DNS 失败、无地址或任一结果不是全局公网地址时，将该 origin 排除出公开 DTO 并产生管理员告警。

浏览器侧 DNS 可能受本地 hosts、污染、分流 DNS 或重绑定影响，服务端无法完全证明用户侧解析结果。MVP 明确接受这一残余风险，并通过“仅平台自有 HTTPS 域名、公开 CA 证书、固定无副作用 GET、`credentials: omit`、禁止重定向、不携带自定义头”降低风险。不得宣称能够从网页 JavaScript 获得或验证目标域名最终连接的 IP。

### 7.4 带路径的 API URL

例如 `https://api.example.com/v1` 的探针仍固定为：

```text
https://api.example.com/.well-known/sub2api/edge-probe
```

这避免错误拼成 `/v1/.well-known/...`。该 origin 只有在部署验收确认固定探针与真实 API 请求经过同一公开入口和同一中转上游后才可启用。

MVP 不区分同一 origin 下的不同路径路由质量。若未来确实存在同域不同路径走不同上游，二期端点注册表增加受控 `probe_path`，本期不接受任意探针路径。

### 7.5 数量边界

现有 `custom_endpoints` 最多 10 个，因此 MVP 最多测试默认 URL 加 10 个自定义 URL。超过该规模时进入二期端点注册表，不在 MVP 中放宽现有限制。

新增公开 URL 时不需要发布新前端，但必须依次完成：

1. 配置 DNS、HTTPS 证书和正常 API 转发。
2. 加入现有 `api_base_url` 或 `custom_endpoints`。
3. 将其精确 origin 加入 `connectivity_probe_allowed_origins`。
4. 验证固定探针、CORS、TAO、no-store 和限流。
5. 验证普通 API 调用不受影响后，才向用户开放该 URL 的检测。

## 8. 探针协议

### 8.1 请求

```text
GET {origin}/.well-known/sub2api/edge-probe?nonce={128-bit随机值}
```

浏览器请求选项：

```ts
{
  method: 'GET',
  mode: 'cors',
  credentials: 'omit',
  cache: 'no-store',
  redirect: 'error',
  referrerPolicy: 'no-referrer',
  signal
}
```

不得携带 Cookie、Authorization、API Key、自定义用户标识或业务参数。

计时从调用 `fetch()` 前开始，不能在收到响应头时结束。只有在完整读取响应体、确认不超过 1 KiB、解析 JSON 并通过 §8.2 协议校验后才停止计时并记为成功样本。

### 8.2 响应

统一使用 HTTP 200 和小型 JSON，不再同时支持 200/204 两种语义：

```json
{
  "ok": true,
  "client_ip": "113.110.12.34",
  "client_location": {
    "country_code": "CN",
    "country": "中国",
    "region": "广东",
    "city": "深圳"
  }
}
```

要求：

- 响应体硬上限为 1 KiB（`client_location` 可选，超出则视为协议错误）
- `Content-Type: application/json; charset=utf-8`
- `Cache-Control: no-store, max-age=0`
- 不设置 Cookie
- 不返回服务器 ID、内部主机名、源站 IP、中转 IP 或拓扑信息
- `client_ip` 仅在 §9 的严格规则通过时返回，否则必须为 `null`
- `client_location` 为对象或 `null`：IP 展示关闭、可信解析失败或安全条件不满足时两者均为 `null`；GeoIP 缺失/损坏/查询失败/无匹配时允许返回已验证 `client_ip` 而 `client_location: null`
- `client_location` 只含国家代码、国家、一级行政区和城市；不返回经纬度、邮编、时区、ASN、运营商、内部节点或代理链

客户端成功样本必须同时满足：

1. Fetch 未超时、未取消且未发生重定向。
2. HTTP 状态严格为 200。
3. Content-Type 的媒体类型严格为 `application/json`，允许标准 charset 参数。
4. 完整响应体不超过 1 KiB。
5. JSON 是对象且 `ok === true`。
6. `client_ip` 为 `null` 或可规范解析的 IPv4/IPv6 字符串。
7. `client_location` 为 `null` 或严格校验通过的 `{country_code,country,region,city}`（各字段为有界字符串、无异常 Unicode 控制字符）；超长、错误类型或控制字符判 `protocol_error`。
8. 所有必需字段完整且类型正确；额外字段不影响 MVP，但不得被前端作为安全或评分依据。

状态为 200 但响应超限、JSON 无法解析或 schema 不匹配时归类为 `protocol_error`，该 origin 本轮显示“检测未完成”，不得计入连接质量等级。

### 8.3 CORS 与 Resource Timing

每个 API URL 必须对面板来源返回：

- `Access-Control-Allow-Origin: {精确面板 Origin}`
- `Vary: Origin`
- `Timing-Allow-Origin: {精确面板 Origin}`

探针不携带凭证，因此不需要 `Access-Control-Allow-Credentials`。

MVP 不通过自定义响应头返回出口 IP 或端点 ID，因此不依赖 `Access-Control-Expose-Headers`。

若 Origin 不在白名单，探针应拒绝跨域读取。生产环境不得使用 `*` 放开所有来源。

TAO 只影响 Resource Timing 的可选阶段明细，不影响 `performance.now()` 记录总耗时。TAO 缺失时仍可完成基础评分，但部署验收必须报告该配置缺口。

### 8.4 中转要求

“中转零改动”不是无条件承诺。每个 URL 上线前必须确认：

1. 根路径 `/.well-known/sub2api/edge-probe` 被转发到同一 Sub2API 主应用。
2. 不缓存探针响应。
3. 不覆盖或剥离 CORS、TAO 和 no-store 响应头。
4. 可信代理链正确追加 `X-Forwarded-For`，不接受客户端伪造覆盖。
5. 不把内部主机名、源站 IP 或调试头写入响应。

现有 catch-all 转发满足以上要求时无需修改；不满足时必须增加一条最小、受控的探针转发配置。不得为了坚持“零改动”而返回错误结果或冒险泄露节点 IP。

## 9. 用户出口 IP 的安全解析

### 9.1 默认关闭

`connectivity_client_ip_enabled` 默认 `false`。只有所有生产入口完成可信代理链测试后才允许开启。

当 `server.trusted_proxies` 未显式配置、`connectivity.client_ip_denied_cidrs` 为空或可信链验收未通过时，后端必须把出口 IP 功能视为未启用并始终返回 `null`，同时记录管理员可见告警。展示开关关闭不妨碍服务端在内存中使用经过验证且 HMAC 化的客户端 IP 做探针限流。

### 9.2 解析算法

不得直接复用兼容模式的 `ip.GetClientIP`，也不能只返回 `c.ClientIP()`。新增专用的“带来源证明的公网客户端 IP 解析”能力：

1. 使用 `net.SplitHostPort` 和 `netip.ParseAddr` 解析 TCP 直接对端 `RemoteAddr`，并对 IPv4-mapped IPv6 执行 `Unmap()`。
2. MVP 只接受一个 `X-Forwarded-For` 字段；重复字段、空 token 或任一非法 token 直接返回空。
3. XFF 按逗号解析为从最早客户端到最近代理的地址列表，末尾再追加 `RemoteAddr`，形成完整链。
4. 完整链最多允许 `connectivity.client_ip_max_hops` 个地址，默认 8；超过上限返回空。
5. `RemoteAddr` 必须属于 `server.trusted_proxies`。从链最右侧开始剥离连续的可信代理地址，第一个非可信地址就是候选公网出口。
6. 若右侧可信链断裂、没有非可信候选、XFF 缺失或候选有歧义，返回空；不得继续向左寻找“看起来更像用户”的地址。
7. 候选必须是合法、全局可路由的公网地址，并拒绝回环、私网、链路本地、组播、未指定、文档保留和其他非全局地址。
8. 候选不得命中 `connectivity.client_ip_denied_cidrs` 中配置的任何我方公网节点、中转或源站地址。
9. `connectivity.allow_direct_client_ip=true` 时，若直接对端不属于可信代理，则完全忽略所有转发头，只校验并返回 `RemoteAddr`；默认配置下不启用该分支。
10. MVP 不读取 `X-Real-IP`、`CF-Connecting-IP` 或自定义客户端 IP 头，避免不同入口产生不一致优先级。

解析函数返回：

```go
type VerifiedClientIP struct {
    IP     netip.Addr
    Source string
    OK     bool
}
```

`Source` 仅供服务端测试和日志判断，不返回前端。

### 9.3 多 URL 出口

出口 IP 来自每个 URL 自己的探针响应，而不是只访问面板域名的单独接口。这样 PAC、VPN 和按域名分流场景不会把一个 URL 的出口误当成所有 URL 的出口。

### 9.4 本地 GeoIP 地区解析

地区解析使用应用服务器本地 MMDB（推荐 MaxMind GeoLite2 City 兼容库），`internal/pkg/geoip` 提供小接口 `Lookup(netip.Addr) (*Location, error)` / `Ready()` / `Close()`，`Location` 只含 `CountryCode / Country / Region / City`。

约束：

1. 不调用 `ip-api.com`、`ipinfo.io` 等第三方接口；用户 IP 不发送给任何第三方。
2. `connectivity.geoip_database_path` 为空表示未配置（地区关闭）；配置了路径但文件打不开/损坏时记录明确告警、地区 fail closed，不影响 Sub2API 启动，也不影响探针 200 与出口 IP。
3. MMDB 启动时打开一次，不在每个探针请求中重新打开；查询只接受已通过 §9.2 验证的公网 IP，非公网不查询。
4. 查询失败按 IP 短时缓存 + 限频告警，避免日志刷屏；国家/地区名称优先中文（`geoip_locale`，默认 `zh-CN`），缺失时按 base locale → en → 任意稳定回退；保留 ISO country code 供前端格式化。
5. 数据库更新属于部署运维流程，不在应用请求路径实现在线下载。

## 10. 浏览器测量

### 10.1 评分使用的可靠指标

每次请求使用 `performance.now()` 记录总耗时，评分只使用：

- 正式样本成功数
- 正式样本失败数
- 成功样本总耗时
- P95 总耗时
- 中位数总耗时
- 抖动
- 连续超时次数

其中**中位数总耗时（`medianMs`）同时用于界面展示“典型延迟”**（四舍五入整数毫秒）；P95/抖动/成功率只用于内部评级，不向用户展示。

### 10.2 尽力采集指标

`PerformanceResourceTiming` 在浏览器允许且 TAO 正确时，可在内存中尽力采集：

- DNS 查询耗时
- TCP 连接耗时
- TLS 握手耗时
- TTFB
- 协商协议

这些值可能因连接复用、隐私限制、浏览器实现或失败请求而缺失或为零，因此：

1. 不参与等级计算。
2. 不向用户展示。
3. 不要求所有浏览器都能采集。
4. 不能据此宣称准确区分 DNS、TCP 或 TLS 故障。

### 10.3 错误分类

浏览器只使用能够可靠判断的分类：

| 分类 | 判定 |
| --- | --- |
| `timeout` | 前端超时控制器主动终止 |
| `http_error` | 浏览器成功读取响应，但状态不是 200 |
| `network_or_cors` | Fetch 统一网络异常，可能是 DNS/TCP/TLS/CORS/重定向 |
| `protocol_error` | 状态为 200，但响应超限、JSON 或 schema 不符合探针协议 |
| `cancelled` | 用户取消、页面隐藏或组件卸载 |
| `rate_limited` | 成功读取 429 |

`network_or_cors` 不再细分为 DNS、TCP、TLS 或 CORS。单个 origin 出现该错误时按失败样本处理；若所有 origin 的所有正式样本都只得到该错误，则整轮显示“检测未完成”，避免把浏览器、CSP 或面板级跨域配置故障全部误判成 URL 不推荐。

### 10.4 采样方式

1. 每个 origin 先执行一次预热请求，预热不参与评分。
2. 默认执行 10 次正式请求。
3. 多个 origin 采用交错轮询，避免先后测试处于不同网络状态。
4. 同时在飞的 origin 默认不超过 3 个。
5. 单次请求默认 10 秒超时。
6. 每次请求使用独立随机 nonce。
7. 相同 origin 只采样一次，结果映射给该 origin 下的所有展示 URL。
8. 页面进入后台或组件卸载时取消整轮检测。浏览器能够通过 `online`、`offline` 或 Network Information API 检测到网络切换时也取消；无法被浏览器观测的网络变化只能自然反映在样本中，不作“全部可检测”承诺。
9. 上述取消结果统一显示“未完成”，不得判为“不推荐”。

## 11. 等级计算

### 11.1 数学定义

设成功样本总耗时升序排列为 `L`：

- 成功率：`success_count / planned_sample_count`
- 中位数：标准 50 分位数
- P95：nearest-rank，索引为 `ceil(0.95 * n) - 1`
- 抖动：成功样本总耗时相对其中位数的绝对偏差中位数（MAD）

正式样本不足计划数量且原因是取消、页面隐藏或设置变化时，本轮为“未完成”，不计算等级。

### 11.2 默认阈值

| 等级 | 默认条件 |
| --- | --- |
| 不推荐 | 成功率低于 80%，或出现 2 次连续超时 |
| 优秀 | 未触发“不推荐”，成功率 100%，P95 ≤ 250ms，MAD ≤ 50ms |
| 良好 | 未触发“不推荐”，成功率至少 90%，P95 ≤ 500ms，MAD ≤ 120ms |
| 一般 | 未触发“不推荐”，成功率至少 80%，但未达到“良好” |

补充规则：

1. 成功率按实际配置的正式样本数计算；默认 10 次时分别对应 10/10、至少 9/10 和至少 8/10。
2. 429 不计入 URL 网络质量，整轮显示“检测过于频繁，请稍后重试”。
3. 设置加载失败、所有 origin 均为统一网络错误或探针协议不匹配时显示“检测未完成”，不得给出网络质量结论。
4. 只有完整执行计划样本后才可显示四档等级。
5. 阈值必须满足“优秀严格于良好，良好严格于一般”的单调关系。
6. 判定顺序固定为：特殊未完成状态 → 不推荐 → 优秀 → 良好 → 一般，确保所有条件互斥。
7. 任一 `protocol_error` 直接使对应 origin 本轮未完成，不进入四档判定。

### 11.3 推荐规则

1. 稳定性优先于最低延迟。
2. 只有“优秀”或“良好”可成为推荐项。
3. 多个 URL 同级时依次比较成功率、P95、MAD 和中位数。
4. 上述指标完全相同时优先管理员设置的默认 API URL；仍相同时按公开端点原始顺序选择第一个，保证结果确定。
5. 不自动修改第三方客户端配置，只提供复制 URL。

## 12. 设置设计

### 12.1 公开设置接口

复用现有准确接口：

```text
GET /api/v1/settings/public
```

新增公开字段：

| 设置键 | 默认 | 合法范围 |
| --- | --- | --- |
| `connectivity_test_enabled` | `false` | 布尔 |
| `connectivity_client_ip_enabled` | `false` | 布尔 |
| `connectivity_grade_thresholds` | §11.2 | 合法 JSON、阈值单调、带 `grading_version` |
| `connectivity_probe_samples` | `10` | 5 到 20 |
| `connectivity_probe_warmup` | `1` | 0 到 2 |
| `connectivity_probe_max_concurrency` | `3` | 1 到 3 |
| `connectivity_probe_timeout_ms` | `10000` | 2000 到 15000 |
| `connectivity_test_endpoints` | `[]` | 后端生成的只读安全 DTO |

`connectivity_probe_allowed_origins`、探针服务端限流、内部节点 CIDR 和直连策略属于管理员或服务端配置，不在公开 DTO 中原样暴露。

`connectivity_probe_allowed_origins` 表示“允许被浏览器测试的目标 API origin”；`cors.allowed_origins` 表示“允许发起跨域读取的面板页面 origin”。两者用途不同，必须分别校验，不能互相替代。

管理员设置新增：

| 设置键 | 默认 | 说明 |
| --- | --- | --- |
| `connectivity_probe_allowed_origins` | `[]` | 允许浏览器主动探测的精确 HTTPS origin，不支持通配符 |
| `connectivity_probe_ip_rpm` | `360` | 单个已验证用户 IP 每分钟探针请求预算 |
| `connectivity_probe_burst` | `250` | 允许一轮检测在短时间内完成的最大突发请求数 |

服务端部署配置新增：

| 配置键 | 默认 | 说明 |
| --- | --- | --- |
| `connectivity.client_ip_denied_cidrs` | `[]` | 我方公网节点、中转、源站及其他绝不允许返回的地址 |
| `connectivity.allow_direct_client_ip` | `false` | 是否允许把非代理直接对端视为用户，生产默认禁止 |
| `connectivity.client_ip_max_hops` | `8` | XFF 加直接对端允许的最大完整链长度，范围 2 到 16 |
| `connectivity.geoip_database_path` | `` | 本地 GeoLite2-City MMDB 路径；空=地区关闭；配置但打不开时告警并 fail closed，不影响启动 |
| `connectivity.geoip_locale` | `zh-CN` | 地区名称语言（BCP-47 风格，如 `zh-CN`、`en`） |

管理员设置页面只提供功能配置和校验反馈，不提供测试历史、用户 IP 查询或诊断数据入口。IP 展示开关旁提供**只读 GeoIP 状态提示**（已就绪 / 未配置 / 数据不可用），不暴露 MMDB 文件路径。该状态只出现在管理员 DTO，不出现在公开 DTO。

### 12.2 刷新语义

现有前端 `fetchPublicSettings()` 默认使用缓存，因此检测对话框打开时必须执行 `fetchPublicSettings(true)`：

- 刷新成功后才允许开始检测。
- 刷新失败时显示“暂时无法加载检测配置”，不使用过期阈值启动新测试。
- 检测开始后冻结本轮配置快照，管理员中途修改设置不改变正在运行的一轮。
- 下一轮重新强制刷新并使用新 `grading_version`。

用户界面不展示 `grading_version`，它只用于本地结果一致性和测试断言。

## 13. 后端实现边界

### 13.1 新增探针路由

```text
GET /.well-known/sub2api/edge-probe
```

路由注册在 API Key/JWT 鉴权和业务中间件之外，但仍经过必要的安全头、CORS 和专用限流。

探针 handler 不允许依赖：

- repository
- 数据库
- Redis 业务缓存
- 计费或订阅 service
- 渠道、账号和模型 service

### 13.2 公开设置

沿用现有 `SettingService.GetPublicSettings`：

1. 加载检测开关与参数。
2. 解析 `api_base_url` 和 `custom_endpoints`。
3. 与允许 origin 求交集。
4. 生成规范化的 `connectivity_test_endpoints`。
5. 不把内部节点配置放入公开 DTO。

探针路由使用启动时或设置更新时生成的只读内存配置快照。每次探针请求不得查询设置表；管理员保存设置后原子替换快照。

### 13.3 IP 解析归属

新增专用可信 IP 解析器位于 `internal/pkg/ip`（`VerifiedClientIPResolver`），由 `service` 定义聚合结果并注入 `internal/pkg/geoip` 的 `Resolver`。`service` 不得导入 `repository`、gorm 或 redis。

- `SettingService.ConnectivityProbeClientContext(req)` 聚合“已验证公网 IP + 可选本地 GeoIP 地区”，供探针 handler 使用；`ConnectivityProbeClientIP` 保留为兼容委托。
- 地区解析使用**本地** `internal/pkg/geoip`（MMDB），不复用 `repository/proxy_probe_service.go` 的 `parseIPAPI`，不调用任何第三方接口。

### 13.4 生成代码

若新增 handler 或依赖注入绑定，必须更新 Wire：

```bash
cd backend
go generate ./cmd/server
```

## 14. 限流、容量与日志

### 14.1 前端请求预算

最大请求量：

```text
唯一 origin 数量 × (预热次数 + 正式样本数)
```

MVP 最多 11 个展示 URL，但相同 origin 去重。服务端和前端都必须限制：

- 单 origin 正式样本不超过 20
- 同时在飞 origin 不超过 3
- 单轮总探针请求不超过 250
- 同一页面不能并行启动两轮

后端在更新采样参数、允许 origin 或公开端点时执行跨字段校验：

```text
unique_origin_count × (warmup_count + sample_count) <= 250
```

超过预算时拒绝保存；应用启动时发现已有配置超限则 fail closed，关闭检测并记录管理员告警。前端仅作第二层防御，不能替代后端校验。

### 14.2 服务端限流

探针使用独立的有界内存限流器，不能直接套用可能不足以容纳一轮测试的普通公开接口 RPM：

- 按经安全解析的公网客户端 IP 限流
- 无法解析用户 IP 时使用直接对端和全局桶兜底
- 支持合理 burst，使一次正常检测可完成
- 超限返回 429 和固定小响应
- 每个应用实例独立限流，Nginx/边缘入口再提供整体请求速率上限
- 内存桶必须有容量上限、空闲 TTL 和周期清理，防止随机来源耗尽内存
- 限流过程不访问 Redis 或业务数据库

### 14.3 日志

“无持久化 MVP”指不建立专用测试数据表或产品诊断记录，不代表网络基础设施天然没有访问日志。

为避免探针造成日志洪泛：

1. 应用访问日志跳过成功的探针请求，或使用不含客户端 IP 的低比例采样。
2. 429、5xx 和协议异常保留采样日志。
3. Nginx/边缘访问日志沿用现有安全与保留策略，不新增 7 天测试数据集。
4. 不把出口 IP、逐次耗时或评分写入业务日志。
5. 内存限流键使用带服务端密钥的 IP HMAC 和短 TTL，不保存完整 IP。

因此管理员不能依靠本功能查询某个用户的完整测试过程；普通运维日志也不等同于管理端诊断功能。

## 15. 安全与隐私

1. 浏览器请求使用 `credentials: "omit"`，不携带 Cookie。
2. 不读取、提交或保存完整 API Key。
3. 不接受用户提供的任意目标 URL。
4. 只测试后端返回的 HTTPS 同源探针 URL。
5. 禁止跨 origin 重定向。浏览器会将重定向与 DNS、TCP、TLS、CORS 等 Fetch 网络异常统一处理为 `network_or_cors`；单个样本作为失败样本，只有所有正式样本均为该错误时整轮显示“检测未完成”。
6. CORS 和 TAO 使用精确面板 Origin，不使用通配符。
7. 出口 IP 默认关闭，开启后仍必须 fail closed。
8. 不向第三方发送用户 IP；地区解析使用本地 GeoIP MMDB（配置路径后启用，未配置/不可用时地区关闭）。
9. 响应不包含内部主机名、源站 IP、中转 IP、容器名或拓扑信息；IP/地区不进入业务日志与缓存。
10. 探针不得反射请求参数、放大响应或访问外部资源。
11. CSP 的 `connect-src` 必须允许已审核 HTTPS API URL；不得为此放开 HTTP 或任意危险 scheme。
12. 任一安全校验失败时只禁用检测，不影响普通 API 调用。
13. 探针响应硬上限 1 KiB；前端对 `client_location` 做严格校验（超长/错误类型/异常控制字符判 `protocol_error`）。

## 16. 错误归因

| 现象 | 用户结果 | 是否影响普通 API |
| --- | --- | --- |
| 设置加载失败 | 检测暂不可用 | 否 |
| 单个 URL 探针 404/5xx | 作为该 URL 的失败样本；管理员需检查转发或应用 | 否 |
| 单个 URL CORS 缺失 | 浏览器表现为 `network_or_cors`，作为该 URL 的失败样本 | 否 |
| 所有 URL 均为统一网络错误 | 整轮检测未完成 | 否 |
| TAO 缺失 | 仍可基础评分，但无可选阶段明细 | 否 |
| 探针 429 | 检测过于频繁 | 否 |
| 浏览器超时或网络错误 | 计入该 URL 本轮连接结果 | 否 |
| 页面隐藏或用户取消 | 本轮未完成 | 否 |
| AI 上游 4xx/5xx | 本功能不可见，由渠道监控处理 | 否 |

探针 5xx属于应用或中转探针异常，不得被描述为用户“线路不好”。只有可完整执行的样本才生成四档等级。

## 17. 前端改造范围

### 17.1 用户端

1. 修改 `frontend/src/components/keys/EndpointPopover.vue`，移除第三方测速跳转。
2. 新增页面内检测对话框或抽屉。
3. 新增连接检测 composable：
   - 配置强制刷新
   - origin 去重
   - 交错采样
   - AbortController 超时与取消
   - 本地评分
   - `sessionStorage` 最小缓存（含 `median_ms`，不含 IP/地区）
4. 展示“典型延迟”（`medianMs` 整数毫秒），无成功样本显示“暂无可用延迟”。
5. 展示已验证出口 IP 与估算地区（顶部汇总/分流逐行/无法识别三种形态），地区文案带“估算”并允许“地区未知”。
6. 新增中文和英文语言键（不得漏 key）。
7. 使用现有图标库，不新增手绘 SVG。
8. 用户组件只接收公开 DTO，不能获得内部服务器字段。
9. 组件卸载和页面隐藏时必须取消请求，避免悬挂任务。
10. IPv6 出口地址完整显示并允许换行，桌面端与手机端不重叠、不横向溢出。

### 17.2 管理端设置

1. 在现有系统设置页面增加功能总开关和出口 IP 开关。
2. 出口 IP 开关旁增加**只读 GeoIP 状态**（已就绪 / 未配置 / 数据不可用），不暴露数据库路径。
3. 增加允许探测 origin 列表，必须从现有公开 API URL 中选择或经过同等严格校验。
4. 采样数、并发、超时和评分阈值放入“高级设置”，普通用户不可见。
5. 保存前展示预计单轮最大请求数，超过 250 时拒绝保存；后端必须执行相同的跨字段校验。
6. 不增加“测试记录”“用户 IP”“地区分布”或“诊断详情”等入口。

## 18. 测试要求

### 18.1 后端单元测试

- 探针固定 200 JSON、响应大小和 no-store
- 探针不需要 API Key/JWT
- 探针不产生数据库、Redis 业务缓存、UsageLog 或上游调用
- CORS/TAO 精确 Origin
- 非白名单 Origin 无法跨域读取
- 端点 URL 标准化与 origin 去重
- HTTP、userinfo、query、fragment、本地主机名、另类数字 IP、私网和未允许 origin 被排除
- DNS 解析失败或任一 A/AAAA 为非全局地址时端点被排除
- 带路径 API URL 正确生成根路径探针
- XFF 重复字段、空 token、非法地址、超过最大跳数和可信链断裂全部 fail closed
- 可信代理链、伪造 XFF、缺失 XFF、直连、内网和我方节点 CIDR全部 fail closed
- 开关与所有数值范围校验
- 单轮请求预算的后端跨字段校验和启动 fail closed
- 429 不进入业务错误分类
- GeoIP：有结果、无匹配、文件缺失、损坏数据库、IPv4/IPv6 查询、非公网 IP 不查询
- IP 展示关闭时 `client_ip` 与 `client_location` 均为空；可信代理链未完整配置时 fail closed
- 探针 handler 响应协议含 `client_location`、`no-store`、不返回内部信息
- embed 构建下探针不返回 SPA HTML；CORS 精确 Origin 与 OPTIONS；限流不回归

### 18.2 前端单元测试

- 不打开新页面
- 强制刷新设置后才开始
- 设置失败时不启动
- 只请求后端 DTO 中的 `probe_url`
- 请求固定为 `credentials: omit`、`redirect: error`、`cache: no-store`
- 相同 origin 去重并映射结果
- 完整读取并校验 JSON 后才停止计时和记录成功
- 响应超限、JSON/schema 错误触发 `protocol_error` 和“未完成”
- nearest-rank P95、MAD 和四档边界
- 429、取消、页面隐藏和配置错误不判为“不推荐”
- 只在浏览器能够观测到网络切换事件时取消，不假设所有切换均可检测
- 多 URL 出口相同、不同和无法识别的展示
- 用户不看到内部指标和评分版本
- 30 分钟缓存不包含 IP 和原始样本
- 移动端布局、文本溢出和色觉无障碍

### 18.3 集成与端到端测试

- 使用至少两个真实 HTTPS 域名跨域检测
- 模拟延迟、抖动、超时、5xx、429 和 CORS 缺失
- 验证中转到主应用的完整探针路径
- 验证 PAC/VPN 分流下不同 URL 可返回不同用户出口
- 验证任何情况下都不返回我方节点 IP
- 验证探针流量不会产生 UsageLog、扣费、订阅消耗、账号封禁或渠道状态变化
- 验证探针失败不会让正常 API 请求失败

## 19. 发布策略

1. 后端先发布探针、设置字段和安全端点 DTO，功能保持关闭。
2. 在每个公开 API URL 上验证探针路径、CORS、TAO、no-store和限流。
3. 验证可信代理链；出口 IP 功能继续保持关闭。
4. 发布前端组件，入口仍由总开关隐藏。
5. 先由管理员在真实家庭宽带、移动网络、代理/VPN 和分流环境测试。
6. 小范围开启连接等级，不开启出口 IP。
7. 确认无 UsageLog、扣费、订阅、渠道和日志洪泛问题后全量开启等级。
8. 单独完成所有节点 IP 防泄露测试后，才可考虑开启出口 IP。
9. 任一端点探针异常时从允许 origin 中移除该端点的检测资格，不下线普通 API URL。

## 20. MVP 验收标准

必须全部满足：

1. 用户检测时不离开 API 密钥页面。
2. 请求由用户当前浏览器直接发往真实 API URL 的固定探针。
3. 用户性能结论只有“优秀、良好、一般、不推荐”四档，每行补充“典型延迟”（`medianMs` 整数毫秒），无成功样本显示“暂无可用延迟”。
4. 文案始终限定为“当前设备和当前网络访问该 URL 的本次表现”。
5. 用户界面和探针响应不额外展示服务器国家、物理角色、我方内部 IP 或中转拓扑。
6. 出口 IP关闭时完全不返回；开启后无法确认即返回空；地区仅由本地 GeoIP 提供，估算并允许“地区未知”。
7. IP/地区/原始样本/P95/MAD 不进入缓存与业务日志；缓存只含 URL/等级/`median_ms`/时间/版本。
7. 不接收或保存 API Key。
8. 不建立测试会话、测试表、管理员诊断页或 7 天数据。
9. 管理员明确知道 MVP 无法查看用户测试详情。
10. 测试不产生 UsageLog、扣费、订阅消耗、账号封禁或上游模型调用。
11. 浏览器不能探测后端未批准的 URL。
12. 429、配置错误和取消不会被误判为“不推荐”。
13. 探针故障不会影响正常 API 调用。
14. 所有设置有默认值、范围校验和关闭回退。
15. 后端 unit、integration、lint、build 和前端 test、typecheck、lint、build 全部通过。

## 21. 二期：管理员诊断数据层

二期独立设计和评审，不是 MVP 的隐藏能力。计划范围：

- 服务端测试会话
- 原始样本上报与服务端重新评分
- 测试编号
- 完整 IP 和用户/API Key 关联
- 管理员明细、搜索和导出
- 原始数据严格保存 7 天后物理删除
- 不含个人信息的长期匿名聚合
- 地区、运营商和 ASN 分布
- 正式端点注册表和超过 10 个端点
- RBAC、敏感读取审计和滥用防护

二期不能直接信任一期浏览器给出的等级；必须上报原始样本并由服务端按对应 `grading_version` 重新计算。

## 22. 已确认决策

1. 页面内检测，不跳转第三方页面。
2. 用户选择 URL，不选择或感知服务器。
3. 测量来自用户当前浏览器和真实网络。
4. MVP 只做基础连接检测，不做真实 API Key 或 AI 模型调用。
5. MVP 评分在浏览器本地完成。
6. MVP 不保存测试数据，不提供管理员诊断和 7 天历史。
7. 管理员诊断、完整 IP 保存 7 天和匿名聚合明确推迟到二期。
8. 用户性能结果只有四档，不展示内部指标（P95/MAD/成功率/原始样本）。
9. 出口 IP 是可选环境信息，默认关闭并严格 fail closed。
10. 不向第三方发送用户 IP；地区解析使用本地 GeoIP MMDB（`connectivity.geoip_database_path`），未配置/不可用时地区关闭。
11. 探针使用固定同 origin HTTPS 路径，不接受任意目标或重定向。
12. 中转是否需要配置以实际验收为准，不作无条件“零改动”承诺。
13. 测试与计费、订阅、UsageLog、渠道监控和账号状态完全隔离。
14. 功能总开关默认关闭，按端点逐步放量。
15. 每行展示“典型延迟”（成功样本 `medianMs` 整数毫秒），延迟不是模型响应速度。
16. 出口 IP 与估算地区是网络环境信息，不参与端点评级和推荐；地区一致性要求同 IP 才展示。
17. 探针响应硬上限 1 KiB；`client_location` 严格校验，超长/错误类型/控制字符判 `protocol_error`。
18. 浏览器 30 分钟缓存只新增 `median_ms`，向后兼容旧记录；IP、地区、P95/MAD、样本与代理链一律不入缓存。
