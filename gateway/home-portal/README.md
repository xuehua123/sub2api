# 首页静态产物

`index.html`、`home.css`、`home.js`、`theme-init.js` 和 `assets/` 构成独立主页。`theme-init.js` 在绘制前恢复外观，避免闪烁。主页静态链接直接指向真实文档文章路径；JS 仅做入口域名匹配和交互。

主页保留现有 `/home/index.html` 的 canonical；Open Graph 分享图与 WebSite 结构化数据已补齐。用户访问的 Vue `/home` 仍可能是 iframe 外壳，主站服务层的路径统一、根 robots 和站长平台验证需正式发布时处理，不应将 `/home/robots.txt` 当成根 robots。

本地运行和测试见相邻 `docs-portal/README.md`。此版本不包含虚构延迟、成功率、SLA 或用户数量。按用户要求，未重新执行 CCSwitch 的真实导入。
