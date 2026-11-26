# 浏览器编译错误诊断（compile_error_reports）

浏览器 WASM 编译全程在用户本地执行，服务端原本看不到任何失败信息：游客编译完全不上报，
已登录用户只有未脱敏的 `compile_jobs.logs`（用户历史），管理员没有查询入口。
本能力补齐「失败 → 脱敏诊断落库 → 管理员可查可筛」的运维闭环。

## 边界：与 `compile_jobs` 的分工

| | `compile_jobs` | `compile_error_reports` |
|---|---|---|
| 用途 | 用户可见的编译历史 | 运维诊断 |
| 写入方 | `POST …/compile/browser`、`browser-failure` | `POST /api/v1/compile-diagnostics` |
| 可见性 | 项目所有者 | 仅 superuser |
| 内容 | 完整日志（≤2 MiB，现状未脱敏） | 白名单脱敏片段（≤32 KiB） |
| 生命周期 | 随项目删除；按历史条数裁剪 | 只追加，按保留期硬删 |

管理后台**只读新表**，不读 `compile_jobs.logs`；用户编译历史**只读 `compile_jobs`**。
两套数据不得互相代替。

## 隐私红线

- **正文不出浏览器**：客户端 `frontend/src/lib/latex/diagnostic-redaction.ts` 先脱敏，
  只保留 TeX 诊断行；服务端 `server/internal/compilediag/redact.go` 用同一套白名单
  再强制跑一遍（不信任客户端）。
- 丢弃：源码回显（`l.NN` 之后的内容只留行号）、`(./chapters/x.tex)` 文件展开、
  `\argument` 展开、正文、ANSI/控制字符。
- `log_excerpt` **只对 `failure_kind=latex` 写入**；资源/超时/服务类失败靠结构化字段
  （`failed_resources` + `error_message`）表达。
- 游客不落 IP；`client_env` 只反序列化固定键（UA / platform / language /
  hardware_concurrency / device_memory_gb / online），未知键自动丢弃。

## 上报接口

`POST /api/v1/compile-diagnostics`（公开，可选登录态，`auth.OptionalAuth`）

- 无 `Authorization` → 游客：`user_id/project_id/compile_job_id` 强制 NULL、`is_guest=true`，
  按 IP 限流（不落库 IP）。
- 有 `Authorization` 但无效/过期 → 401，**不静默降级为游客**。
- 有效 token：`user_id` 取 token；`project_id` / `compile_job_id` 校验归属，不属于本人则置 NULL
  并记 `slog.Warn`（不丢诊断，也不允许挂到别人的项目上）。
- 体积：`http.MaxBytesReader` 128 KiB，超限 413；`log_excerpt` ≤32 KiB、
  `error_message` ≤500 字符、`failed_resources` ≤20 条。
- 限流（进程内滑动窗口 `internal/ratelimit`；多实例部署需换 Redis）：
  - 登录用户：6 次/分钟、30 次/小时（按 user_id）
  - 游客：3 次/分钟、10 次/小时（按 IP）
  - 超限 429 `{"code":429,...,"info":"Too many requests"}`
- 枚举归一：`engine ∈ {pdflatex,xelatex}`（其他 → 422）、`failure_kind` 未知 → `unknown`、
  `failure_stage` 未知 → `compile`、`client_kind` 未知 → `web`。
- 成功：`{"code":200,"data":{"id":"<uuid>"},"info":""}`，id 供错误面板展示「诊断编号」。

## 管理员查询

全部挂 `auth.AdminRequired()`（superuser）：

- `GET /api/v1/admin/compile-diagnostics`
  - `page`（默认 1）、`page_size`（默认 20，上限 100）
  - `failure_kind`、`engine`、`is_guest`、`user_id`、`project_id`、`from`、`to`
    （RFC3339 或 `YYYY-MM-DD`；日期形式的上界含当天）
  - 非法枚举/UUID → 422
  - 列表不返回 `log_excerpt` / `client_env`
- `GET /api/v1/admin/compile-diagnostics/:id` → 全字段；不存在 → 404

查询用 `LEFT JOIN users / projects`，项目软删后仍能显示行（名称为空表示已删除）。

## 保留期

`COMPILE_DIAGNOSTIC_RETENTION_DAYS`（默认 30，clamp 1–365，见 `deploy/.env.example`）。
沿用「请求路径惰性清理」（与 `ExpireStaleRunningJobs` 同模式）：每次上报时若距上次清理
≥1 小时，同步 `DELETE … WHERE created_at < cutoff`，不引入定时器。

## 前端接入

- `compile-manager.ts` 在真实失败分支异步上报：不 await、失败静默、离线跳过；
  **用户主动取消不上报**。
- 错误面板展示 `diagnosticStatus`：上报中 / 已上报（编号）/ 上报失败（不影响编译，
  可复制日志反馈管理员）。
- 上报请求带 `skipGlobalLoading` + `skipAuthRefresh` + 10s 超时；`client.ts` 的 403
  不再清 token 跳登录（403 = 已登录但无权限，由管理页自行渲染提示）。

## 测试入口

- 后端：`cd server && go test ./internal/compilediag/ ./internal/ratelimit/ ./internal/auth/ ./internal/store -count=1`
  - 覆盖游客/登录落库、他人项目不挂链、超限 429、超体 413、白名单脱敏、
    仅管理员可查、筛选分页、详情 404、保留期清理、OptionalAuth
- 前端：`cd frontend && pnpm test -- diagnostic-redaction compile-manager client`
- E2E：`frontend/tests/e2e/writing/compile-diagnostics-report.spec.ts`（引擎桩）、
  `frontend/tests/e2e/admin/compile-diagnostics.spec.ts`（列表/筛选/详情/空/错误/403/320px）
- 迁移：`TEST_PG_DSN=… go test -tags=integration ./internal/store/ -run TestMigrateUp`、
  `TEST_MYSQL_DSN=… go test -tags=integration ./internal/store/ -run TestMySQLMigrate`、
  `go test ./internal/store -run TestSchemaDrift`
