const articleGroups = [
  {
    title: "开始使用",
    items: [
      ["quickstart", "快速开始"],
      ["endpoint-selector", "选择 API 端点"],
      ["billing", "余额、套餐与密钥"],
    ],
  },
  {
    title: "工具接入",
    items: [
      ["clients", "选择你的工具"],
      ["guide/codex", "Codex"],
      ["guide/claude", "Claude Code"],
      ["guide/ccswitch", "CCSwitch"],
      ["guide/cursor", "Cursor"],
      ["guide/cherry", "Cherry Studio"],
      ["guide/cline", "Cline / Roo Code"],
      ["guide/chatbox", "Chatbox"],
      ["guide/openwebui", "OpenWebUI"],
    ],
  },
  {
    title: "API 与开发",
    items: [
      ["sdk", "SDK 与代码示例"],
      ["guide/python", "Python"],
      ["guide/node", "Node.js / TypeScript"],
      ["guide/http-sdk", "Java / HTTP"],
      ["api-reference", "接口概览"],
      ["guide/models-api", "查询模型"],
      ["guide/chat-api", "聊天补全"],
      ["guide/responses-api", "Responses"],
      ["guide/stream-api", "流式输出"],
      ["guide/image-api", "图片生成"],
    ],
  },
  {
    title: "排查与支持",
    items: [
      ["errors", "错误码与 502"],
      ["context", "上下文与自动压缩"],
      ["connectivity-guide", "连接与速度"],
      ["faq", "常见问题"],
    ],
  },
];
const callout = (title, text) =>
  `<aside class="callout"><strong>${title}</strong><p>${text}</p></aside>`;
const cards = (items) =>
  `<div class="article-cards">${items.map(([id, title, desc]) => `<a href="#${id}"><span class="card-arrow">↗</span><h3>${title}</h3><p>${desc}</p></a>`).join("")}</div>`;
function osSelector() {
  return `<div class="os-selector" role="group" aria-label="命令使用的系统"><button data-os="windows" aria-pressed="${state.os === "windows"}">Windows · PowerShell</button><button data-os="unix" aria-pressed="${state.os === "unix"}">macOS / Linux · Bash / Zsh</button></div>`;
}
// Read secrets at the local terminal, not into the documentation page or command history.
function keySetup(variable = "PIPIXIA_API_KEY") {
  return state.os === "windows"
    ? `$env:${variable} = [System.Net.NetworkCredential]::new("", (Read-Host "API Key" -AsSecureString)).Password`
    : `printf 'API Key: '\nread -r -s ${variable}\nprintf '\\n'\nexport ${variable}`;
}
function modelsCommand() {
  return state.os === "windows"
    ? `${keySetup()}\nInvoke-RestMethod -Uri "${state.baseURL}/v1/models" \u0060\n  -Headers @{ Authorization = "Bearer $env:PIPIXIA_API_KEY"; "User-Agent" = "pipixia-client/1.0" }`
    : `${keySetup()}\ncurl "${state.baseURL}/v1/models" \\\n  -H "User-Agent: pipixia-client/1.0" \\\n  -H "Authorization: Bearer $PIPIXIA_API_KEY"`;
}
function sdkEnvironment() {
  const model =
    state.os === "windows"
      ? '$env:PIPIXIA_MODEL = Read-Host "模型名称（从当前 Key 的列表复制）"'
      : `printf '模型名称: '\nread -r PIPIXIA_MODEL\nexport PIPIXIA_MODEL`;
  return `${osSelector()}${snippet(state.os === "windows" ? "PowerShell · 设置当前会话" : "Bash / Zsh · 设置当前会话", keySetup() + "\n" + model)}`;
}
guides.codex = {
  title: "Codex 接入",
  body: () =>
    `<p>使用控制台为当前 Key 生成的配置接入，避免覆盖您已有的模型与项目设置。</p><h2>1. 创建可用的 Key</h2><p>进入<a data-console-path="/keys" href="${state.baseURL}/keys">API 密钥</a>，选择支持目标模型的编程分组，并确认消费来源是余额还是某张套餐卡。</p><h2>2. 获取当前配置</h2><p>在这把 Key 的“使用”入口选择 Codex，按您的操作系统复制配置。模型名以该 Key 的可用列表为准，不要照搬其他账号的名称。</p>${snippet("Base URL", state.baseURL + "/v1")}${callout("已有配置？先保留副本", '合并控制台给出的供应商配置，不要直接覆盖整个配置文件。通过 CCSwitch 管理的用户请使用 <a href="#guide/ccswitch">CCSwitch 教程</a>，避免两个入口互相覆盖。')}<h2>3. 验证模型权限</h2>${osSelector()}${snippet(state.os === "windows" ? "PowerShell" : "Shell", modelsCommand())}<p>预期：返回模型列表，包含您准备使用的模型。401 表示需要检查认证；模型不在列表中时先检查分组和权限。</p><h2>4. 启动并发送简单任务</h2><p>重新打开 Codex，选择刚配置的供应商和模型，发送一条简单请求；再到控制台调用记录确认模型和消费来源。</p>${callout("长对话失败", '出现 context limit 或 remote compaction 错误时，查看<a href="#context">上下文与自动压缩排查</a>。仅修改客户端窗口数字不会增加上游能力。')}`,
};
guides.claude.body = () =>
  `<p>先确认 Key 的分组支持需要的 Claude 模型，再选择您使用的终端。</p><h2>1. 设置连接</h2>${osSelector()}${snippet(state.os === "windows" ? "PowerShell" : "Shell", state.os === "windows" ? `$env:ANTHROPIC_BASE_URL = "${state.baseURL}"\n${keySetup("ANTHROPIC_AUTH_TOKEN")}\nclaude` : `export ANTHROPIC_BASE_URL="${state.baseURL}"\n${keySetup("ANTHROPIC_AUTH_TOKEN")}\nclaude`)}${callout("不要追加 /v1", "这里设置的是 ANTHROPIC_BASE_URL 根地址。执行命令时在本机输入令牌；不要把真实密钥贴到工单或提交到 Git。")}<h2>2. 确认配置已生效</h2><p>完全关闭旧的终端会话，在已设置环境变量的终端启动 Claude Code。使用当前分组允许的模型，发送一条简单请求。</p><h2>3. 查看结果</h2><p>预期：收到回复，并在控制台看到相应调用记录。若没有记录，检查工具是否仍使用其他供应商配置；若返回错误，记录错误码并查阅<a href="#errors">排查指南</a>。</p>`;
guides.python.body = () =>
  `<p>先在本机设置 <code>PIPIXIA_API_KEY</code> 和 <code>PIPIXIA_MODEL</code>。模型名称从当前 Key 的模型列表选择。</p><h2>1. 安装 SDK</h2>${snippet("终端", "pip install openai")}<h2>2. 配置环境并发送请求</h2>${sdkEnvironment()}${snippet("Python", `import os\nfrom openai import OpenAI\n\nclient = OpenAI(\n    api_key=os.environ["PIPIXIA_API_KEY"],\n    base_url="${state.baseURL}/v1",\n    default_headers={"User-Agent": "pipixia-client/1.0"},\n)\nreply = client.chat.completions.create(\n    model=os.environ["PIPIXIA_MODEL"],\n    messages=[{"role": "user", "content": "你好"}],\n)\nprint(reply.choices[0].message.content)`)}<h2>3. 验证</h2><p>运行脚本后应输出模型回复。若环境变量缺失，先设置再运行；若模型不支持 Chat Completions，请改用对应协议的示例。</p>`;
guides.node.body = () =>
  `<p>设置本机环境变量 <code>PIPIXIA_API_KEY</code> 和 <code>PIPIXIA_MODEL</code> 后，运行下面的示例。</p><h2>1. 安装 SDK</h2>${snippet("终端", "pnpm add openai")}<h2>2. 配置环境并发送请求</h2>${sdkEnvironment()}${snippet("JavaScript / TypeScript", `import OpenAI from "openai";\n\nconst client = new OpenAI({\n  apiKey: process.env.PIPIXIA_API_KEY,\n  baseURL: "${state.baseURL}/v1",\n  defaultHeaders: { "User-Agent": "pipixia-client/1.0" },\n});\nif (!process.env.PIPIXIA_MODEL) throw new Error("请设置 PIPIXIA_MODEL");\nconst result = await client.chat.completions.create({\n  model: process.env.PIPIXIA_MODEL,\n  messages: [{ role: "user", content: "你好" }],\n});\nconsole.log(result.choices[0]?.message.content);`)}<h2>3. 验证</h2><p>预期输出模型回复。在控制台调用记录中确认模型、用量和扣费来源。</p>`;
const articles = {
  quickstart: {
    title: "快速开始",
    description: "从你正在用的工具出发，完成第一次调用。",
    body: () =>
      `<div class="welcome-strip"><span>PPX / GET STARTED</span><span>准备 → 配置 → 验证</span></div><h2>你想用 AI 做什么？</h2>${cards(
        [
          ["guide/codex", "用 Codex 写代码", "配置供应商，继续你的项目。"],
          ["guide/claude", "接入 Claude Code", "按操作系统设置连接。"],
          ["sdk", "开发自己的应用", "从 SDK 或 HTTP 请求开始。"],
          ["guide/image-api", "生成图片", "配置图片分组与独立密钥。"],
        ],
      )}<h2>接入前，准备三样东西</h2><ol class="steps"><li><strong>一个可用的账户</strong><p>在控制台确认余额或有效套餐，以及套餐支持的分组。</p><a data-console-path="/purchase" href="${state.baseURL}/purchase">查看套餐 →</a></li><li><strong>一把对应分组的 Key</strong><p>创建 Key 时确认分组、消费来源和独立限额。不要把不同工具的设置混在一起。</p><a data-console-path="/keys" href="${state.baseURL}/keys">创建 API Key →</a></li><li><strong>一个当前可用的模型</strong><p>模型名从该 Key 的模型列表选择；公开模型广场不等于每把 Key 都能调用。</p><a href="#guide/models-api">查询模型 →</a></li></ol><h2>先验证，再接入复杂任务</h2>${osSelector()}${snippet(state.os === "windows" ? "PowerShell · 查询模型" : "Shell · 查询模型", modelsCommand())}<p>预期结果是模型列表。确认连接与权限后，再打开上面的工具教程发送一条简单请求。</p>${callout("地址不要重复拼接", "OpenAI 兼容 Base URL 通常以 /v1 结尾；Claude Code 的 ANTHROPIC_BASE_URL 使用根地址。每篇教程给出的字段值已经包含所需路径。")}`,
  },
  "endpoint-selector": {
    title: "选择 API 端点",
    description: "端点决定连接路径，不改变账户与已购买权益。",
    body: () =>
      `<h2>当前选择</h2>${snippet("OpenAI 兼容 Base URL", state.baseURL + "/v1")}${snippet("Claude Code 根地址", state.baseURL)}<p>使用页面的“当前 API 端点”选择器切换。此偏好保存在当前浏览器中，所有教程示例会同步更新；已经保存到客户端的配置需要您手动修改。</p><h2>怎么选更合适？</h2><ol><li>进入控制台 API 密钥页面，运行连接检测。</li><li>在自己的设备和网络上比较结果，优先使用测试表现良好的入口。</li><li>用同一模型、同一条简单请求验证，避免同时更改多个设置。</li></ol>${callout("没有对所有网络都最快的入口", "连接检测反映当前设备、时间和网络的表现，不代表对所有用户的保证。更换入口不需要重新充值。")}<h2>切换后验证</h2><p>重启需要重新加载配置的客户端，并检查调用记录。若仍然失败，按<a href="#connectivity-guide">连接与速度排查</a>记录结果。</p>`,
  },
  billing: {
    title: "余额、套餐与密钥",
    description: "有额度只是第一步，Key 的消费来源和权限也需要对应。",
    body: () =>
      `<h2>四个概念，一次分清</h2><div class="definition-list"><div><strong>账户余额</strong><p>账户持有的余额。使用余额的 Key 还需满足对应分组的余额访问规则。</p></div><div><strong>套餐 / 额度卡</strong><p>有独立的有效期、周期额度和分组覆盖范围。新增一张卡不代表原有 Key 自动改绑。</p></div><div><strong>API 分组</strong><p>决定这把 Key 可以访问的模型和计费规则。模型同名也不能推定所有渠道的上下文上限一致。</p></div><div><strong>Key 独立限额</strong><p>单把密钥的使用上限，与账户余额、套餐剩余额度分开。Key 已耗尽时，账户有钱也可能不能继续调用。</p></div></div><h2>有余额或套餐，为什么不能用？</h2><ol><li>确认 Key 未被停用、未过期，独立限额没有耗尽。</li><li>查看 Key 选择的是余额还是套餐；绑定的卡是否仍有效。</li><li>确认套餐覆盖 Key 的分组，该分组允许目标模型。</li><li>检查限流、并发及模型权限，保存请求 ID。</li></ol><h2>公司多人使用</h2><p>给不同成员或项目创建独立 Key，分别设置限额。改绑套餐之前先确认覆盖范围；现有 Key 不应因为新增套餐就被批量切换。</p>${callout("转余额后如何使用？", "转入余额不会自动改变已有 Key 的消费来源。需要用余额调用时，请在控制台选择支持余额的分组并配置对应 Key；原套餐 Key 继续按原绑定使用。")}<h2>核对实际扣费</h2><p>发送一条简单请求后，在调用记录查看消费来源、实际费用和模型。不要把套餐计费额度直接当作支付金额。</p>`,
  },
  clients: {
    title: "选择你的工具",
    description: "每个教程都有独立链接，可以保存或分享给团队。",
    body: () =>
      cards(
        articleGroups[1].items
          .slice(1)
          .map(([id, title]) => [id, title, "查看配置步骤与注意事项"]),
      ),
  },
  sdk: {
    title: "SDK 与代码示例",
    description: "选择熟悉的语言。真实密钥始终留在本机环境中。",
    body: () =>
      cards([
        ["guide/python", "Python", "使用 OpenAI 兼容 SDK。"],
        ["guide/node", "Node.js / TypeScript", "设置环境变量并发送请求。"],
        ["guide/http-sdk", "Java / HTTP", "查看 HTTP 请求片段。"],
        ["api-reference", "直接调用 HTTP", "查询模型、对话、流式与生图。"],
      ]),
  },
  "api-reference": {
    title: "接口概览",
    description: "选择接口前，先确认目标模型和分组支持对应能力。",
    body: () =>
      cards([
        ["guide/models-api", "GET /v1/models", "查询当前 Key 的可用模型。"],
        ["guide/chat-api", "Chat Completions", "发送消息列表。"],
        ["guide/responses-api", "Responses", "发送 Responses 格式请求。"],
        ["guide/stream-api", "流式输出", "持续读取 SSE 响应。"],
        ["guide/image-api", "图片生成", "配置支持生图的分组与模型。"],
      ]),
  },
  context: {
    title: "上下文与自动压缩",
    description: "先区分窗口超限、自动压缩失败和客户端显示值。",
    body: () =>
      `<h2>上下文超限</h2><p><code>context_length_exceeded</code> 或 <code>exceeds the model context limit</code> 表示本次输入超出服务允许的范围。模型、分组和上游渠道的限制可能不同；默认窗口与最大窗口也可能不同。</p><p>先缩小输入、减少过长的工具输出，或整理任务进度后新建会话。客户端手动填写更大的窗口不会扩展服务端能力。</p><h2>自动压缩失败</h2><p><code>remote compaction ... expected exactly one compaction output item, got 0</code> 表示客户端没有收到预期的压缩结果。仅凭这一条信息无法确定是上游返回、协议兼容还是会话状态问题。</p><ol><li>保留原会话和完整错误文本，避免连续重复点击继续。</li><li>把任务目标、已完成内容、关键文件和下一步整理到新会话，临时继续工作。</li><li>记录客户端版本、模型、分组、发生时间和请求 ID，提交给支持排查。</li></ol><h2>查询声明的窗口</h2>${snippet("Shell · Codex 模型清单", `curl "${state.baseURL}/backend-api/codex/models" \\\n  -H "User-Agent: pipixia-client/1.0" \\\n  -H "Authorization: Bearer $PIPIXIA_API_KEY"`)}<p>若返回相关字段，<code>context_window</code> 为默认窗口，<code>max_context_window</code> 为声明的最大窗口。字段可能缺失；声明值也不是经过长输入验证的保证。</p><h2>反馈时提供什么？</h2><p>发生时间及其时区、客户端及版本、使用的模型和分组、脱敏后的错误截图、请求 ID。不要提供真实密钥或完整私有对话。</p>`,
  },
  errors: {
    title: "错误码与 502 排查",
    description: "保留错误信息，每次只调整一项设置。",
    body: () => legacyArticles.errors.html,
  },
  "connectivity-guide": {
    title: "连接与速度",
    description: "固定模型和请求，对比端点与网络路径。",
    body: () => {
      const doc = new DOMParser().parseFromString(
        legacyArticles.errors.html,
        "text/html",
      );
      return doc.getElementById("connectivity-guide").innerHTML;
    },
  },
  faq: {
    title: "常见问题",
    description: "关于端点、模型、分组和连接的常用解答。",
    body: () => legacyArticles.faq.html,
  },
};
for (const [id, guide] of Object.entries(guides))
  articles["guide/" + id] = {
    title: guide.title,
    description: "选择适合当前网络的端点，按下面的步骤完成接入。",
    body: guide.body,
  };
const legacyAliases = {
  top: "quickstart",
  ccswitch: "guide/ccswitch",
  "quickstart-title": "quickstart",
  "endpoint-title": "endpoint-selector",
  "clients-title": "clients",
  "api-reference-title": "api-reference",
  "errors-title": "errors",
  "faq-title": "faq",
  "connectivity-guide-title": "connectivity-guide",
  "codex-reconnecting": "codex-connection",
};

// Platform integration requirement: make UA visible before users copy requests.
const userAgentNotice = () =>
  callout(
    "API 对接必查：请求 Header 带上 User-Agent（UA）",
    "自建 API 请求请显式设置 <code>User-Agent: pipixia-client/1.0</code>，也可以替换为您真实的应用名称与版本，例如 <code>my-app/1.0</code>。字段名是 <code>User-Agent</code>，不是 <code>UA</code>；它放在请求头里，不是 JSON 请求体。不要填密钥或个人信息。",
  );
for (const id of [
  "quickstart",
  "sdk",
  "api-reference",
  "guide/python",
  "guide/node",
  "guide/http-sdk",
  "guide/models-api",
  "guide/chat-api",
  "guide/stream-api",
  "guide/responses-api",
  "guide/image-api",
]) {
  const previous = articles[id].body;
  articles[id].body = () => userAgentNotice() + previous();
}
const clientHeadersNotice = () =>
  callout(
    "第三方工具：确认 UA 被保留",
    "使用客户端或中间代理对接时，确认实际发出的请求带有 <code>User-Agent</code>。工具已正常发送自身 UA 时保留它；支持自定义 Headers 的工具可补充真实应用标识。不要假定 CCSwitch 本身的配置会替所有工具发出 UA，也不要用 UA 冒充其他工具。",
  );
for (const id of [
  "guide/ccswitch",
  "guide/codex",
  "guide/claude",
  "guide/cursor",
  "guide/cherry",
  "guide/cline",
  "guide/chatbox",
  "guide/openwebui",
]) {
  const previous = guides[id.slice(6)].body;
  guides[id.slice(6)].body = () => clientHeadersNotice() + previous();
  articles[id].body = guides[id.slice(6)].body;
}
