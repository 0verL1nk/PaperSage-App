# Capability: single-binary-delivery

## ADDED Requirements

### Requirement: 单进程同源交付
系统 SHALL 以单个 Go 二进制文件交付，内嵌前端构建产物，启动后在一个端口上同时提供 `/api/v1` API 与前端 SPA 页面，不依赖 Python 运行时或独立静态资源服务器。

#### Scenario: 启动二进制即得完整站点
- **WHEN** 在仅有 PostgreSQL/Redis/S3/CLSI 外部依赖的环境下启动该二进制，浏览器访问根路径 `/`
- **THEN** 返回前端 SPA 的 `index.html`，页面加载后可在同源完成登录并使用全部功能

#### Scenario: API 与静态资源同源共存
- **WHEN** 请求 `/api/v1/auth/login` 与 `/assets/<hash>.js`
- **THEN** 前者命中 API 路由并返回 `{code, data, info}` 信封，后者从内嵌文件系统返回静态资源

### Requirement: SPA 路由回退
系统 SHALL 对所有非 API、非已存在静态文件的路径返回 `index.html`（SPA fallback），使 TanStack Router 的 history 模式深链接可直接刷新访问。

#### Scenario: 深链接刷新
- **WHEN** 用户停留在 `/app/projects` 前端路由后刷新页面
- **THEN** 服务端返回 `index.html`（HTTP 200），前端路由恢复该页面，而非 404

### Requirement: 静态资源缓存策略
系统 SHALL 对内容寻址的静态资产设置长缓存，对入口文档设置禁缓存。

#### Scenario: 带 hash 的资产长缓存
- **WHEN** 请求 `/assets/` 下任何文件
- **THEN** 响应含 `Cache-Control: public, max-age=31536000, immutable`

#### Scenario: 入口文档不缓存
- **WHEN** 请求 `index.html`
- **THEN** 响应含 `Cache-Control: no-cache`，保证新版本发布后入口指向新的资产 hash

### Requirement: 既有行为等价
Go 实现 SHALL 与被替换的 Python 实现行为等价：API 路径、请求/响应格式、状态码与错误信封逐字段一致；同一 `SECRET_KEY` 下既有 JWT 在新实现中仍有效；数据库 schema 零变更。

#### Scenario: E2E 套件回归
- **WHEN** 对 Go 二进制运行既有 Playwright E2E 全套件（不做任何用例修改）
- **THEN** 全部用例通过，作为删除 Python 后端的前置门槛

#### Scenario: 存量用户无感切换
- **WHEN** 迁移前签发的有效 accessToken 被旧前端持有者在切换后用于调用 API
- **THEN** Go 侧校验通过，请求正常处理，用户无需重新登录

#### Scenario: 数据库平滑接管
- **WHEN** Go 二进制首次启动并接管一个由 Alembic 管理的既有数据库
- **THEN** 迁移交接完成后应用正常服务，既有数据完整可读
