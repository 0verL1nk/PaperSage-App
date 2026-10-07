# 前端认证与密码找回

本文对应当前 React + Go 实现。旧版 FastAPI 路径和 MD5 示例不再是运行契约。

## 登录与注册

- 登录与邮箱注册沿用现有 `multipart/form-data` 请求。浏览器先对密码做 SHA-256，Go 后端再用 bcrypt 保存；登录和注册契约为兼容已有账号保留。
- 邮箱注册账号必须先验证邮箱，登录接口在密码正确后检查 `email_verified`。
- Access token 与 refresh token 保存在 session store 的 localStorage；API client 自动注入 access token，并在适用的 401 响应后刷新会话。
- 登录与注册共用一张表单卡，认证页以缓慢漂移的 Aurora 光色作为背景；系统开启减少动态效果时停用背景动画。游客入口在卡片内作为独立的免注册按钮，点击后原地准备浏览器存储并直接进入项目列表，不再要求在单独说明页二次点击。
- 登录页右上角和已登录工作台顶部都显示“后端 在线/本地”选择器。选择在线目标使用部署时注入的 API 地址，选择本地目标使用 `VITE_LOCAL_API_BASE_URL`（默认 `http://localhost:5478`）；切换后整页刷新以清理旧后端签发的会话和缓存。

认证入口的信息层级参考 [GitHub 登录页](https://github.com/login) 的账号/密码、找回密码和注册路径组织，以及 [Linear 登录方式说明](https://linear.app/docs/login-methods) 对登录选项的清晰分组。本项目仅展示已经实现的学校邮箱与密码登录，不展示未接入的第三方认证；游客和后端环境作为可发现的次级操作。

## 密码找回

1. 登录页的“忘记密码？”进入 `/auth/forgot-password`。前端提交邮箱到 `POST /api/v1/auth/password-reset/request`，无论邮箱是否存在，都显示相同的提示。
2. 只有活跃且已验证邮箱的账号会收到重置邮件。邮件使用一次性 256-bit 随机 token，有效期 15 分钟；数据库只保存 SHA-256 token hash。
3. 邮件链接将 token 放在 URL fragment（`/auth/reset-password#token=...`），不随页面请求发送到 Web 服务器。SPA 读取后立即移除地址栏 token；整个站点设置 `Referrer-Policy: no-referrer`。
4. `/auth/reset-password` 检查至少 8 位密码及重复输入，再提交到 `POST /api/v1/auth/password-reset/confirm`。密码仍使用现有 SHA-256 + bcrypt 存储兼容契约。
5. 后端在事务中单次消费 token、更新密码、作废此账号其他未用 token，并递增认证版本。旧 access token 立即失效，旧 refresh token 也不能续期；用户重新登录。系统会另发密码变更提醒邮件。

密码找回不向游客工作区开放。无效、过期和已消费的 token 均提示重新申请，不改变密码。页面在 320px 手机视口起支持纵向滚动，操作按钮不小于 44px。

## 邮件验证与接口

| 接口 | 方法 | 作用 |
|---|---|---|
| `/api/v1/auth/register` | POST 表单 | 注册并发送邮箱验证邮件 |
| `/api/v1/auth/verify-email?token=...` | GET | 消费邮箱验证 token |
| `/api/v1/auth/resend-verification` | POST JSON | 限速重发验证邮件 |
| `/api/v1/auth/password-reset/request` | POST JSON | 防枚举地申请密码重置 |
| `/api/v1/auth/password-reset/confirm` | POST JSON | 消费重置 token 并更新密码 |

验证邮件和密码重置共用 `verification_tokens`，通过 `purpose` 值隔离两种操作；无需另建 token 表。重置申请按进程内 IP 和邮箱限速。多实例部署时须将此限速器迁到 Redis 或边缘网关等共享组件。

## 相关代码和测试

- API 路由与 SHA-256：`frontend/src/services/api-routes.ts`、`frontend/src/services/auth-api.ts`
- 页面路由：`frontend/src/views/auth/auth-page.tsx`、`forgot-password-page.tsx`、`reset-password-page.tsx`
- 服务端实现：`server/internal/auth/email_flow.go`、`password_reset.go`
- 安全方案及迁移：`docs/backend/password-recovery.md`、`server/migrations/mysql/0016_email_password_recovery.up.sql`、`server/migrations/postgres/0016_email_password_recovery.up.sql`
- 自动化覆盖：`server/internal/auth/password_reset_test.go`、`frontend/src/views/auth/*password-page.test.tsx`、`frontend/tests/e2e/password-recovery.spec.ts`
