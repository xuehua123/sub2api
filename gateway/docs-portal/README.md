# 皮皮虾 AI 静态文档站

`index.html` 是无构建依赖的静态站入口，适合部署在 `doc.psydo.top` 和 `doc.ppxcode.com`。

## 部署边界

- 仅部署静态文件，不连接 Sub2API 数据库、Redis 或内部管理 API。
- 文档中的端点选择器仅改变教程内显示的 Base URL；不会改动用户账户、密钥、余额、套餐或分组。
- 发布前确认公开端点仍为：`https://api.psydo.top`、`https://api.ppx-ai.com`、`https://us.psydo.top`、`https://cn2.ppxcode.com`、`https://cf.ppxcode.com`。

## 本地预览

在本目录运行任意静态 HTTP 服务器，例如：

```powershell
python -m http.server 4173
```

然后打开 `http://127.0.0.1:4173`。
