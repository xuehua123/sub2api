# 静态文档站：编辑、生成、验证

## 来源与产物

`template.html` 是外壳源文件。文章来源为 `articles.js`、`guides.js`、`legacy-content.js`、`troubleshooting.js`；`routes.js` 统一真实 URL 与旧 hash 兼容。不要直接编辑自动生成的各目录 `index.html`。

从仓库根目录执行（使用 frontend 已有的 jsdom 开发依赖，不增加线上运行时依赖）：

```powershell
node gateway/build-docs.cjs
node --test gateway/site-portals.test.cjs gateway/static-docs.test.cjs
node gateway/preview-sites.cjs
```

预览：`http://127.0.0.1:4179/docs-portal/` 和 `/home-portal/`。预览服务器仅监听 127.0.0.1，响应带 noindex；控制台操作链接仍打开真实公开控制台。

## SEO 与静态路径

- 28 篇文章生成独立 HTML，正文、目录和文章链接均无需 JavaScript 即可读取。
- 例如 `/guides/ccswitch/`、`/billing/`、`/codex-connection/`；旧 `#guide/ccswitch`、章节分享等继续兼容。
- 每页独立 title、description、canonical、OG/Twitter 元信息、TechArticle 结构化数据。
- 主文档域为 `https://doc.psydo.top/`，`doc.ppxcode.com` 镜像的 canonical 指向同一主文档域。
- `sitemap.xml` 只包含真实文章 URL，不含 hash；`robots.txt` 应部署到文档域根目录。
- `404.html` 为 noindex。`nginx-static.conf` 是 **server 块内**的配置示例，必须与现有配置审查合并，不能整体替换生产 Nginx。
- 首页仍按现有静态 `/home/index.html` canonical；Vue `/home` 的 iframe 外壳属于单独的主站 SEO 边界，不能将静态站 robots 文件直接覆盖主站根 robots。发布说明给出所需检查。
- SEO 技术检查不保证搜索排名或实际收录。Search Console、站长平台提交及线上 CWV 需发布后完成。

## 交互、内容与版本

端点、系统和主题仅保存公开偏好。搜索首次打开才建立全文索引；切换端点同步代码和控制台链接。真实 Key 不进入文档页面。静态 HTML 默认标准端点和 Windows 示例；无需 JS 时保留正文、导航，并隐藏不可工作的复制/搜索按钮。

CCSwitch 推荐走 API 密钥页面的「导入到 CCS」，设置后完全退出目标工具并重开。用户已明确不需要再次验证真实导入，本轮不重复执行。配图使用用户提供的皮皮虾截图；API 密钥按钮位置图仍明确标为示意，因尚未提供真实后台截图，不会伪造实拍图。

内容维护日期表示文档维护，不表示每个第三方客户端都已完成真机验证。Codex CLI 诊断和版本依赖见文章自身说明。UA 示例统一使用真实应用标识占位值，保留工具已有 UA，不冒充其他客户端。

## 发布

生成后完整发布 `docs-portal` 的公开文件，排除 `template.html` 和 README/config 源文件。普通静态服务器即可，无 Node 服务、数据库或 Redis 依赖。`build-docs.cjs` 和两组测试已接入现有 CI frontend job；CI 会检查已提交产物是否与源码一致。

生产发布仍遵循仓库 runbook；此次仅生成与打包，没有修改生产服务、DNS 或任何账户。上线前核对全部公开域名、登录后的目标地址保留、根 robots 和 Nginx 404/gzip 配置。保留上一版静态目录，可原子切回。
