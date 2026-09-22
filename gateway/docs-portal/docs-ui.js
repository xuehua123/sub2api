(() => {
  document.documentElement.dataset.enhanced = "true";
  const $ = (id) => document.getElementById(id);
  const nav = $("navigation");
  let active = "quickstart";
  let headingObserver;
  let searchIndex = [];
  let timer;
  const flat = articleGroups.flatMap((group) =>
    group.items.map(([id, title]) => ({ id, title, group: group.title })),
  );
  nav.innerHTML = articleGroups
    .map(
      (group) =>
        `<div class="nav-group"><h2>${group.title}</h2>${group.items.map(([id, title]) => `<a href="${documentURL(id)}" data-page="${id}">${title}</a>`).join("")}</div>`,
    )
    .join("");
  const notify = (message) => {
    $("toast").textContent = message;
    $("toast").classList.add("visible");
    clearTimeout(timer);
    timer = setTimeout(() => $("toast").classList.remove("visible"), 2400);
  };
  async function copy(value, button) {
    try {
      await navigator.clipboard.writeText(value);
      notify("已复制");
      if (button) {
        const original = button.textContent;
        button.textContent = "已复制";
        setTimeout(() => {
          if (button.isConnected) button.textContent = original;
        }, 1800);
      }
    } catch {
      notify("无法访问剪贴板，请选中文字手动复制");
    }
  }
  function consoleLinks() {
    document.querySelectorAll("[data-console-path]").forEach((a) => {
      a.href = state.baseURL + a.dataset.consolePath;
      a.target = "_blank";
      a.rel = "noopener noreferrer";
    });
  }
  function closeMenu() {
    $("sidebar").classList.remove("open");
    $("menu-toggle").setAttribute("aria-expanded", "false");
    $("menu-toggle").setAttribute("aria-label", "打开目录");
  }
  nav.addEventListener("click", (event) => {
    if (event.target.closest("a")) closeMenu();
  });
  function render({ focus = false, scroll = true } = {}) {
    let raw;
    try {
      raw = decodeURIComponent(location.hash.slice(1));
    } catch {
      raw = "";
    }
    let route = document.documentElement.dataset.pageId || "quickstart";
    let anchor = raw;
    const [legacyID, legacySection = ""] = raw.split("~");
    const alias = Object.hasOwn(legacyAliases, legacyID)
      ? legacyAliases[legacyID]
      : legacyID;
    if (Object.hasOwn(articles, alias)) {
      route = alias;
      anchor = legacySection;
      history.replaceState(null, "", documentURL(route, anchor));
    } else if (legacyID) {
      const old = new DOMParser().parseFromString(
        legacyArticles.errors.html,
        "text/html",
      );
      if (old.getElementById(legacyID)) {
        route = "errors";
        anchor = legacyID;
        history.replaceState(null, "", documentURL(route, anchor));
      } else if (
        location.pathname === docsRoot.pathname &&
        !legacyID.startsWith("section-")
      ) {
        route = "not-found";
      }
    }
    active = route;
    document.documentElement.dataset.pageId = active;
    const article = Object.hasOwn(articles, active) ? articles[active] : null;
    const item = flat.find((x) => x.id === active);
    $("breadcrumb").textContent = article
      ? (item?.group || "接入指南") + " / " + article.title
      : "文档 / 未找到页面";
    $("article").innerHTML = article
      ? `<p class="eyebrow">${item?.group || "接入指南"}</p><h1>${article.title}</h1><p class="article-description">${article.description}</p>${article.body()}`
      : `<h1>这个文档地址没有找到</h1><p>链接可能已经变化。您可以搜索关键词，或<a href="${documentURL("quickstart")}">返回快速开始</a>。</p>`;
    document.title = (article?.title || "未找到页面") + " · 皮皮虾 AI 文档";
    const description =
      article?.description || "该文档不存在，请搜索文档或返回快速开始。";
    document
      .querySelector('meta[name="description"]')
      ?.setAttribute("content", description);
    document
      .querySelector('meta[property="og:title"]')
      ?.setAttribute("content", document.title);
    document
      .querySelector('meta[property="og:description"]')
      ?.setAttribute("content", description);
    // Legacy sections already contain title blocks; the article shell supplies the title.
    $("article")
      .querySelectorAll(".section-heading")
      .forEach((e) => e.remove());
    $("article")
      .querySelectorAll(".guide-open")
      .forEach((button) => {
        const a = document.createElement("a");
        a.href = "#guide/" + button.dataset.guide;
        a.textContent = button.textContent;
        button.replaceWith(a);
      });
    $("article")
      .querySelectorAll('a[target="_blank"]')
      .forEach((a) => (a.rel = "noopener noreferrer"));
    nav.querySelectorAll("a").forEach((a) => {
      if (a.dataset.page === active) a.setAttribute("aria-current", "page");
      else a.removeAttribute("aria-current");
    });
    $("article")
      .querySelectorAll('[role="table"] [role="row"]')
      .forEach((row) => {
        const header =
          row.classList.contains("error-head") ||
          row.classList.contains("combination-head");
        [...row.children].forEach((cell) =>
          cell.setAttribute("role", header ? "columnheader" : "cell"),
        );
      });
    const headings = [...$("article").querySelectorAll("h2,h3")];
    headings.forEach((h, i) => {
      if (!h.id) h.id = "section-" + i;
    });
    $("toc").innerHTML =
      "<p>本页内容</p>" +
      headings
        .filter(
          (h) =>
            h.tagName === "H2" ||
            headings.filter((x) => x.tagName === "H2").length === 0,
        )
        .map(
          (h) =>
            `<a href="${documentURL(active, h.id)}" data-scroll="${escapeHTML(h.id)}">${escapeHTML(h.textContent)}</a>`,
        )
        .join("");
    headingObserver?.disconnect();
    headingObserver = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            $("toc")
              .querySelectorAll("a")
              .forEach((a) =>
                a.classList.toggle(
                  "active",
                  a.dataset.scroll === entry.target.id,
                ),
              );
          }
        });
      },
      { rootMargin: "-80px 0px -65% 0px" },
    );
    headings.forEach((h) => headingObserver.observe(h));
    const index = flat.findIndex((x) => x.id === active);
    $("article-pagination").innerHTML = article
      ? [flat[index - 1], flat[index + 1]]
          .map((x, i) =>
            x
              ? `<a href="${documentURL(x.id)}"><small>${i ? "下一篇 →" : "← 上一篇"}</small><strong>${x.title}</strong></a>`
              : "<span></span>",
          )
          .join("")
      : "";
    if (article) {
      const note = document.createElement("p");
      note.className = "article-maintenance";
      note.textContent = "内容维护：2026-09-22 · 客户端界面可能随版本变化";
      $("article").append(note);
      document
        .querySelector('meta[property="og:url"]')
        ?.setAttribute("content", canonicalDocumentURL(active));
      document
        .querySelector('meta[name="twitter:title"]')
        ?.setAttribute("content", document.title);
      document
        .querySelector('meta[name="twitter:description"]')
        ?.setAttribute("content", description);
      const schema = document.querySelector(
        'script[type="application/ld+json"]',
      );
      if (schema)
        schema.textContent = JSON.stringify({
          "@context": "https://schema.org",
          "@type": "TechArticle",
          headline: article.title,
          description,
          inLanguage: "zh-CN",
          url: canonicalDocumentURL(active),
          publisher: { "@type": "Organization", name: "皮皮虾 AI" },
        });
      document
        .querySelector('meta[name="robots"]')
        ?.setAttribute("content", "index,follow");
    }
    normalizeArticleLinks(document);
    consoleLinks();
    const canonical = document.querySelector('link[rel="canonical"]');
    if (canonical && article) canonical.href = canonicalDocumentURL(active);
    if (!article)
      document
        .querySelector('meta[name="robots"]')
        ?.setAttribute("content", "noindex,follow");
    closeMenu();
    if (scroll) {
      if (anchor) {
        const target = document.getElementById(anchor);
        if (target?.closest("details")) target.closest("details").open = true;
        target?.scrollIntoView();
      } else window.scrollTo(0, 0);
    }
    if (focus) $("article").focus({ preventScroll: true });
    // Expose static routes to the generator without adding production globals.
  }
  function buildIndex() {
    searchIndex = flat.map((item) => {
      const article = articles[item.id];
      const doc = new DOMParser().parseFromString(article.body(), "text/html");
      return {
        ...item,
        text: (article.description + " " + doc.body.textContent).replace(
          /\s+/g,
          " ",
        ),
      };
    });
  }
  function search() {
    const q = $("search-input").value.trim().toLocaleLowerCase();
    const terms = q.split(/\s+/).filter(Boolean);
    const results = (
      q
        ? searchIndex
            .filter((item) =>
              terms.every((term) =>
                (item.title + " " + item.text)
                  .toLocaleLowerCase()
                  .includes(term),
              ),
            )
            .sort(
              (a, b) =>
                Number(b.title.toLocaleLowerCase().includes(q)) -
                Number(a.title.toLocaleLowerCase().includes(q)),
            )
        : searchIndex.slice(0, 6)
    ).slice(0, 16);
    $("search-count").textContent = q
      ? results.length
        ? "找到 " + results.length + " 篇相关文档"
        : "没有匹配的内容，试试“模型”“额度”或错误码。"
      : "常用文档";
    $("search-results").innerHTML = results
      .map((item) => {
        const pos = Math.max(
          0,
          item.text.toLocaleLowerCase().indexOf(terms[0] || "") - 30,
        );
        return `<a href="${documentURL(item.id)}"><small>${item.group}</small><strong>${escapeHTML(item.title)} <span>↗</span></strong><p>${escapeHTML(item.text.slice(pos, pos + 115))}…</p></a>`;
      })
      .join("");
  }
  const dialog = $("search-dialog");
  function openSearch() {
    if (!searchIndex.length) buildIndex();
    if (!dialog.open) dialog.showModal();
    search();
    $("search-input").focus();
  }
  $("open-search").addEventListener("click", openSearch);
  $("close-search").addEventListener("click", () => dialog.close());
  $("search-input").addEventListener("input", search);
  dialog.addEventListener("click", (e) => {
    if (e.target === dialog) dialog.close();
  });
  $("search-results").addEventListener("click", (e) => {
    if (e.target.closest("a")) {
      dialog.close();
      setTimeout(() => $("article").focus({ preventScroll: true }), 0);
    }
  });
  document.addEventListener("keydown", (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
      e.preventDefault();
      openSearch();
    }
    if (e.key === "Escape" && $("sidebar").classList.contains("open")) {
      closeMenu();
      $("menu-toggle").focus();
    }
  });
  $("menu-toggle").addEventListener("click", () => {
    const open = $("sidebar").classList.toggle("open");
    $("menu-toggle").setAttribute("aria-expanded", String(open));
    $("menu-toggle").setAttribute("aria-label", open ? "关闭目录" : "打开目录");
  });
  $("endpoint").value = state.baseURL;
  $("endpoint").addEventListener("change", (e) => {
    if (!endpointOptions.includes(e.target.value)) return;
    state.baseURL = e.target.value;
    savePreference("ppx-docs-endpoint", state.baseURL);
    render({ scroll: false });
    searchIndex = [];
    notify("教程地址已更新，请同步修改客户端配置");
  });
  document.addEventListener("click", (e) => {
    const button = e.target.closest("[data-dialog-copy]");
    if (button) copy(decodeURIComponent(button.dataset.dialogCopy), button);
    const os = e.target.closest("[data-os]");
    if (os) {
      state.os = os.dataset.os;
      savePreference("ppx-docs-os", state.os);
      render({ scroll: false });
      document.querySelector(`[data-os="${state.os}"]`)?.focus();
      searchIndex = [];
    }
    const link = e.target.closest("[data-scroll]");
    if (link) {
      e.preventDefault();
      history.replaceState(null, "", link.getAttribute("href"));
      const target = document.getElementById(link.dataset.scroll);
      if (target?.closest("details")) target.closest("details").open = true;
      target?.scrollIntoView({
        behavior: matchMedia("(prefers-reduced-motion: reduce)").matches
          ? "instant"
          : "smooth",
      });
    }
  });
  $("share-article").addEventListener("click", (e) =>
    copy(location.href, e.currentTarget),
  );
  function theme(value) {
    document.documentElement.dataset.theme = value;
    $("theme-toggle").setAttribute(
      "aria-label",
      value === "dark" ? "切换浅色主题" : "切换深色主题",
    );
  }
  const saved = readPreference("ppx-theme");
  theme(
    ["dark", "light"].includes(saved)
      ? saved
      : matchMedia("(prefers-color-scheme: dark)").matches
        ? "dark"
        : "light",
  );
  $("theme-toggle").addEventListener("click", () => {
    const next =
      document.documentElement.dataset.theme === "dark" ? "light" : "dark";
    theme(next);
    savePreference("ppx-theme", next);
  });
  window.addEventListener("hashchange", () => render({ focus: true }));
  render({ scroll: Boolean(location.hash) });
  // Full-text parsing is deferred until search is requested.
})();
