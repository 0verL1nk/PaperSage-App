# 将后端从 Python/FastAPI 迁移到 Go，并实现单二进制交付

## Why

PaperSage 目前的部署形态是「Python uvicorn 后端 + Vite 构建前端 + Nginx 托管静态资源」三个部件，演示和分发都要拖着一整套 Python 运行时环境。作为毕业设计交付物，希望收敛为**一个 Go 二进制文件**：前端构建产物通过 `go:embed` 打进二进制，启动该文件即同时提供前端页面与后端 API，无需安装任何语言运行时。

同时，项目路线图中的 AI 辅助模块（v0.5）需要 Agent 基座。Go 生态在 2026 年已有成熟选择（本提案调研结论为 CloudWeGo Eino，见 design.md D8），将后端统一到 Go 后，AI 编排、HTTP 服务、编译队列可以共用一套语言栈与部署产物。

现有 Python 后端规模可控（约 6,100 行实现代码 + 2,750 行测试，7 组路由 49 个端点，8 张表），且行为已由 Playwright E2E 套件锁定，具备一次性等价移植（behavior-parity port）的条件。

## What Changes

- 新增 `server/` Go 模块：chi 路由 + GORM（pgx 驱动）+ golang-migrate，**1:1 等价移植**现有 FastAPI 后端的全部路由、服务层逻辑与数据模型
- 前端 `pnpm build` 产物（`frontend/dist`）构建时复制到 `server/web/` 并 `go:embed` 进二进制；Go 侧新增 SPA 静态服务（index.html 回退、/assets 长缓存）
- 兼容性承诺：API 路径与请求/响应格式（`{code, data, info}` 信封）、表单式登录、双重口令哈希（前端 SHA-256 → 后端 bcrypt）、JWT 双 token（同一 `SECRET_KEY`，旧 token 在新实现下仍然有效）全部保持不变
- CLSI（`latex_render/`，Node 服务）、PostgreSQL、Redis、S3 存储保持外部独立服务，不在本次迁移范围内
- 采用**并行开发、末端切换**策略：`server/` 与 `backend/` 并存直至 Playwright E2E 对 Go 二进制全绿，然后删除 `backend/`、切换 Makefile / CI 工作流 / docker-compose
- 确定后续 AI 模块的 Agent 基座选型为 Eino（本次只做选型决策，不实现 agent 功能）

## Capabilities

### New Capabilities

- `single-binary-delivery`: 单二进制交付 — Go 二进制内嵌前端产物，单进程同源提供 SPA 与 `/api/v1` API，含静态资源缓存策略与既有行为兼容验收

### Modified Capabilities

- 无。现有 capabilities（async-compile-queue、user-settings、latex-source-storage 等）的行为规格不变，本次仅更换实现语言；等价性由 E2E 套件回归保证。

## Impact

- **新增代码**：`server/`（Go 模块：cmd、internal、migrations、web）约等于现有 backend 的等价规模
- **最终删除**：`backend/`（Python）、`frontend/Dockerfile`（Nginx 镜像）、CI 的 backend 工作流
- **CI**：新增 `server-ci.yml`（gofmt / go vet / golangci-lint / go test）；`docker.yml` 改为单镜像多阶段构建（pnpm build → go build with embed → distroless）；`e2e.yml` 改为启动单个二进制
- **部署**：`deploy/docker-compose.yml` 中 backend + frontend 两个服务合并为一个 `papersage` 服务；clsi / db / redis 保留
- **前端**：构建产物与 dev 模式均不变（dev 仍用 Vite 5173 + `VITE_API_BASE_URL` 指向 5478；单二进制模式下前端本就默认同源相对路径）
- **数据库**：schema 不变；迁移管理从 Alembic 切换到 golang-migrate（baseline 对接，见 design.md D3）
