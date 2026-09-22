const { test } = require("node:test"),
  assert = require("node:assert/strict"),
  fs = require("node:fs"),
  path = require("node:path");
const { JSDOM } = require(
  require.resolve("jsdom", { paths: [path.join(__dirname, "../frontend")] }),
);
const dir = path.join(__dirname, "docs-portal"),
  items = JSON.parse(
    fs.readFileSync(path.join(dir, "page-manifest.json"), "utf8"),
  );
test("all generated articles contain crawlable body, links and unique SEO without JavaScript", () => {
  const titles = new Set(),
    canonicals = new Set();
  for (const item of items) {
    const file = path.join(dir, item.path, "index.html"),
      dom = new JSDOM(fs.readFileSync(file, "utf8"), {
        url: "https://doc.psydo.top/" + item.path,
      }),
      d = dom.window.document;
    assert.equal(d.querySelector("#article h1").textContent, item.title);
    assert.ok(d.querySelector("#article").textContent.length > 150);
    assert.equal(d.querySelectorAll("#navigation a").length, items.length);
    assert.ok(d.querySelector("meta[name=description]").content.length > 10);
    assert.equal(d.querySelector("meta[name=robots]").content, "index,follow");
    const canonical = d.querySelector("link[rel=canonical]").href;
    assert.equal(canonical, "https://doc.psydo.top/" + item.path);
    assert.ok(!canonicals.has(canonical));
    canonicals.add(canonical);
    assert.ok(!titles.has(d.title));
    titles.add(d.title);
    const schema = JSON.parse(
      d.querySelector('script[type="application/ld+json"]').textContent,
    );
    assert.equal(schema.headline, item.title);
    assert.equal(schema.url, canonical);
    assert.equal(d.querySelectorAll("#article .article-maintenance").length, 1);
    for (const el of d.querySelectorAll(
      "a[href],script[src],link[href],img[src]",
    )) {
      const raw = el.getAttribute("src") || el.getAttribute("href");
      if (raw.startsWith("#")) continue;
      const url = new URL(raw, "https://doc.psydo.top/" + item.path);
      if (url.origin !== "https://doc.psydo.top") continue;
      const target = path.join(dir, decodeURIComponent(url.pathname));
      assert.ok(target.startsWith(dir));
      assert.ok(fs.existsSync(target), item.id + ": missing " + url.pathname);
      if (el.matches("#navigation a,#article a") && url.hash)
        assert.ok(!url.hash.startsWith("#guide/"), "hash article link remains");
    }
    for (const el of d.querySelectorAll("[data-console-path]")) {
      assert.equal(el.target, "_blank");
      assert.match(el.rel, /noopener/);
    }
    dom.window.close();
  }
});
test("sitemap contains all real paths once and never includes fragment URLs", () => {
  const xml = fs.readFileSync(path.join(dir, "sitemap.xml"), "utf8");
  assert.equal((xml.match(/<loc>/g) || []).length, items.length);
  assert.ok(!xml.includes("#"));
  for (const item of items)
    assert.ok(
      xml.includes("<loc>https://doc.psydo.top/" + item.path + "</loc>"),
    );
});
test("404 is noindex and assets resolve from domain root", () => {
  const dom = new JSDOM(fs.readFileSync(path.join(dir, "404.html"), "utf8"), {
      url: "https://doc.psydo.top/missing/nested/",
    }),
    d = dom.window.document;
  assert.equal(d.querySelector("base").href, "https://doc.psydo.top/");
  assert.match(d.querySelector("meta[name=robots]").content, /noindex/);
  assert.match(d.querySelector("h1").textContent, /没有找到/);
  dom.window.close();
});
