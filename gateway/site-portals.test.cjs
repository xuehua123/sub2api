const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const root = path.resolve(__dirname, "..");
const { JSDOM, VirtualConsole } = require(
  require.resolve("jsdom", { paths: [path.join(root, "frontend")] }),
);

function loadPortal(
  folder,
  { url, preferences = {}, blockedStorage = false } = {},
) {
  const errors = [];
  const clipboard = [];
  const virtualConsole = new VirtualConsole();
  virtualConsole.on("jsdomError", (error) => errors.push(error.message));
  const html = fs.readFileSync(
    path.join(
      root,
      "gateway",
      folder,
      folder === "docs-portal" ? "template.html" : "index.html",
    ),
    "utf8",
  );
  const dom = new JSDOM(html, {
    url: url || `https://doc.psydo.top/`,
    runScripts: "outside-only",
    pretendToBeVisual: true,
    virtualConsole,
  });
  const { window } = dom;
  window.matchMedia = () => ({ matches: false });
  window.scrollTo = () => {};
  window.HTMLElement.prototype.scrollIntoView = () => {};
  window.IntersectionObserver = class {
    observe() {}
    disconnect() {}
  };
  window.HTMLDialogElement.prototype.showModal = function () {
    this.open = true;
  };
  window.HTMLDialogElement.prototype.close = function () {
    this.open = false;
  };
  Object.defineProperty(window.navigator, "clipboard", {
    value: { writeText: async (text) => clipboard.push(text) },
  });
  for (const [key, value] of Object.entries(preferences))
    window.localStorage.setItem(key, value);
  if (blockedStorage)
    Object.defineProperty(window, "localStorage", {
      get() {
        throw new Error("Storage blocked");
      },
    });
  for (const script of window.document.querySelectorAll("script[src]")) {
    const file = path.resolve(
      root,
      "gateway",
      folder,
      script.getAttribute("src").split("?")[0],
    );
    vm.runInContext(fs.readFileSync(file, "utf8"), dom.getInternalVMContext(), {
      filename: file,
    });
  }
  return { dom, window, document: window.document, errors, clipboard };
}
const settle = () => new Promise((resolve) => setTimeout(resolve, 15));

// These checks exercise behavior across content, state and navigation, rather than CSS snapshots.
test("every document can be opened directly and all internal links resolve", async () => {
  const app = loadPortal("docs-portal");
  try {
    const links = [...app.document.querySelectorAll("#navigation a")].map(
      (a) => "#" + a.dataset.page,
    );
    assert.ok(links.length >= 20);
    for (const hash of links) {
      app.window.location.hash = hash;
      await settle();
      assert.equal(
        app.document.querySelectorAll("#article h1").length,
        1,
        hash,
      );
      assert.ok(
        !app.document
          .querySelector("#article")
          .textContent.includes("undefined"),
        hash,
      );
      for (const a of app.document.querySelectorAll('#article a[href^="#"]')) {
        const value = a.getAttribute("href").slice(1).split("~")[0];
        const known =
          links.includes("#" + value) || ["ccswitch", "top"].includes(value);
        assert.ok(known, `${hash}: dangling article link ${value}`);
      }
    }
    assert.deepEqual(app.errors, []);
  } finally {
    app.dom.window.close();
  }
});

test("endpoint and OS preferences update current commands, links and copied text", async () => {
  const app = loadPortal("docs-portal", {
    url: "https://doc.ppxcode.com/#guide/codex",
  });
  try {
    assert.equal(
      app.document.querySelector("#endpoint").value,
      "https://cn2.ppxcode.com",
    );
    const selector = app.document.querySelector("#endpoint");
    selector.value = "https://cf.ppxcode.com";
    selector.dispatchEvent(new app.window.Event("change", { bubbles: true }));
    app.document.querySelector('[data-os="unix"]').click();
    const codes = [...app.document.querySelectorAll("pre")]
      .map((p) => p.textContent)
      .join("\n");
    assert.ok(codes.includes("https://cf.ppxcode.com/v1/models"));
    assert.ok(!codes.includes("https://cn2.ppxcode.com"));
    assert.equal(
      app.window.localStorage.getItem("ppx-docs-endpoint"),
      "https://cf.ppxcode.com",
    );
    assert.equal(app.window.localStorage.getItem("ppx-docs-os"), "unix");
    app.document.querySelector(".copy-button").click();
    await settle();
    assert.equal(app.clipboard[0], "https://cf.ppxcode.com/v1");
    for (const a of app.document.querySelectorAll("[data-console-path]"))
      assert.ok(a.href.startsWith("https://cf.ppxcode.com/"));
  } finally {
    app.dom.window.close();
  }
});

test("Chinese full-text search finds troubleshooting, supports empty results and escapes input", () => {
  const app = loadPortal("docs-portal");
  try {
    app.document.querySelector("#open-search").click();
    const input = app.document.querySelector("#search-input");
    input.value = "自动压缩";
    input.dispatchEvent(new app.window.Event("input"));
    assert.ok(
      app.document.querySelector('#search-results a[href="/context/"]'),
    );
    assert.equal(
      app.document.querySelector("#search-results a").pathname,
      "/context/",
    );
    input.value = "<img src=x onerror=alert(1)>";
    input.dispatchEvent(new app.window.Event("input"));
    assert.equal(app.document.querySelectorAll("#search-results a").length, 0);
    assert.equal(
      app.document.querySelectorAll("#search-results img").length,
      0,
    );
    assert.match(
      app.document.querySelector("#search-count").textContent,
      /没有匹配/,
    );
  } finally {
    app.dom.window.close();
  }
});

test("legacy URLs, shared section links, invalid hashes and restricted storage are safe", async () => {
  const app = loadPortal("docs-portal", {
    url: "https://doc.psydo.top/#ccswitch",
    blockedStorage: true,
  });
  try {
    assert.equal(app.document.querySelector("h1").textContent, "CCSwitch");
    app.window.location.hash = "#guide/codex~section-1";
    await settle();
    assert.equal(app.document.querySelector("h1").textContent, "Codex 接入");
    assert.ok(app.document.getElementById("section-1"));
    app.window.history.replaceState(null, "", "/#constructor");
    app.window.dispatchEvent(new app.window.HashChangeEvent("hashchange"));
    await settle();
    assert.match(app.document.querySelector("h1").textContent, /没有找到/);
    app.window.document.documentElement.dataset.pageId = "quickstart";
    app.window.location.hash = "#%E0%A4%A";
    await settle();
    assert.equal(app.document.querySelector("h1").textContent, "快速开始");
    app.document.querySelector("#theme-toggle").click();
    assert.equal(app.document.documentElement.dataset.theme, "dark");
    assert.deepEqual(app.errors, []);
  } finally {
    app.dom.window.close();
  }
});

test("home tabs support keyboard navigation and route edge visitors to matching docs", () => {
  const app = loadPortal("home-portal", {
    url: "https://cn2.ppxcode.com/home/index.html",
  });
  try {
    const tabs = [...app.document.querySelectorAll("[data-demo]")];
    tabs[0].dispatchEvent(
      new app.window.KeyboardEvent("keydown", {
        key: "ArrowRight",
        bubbles: true,
      }),
    );
    assert.equal(tabs[1].getAttribute("aria-selected"), "true");
    assert.match(
      app.document.querySelector("#demo-code").textContent,
      /Responses/,
    );
    assert.equal(
      app.document.querySelector("#demo-link").href,
      "https://doc.ppxcode.com/guides/codex/",
    );
    for (const a of app.document.querySelectorAll("[data-app-path]"))
      assert.ok(a.href.startsWith("https://cn2.ppxcode.com/"));
    assert.deepEqual(app.errors, []);
  } finally {
    app.dom.window.close();
  }
});

test("local preview links join both portals and all document assets are present", () => {
  const app = loadPortal("home-portal", {
    url: "http://127.0.0.1:4178/home-portal/",
  });
  try {
    assert.equal(
      app.document.querySelector("#demo-link").href,
      "http://127.0.0.1:4178/docs-portal/guides/ccswitch/",
    );
    for (const folder of ["home-portal", "docs-portal"]) {
      const html = new JSDOM(
        fs.readFileSync(
          path.join(
            root,
            "gateway",
            folder,
            folder === "docs-portal" ? "template.html" : "index.html",
          ),
          "utf8",
        ),
      );
      for (const element of html.window.document.querySelectorAll(
        'script[src],link[rel="stylesheet"],img[src]',
      )) {
        const src = (
          element.getAttribute("src") || element.getAttribute("href")
        ).split("?")[0];
        assert.ok(
          fs.existsSync(path.resolve(root, "gateway", folder, src)),
          `${folder}/${src}`,
        );
      }
      html.window.close();
    }
  } finally {
    app.dom.window.close();
  }
});

test("CCSwitch is recommended, with explicit restart and image-derived diagnostics", async () => {
  const app = loadPortal("docs-portal", {
    url: "https://doc.psydo.top/#guide/ccswitch",
  });
  try {
    const body = app.document.querySelector("#article").textContent;
    assert.match(body, /保存并启用 → 完全退出 → 重新打开/);
    assert.match(body, /只关对话、新建任务或只重启 CCSwitch/);
    assert.match(body, /VS Code/);
    const recommended = app.document.querySelector(
      '#navigation a[data-page="guide/ccswitch"]',
    );
    assert.match(recommended.textContent, /推荐/);
    app.window.location.hash = "#codex-connection";
    await settle();
    const diagnostic = app.document.querySelector("#article").textContent;
    for (const text of [
      "codex doctor",
      "HTTP 101",
      "supports_websockets",
      "~/.codex/.env",
      "当前客户端",
      "TUN",
      "实际 HTTP",
    ])
      assert.ok(diagnostic.includes(text), text);
    app.document.querySelector('[data-os="unix"]').click();
    assert.match(
      app.document.querySelector("#article").textContent,
      /https_proxy/,
    );
    app.window.location.hash = "#connectivity-guide";
    await settle();
    const troubleshooting = app.document.querySelector("#article").textContent;
    for (const text of [
      "检测 URL",
      "代理节点",
      "分组选择",
      "渠道状态",
      "额度卡选择",
      "美国或加拿大",
    ])
      assert.ok(troubleshooting.includes(text), text);
    app.window.location.hash = "#support-checklist";
    await settle();
    app.document.querySelector(".copy-button").click();
    await settle();
    assert.ok(
      app.clipboard[0].includes("正在使用的 URL：https://api.psydo.top"),
    );
    assert.ok(app.clipboard[0].includes("目标工具已完全退出并重开"));
    assert.deepEqual(app.errors, []);
  } finally {
    app.dom.window.close();
  }
});

test("API examples and copied commands explicitly include User-Agent", async () => {
  const app = loadPortal("docs-portal");
  try {
    for (const route of [
      "quickstart",
      "guide/codex",
      "guide/python",
      "guide/node",
      "guide/http-sdk",
      "guide/models-api",
      "guide/chat-api",
      "guide/stream-api",
      "guide/responses-api",
      "guide/image-api",
      "context",
    ]) {
      app.window.location.hash = "#" + route;
      await settle();
      for (const os of ["windows", "unix"]) {
        app.document.querySelector(`[data-os="${os}"]`)?.click();
        const commands = [...app.document.querySelectorAll("pre")].filter((e) =>
          /curl |Invoke-RestMethod|client = (?:new )?OpenAI|HttpRequest request/.test(
            e.textContent,
          ),
        );
        assert.ok(commands.length > 0, route);
        for (const command of commands) {
          assert.match(command.textContent, /User-Agent/, route + ": " + os);
          assert.match(command.textContent, /pipixia-client\/1\.0/, route);
          assert.ok(
            !command.textContent.includes("\\n  -H"),
            route + ": literal newline escape",
          );
        }
      }
    }
    app.window.location.hash = "#guide/models-api";
    await settle();
    app.document.querySelector(".copy-button").click();
    await settle();
    assert.match(app.clipboard.at(-1), /-H "User-Agent: pipixia-client\/1\.0"/);
    app.window.location.hash = "#guide/ccswitch";
    await settle();
    assert.match(
      app.document.querySelector("#article").textContent,
      /第三方工具：确认 UA 被保留/,
    );
    assert.deepEqual(app.errors, []);
  } finally {
    app.dom.window.close();
  }
});

test("CCSwitch one-click import tutorial shows diagrams and opens console links separately", () => {
  const app = loadPortal("docs-portal", {
    url: "https://doc.psydo.top/#guide/ccswitch",
  });
  try {
    const body = app.document.querySelector("#article");
    assert.match(body.textContent, /导入到 CCS/);
    assert.ok(
      body.querySelector(
        'a[href="https://github.com/farion1231/cc-switch/releases"]',
      ),
    );
    const visibleImages = [...body.querySelectorAll("img")].filter(
      (img) => !img.closest("details"),
    );
    assert.ok(visibleImages.length >= 2);
    for (const image of visibleImages)
      assert.ok(
        fs.existsSync(
          path.resolve(
            root,
            "gateway/docs-portal",
            image.getAttribute("src").replace(/^\//, ""),
          ),
        ),
      );
    for (const link of app.document.querySelectorAll("[data-console-path]")) {
      assert.equal(link.target, "_blank");
      assert.equal(link.rel, "noopener noreferrer");
    }
    assert.ok(body.querySelector('[data-console-path="/keys"]'));
    assert.ok(body.querySelector('[data-console-path="/usage"]'));
  } finally {
    app.dom.window.close();
  }
});

test("review fixes keep article metadata and contextual console destinations accurate", async () => {
  const app = loadPortal("docs-portal");
  try {
    app.window.location.hash = "#guide/python";
    await settle();
    assert.equal(
      app.document.querySelector('meta[property="og:title"]').content,
      app.document.title,
    );
    assert.match(
      app.document.querySelector('meta[name="description"]').content,
      /选择适合/,
    );
    assert.ok(
      app.document.querySelector('#article [data-console-path="/usage"]'),
    );
    app.window.location.hash = "#errors";
    await settle();
    assert.ok(
      app.document.querySelector('#article [data-console-path="/issues/new"]'),
    );
    app.document.querySelector("#menu-toggle").click();
    app.document.dispatchEvent(
      new app.window.KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
    assert.equal(app.document.activeElement.id, "menu-toggle");
    app.window.location.hash = "#guide/ccswitch";
    await settle();
    const shot = app.document.querySelector(
      'img[src$="pipixia-provider.webp"]',
    );
    assert.equal(shot.getAttribute("width"), "1966");
    assert.equal(shot.getAttribute("height"), "610");
    assert.deepEqual(app.errors, []);
  } finally {
    app.dom.window.close();
  }
});
