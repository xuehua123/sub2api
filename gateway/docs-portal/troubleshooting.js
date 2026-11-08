// Operator-provided troubleshooting posters, transcribed into usable articles.
// Version-dependent Codex behavior is explicitly scoped; no visitor secrets are collected.
const restartNotice = () =>
  callout(
    "必须完成：保存并启用 → 完全退出 → 重新打开",
    "在 CCSwitch 修改配置后，先保存手头任务，再<strong>完全退出 Codex、Claude Code 等目标工具</strong>。桌面端退出应用及托盘/菜单栏中的后台实例；CLI 结束正在运行的会话；VS Code 扩展需关闭承载它的 VS Code 后重新打开。只关对话、新建任务或只重启 CCSwitch，都不等于重启目标工具。",
  );
const originalCCSwitchBody = guides.ccswitch.body;
guides.ccswitch.body = () =>
  `<div class="recommended-banner"><span>推荐路径 · 更新后编辑现有供应商</span><strong>刷新模型列表 → 保留 5.6 及以上 → Terra 用最新版本</strong><p>教程使用自绘示意，不展示真实账户、Key、余额或本机路径。已有皮皮虾 AI 供应商直接编辑即可；没有供应商时再从 API 密钥页面导入。</p><div class="doc-actions"><a class="doc-primary" data-console-path="/keys" href="${state.baseURL}/keys" target="_blank" rel="noopener noreferrer">打开 API 密钥页面 ↗</a><a class="doc-secondary" href="https://github.com/farion1231/cc-switch/releases" target="_blank" rel="noopener noreferrer">下载最新 CCSwitch ↗</a></div></div>
  <h2>1. 更新 CCSwitch</h2><p>前往 <a href="https://github.com/farion1231/cc-switch/releases" target="_blank" rel="noopener noreferrer">CCSwitch 官方 Releases 页面</a>，安装最新版本。更新后重新打开 CCSwitch；如果它仍显示旧界面，完全退出后再启动一次。</p>
  <h2>2. 打开 Codex 供应商编辑页</h2><ol><li>在顶部切换到 <b>Codex</b>。</li><li>找到正在使用的皮皮虾 AI 供应商，点击卡片右侧的铅笔形<b>编辑</b>按钮。</li><li>确认 API 请求地址是 <code>${state.baseURL}/v1</code>，协议选择 <b>Responses</b>。不要把 API Key 写进教程、截图、工单或 Git。</li></ol>${callout("没有现成供应商？", "先在<a data-console-path=\"/keys\" href=\"${state.baseURL}/keys\" target=\"_blank\" rel=\"noopener noreferrer\">API 密钥页面 ↗</a>创建或选择对应编程分组的 Key，再使用「导入到 CCS」。导入链接包含 Key，只在本机浏览器和 CCSwitch 之间使用。")}
  <h2>3. 获取模型列表并清理旧模型</h2><p>在编辑页的“模型列表”区域点击<b>获取模型列表</b>，等待当前 Key 的列表刷新。只勾选 5.6 及以上模型；按当前版本可保留 <code>gpt-5.6-sol</code>、<code>gpt-5.6-terra</code>、<code>gpt-6-astra</code>、<code>gpt-6.1-sol</code> 和 <code>gpt-6.1-terra</code>。取消不需要的旧模型，并删除列表中残留的 <code>gpt-5.5</code>。</p><figure class="guide-figure"><img src="./assets/ccswitch/model-selection.svg" width="1100" height="620" alt="自绘示意：刷新模型列表后仅保留 gpt-5.6 及以上模型并移除 gpt-5.5" loading="lazy"/><figcaption>图 1 · 自绘模型选择示意。实际可用模型以当前 Key 刷新后的列表为准。</figcaption></figure><p>点击“应用”或“保存模型列表”，确认 5.5 没有继续出现在已选列表里。</p>
  <h2>4. 设置默认模型、子代理和自动审批</h2><ol><li>默认模型选择 <code>gpt-5.6-sol</code>。</li><li>“子代理模型”选择当前列表中的最新 Terra，本次应为 <code>gpt-6.1-terra</code>。</li><li>“自动审批模型”“自动审批使用模型”或同义字段也选择 <code>gpt-6.1-terra</code>。如果当前版本只有一个自动审批下拉框，就选择同一个最新 Terra。</li><li>保存供应商并启用它。保存前检查这些字段没有回退到 <code>gpt-5.5</code>。</li></ol><figure class="guide-figure"><img src="./assets/ccswitch/terra-settings.svg" width="1100" height="560" alt="自绘示意：默认模型为 gpt-5.6-sol，子代理和自动审批模型为 gpt-6.1-terra" loading="lazy"/><figcaption>图 2 · 自绘设置示意。字段名称可能随 CCSwitch 版本变化，按含义选择最新 Terra。</figcaption></figure>
  <h2>5. config.toml 的推荐值</h2><p>如果你通过 CCSwitch 的 Config 视图或本地 <code>~/.codex/config.toml</code> 管理 Codex，可以把主模型和供应商配置整理成下面的结构。真实 Key 由本机认证流程提供，不要粘贴到文档或共享配置。</p>${snippet("config.toml", `model_provider = "custom"\nmodel = "gpt-5.6-sol"\nmodel_reasoning_effort = "high"\ndisable_response_storage = true\nservice_tier = "priority"\n\n[model_providers.custom]\nname = "皮皮虾 AI"\nbase_url = "${state.baseURL}"\nwire_api = "responses"\nrequires_openai_auth = true`)}<p>若 CCSwitch 会自动写入供应商段落，以 CCSwitch 的保存结果为准，不要同时手改多个入口。模型名必须来自刚刷新过的当前 Key 列表。</p>
  <h2>6. 完全退出 Codex，再重新打开</h2>${restartNotice()}
  <h2>7. 发送简单任务并核对记录</h2><p>重开后发送一条短请求，确认实际使用的是刚设置的模型。然后打开<a data-console-path="/usage" href="${state.baseURL}/usage" target="_blank" rel="noopener noreferrer">调用记录 ↗</a>，核对模型、请求状态和消费来源。仍然 Reconnecting 时按<a href="#codex-connection">连接问题指南</a>排查。</p>${clientHeadersNotice()}
  <h2>备用：手动配置与项目生图</h2><p>一键导入不可用，或需要额外配置时再展开下面内容。生图 Key 与编程 Key 分开保存。</p><details class="guide-details"><summary>展开手动配置与项目生图</summary><div>${originalCCSwitchBody()}</div></details>`;
// Keep the generic manual paths available, but make the recommended path unambiguous.
for (const id of ["codex", "claude"]) {
  const original = guides[id].body;
  guides[id].body = () =>
    `${callout("优先推荐 CCSwitch 接入", '通过 <a href="#guide/ccswitch">CCSwitch 图文教程</a> 配置并启用供应商，完成后必须完全退出目标工具再重新打开。下面保留手动配置方式，供不使用 CCSwitch 的用户参考。')}${original()}`;
}
for (const id of ["ccswitch", "codex", "claude"])
  articles["guide/" + id].body = guides[id].body;
articles["guide/ccswitch"].description =
  "更新 CCSwitch → 编辑现有供应商 → 刷新模型列表 → 设置最新 Terra → 完全退出并重开。";
const toolsGroup = articleGroups.find((g) => g.title === "工具接入");
toolsGroup.items = toolsGroup.items.filter(([id]) => id !== "guide/ccswitch");
toolsGroup.items.splice(1, 0, ["guide/ccswitch", "CCSwitch · 推荐"]);
const supportGroup = articleGroups.find((g) => g.title === "排查与支持");
supportGroup.items.splice(1, 0, [
  "codex-connection",
  "Codex 连接 / Reconnecting",
]);
supportGroup.items.splice(supportGroup.items.length - 1, 0, [
  "support-checklist",
  "提交排查信息",
]);
const quickstartBody = articles.quickstart.body;
articles.quickstart.body = () =>
  `<div class="recommended-banner"><span>推荐路径 · CCSwitch</span><h2>先把工具接好，再开始工作。</h2><p>先更新 CCSwitch，然后编辑 Codex 中现有的皮皮虾 AI 供应商，点击「获取模型列表」，保留 5.6 及以上并把子代理、自动审批设为最新 Terra；保存启用后，<strong>完全退出 Codex / Claude Code，再重新打开</strong>。</p><a class="doc-primary" href="#guide/ccswitch">开始 CCSwitch 图文配置 <span>→</span></a></div>${quickstartBody()}`;
const supportTemplate = () => `【皮皮虾 AI 排查信息】
发生时间（含时区）：
工具及版本（Codex / Claude Code / 其他）：
操作系统：
正在使用的 URL：${state.baseURL}
请求 User-Agent（应用名称/版本，不含密钥）：
模型名称：
所选分组：
Clash 是否开启、代理节点与日志截图（脱敏）：
渠道/公开服务状态及检测时间：
额度卡类型、剩余额度或余额状态：
Key 消费来源、独立限额是否耗尽（不要贴 Key）：
错误信息 / 状态码 / 请求 ID：
CCSwitch 已保存并启用：是 / 否
目标工具已完全退出并重开：是 / 否
已做过的测试与结果：

请勿提供 API Key、Cookie、代理订阅地址或完整私有对话。`;
articles["support-checklist"] = {
  title: "提交排查信息",
  description: "一次给出关键线索，减少来回确认。",
  body: () =>
    `<h2>先准备这五项</h2><ol class="checklist"><li><strong>正在使用的 URL</strong><p>复制客户端实际配置的地址，不只提供站点名称。</p></li><li><strong>Clash 代理节点及日志截图</strong><p>说明是否开启代理、是否命中正确线路；截图遮住订阅地址、密钥等内容。</p></li><li><strong>所选分组</strong><p>同时给出实际模型名称，便于检查匹配关系。</p></li><li><strong>渠道状态</strong><p>普通用户提供公开服务状态与检测时间；管理员可补充渠道管理中的状态及对话延迟。</p></li><li><strong>额度卡类型和余额状态</strong><p>提供到期时间、剩余额度、Key 消费来源与独立限额情况。</p></li></ol><h2>复制后填写</h2>${snippet("可复制的反馈模板", supportTemplate())}<p>填写后通过控制台的问题中心或客服入口提交。此页面不会自动发送或保存反馈。</p><a class="doc-primary" data-console-path="/issues" href="${state.baseURL}/issues">前往问题中心 ↗</a>`,
};
articles["connectivity-guide"] = {
  title: "连通性与速度排查",
  description:
    "按 URL、代理、分组、状态和额度卡的顺序检查，每次只改变一个变量。",
  body: () =>
    `<div class="diagnosis-strip"><span>01 URL</span><i>→</i><span>02 代理</span><i>→</i><span>03 分组</span><i>→</i><span>04 状态</span><i>→</i><span>05 额度</span></div><h2>1. 检测 URL</h2><p class="location-tag">排查位置：API 密钥页面</p><p>确认当前使用的检测链接是否连通性良好，并核对客户端实际 Base URL。页面上的当前端点选择不会自动修改您的客户端。</p><a data-console-path="/keys" href="${state.baseURL}/keys">去 API 密钥页面运行检测 →</a><h2>2. 代理节点</h2><p class="location-tag">排查位置：Clash 客户端及日志页面</p><p>按域名过滤日志，确认检测 URL 经过预期的代理节点。需要代理时，可先测试美国或加拿大节点，再按本机实际检测结果选择；节点所在地区不能保证速度。</p><ul><li><b>没有请求日志：</b>检查工具是否读取代理配置，以及是否走了其他代理或直连。</li><li><b>命中 DIRECT：</b>请求进入 Clash，但规则选择了直连。</li><li><b>命中预期代理组/节点：</b>固定 URL，只切换节点比较结果。</li></ul><h2>3. 分组选择</h2><p class="location-tag">排查位置：Key 分组设置 / 模型配置</p><p>确认所选分组与模型匹配。用当前 Key 查询模型列表，不要用其他账户的可用模型来判断。</p><h2>4. 渠道状态</h2><p class="location-tag">排查位置：公开服务状态；管理员可查看渠道管理</p><p>确认对应服务当前是否可用。首 Token 等待时间可与状态页的对话延迟作参考，但输入长度、模型、工具调用和网络也会影响结果。出现明显差异时保留检测时间与请求 ID。</p><a data-console-path="/monitor" href="${state.baseURL}/monitor">查看公开服务状态 →</a><h2>5. 额度卡选择</h2><p class="location-tag">排查位置：我的订阅 / API 密钥 / 余额页面</p><p>确认额度卡类型、剩余额度、有效期与分组覆盖范围适用于当前模型。也要检查 Key 是否绑定了正确的卡，以及独立限额是否已用完。</p>${callout("配置变更后必须重开工具", "使用 CCSwitch 保存并启用配置后，完全退出 Codex、Claude Code 或相应扩展宿主，再重新打开。只在 CCSwitch 内切换，不会让已有进程自动重新加载全部设置。")}<h2>仍未恢复？按模板反馈</h2><p>请提供正在使用的 URL、Clash 节点及日志截图、所选分组、渠道状态、额度卡类型和余额状态。</p>${snippet("反馈模板", supportTemplate())}<p>如果是持续 <code>Reconnecting 1/5</code>，继续查看<a href="#codex-connection">Codex 连接问题</a>；如果是 502，请参考<a href="#errors">错误码与 502 排查</a>。</p>`,
};
const proxyEnvironment =
  () => `# 将 7890 改成本机代理软件的 HTTP / mixed 监听端口
HTTP_PROXY=http://127.0.0.1:7890
HTTPS_PROXY=http://127.0.0.1:7890
http_proxy=http://127.0.0.1:7890
https_proxy=http://127.0.0.1:7890
NO_PROXY=127.0.0.1,localhost,::1
no_proxy=127.0.0.1,localhost,::1`;
const proxySession = () =>
  state.os === "windows"
    ? `# 改成本机实际 HTTP / mixed 端口；不要填写 SOCKS 端口
$proxyPort = Read-Host "HTTP 代理端口"
if ($proxyPort -notmatch '^\\d+$' -or [int]$proxyPort -lt 1 -or [int]$proxyPort -gt 65535) { throw "代理端口无效" }
$env:HTTP_PROXY = "http://127.0.0.1:$proxyPort"
$env:HTTPS_PROXY = $env:HTTP_PROXY
# Windows 环境变量名称不区分大小写，无需重复设置小写
$env:NO_PROXY = "127.0.0.1,localhost,::1"
# 从这个终端启动 CLI，才能继承以上变量
codex`
    : `printf 'HTTP 代理端口: '
read -r proxy_port
export HTTP_PROXY="http://127.0.0.1:$proxy_port"
export HTTPS_PROXY="$HTTP_PROXY"
export http_proxy="$HTTP_PROXY"
export https_proxy="$HTTP_PROXY"
export NO_PROXY="127.0.0.1,localhost,::1"
export no_proxy="$NO_PROXY"
# 从这个终端启动 CLI；不代表已运行的桌面端也会继承
codex`;
articles["codex-connection"] = {
  title: "Codex 连接问题",
  description:
    "一直 Reconnecting 1/5？先确认连接层，再配置代理、完全退出并验证。",
  body: () =>
    `<div class="diagnosis-strip"><span>诊断</span><i>→</i><span>确认代理</span><i>→</i><span>彻底重启</span><i>→</i><span>验证</span></div>${callout("先确认 CCSwitch 配置已经加载", "在 CCSwitch 保存并启用供应商后，完全退出 Codex 等工具，再重新打开。先做这一步，避免用旧进程反复测试新配置。")}<h2>1. 先确认问题</h2>${snippet("终端 · 查询版本与诊断能力", "codex --version\ncodex doctor --help\ncodex doctor")}<p>支持该诊断命令的版本可输出安装、配置、认证及运行环境报告。如果显示未知命令，请先按当前客户端的官方说明更新或使用内置诊断。</p><p>如果报告或日志包含 <code>Connectivity → websocket</code>，观察是否出现 <code>HTTP 101 Switching Protocols</code> 或 <code>websocket connected</code>。这说明握手/连接阶段成功，仍需发送简单任务验证模型响应；不同版本的输出字段可能不同。</p><p>持续 Reconnecting 可能与 WSS/代理路径有关，也可能是认证、供应商配置或上游异常，不能仅凭重连提示确定原因。</p><h2>2. 配置代理环境变量</h2><p>先在 Clash / v2rayN 中确认<strong>实际 HTTP 或 mixed 监听端口</strong>。图中的 7890、10808 只是常见示例；若该端口仅支持 SOCKS，就不能直接填写成 <code>http://</code>。</p>${osSelector()}${snippet(state.os === "windows" ? "PowerShell · 仅对当前会话及子进程生效" : "Bash / Zsh · 仅对当前会话及子进程生效", proxySession())}<h3>关于 ~/.codex/.env</h3><p>原图建议把下列变量写入 <code>~/.codex/.env</code>。仅在<strong>确认当前客户端/启动器支持读取该文件</strong>时使用：不能假定所有 Codex Desktop、CLI 和 VS Code 版本都自动加载它。不要覆盖已有文件；合并需要的变量并按当前客户端文档验证。</p>${snippet(".env 变量示例 · 需确认当前客户端支持", proxyEnvironment())}<p>以上本地终端方法不会自动修改已运行的桌面应用环境。桌面端或扩展须使用其支持的代理设置/启动方式，并在重开后检查日志是否命中代理。</p><h2>3. 完全退出后验证</h2>${restartNotice()}<ol><li>确认代理软件在运行，端口与规则没有写错。</li><li>重新启动目标工具，再运行可用的诊断或查看客户端日志。</li><li>若使用 WebSocket，检查升级/连接是否成功，再发送一条简单任务。</li><li>检查皮皮虾调用记录，确认请求实际到达且返回正常。</li></ol><h2>4. 避免这些误区</h2><ul><li><code>supports_websockets = false</code> 不是通用代理开关；它属于具体 <code>model_providers.&lt;id&gt;</code> 的传输能力配置。不要在不清楚供应商要求时盲目添加。</li><li>HTTP 101 不是“整条模型调用成功”，更不是速度保证。</li><li>TUN 影响范围较大，先排查应用配置与日志，再根据需要考虑；启用后仍需验证实际路径。</li><li>更改代理和端点时一次只改一项，避免失去对照。</li></ul><h2>带上这些信息再反馈</h2>${snippet("可复制的排查模板", supportTemplate())}<p class="guide-source">诊断命令与配置字段参考：<a href="https://developers.openai.com/codex/cli/reference/" target="_blank" rel="noopener noreferrer">Codex CLI reference</a> · <a href="https://developers.openai.com/codex/config-reference/" target="_blank" rel="noopener noreferrer">Configuration reference</a>。本教程结合平台排障图整理，具体日志与代理加载行为以当前客户端为准。</p>`,
};
// Relevant console destinations stay close to the task and always open separately.
const consoleResources = {
  quickstart: [
    ["/keys", "创建 / 管理 API 密钥"],
    ["/model-plaza", "查看模型与价格"],
  ],
  "guide/python": [
    ["/keys", "获取 Key 与配置"],
    ["/usage", "核对本次调用"],
  ],
  "guide/node": [
    ["/keys", "获取 Key 与配置"],
    ["/usage", "核对本次调用"],
  ],
  "guide/http-sdk": [
    ["/keys", "获取 API 密钥"],
    ["/usage", "查看调用记录"],
  ],
  "guide/models-api": [
    ["/keys", "检查 Key 分组"],
    ["/model-plaza", "比较模型与价格"],
  ],
  "guide/chat-api": [
    ["/keys", "获取 API 密钥"],
    ["/usage", "查看调用结果"],
  ],
  "guide/responses-api": [
    ["/keys", "获取 API 密钥"],
    ["/usage", "查看调用结果"],
  ],
  "guide/stream-api": [
    ["/usage", "核对调用与用量"],
    ["/issues/new", "提交流式问题"],
  ],
  "codex-connection": [
    ["/keys", "检测当前入口"],
    ["/monitor", "核对服务状态"],
    ["/issues/new", "新建问题反馈"],
  ],
  faq: [
    ["/keys", "查看密钥配置"],
    ["/subscriptions", "查看我的套餐"],
    ["/issues", "查找已有问题"],
  ],
  billing: [
    ["/keys", "核对 Key 消费来源"],
    ["/subscriptions", "查看我的订阅"],
    ["/usage", "查看实际扣费"],
  ],
  "endpoint-selector": [
    ["/keys", "打开连接检测"],
    ["/monitor", "查看服务状态"],
  ],
  sdk: [
    ["/keys", "获取接入配置"],
    ["/usage", "核对调用结果"],
  ],
  "api-reference": [
    ["/keys", "管理 API 密钥"],
    ["/model-plaza", "查看模型能力"],
  ],
  errors: [
    ["/monitor", "查看服务状态"],
    ["/usage", "查找请求记录"],
    ["/issues/new", "提交问题"],
  ],
  context: [
    ["/usage", "查找请求记录"],
    ["/issues/new", "提交排查信息"],
  ],
  "guide/image-api": [
    ["/keys", "创建图片分组 Key"],
    ["/model-plaza", "查询图片模型"],
  ],
};
for (const [id, links] of Object.entries(consoleResources)) {
  const previous = articles[id].body;
  articles[id].body = () =>
    previous() +
    `<aside class="console-resources"><strong>继续在皮皮虾控制台操作</strong><p>以下入口在新标签页打开，当前教程会保留。</p><div class="doc-actions">${links.map(([path, label]) => `<a class="doc-secondary" data-console-path="${path}" href="${state.baseURL}${path}" target="_blank" rel="noopener noreferrer">${label} ↗</a>`).join("")}</div></aside>`;
}
