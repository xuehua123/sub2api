const state = { baseURL: 'https://api.psydo.top', label: '默认端点' }

const guides = {
  cherry: {
    title: 'Cherry Studio',
    body: () => `<ol><li>打开“设置” → “模型服务”，新增 OpenAI 服务商。</li><li>API 地址填写 <code>${state.baseURL}/v1</code>。</li><li>粘贴您在皮皮虾 AI 控制台创建的 API Key。</li><li>点击获取模型或手动添加控制台可见的模型名称。</li></ol>${snippet('Base URL', `${state.baseURL}/v1`)}`,
  },
  cursor: {
    title: 'Cursor',
    body: () => `<ol><li>打开 Cursor Settings，进入 Models。</li><li>启用 OpenAI Compatible 或自定义 OpenAI API Provider。</li><li>Base URL 填写 <code>${state.baseURL}/v1</code>，再保存 API Key。</li><li>模型名以皮皮虾 AI 控制台的模型列表为准。</li></ol>${snippet('Base URL', `${state.baseURL}/v1`)}`,
  },
  claude: {
    title: 'Claude Code',
    body: () => `<p>在终端中设置环境变量后再启动 Claude Code。模型可用性取决于您的控制台权限。</p>${snippet('PowerShell', `$env:ANTHROPIC_BASE_URL = "${state.baseURL}"\n$env:ANTHROPIC_AUTH_TOKEN = "您的皮皮虾 API Key"\nclaude`)}`,
  },
  openwebui: {
    title: 'OpenWebUI',
    body: () => `<ol><li>以管理员身份打开“Settings” → “Connections”。</li><li>添加 OpenAI API Connection。</li><li>URL 填写 <code>${state.baseURL}/v1</code>，API Key 填写皮皮虾 AI 密钥。</li><li>保存后刷新模型列表。</li></ol>${snippet('Connection URL', `${state.baseURL}/v1`)}`,
  },
  python: {
    title: 'Python SDK',
    body: () => `<p>安装官方 SDK：<code>pip install openai</code></p>${snippet('Python', `from openai import OpenAI\n\nclient = OpenAI(\n    api_key="您的皮皮虾 API Key",\n    base_url="${state.baseURL}/v1",\n)\n\nreply = client.chat.completions.create(\n    model="gpt-5.4-mini",\n    messages=[{"role": "user", "content": "你好"}],\n)\nprint(reply.choices[0].message.content)`)}`,
  },
  chatbox: {
    title: 'Chatbox',
    body: () => `<ol><li>打开设置，新增自定义模型提供商。</li><li>接口类型选择 OpenAI API 或 OpenAI Compatible。</li><li>Base URL 填写 <code>${state.baseURL}/v1</code>，再粘贴皮皮虾 API Key。</li><li>模型名请从控制台模型列表复制，保存后开始对话。</li></ol>${snippet('Base URL', `${state.baseURL}/v1`)}`,
  },
  cline: {
    title: 'Cline / Roo Code',
    body: () => `<ol><li>在 VS Code 扩展设置中选择 OpenAI Compatible 提供商。</li><li>Base URL 填写 <code>${state.baseURL}/v1</code>。</li><li>输入皮皮虾 API Key，并填写控制台中可见的模型名称。</li><li>首次调用失败时先确认 API Key、模型权限和余额。</li></ol>${snippet('Base URL', `${state.baseURL}/v1`)}`,
  },
  node: {
    title: 'Node.js / TypeScript',
    body: () => `<p>安装 SDK：<code>npm install openai</code></p>${snippet('TypeScript', `import OpenAI from "openai"\n\nconst client = new OpenAI({\n  apiKey: process.env.PIPIXIA_API_KEY,\n  baseURL: "${state.baseURL}/v1",\n})\n\nconst completion = await client.chat.completions.create({\n  model: "gpt-5.4-mini",\n  messages: [{ role: "user", content: "你好" }],\n})\n\nconsole.log(completion.choices[0]?.message.content)`)}`,
  },
  'http-sdk': {
    title: 'Java / Go',
    body: () => `<p>无论使用 Java、Go、PHP 还是其他语言，核心都是向 OpenAI 兼容 HTTP 接口发送请求。</p>${snippet('Java HttpClient', `HttpRequest request = HttpRequest.newBuilder()\n    .uri(URI.create("${state.baseURL}/v1/chat/completions"))\n    .header("Authorization", "Bearer " + apiKey)\n    .header("Content-Type", "application/json")\n    .POST(HttpRequest.BodyPublishers.ofString(body))\n    .build();`)}`,
  },
  ccswitch: {
    title: 'CCSwitch',
    body: () => `<p>CCSwitch 用于在本机统一管理 Claude Code、Codex 等客户端的供应商配置。请先升级到支持自定义 Provider 的版本，再添加皮皮虾 AI 供应商。</p>
      <div class="guide-figures"><figure class="guide-figure"><img src="./assets/ccswitch/main-zh.png" alt="CCSwitch 供应商列表和 Claude、Codex 切换入口" loading="lazy" /><figcaption><b>第 1 步：</b>打开 CCSwitch，在顶部选择 Claude 或 Codex；右上角“+”用于新增供应商。</figcaption></figure><figure class="guide-figure"><img src="./assets/ccswitch/add-zh.png" alt="CCSwitch 添加 Claude Code 供应商页面" loading="lazy" /><figcaption><b>第 2 步：</b>进入添加供应商页面后，选择“自定义配置”，不要误选其他平台的预设供应商。</figcaption></figure></div>
      <h3>Claude Code</h3>
      <ol><li>在 CCSwitch 顶部选择 <b>Claude</b>，点击右上角“+”，选择<b>自定义配置</b>，供应商名称可填“皮皮虾 AI”。</li><li>Endpoint 填写 <code>${state.baseURL}</code>，<b>不要添加 /v1</b>。</li><li>认证令牌填写您自己的皮皮虾 AI API Key；保存后获取模型列表，模型以控制台可见范围为准。</li><li>启用该供应商后，完全退出再重新打开 Claude Code。</li></ol>
      ${snippet('Claude Endpoint', state.baseURL)}
      <h3>Codex</h3>
      <ol><li>在 CCSwitch 顶部选择 <b>Codex</b>，点击右上角“+”，选择<b>自定义配置</b>。</li><li>Base URL / Endpoint 填写 <code>${state.baseURL}/v1</code>。</li><li>API Key 填写您自己的皮皮虾 AI API Key，接口协议选择 <b>Responses</b>。</li><li>模型名称以控制台模型列表和您的当前权限为准；CCSwitch 的默认 Codex 示例模型为 <code>gpt-5.5</code>，不可用时请在控制台确认后替换。</li></ol>
      ${snippet('Codex Base URL', `${state.baseURL}/v1`)}
      <h3>让 Codex 在项目内调用生图</h3>
      <p>这是推荐方式：CCSwitch 继续使用<b>普通编程分组 Key</b>驱动 Codex；生图分组 Key 只保存在目标项目的本地配置文件里。需要生图时，直接在项目中对 Codex 说“调用皮皮虾生图 Skill 生成一张……”。</p>
      <ol><li>在控制台“API 密钥”额外创建一把 Key，分组选择<b>生图 API 分组</b>。不要改动 CCSwitch 正在使用的编程 Key。</li><li>在需要生图的项目根目录创建 <code>.pipixia-image/config.json</code>，写入下方配置；它只存在于本机项目中。</li><li>把 <code>.pipixia-image/config.json</code> 加入该项目的 <code>.gitignore</code>，随后让 Codex 创建并使用生图 Skill。</li><li>以后只需告诉 Codex 图片需求和保存位置；它通过 Skill 读取本地配置并调用图片生成接口。</li></ol>
      ${snippet('.pipixia-image/config.json', `{\n  "base_url": "${state.baseURL}/v1",\n  "api_key": "在这里粘贴生图 API 分组 Key",\n  "model": "在控制台模型列表确认的图片模型名称"\n}`)}
      ${snippet('.gitignore', `.pipixia-image/config.json`)}
      <p><b>把下面整段发给项目里的 Codex：</b></p>
      ${snippet('给 Codex 的 Skill 创建提示词', `请在当前项目创建一个本地生图 Skill，供后续按自然语言调用皮皮虾 AI 图片生成。\n\n要求：\n1. 创建 .codex/skills/pipixia-image/SKILL.md，说明何时调用、参数、失败处理与输出目录。\n2. 只从项目根目录的 .pipixia-image/config.json 读取 base_url、api_key、model；若文件或字段缺失，明确提示用户补充，绝不猜测或写入真实 Key。\n3. 使用 POST {base_url}/images/generations，Authorization 为 Bearer {api_key}，请求 JSON 包含 model、prompt、size。\n4. 同时兼容响应里的 data[].url 与 data[].b64_json；将图片保存到 output/images/，文件名使用安全的时间戳与序号。\n5. 创建一个可重复调用的本地脚本，不要把 Key 写进代码、日志、命令历史、Git 或输出文件。\n6. 在 .gitignore 中确保 .pipixia-image/config.json 被忽略；不要修改或覆盖现有 .gitignore 规则。\n7. 调用成功后只汇报生成文件路径、模型和必要的错误信息；不得回显 API Key。\n8. 先检查现有项目技术栈，沿用已有语言和脚本约定；完成后给出一次不含真实 Key 的验证说明。`)}
      <p><b>生图 Key 说明：</b>生图 Key 与 CCSwitch 的编程 Key 都属于同一账户，共用余额和套餐权益；两者区别只在绑定的 API 分组和可用模型。切换本页顶部端点时，只需同步修改 <code>config.json</code> 的 <code>base_url</code>。</p><p><b>说明：</b>CCSwitch 中切换 API 端点时，只修改 Endpoint/Base URL。遇到 429 时，请先检查余额、模型配额以及套餐/分组权益，不要只当作限流。</p><p class="guide-source">操作界面图来自 <a href="https://github.com/farion1231/cc-switch" target="_blank" rel="noreferrer">CCSwitch 开源项目</a>，仅用于说明入口位置；图中第三方供应商与皮皮虾 AI 无关。</p>`,
  },
  'chat-api': {
    title: '聊天补全 API',
    body: () => `${snippet('cURL', `curl "${state.baseURL}/v1/chat/completions" \\\n+  -H "Authorization: Bearer $PIPIXIA_API_KEY" \\\n+  -H "Content-Type: application/json" \\\n+  -d '{"model":"gpt-5.4-mini","messages":[{"role":"user","content":"你好"}]}'`)}`,
  },
  'stream-api': {
    title: '流式输出',
    body: () => `<p>将 <code>stream</code> 设为 <code>true</code>。客户端需要按 Server-Sent Events 流持续读取响应。</p>${snippet('cURL', `curl -N "${state.baseURL}/v1/chat/completions" \\\n+  -H "Authorization: Bearer $PIPIXIA_API_KEY" \\\n+  -H "Content-Type: application/json" \\\n+  -d '{"model":"gpt-5.4-mini","stream":true,"messages":[{"role":"user","content":"写一句问候"}]}'`)}`,
  },
  'responses-api': {
    title: 'Responses API',
    body: () => `${snippet('cURL', `curl "${state.baseURL}/v1/responses" \\\n+  -H "Authorization: Bearer $PIPIXIA_API_KEY" \\\n+  -H "Content-Type: application/json" \\\n+  -d '{"model":"gpt-5.4-mini","input":"你好"}'`)}`,
  },
  'image-api': {
    title: '图片生成 API',
    body: () => `<p><b>图片生成需要单独的生图 API Key。</b>API Key 会绑定一个 API 分组：请在控制台“API 密钥”中新建 Key，分组选择<b>生图 API 分组</b>，不要直接使用 CCSwitch 中 Claude/Codex 的编程分组 Key。</p><ol><li>打开控制台“API 密钥”，点击创建新的 API Key。</li><li>在分组下拉框选择<b>生图 API 分组</b>，创建并保存这把新 Key。</li><li>在您的生图客户端、脚本或插件中，将这把生图 Key 填入 <code>Authorization: Bearer ...</code>。</li><li>请求地址填写 <code>${state.baseURL}/v1/images/generations</code>；图片模型名称以该 Key 查询到的模型列表为准。</li></ol><p>不要把原来的 Claude/Codex Key 改到生图分组。应保留原 Key 给 CCSwitch 使用，并额外创建生图 Key。两个 Key 共用同一账户余额和套餐权益，但各自按绑定分组决定可用模型。</p>${snippet('cURL', `curl "${state.baseURL}/v1/images/generations" \\\n+  -H "Authorization: Bearer $PIPIXIA_IMAGE_API_KEY" \\\n+  -H "Content-Type: application/json" \\\n+  -d '{"model":"图片模型名称","prompt":"一幅夜晚海边的水彩画","size":"1024x1024"}'`)}`,
  },
  'models-api': {
    title: '获取模型列表',
    body: () => `<p>返回结果会受您的 API Key、分组和权限影响，因此它是确认可用模型名称的唯一可靠来源。</p>${snippet('cURL', `curl "${state.baseURL}/v1/models" \\\n+  -H "Authorization: Bearer $PIPIXIA_API_KEY"`)}`,
  },
}

guides['image-api'].body = () => `<p><b>推荐给 Codex 用户的生图方式：</b>CCSwitch 继续使用<b>普通编程分组 Key</b>运行 Codex；另外新建一把绑定<b>生图 API 分组</b>的 Key，只让项目里的生图 Skill 读取和调用。</p><ol><li>在控制台“API 密钥”新建一把 Key，分组选择<b>生图 API 分组</b>。不要把正在给 CCSwitch 使用的编程 Key 改到生图分组。</li><li>在需要生图的项目根目录创建 <code>.pipixia-image/config.json</code>，填入生图 Key、地址和图片模型。</li><li>将 <code>.pipixia-image/config.json</code> 加入该项目 <code>.gitignore</code>，确保绝不提交到 Git。</li><li>把下方“给 Codex 的提示词”发到该项目的 Codex 对话中。完成后，以后直接说“调用皮皮虾生图 Skill 生成一张……”即可。</li></ol>${snippet('.pipixia-image/config.json', `{\n  "base_url": "${state.baseURL}/v1",\n  "api_key": "在这里粘贴生图 API 分组 Key",\n  "model": "在控制台模型列表确认的图片模型名称"\n}`)}${snippet('.gitignore', `.pipixia-image/config.json`)}${snippet('给 Codex 的提示词', `请在当前项目创建一个本地生图 Skill。只读取项目根目录 .pipixia-image/config.json 中的 base_url、api_key、model，通过 POST {base_url}/images/generations 调用皮皮虾 AI 生图。\n\n创建 .codex/skills/pipixia-image/SKILL.md 和可重复调用的本地脚本：支持 data[].url 与 data[].b64_json 响应，将图片保存到 output/images/。确保 .pipixia-image/config.json 已加入 .gitignore。严禁把 API Key 写进源码、日志、命令输出、Git 或生成文件；成功后只汇报图片文件路径、模型和必要错误信息。先检查当前项目技术栈，沿用已有脚本约定。`)}<p><b>只有在您自己写脚本或接入第三方生图客户端时</b>，才直接使用下面的 HTTP 示例。该示例中的 <code>$PIPIXIA_IMAGE_API_KEY</code> 必须是生图 API 分组的 Key。</p>${snippet('cURL', `curl "${state.baseURL}/v1/images/generations" \\\n  -H "Authorization: Bearer $PIPIXIA_IMAGE_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"图片模型名称","prompt":"一幅夜晚海边的水彩画","size":"1024x1024"}'`)}`

function snippet(label, value) {
  return `<div class="code-panel"><div class="code-panel-head"><span>${label}</span><button class="copy-button" type="button" data-dialog-copy="${encodeURIComponent(value)}">复制</button></div><pre><code>${escapeHTML(value)}</code></pre></div>`
}

function escapeHTML(value) {
  return value.replace(/[&<>'"]/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[char]))
}

function updateEndpoint() {
  document.getElementById('base-url-value').textContent = `${state.baseURL}/v1`
  document.getElementById('selected-endpoint-label').textContent = state.label
  document.querySelectorAll('[data-base-url]').forEach((element) => { element.textContent = state.baseURL })
}

function notify(message) {
  const toast = document.getElementById('toast')
  toast.textContent = message
  toast.classList.add('is-visible')
  window.setTimeout(() => toast.classList.remove('is-visible'), 1800)
}

async function copy(value) {
  try {
    await navigator.clipboard.writeText(value)
    notify('已复制到剪贴板')
  } catch {
    notify('复制失败，请手动复制')
  }
}

document.querySelectorAll('.endpoint-card').forEach((button) => {
  button.addEventListener('click', () => {
    state.baseURL = button.dataset.endpoint
    state.label = button.dataset.label
    document.querySelectorAll('.endpoint-card').forEach((item) => {
      const selected = item === button
      item.classList.toggle('is-selected', selected)
      item.setAttribute('aria-checked', String(selected))
    })
    updateEndpoint()
    notify(`已选择${state.label}`)
  })
})

document.addEventListener('click', (event) => {
  const target = event.target.closest('[data-copy-target]')
  if (target) copy(document.getElementById(target.dataset.copyTarget).textContent)
})

const dialog = document.getElementById('guide-dialog')
document.querySelectorAll('.guide-open').forEach((button) => {
  button.addEventListener('click', () => {
    const guide = guides[button.dataset.guide]
    document.getElementById('dialog-title').textContent = guide.title
    document.getElementById('dialog-body').innerHTML = guide.body()
    dialog.showModal()
  })
})

dialog.addEventListener('click', (event) => {
  const copyButton = event.target.closest('[data-dialog-copy]')
  if (copyButton) copy(decodeURIComponent(copyButton.dataset.dialogCopy))
  if (event.target.classList.contains('dialog-close')) dialog.close()
  if (event.target === dialog) dialog.close()
})

updateEndpoint()
