(() => {
  const hosts = new Set([
    "api.psydo.top",
    "api.ppx-ai.com",
    "us.psydo.top",
    "cn2.ppxcode.com",
    "cf.ppxcode.com",
  ]);
  const local = ["localhost", "127.0.0.1"].includes(location.hostname);
  let entry = hosts.has(location.hostname)
    ? location.origin
    : "https://api.psydo.top";
  try {
    const ref = new URL(document.referrer);
    if (hosts.has(ref.hostname)) entry = ref.origin;
  } catch {}
  const docs = local
    ? "../docs-portal/"
    : ["cn2.ppxcode.com", "cf.ppxcode.com"].includes(new URL(entry).hostname)
      ? "https://doc.ppxcode.com/"
      : "https://doc.psydo.top/";
  function links() {
    document
      .querySelectorAll("[data-app-path]")
      .forEach((a) => (a.href = entry + a.dataset.appPath));
    document.querySelectorAll("[data-doc]").forEach((a) => {
      const id = a.dataset.doc.replace(/^#/, "");
      const route =
        id === "quickstart" || !id
          ? ""
          : id.startsWith("guide/")
            ? "guides/" + id.slice(6) + "/"
            : id + "/";
      a.href = docs + route;
      a.target = "_top";
    });
  }
  const themeButton = document.querySelector(".theme-button");
  let theme;
  try {
    theme = localStorage.getItem("ppx-theme");
  } catch {}
  function setTheme(value) {
    document.documentElement.dataset.theme = value;
    themeButton.setAttribute(
      "aria-label",
      value === "dark" ? "切换浅色主题" : "切换深色主题",
    );
  }
  setTheme(["dark", "light"].includes(theme) ? theme : "dark");
  themeButton.addEventListener("click", () => {
    const value =
      document.documentElement.dataset.theme === "dark" ? "light" : "dark";
    setTheme(value);
    try {
      localStorage.setItem("ppx-theme", value);
    } catch {}
  });
  const examples = {
    ccswitch: {
      file: "CCSwitch · 自定义供应商",
      code: "供应商    皮皮虾 AI\n工具      Codex / Claude Code\n配置      一键导入 → 启用 → 完全退出 → 重开",
      note: "必须完全退出 Codex 等工具后重新打开，配置才会重新加载。",
      title: "打开 CCSwitch 完整教程",
      route: "#guide/ccswitch",
    },
    codex: {
      file: "接入清单 · Responses",
      code: "01  在控制台创建编程分组 Key\n02  Base URL 填写到 /v1\n03  选择 Responses 协议与可用模型",
      note: "推荐先用 CCSwitch 配置；设置后完全退出 Codex，再重新打开。",
      title: "打开 Codex 完整教程",
      route: "#guide/codex",
    },
    claude: {
      file: "接入清单 · Messages",
      code: "01  创建有 Claude 模型权限的 Key\n02  ANTHROPIC_BASE_URL 填根地址\n03  配置令牌后重新打开终端工具",
      note: "Claude Code 的根地址不追加 /v1。",
      title: "打开 Claude Code 完整教程",
      route: "#guide/claude",
    },
    api: {
      file: "Python · 配置示例",
      code:
        'client = OpenAI(\n    api_key=os.environ["PIPIXIA_API_KEY"],\n    base_url="' +
        entry +
        '/v1",\n    default_headers={"User-Agent": "pipixia-client/1.0"},\n)',
      note: "请求头带上 User-Agent（UA），密钥从环境变量读取。",
      title: "打开 Python 接入教程",
      route: "#guide/python",
    },
  };
  const tabs = [...document.querySelectorAll("[data-demo]")];
  function select(tab) {
    const item = examples[tab.dataset.demo];
    tabs.forEach((t) => {
      t.setAttribute("aria-selected", String(t === tab));
      t.tabIndex = t === tab ? 0 : -1;
    });
    document
      .getElementById("demo-panel")
      .setAttribute("aria-labelledby", tab.id);
    document.getElementById("demo-file").textContent = item.file;
    document.getElementById("demo-code").textContent = item.code;
    document.getElementById("demo-note").textContent = item.note;
    const link = document.getElementById("demo-link");
    link.textContent = item.title + " →";
    link.dataset.doc = item.route;
    links();
  }
  tabs.forEach((tab, index) => {
    tab.addEventListener("click", () => select(tab));
    tab.addEventListener("keydown", (e) => {
      let next;
      if (e.key === "ArrowRight") next = (index + 1) % tabs.length;
      else if (e.key === "ArrowLeft")
        next = (index + tabs.length - 1) % tabs.length;
      else if (e.key === "Home") next = 0;
      else if (e.key === "End") next = tabs.length - 1;
      else return;
      e.preventDefault();
      select(tabs[next]);
      tabs[next].focus();
    });
  });
  links();
  select(tabs[0]);
})();
