# PaperSage Go 后端部署到 Vercel 调研

> 状态：Go API 与 Vite 前端均通过 Git 自动部署到 Vercel；API 生产别名为 `api.paper.overlink.top`，前端为 `paper.overlink.top`。曾因仓库级 `.vercelignore` 同时排除 `.git` 与 `frontend/`，导致 Go 取 VCS 信息失败、前端找不到 Vite；已移除根级忽略并分别在 `server/` 配置忽略规则、在 `frontend/vercel.json` 固定 pnpm 构建。核对日期：2026-10-08。

## 结论

PaperSage 的 Go + Gin API 使用 Vercel Go Framework Preset 构建即可。Vercel 根据 `go.mod` 和 `cmd/server/main.go` 执行 Go 构建并托管 HTTP 服务；不需要容器，也不需要把每条路由拆成单独 handler。Go Runtime 仍处于 Beta，运行实例遵循 Function 的弹性生命周期和执行时限，不能当作永久常驻服务器。

建议先用**独立 Vercel 后端项目**验证：连接同一 Git 仓库，将 Project Root 指向 `server/`，让 Go 服务先获得独立预览 URL；前端现有 `VITE_API_BASE_URL` 指向该 URL。正式稳定后可考虑绑定 `api.paper.overlink.top`，这是前端 `paper.overlink.top` 下独立的后端子域，不会占用 `api.overlink.top`。Vercel 会给出该域名要求的 DNS 记录；应在 Cloudflare 中只新增对应子域记录，不改动现有记录。验证通过后，再决定是否采用 Vercel Services 将前后端统一到同一域名。Services 与 Go Runtime 目前均为 Beta，首轮不建议把路由、构建和域名同时迁移。

## 与当前代码的匹配

| 项目 | 当前代码 | 对 Vercel 的影响 |
|---|---|---|
| Go 模块 | `server/go.mod`，声明 `go 1.26.6` | 将 Vercel 项目根设为 `server/` 才能在根目录找到 `go.mod`；部署时需验证 Beta Runtime 对该工具链版本的支持。 |
| 启动入口 | `server/cmd/papersage/main.go`，启动 Gin HTTP Server | 已新增 Vercel 可识别的 `cmd/server/main.go`，并抽出共享生命周期包，两个入口使用相同配置、装配与优雅停机逻辑。`server/vercel.json` 指定 Go Framework Preset；部署项目根目录仍须设为 `server/`。 |
| 监听端口 | `HOST` 默认 `0.0.0.0`，`PORT` 配置默认 5478 | Vercel 提供 `PORT`；需确保它覆盖默认值并监听该端口。 |
| 静态前端 | Go 二进制可嵌入 `server/web/assets`，前端另有 Vercel 项目 | API 独立部署时，建议构建为 API 服务，不依赖嵌入的 SPA 资源；现有 no-route SPA fallback 和前后端分离路由需要在预览环境确认。 |
| 数据库 | Go 进程启动时执行迁移；连接池上限和空闲数可通过环境变量配置，默认 10/2 | Functions 可扩出多个进程，连接池上限会随实例数放大。Neon 使用 Pooler；生产 schema 变更宜通过独立迁移命令执行，避免多个冷启动同时迁移。 |
| 对象存储 | S3 兼容存储由环境变量配置 | 保持使用外部持久对象存储；不要依赖函数本地文件作为持久数据。 |
| LaTeX 编译 | 当前仍有 CLSI 客户端、队列和启动恢复；`LATEX_CLSI_ENABLED=false` 可拒绝编译请求 | 后续计划转为浏览器编译后，可在后端部署首轮关闭该能力，确认前端不再依赖相关接口后再清理 CLSI/队列依赖。Vercel 不是必须承载 CLSI。 |
| 后台工作 | 文档确认后通过 goroutine 启动 RAG 索引；编译队列也由 goroutine 执行 | 请求返回后进程可能缩容或结束，后台任务不能依赖内存 goroutine 保证完成。首轮改为请求内执行且受限、持久化队列/外部 worker，或使用经验证兼容 Go 的持久任务系统。 |
| Agent 流式响应 | 当前 chat 在请求内执行 Agent 并返回 SSE | 要求单次请求在 Function 时限内结束；Hobby Fluid Compute 当前上限为 300 秒，超时会被终止。若后续需要长 Agent 任务，应改为持久 run + 查询/事件流及独立 worker。 |
| CORS | 当前 Gin 中间件允许任意 Origin | 前后端不同域时可以工作，但生产应基于正式前端域名收紧 CORS，并核对认证 Cookie/Token 与自定义域名策略。 |

数据库自动迁移尤其要拆分：当前 `store.Open()` 在服务启动时调用 `MigrateUp()`。上线策略应先在受控步骤执行一次迁移，再部署应用；同时确保应用启动逻辑不会让多个并发 Function 实例同时执行破坏性或长时间迁移。该改动应在部署实施任务中完成并测试。

## 部署形态对比

| 方案 | 做法 | 适合当前阶段 | 主要取舍 |
|---|---|---|---|
| 独立 Go 项目（当前方案） | Vercel Git Project Root=`server/`，Go Framework Preset，API 单独域名；前端配置 `VITE_API_BASE_URL` | 使用 Vercel 支持的 Go 构建流程部署 Gin HTTP 服务 | 需要跨域配置；Go Runtime Beta，进程与任务受 Function 生命周期约束。 |
| Vercel Services | 根 `vercel.json` 定义前端与后端 service，通过 `/api` rewrite 到后端、其他路径交给前端 | 两端都稳定后统一域名与部署 | Services Beta；统一路由后必须检查已有 SPA fallback、API 前缀、preview/prod 环境变量。 |
| 传统常驻 Go 服务 | 部署到支持常驻容器的应用平台 | Agent/队列要长期在后台运行时 | 不依赖 Function 请求时限，但多一项后端平台与运维配置。 |

Vercel Services 的 path 前缀是入口路由规则，服务收到的请求路径仍包含原路径；`/api/(.*)` 路由到后端时，后端路由应继续识别 `/api/...`。如需移除前缀，需在服务内另配路径转换，不能假定平台自动剥除。

## 建议实施顺序

1. **先在预览分支独立部署，不接生产数据库。** Vercel Project Root=`server/`，Framework Preset=`Go`。准备隔离的预览数据库和对象存储桶；不要把开发环境 `server/.env` 的生产凭据复制到 Preview 环境。
2. **解决运行安全问题。** 保证 `PORT` 生效；将启动迁移拆出部署步骤；把数据库池上限改成可配置且符合实例并发预算；关闭尚未切换的后端 LaTeX 编译。
3. **清理请求之外的 goroutine 假设。** 特别是文档索引；要么请求内完成并限制耗时，要么写入可恢复的持久任务队列。Agent SSE 设定明确超时并验证断线/取消行为。
4. **配置预览环境变量。** 至少核对 `DATABASE_URL`、`SECRET_KEY`、S3 endpoint/access/secret/bucket/public URL、邮件 Provider 凭据、`APP_URL`、模型配置，以及 `LATEX_CLSI_ENABLED=false`。只在 Vercel 项目环境设置密钥，不写入源码、构建日志或文档。
5. **接入前端 Preview。** 设置 `VITE_API_BASE_URL` 为 API Preview 地址并重建前端；验证登录/刷新 token、项目 CRUD、文件上传下载、邮件验证、Agent SSE 和健康检查。核实 Vite build-time 环境变量确实来自同一 Preview 环境。
6. **通过验证后绑定后端子域并评估同域 Services。** 在 Vercel 后端项目添加 `api.paper.overlink.top`，照 Vercel 给出的 DNS 目标在 Cloudflare 新增该主机记录；不要改写根域或现有 `api.overlink.top` 记录。生产数据库先单独备份并执行迁移，再更换 API 地址；监控 Function 终止、冷启动、数据库连接数、对象存储与邮件失败率。

## 本次调研与生产核对

Go Runtime 的 `cmd/server/main.go` 入口和共享启动包由 Vercel 根据 Go 模块自动构建。Vercel 项目 `papersage-api` 关联 GitHub 仓库 `0verL1nk/GraduateProject`，生产分支为 `main`，Root Directory 为 `server`，区域为 `sin1`，域名为 `api.paper.overlink.top`。2026-10-08 的最新生产部署使用 Go 1.26.6，Function 18.59 MB；`/health` 返回 200。修复部署配置后，`GET /api/v1/projects/{id}/writing-state` 在无认证请求下返回 401（已进入受保护路由），不再是 404；前端 `/auth` 与 `/` 均返回 200。

生产数据库此前缺少 `users` 等表；已执行 `go run ./cmd/migrate`，并关闭 Vercel Production 的启动自动迁移。Neon pooler 的 pgx prepared statement 冲突通过 simple protocol 规避。注册失败提示现在同时出现在注册面板；邮件发送错误、token 写库失败和重发失败都会返回 503，未配置的 `noop` sender 也会报错。Resend 查询到当天两封验证邮件的事件为 `delivered`，但这只证明收件方邮件服务器接受，不证明进入主收件箱；检查垃圾邮件或学校邮箱隔离区。邮件失败分支由 Go 测试覆盖，尚未通过真实生产 Provider 故障注入测试。客户端 LaTeX 编译仍是后续方向，后端仍保留 CLSI 集成。`.env` 与本地数据库/构建目录由各项目自身忽略规则排除。

2026-10-09 检查新项目文件时发现，Neon 中的项目与 `latex_files` 元数据正常，但生产函数的 `S3_ENDPOINT` 指向 `http://localhost:9002`，请求 R2 对象时失败，所以编辑器只显示空白内容。已用正式 `main.tex` 模板补齐该项目的 Cloudflare R2 对象与数据库元数据，并将 Vercel Production 的 `S3_ENDPOINT`、`S3_REGION`、`S3_BUCKET`、`S3_ACCESS_KEY`、`S3_SECRET_KEY` 更新为 `server/.env` 中已验证可读写 R2 的配置。随后已由 Git 触发生产重新部署，`api.paper.overlink.top/health` 返回 200；未带登录凭证访问文件接口返回预期的 401。当前浏览器没有登录会话，因此尚未从线上编辑器完成带认证的文件内容读取复验。

## 官方资料

- [Using the Go Runtime with Vercel Functions](https://vercel.com/docs/functions/runtimes/go)：Go HTTP Server/Gin、入口识别、`PORT`、Go 版本、构建命令。
- [Vercel Services](https://vercel.com/docs/services)：多服务部署、路由、Go 服务与容器服务。
- [Configuring Maximum Duration](https://vercel.com/docs/functions/configuring-functions/duration)：各计划的 Function 执行时长与终止语义。
- [Running Docker on Vercel](https://vercel.com/kb/guide/docker)：`Dockerfile.vercel`、容器 Function 生命周期和扩缩容。
