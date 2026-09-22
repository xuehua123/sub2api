/** Build static, crawlable documents from the same content used by the browser. */
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const root = __dirname;
const { JSDOM, VirtualConsole } = require(
  require.resolve("jsdom", { paths: [path.join(root, "../frontend")] }),
);
const dir = path.join(root, "docs-portal");
const template = fs.readFileSync(path.join(dir, "template.html"), "utf8");
const baseURL = "https://doc.psydo.top/";
function render(id) {
  const errors = [];
  const vc = new VirtualConsole();
  vc.on("jsdomError", (e) => errors.push(e.message));
  const dom = new JSDOM(template, {
    url: baseURL,
    runScripts: "outside-only",
    pretendToBeVisual: true,
    virtualConsole: vc,
  });
  const w = dom.window,
    d = w.document;
  d.documentElement.dataset.pageId = id;
  w.matchMedia = () => ({ matches: false });
  w.scrollTo = () => {};
  w.HTMLElement.prototype.scrollIntoView = () => {};
  w.IntersectionObserver = class {
    observe() {}
    disconnect() {}
  };
  for (const el of d.querySelectorAll("script[src]")) {
    const file = path.join(dir, el.getAttribute("src").split("?")[0]);
    vm.runInContext(fs.readFileSync(file, "utf8"), dom.getInternalVMContext(), {
      filename: file,
    });
  }
  const items = vm.runInContext(
    "articleGroups.flatMap(g=>g.items.map(([id,title])=>({id,title:articles[id].title,navTitle:title,group:g.title,path:documentPath(id),description:articles[id].description})))",
    dom.getInternalVMContext(),
  );
  if (errors.length) throw new Error(errors.join("\n"));
  const item = items.find((x) => x.id === id);
  const route = item?.path || "404.html";
  const canonical = baseURL + (id === "quickstart" ? "" : route);
  function meta(attr, key, value) {
    let e = d.querySelector(`meta[${attr}="${key}"]`);
    if (!e) {
      e = d.createElement("meta");
      e.setAttribute(attr, key);
      d.head.append(e);
    }
    e.content = value;
  }
  meta("name", "robots", item ? "index,follow" : "noindex,follow");
  meta("property", "og:url", canonical);
  meta("property", "og:image", baseURL + "assets/social-card.png");
  meta("property", "og:image:width", "1200");
  meta("property", "og:image:height", "630");
  meta(
    "property",
    "og:image:alt",
    "皮皮虾 AI 开发文档：CCSwitch 接入、API 开发与排障",
  );
  meta("name", "twitter:card", "summary_large_image");
  meta("name", "twitter:title", d.title);
  meta(
    "name",
    "twitter:description",
    d.querySelector('meta[name="description"]').content,
  );
  meta("name", "twitter:image", baseURL + "assets/social-card.png");
  const link = d.createElement("link");
  link.rel = "canonical";
  link.href = canonical;
  d.head.append(link);
  const ld = d.createElement("script");
  ld.type = "application/ld+json";
  ld.textContent = JSON.stringify({
    "@context": "https://schema.org",
    "@type": "TechArticle",
    headline: item?.title || "未找到页面",
    description: item?.description || "文档不存在",
    inLanguage: "zh-CN",
    url: canonical,
    publisher: { "@type": "Organization", name: "皮皮虾 AI" },
    isPartOf: { "@type": "WebSite", name: "皮皮虾 AI 开发文档", url: baseURL },
  });
  if (item) d.head.append(ld);
  if (!item) {
    const base = d.createElement("base");
    base.href = "/";
    d.head.prepend(base);
  }
  const outputFile =
    id === "quickstart"
      ? path.join(dir, "index.html")
      : item
        ? path.join(dir, route, "index.html")
        : path.join(dir, "404.html");
  const rel = path
    .relative(path.dirname(outputFile), dir)
    .replaceAll("\\", "/");
  const prefix = rel ? rel + "/" : "./";
  // Assets and document links remain valid when previewed at /docs-portal/.
  for (const el of d.querySelectorAll("[src],a[href],link[href]")) {
    const attr = el.hasAttribute("src") ? "src" : "href";
    let value = el.getAttribute(attr);
    if (value.startsWith("./")) el.setAttribute(attr, prefix + value.slice(2));
    else if (value.startsWith("/") && !value.startsWith("//"))
      el.setAttribute(attr, prefix + value.slice(1));
  }
  const nojs = d.createElement("style");
  nojs.textContent =
    "html:not([data-enhanced]) #open-search,html:not([data-enhanced]) #share-article,html:not([data-enhanced]) .copy-button,html:not([data-enhanced]) .os-selector,html:not([data-enhanced]) #theme-toggle{display:none} @media(max-width:760px){html:not([data-enhanced]) .sidebar{display:block;position:static;width:auto;height:auto;max-height:none;box-shadow:none}html:not([data-enhanced]) .right-rail{order:0}}";
  d.head.append(nojs);
  d.documentElement.removeAttribute("data-theme");
  d.documentElement.removeAttribute("data-enhanced");
  fs.mkdirSync(path.dirname(outputFile), { recursive: true });
  fs.writeFileSync(
    outputFile,
    "<!doctype html>\n" + d.documentElement.outerHTML.replace(/[ \t]+$/gm, "") + "\n",
  );
  dom.window.close();
  return items;
}
const items = render("quickstart");
for (const item of items) if (item.id !== "quickstart") render(item.id);
render("not-found");
const xml =
  '<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n' +
  items
    .map((x) => "  <url><loc>" + baseURL + x.path + "</loc></url>")
    .join("\n") +
  "\n</urlset>\n";
fs.writeFileSync(path.join(dir, "sitemap.xml"), xml);
fs.writeFileSync(
  path.join(dir, "robots.txt"),
  "User-agent: *\nAllow: /\nDisallow: /template.html\nSitemap: " +
    baseURL +
    "sitemap.xml\n",
);
fs.writeFileSync(
  path.join(dir, "page-manifest.json"),
  JSON.stringify(items, null, 2) + "\n",
);
console.log(
  `Generated ${items.length} static articles, 404, sitemap and robots.txt`,
);
