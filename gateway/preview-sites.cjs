/** Local static preview with production-style 404 semantics; no production services. */
const http = require("node:http"),
  fs = require("node:fs"),
  path = require("node:path");
const root = __dirname,
  port = Number(process.env.PORT || 4179);
const types = {
  ".html": "text/html; charset=utf-8",
  ".js": "application/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".xml": "application/xml; charset=utf-8",
  ".txt": "text/plain; charset=utf-8",
  ".png": "image/png",
  ".svg": "image/svg+xml",
  ".webp": "image/webp",
};
http
  .createServer((req, res) => {
    let pathname;
    try {
      pathname = decodeURIComponent(
        new URL(req.url, "http://localhost").pathname,
      );
    } catch {
      res.writeHead(400);
      return res.end();
    }
    const file = path.resolve(root, "." + pathname);
    if (
      !file.startsWith(root + path.sep) ||
      !["GET", "HEAD"].includes(req.method)
    ) {
      res.writeHead(404);
      return res.end();
    }
    let selected = file;
    if (fs.existsSync(file) && fs.statSync(file).isDirectory()) {
      if (!pathname.endsWith("/")) {
        res.writeHead(301, { Location: pathname + "/" });
        return res.end();
      }
      selected = path.join(file, "index.html");
    }
    let status = 200;
    if (
      pathname.endsWith("/template.html") ||
      !fs.existsSync(selected) ||
      !fs.statSync(selected).isFile()
    ) {
      status = 404;
      selected = path.join(root, "docs-portal", "404.html");
    }
    let body = fs.readFileSync(selected);
    if (status === 404)
      body = Buffer.from(
        body
          .toString()
          .replace('<base href="/">', '<base href="/docs-portal/">'),
      );
    res.writeHead(status, {
      "Content-Type":
        types[path.extname(selected)] || "application/octet-stream",
      "Content-Length": body.length,
      "Cache-Control": "no-store",
      "X-Content-Type-Options": "nosniff",
      "X-Robots-Tag": "noindex",
    });
    res.end(req.method === "HEAD" ? undefined : body);
  })
  .listen(port, "127.0.0.1", () =>
    console.log(`Preview http://127.0.0.1:${port}/docs-portal/ (noindex)`),
  );
