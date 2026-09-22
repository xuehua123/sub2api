// Shared, deterministic document URLs. The site can be mounted at / or /docs-portal/.
const docsScript = document.querySelector('script[src*="docs-ui.js"]');
const docsRoot = new URL(
  ".",
  new URL(docsScript.getAttribute("src"), document.baseURI),
);
function documentPath(id) {
  if (id === "quickstart") return "";
  if (id.startsWith("guide/")) return "guides/" + id.slice(6) + "/";
  return id + "/";
}
function documentURL(id, anchor = "") {
  const url = new URL(documentPath(id), docsRoot);
  if (anchor) url.hash = anchor;
  return url.pathname + url.hash;
}
function canonicalDocumentURL(id) {
  return "https://doc.psydo.top/" + documentPath(id);
}
function normalizeArticleLinks(scope) {
  scope.querySelectorAll('a[href^="#"]').forEach((a) => {
    if (a.hasAttribute("data-scroll")) return;
    const [id, anchor = ""] = a.getAttribute("href").slice(1).split("~");
    const resolved = Object.hasOwn(legacyAliases, id) ? legacyAliases[id] : id;
    if (Object.hasOwn(articles, resolved))
      a.setAttribute("href", documentURL(resolved, anchor));
  });
  scope
    .querySelectorAll('img[src^="./assets/"],a[href^="./assets/"]')
    .forEach((el) => {
      const attr = el.tagName === "IMG" ? "src" : "href";
      el.setAttribute(attr, new URL(el.getAttribute(attr), docsRoot).pathname);
    });
}
