// Operator-provided troubleshooting posters, transcribed into usable articles.
// Version-dependent Codex behavior is explicitly scoped; no visitor secrets are collected.
const restartNotice = () =>
  callout(
    "必须完成：保存并启用 → 完全退出 → 重新打开",
    "在 CCSwitch 修改配置后，先保存手头任务，再<strong>完全退出 Codex、Claude Code 等目标工具</strong>。桌面端退出应用及托盘/菜单栏中的后台实例；CLI 结束正在运行的会话；VS Code 扩展需关闭承载它的 VS Code 后重新打开。只关对话、新建任务或只重启 CCSwitch，都不等于重启目标工具。",
  );
const originalCCSwitchBody = guides.ccswitch.body;
guides.ccswitch.body = () =>
  `<div class="recommended-banner"><span>推荐方式 · 从皮皮虾一键导入</span><strong>打开 API 密钥页面，点击「导入到 CCS」</strong><p>先安装 CCSwitch，再从目标 Key 的操作栏导入。无需优先手填整套供应商配置。</p><div class="doc-actions"><a class="doc-primary" data-console-path="/keys" href="${state.baseURL}/keys" target="_blank" rel="noopener noreferrer">打开 API 密钥页面 ↗</a><a class="doc-secondary" href="https://github.com/farion1231/cc-switch/releases" target="_blank" rel="noopener noreferrer">下载 CCSwitch ↗</a></div></div>
  <h2>1. 下载并安装 CCSwitch</h2><p>前往 <a href="https://github.com/farion1231/cc-switch/releases" target="_blank" rel="noopener noreferrer">CCSwitch 官方下载页面（GitHub Releases）</a>，选择适合 Windows、macOS 或 Linux 的安装包。安装后先打开一次，让系统注册 CCSwitch 链接的打开方式。</p>
  <h2>2. 在 API 密钥页面一键导入</h2><p>打开<a data-console-path="/keys" href="${state.baseURL}/keys" target="_blank" rel="noopener noreferrer">皮皮虾 API 密钥页面 ↗</a>，找到准备使用的 Key；没有 Key 时先创建，确认分组、消费来源和模型权限。</p><p>在这行的<strong>操作栏点击「导入到 CCS」</strong>，不是旁边的「使用」按钮。浏览器询问是否打开 CCSwitch 时，允许打开本机应用。</p><figure class="guide-figure"><img src="./assets/ccswitch/import-entry.svg" width="1100" height="450" alt="API 密钥操作栏示意：目标 Key 右侧的导入到 CCS 按钮" loading="lazy"/><figcaption>图 1 · 根据当前项目操作栏绘制的位置示意，非生产账户截图。认准「导入到 CCS」。</figcaption></figure>
  <p>导入会根据 Key 的平台配置选择目标工具：OpenAI 分组对应 Codex，Anthropic 类分组对应 Claude；部分平台会弹出工具选择。不要为了换工具随意更改原 Key 的分组。</p>${callout("没有弹出 CCSwitch？", "先确认已安装并启动过 CCSwitch，检查浏览器是否阻止打开外部应用。按钮可能受站点设置影响；找不到按钮时联系支持，或使用文末手动配置。导入链接包含 Key，不要复制给他人或发到工单。")}
  <h2>3. 在 CCSwitch 确认并启用</h2><p>核对导入的供应商名称、地址和目标工具，确认导入后启用皮皮虾供应商。模型名称以<a data-console-path="/model-plaza" href="${state.baseURL}/model-plaza" target="_blank" rel="noopener noreferrer">模型广场 ↗</a>及当前 Key 的可用列表为准；导入默认模型不可用时先核对分组权限。</p><figure class="guide-figure"><img src="./assets/ccswitch/pipixia-provider.webp" width="1966" height="610" alt="皮皮虾 AI 在 CCSwitch Codex 页签中显示为使用中的真实截图" loading="lazy"/><figcaption>图 2 · 皮皮虾 AI 导入后的实际界面：选择 Codex 页签，确认皮皮虾 AI 显示「使用中」。余额为截图当时的数据，以您自己的账户为准。<a href="./assets/ccswitch/pipixia-provider-original.png" target="_blank" rel="noopener noreferrer">查看完整原图 ↗</a></figcaption></figure>
  <h2>4. 完全退出目标工具，再重新打开</h2>${restartNotice()}
  <h2>5. 发送简单任务，核对调用记录</h2><p>重新启动后发送一条简单请求，打开<a data-console-path="/usage" href="${state.baseURL}/usage" target="_blank" rel="noopener noreferrer">调用记录 ↗</a>，确认请求、模型和消费来源。仍然 Reconnecting 时按<a href="#codex-connection">连接问题指南</a>检查；有余额却不能使用时查看<a href="#billing">余额与套餐说明</a>。</p>${clientHeadersNotice()}
  <h2>备用：手动配置与项目生图</h2><p>一键导入不可用，或需要额外配置时再参考下面内容。</p><details class="guide-details"><summary>展开手动配置截图与生图配置</summary><div>${originalCCSwitchBody()}</div></details>`;
// Keep the generic manual paths available, but make the recommended path unambiguous.
for (const id of ["codex", "claude"]) {
  const original = guides[id].body;
  guides[id].body = () =>
    `${callout("优先推荐 CCSwitch 接入", '通过 <a href="#guide/ccswitch">CCSwitch 图文教程</a> 配置并启用供应商，完成后必须完全退出目标工具再重新打开。下面保留手动配置方式，供不使用 CCSwitch 的用户参考。')}${original()}`;
}
for (const id of ["ccswitch", "codex", "claude"])
  articles["guide/" + id].body = guides[id].body;
articles["guide/ccswitch"].description =
  "安装 CCSwitch → API 密钥页面一键导入 → 启用 → 完全退出并重开 → 验证。";
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
  `<div class="recommended-banner"><span>推荐路径 · CCSwitch</span><h2>先把工具接好，再开始工作。</h2><p>先在 <a data-console-path="/keys" href="${state.baseURL}/keys" target="_blank" rel="noopener noreferrer">API 密钥页面 ↗</a>点击「导入到 CCS」，在 CCSwitch 确认并启用，<strong>完全退出 Codex / Claude Code，再重新打开</strong>。</p><a class="doc-primary" href="#guide/ccswitch">开始 CCSwitch 图文配置 <span>→</span></a></div>${quickstartBody()}`;
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
