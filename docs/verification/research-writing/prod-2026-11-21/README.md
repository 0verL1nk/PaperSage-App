# 生产复验（2026-11-21）

对应验收矩阵 8.4：从生产 Vercel 页面复验一次，确认前端/API 可用，并核对本次发布版本。

## 环境

- 前端：`https://<FRONTEND_ALIAS>`（307 → `https://<FRONTEND_DOMAIN>/`），Vercel 项目 `<VERCEL_PROJECT>`
- API：`https://<API_DOMAIN>`
- 代码：`main` @ `f57c689`，发布标签 `v0.1.12` → `b761386`（其后只有文档/规格提交）
- 前端最近一次生产构建：2026-11-21 12:54（CST），对应提交 `6afd918`；此后仅服务端/文档变更，前端产物未变

## 检查

| 检查 | 结果 |
|---|---|
| `GET https://<FRONTEND_ALIAS>/` | 307 → `https://<FRONTEND_DOMAIN>/`，最终 200 |
| 页面标题 | `PaperSage` |
| `GET https://<API_DOMAIN>/health` | 200 `{"status":"healthy"}`（公网经代理，按设计不返回 version/commit） |
| 匿名 `GET /api/v1/projects` | 401 `{"code":401,"data":null,"info":"Not authenticated"}` |
| 服务端发布 | Release `v0.1.12` 部署成功，部署日志：`active: v0.1.12 b76138667c06f80cffcaa0342a9301c75b7c222f`；部署后 `/health` 仍返回 200 |

## 局限

- 未在生产页面走登录后的完整对话旅程；真实模型对话证据见 `local-2026-11-15/` 与 `prod-2026-11-08/`。
- 公网 `/health` 不暴露版本，前端/API 版本匹配以本次发布标签与 Vercel 部署时间核对，未读取登录后“关于”页。
