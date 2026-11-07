# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**PaperSage** - Full-stack academic paper management app with LaTeX editing and compilation. React 19 + TypeScript frontend on Vercel, API-only Go (gin + GORM) backend shipped as a standalone binary. Browser WASM compilation is the default; the vendored Overleaf CLSI remains an optional external compiler.

## CI Status

| Workflow | Status |
|---|---|
| Server CI (gofmt + vet + golangci-lint + tidy 漂移 + go test -race -shuffle, coverage >=48% baseline + govulncheck) | ![Server CI](https://github.com/0verL1nk/GraduateProject/actions/workflows/server-ci.yml/badge.svg) |
| Frontend CI (typecheck + lint + vitest + dependency review) | ![Frontend CI](https://github.com/0verL1nk/GraduateProject/actions/workflows/frontend.yml/badge.svg) |
| E2E (Playwright) | ![E2E](https://github.com/0verL1nk/GraduateProject/actions/workflows/e2e.yml/badge.svg) |
| Docker Build & Push (SBOM + provenance) | ![Docker](https://github.com/0verL1nk/GraduateProject/actions/workflows/docker.yml/badge.svg) |
| Release (goreleaser, tag 触发) | ![Release](https://github.com/0verL1nk/GraduateProject/actions/workflows/release.yml/badge.svg) |
| PR Labeler（按路径打标） | — |

> **待开启**（Settings → Code security）：Code scanning（SAST）与 Dependency graph。
> 私有仓库需 GHAS；开启后把 `.github/disabled/codeql.yml` 移回 `workflows/`
> 并恢复 frontend.yml 的 dependency-review job（见 git 历史）。

## Architecture

### Frontend (React 19 + TypeScript)

```
frontend/src/
├── app/                # Router (router.tsx), providers, shell.tsx (main layout)
├── components/
│   ├── domain/         # Domain-specific components (ProjectCard, CreateProjectDialog)
│   ├── shared/         # Shared components
│   └── ui/             # shadcn/ui primitives
├── hooks/              # Custom React hooks (use-latex.ts, etc.)
├── lib/
│   ├── lezer-latex/    # Custom CodeMirror LaTeX grammar
│   ├── monaco-latex-lsp.ts  # Monaco editor LaTeX integration
│   └── pdf/            # PDF rendering utilities
├── routes/             # TanStack Router route tree (main/, project/)
├── services/           # API client layer (client.ts + domain APIs)
├── store/              # Zustand stores
├── types/              # Shared TypeScript types
└── views/              # Page-level view components
    ├── auth/           # Login/register views
    ├── layout/         # App shell, sidebar, navigation
    ├── projects/       # Project list, preparation/review workbench, LaTeX writing editor
    ├── settings/       # User settings (general / editor / agent)
    ├── trash/          # Trash/deleted items
    └── uploading/      # Reference upload module (folder tree, dropzone, data table)
```

**Key patterns:**
- Routing: TanStack Router with file-based conventions in `routes/`
- State: TanStack Query (server) + Zustand (client)
- Forms: React Hook Form + Zod validation
- HTTP: Axios with token refresh interceptor (`services/client.ts`)
- LaTeX editing: CodeMirror + Monaco editor integration
- Auth: JWT (`accessToken` + `refreshToken`) stored in localStorage
- Project workflow: `/project/:projectId/prepare`, `/writing`, `/review` plus the reference library at `/uploading`; old planned stage URLs redirect to preparation. Research brief, requirement notes, and user-maintained phase state use the owner-scoped `/projects/:id/writing-state` API with revision-based conflict handling.

### Backend (Go — gin + GORM, single binary)

```
server/
├── cmd/
│   ├── papersage/     # 主程序入口（装配依赖 + 优雅停机）
│   └── seed/          # E2E/开发种子数据（幂等）
├── internal/
│   ├── bootstrap/     # 装配：Build() 完成 域服务→路由→DAO 全局注入（cmd 薄壳化）
│   ├── config/        # env 配置（变量名与旧 Python 版 1:1）
│   ├── httpapi/       # gin 装配、{code,data,info} 信封、错误映射、CORS、版本与健康检查
│   ├── auth/          # JWT 双 token + bcrypt + AuthRequired 中间件
│   ├── projects/      # 项目/分组/回收站域
│   ├── documents/     # 文档上传/文件夹域（camelCase DTO）
│   ├── latex/         # latex_files + CLSI 客户端 + 异步编译队列
│   ├── settings/      # 用户设置域（分类 JSONB + 默认值合并）
│   ├── store/         # GORM 模型 + golang-migrate（Alembic 交接）+ query/（gen 生成 DAO）
│   ├── cache/         # Redis / 内存双实现
│   ├── storage/       # S3 客户端（aws-sdk-go-v2）
│   ├── dto/           # JSONTime（Python isoformat 字节级兼容）
│   └── testutil/      # 跨域契约测试装配
├── migrations/        # golang-migrate SQL（embed）
├── scripts/           # parity_check.py（双跑对比，迁移期工具）
└── Dockerfile         # 可选 API-only distroless 镜像；小服务器直接运行二进制
```

**Key patterns:**
- gin + 按域分包（域内 handler+service；store 集中放模型防 import 环）
- 装配集中在 internal/bootstrap.Build()；cmd/ 只有进程生命周期
- 删除语义**分层**（不再"全表软删除"）：用户资产（projects/groups/folders/documents/latex_files）与会话记录（agent_conversations/agent_messages）走软删；派生索引（rag_chunks/agent_wiki_pages）与基础设施（refresh_tokens/verification_tokens/system_settings/user_settings）走硬删——给它们挂 `deleted_at` 只会让"已删除"的判定悬空（模型未映射时查询会返回已删行、`Delete()` 退化为物理删除）
- 软删是 `UPDATE`，**不会触发出键的 `ON DELETE CASCADE` / `SET NULL`**：删除父级时必须显式处理子行（参见 `projects.deleteGroup`、`documents.deleteFolder`）
- 唯一键需要"删除后同名可重建"的表用 `store.SoftDeleteModel` + `deleted_seq`（活跃 0 / 删除为 unix 微秒），唯一键带上该列；只用 `gorm.DeletedAt` 的表会被已删行永久占位（`server/internal/store/softdelete.go`）
- 查询走 `.Model(...)` 而非 `.Table(...)`：后者没有挂模型，GORM 的软删过滤不会生效
- 业务时间戳一律由应用层以 UTC 写入；`gorm.Config.NowFunc` 已固定 UTC（`store.gormConfig`），禁止裸方言时间函数（如 `now()`，SQLite 无此函数且错误会被静默丢弃）
- `server/schema/tables/*.sql`、`server/migrations/{postgres,mysql}/`、Go 模型是同一 schema 的三份表述；`internal/store/schema_drift_test.go` 会逐表比对模型与 truth source 的列集合，漂移即测试失败
- CRUD 走 gorm.io/gen 生成的类型安全 DAO（internal/store/query；`make generate-dao`）
- SQL-first：schema/tables/*.sql 每表一份 DDL（真相源）；`make schema SCHEMA_DSN=...` 从真实库刷新；`make generate-dao SCHEMA_DSN=...` 从 SQL 建库生成 DAO
- 构造函数依赖注入（无全局单例）；测试用 testutil 内存替身
- JWE 加密双 token（A256KW+A256GCM，载荷不明文）；refresh 注册表落库持久化，真轮换（旧 token 用后即废）且重启不丢登录态
- API envelope: `{code, data, info}`；路由级 404/405 是裸 `{"detail": ...}`（对齐 FastAPI 实测行为）
- Form-based auth；双重口令哈希：前端 SHA-256 → 后端 bcrypt（存量用户可直接登录）
- 测试覆盖率回归门槛 ≥48%（server-ci 强制）；80% 是逐步提升的目标
- API-only 交付：`make build-server` 不构建或嵌入前端，未知非 API 路径也返回 JSON 404
- 迁移使用 `cmd/migrate` 独立执行；正式服务配置 `RUN_DB_MIGRATIONS=false`，不在请求启动路径改 schema
- **Agent 运行时（模式 B）**：一次提交 = Turn + Run；`POST /agent/conversations/:id/runs` 建 Run 并**立刻返回**，执行在独立 goroutine 里跑（`internal/agent/runexec.go`），客户端断开也照常跑完；`GET /agent/runs/:id/events` 订阅事件
- **会话要先显式建**：`POST /projects/:id/agent/conversations`。Run 路径要求会话已存在，面板在**第一条消息**时先建会话再发 Run——没有这个入口，新项目的第一句话会静默丢失
- **事件真相在 `agent_events`**（Run 内单调 sequence），进程内 hub 只做唤醒——多实例/重启不丢事件
- **SSE 序号与续传**：每条事件用标准 `id:` 携带 `agent_events.sequence`；`GET /agent/runs/:id/events?afterSeq=` 先补历史再跟随。断线只停止推送、不取消 Run；心跳不带 `id:`，避免污染续传位置
- **传输协议对齐 AI SDK UI Message Stream**（`internal/agent/protocol.go`，响应头 `x-vercel-ai-ui-message-stream: v1`）：part 词汇为 `start` / `text-*` / `tool-input-*` / `tool-output-*` / `data-*` / `finish` / `[DONE]`。工具事件带 `toolCallId`，**不再按同名工具配对**
- **消息由 Part 组成**（`agent_message_parts`），不再有 `content` / `tool_events` 两列；前端用 Normalized Store + Reducer 按 part id 幂等归并（`frontend/src/store/chat-store.ts`）
- **转录在 Run 终态重建**：从 `agent_events` + `agent_tool_calls` + `agent_change_proposals` 生成 text / tool / proposal / error parts；建议状态以建议表为准，子 agent 的 silent 事件不进入主转录
- **工具调用是可审计状态机**（`agent_tool_calls`）：普通工具 `executing → succeeded|failed`，需审批工具停在 `awaiting_approval`；带 `seq` 与 `idempotency_key`。所有工具必须在 `toolSpecs` 登记风险级别，未登记不得默认只读
- **取消/重试**：取消幂等，界面区分 `cancelling`；重试只允许 failed/cancelled，携带 `clientRequestId` 防重复计费。单实例部署启动时收敛上一进程遗留的 `running` Run（`ReconcileOrphanRuns`）；多实例不在当前范围
- **助手不直接改稿**：主 agent 无写盘工具，`propose_edit` 只产出建议；用户确认后由服务端**重新校验基准版本**再应用（`internal/agent/proposals.go`）。旧建议在稿件变化后转 `stale`，不得用旧基准覆盖新内容
- **Agent 基座用 Eino prebuilt `DeepAgent`**（`adk/prebuilt/deep`）：多 agent 是**声明式**的——子 agent 放进 `deep.Config.SubAgents`，由它生成 `task` 工具编排；**不要手工包 `adk.NewAgentTool`**，也不要自己解析工具参数去判断「哪个子 agent」——开 `ToolsConfig.EmitInternalEvents` 后 `AgentEvent.AgentName` 已经给出事件来源
- **多 Agent 拓扑**：主 agent 只挂领域工具；`search_references` 是声明的子 agent，调用时开子 Run（`parent_run_id`）形成 Run 树。子 agent 的事件写进**它自己的子 Run** 留档，**不计入主回答**——用户始终只面对一个 Assistant。子 agent 只回最终结论，其多轮检索过程不外泄到主上下文
- 不设 `Backend` / `Shell`（不注册文件系统与命令工具）；`AGENT_MAX_ITERATIONS` 是 Eino DeepAgent 的显式上限（默认 20，框架不接受 0 表示无限）
- 幂等：`clientRequestId`（Turn/Run）、`idempotency_key`（工具副作用）、`expectedThreadVersion` 乐观锁（会话版本）

### LaTeX Pipeline

The project vendors a modified Overleaf CLSI (Compile LaTeX Service Interface):
- `latex_render/` — Active CLSI fork with custom patches
- `vendor/overleaf/` — Original Overleaf source reference
- Backend `internal/latex/clsi.go` (ClsiClient interface) communicates with CLSI via HTTP
- CLSI runs on port 3013, backend on 5478, frontend on 5173

## Development Commands

### Quick Start

```bash
make install            # Install all deps (frontend + CLSI)
make dev                # Start all services with health checks and PID tracking
make stop               # Gracefully stop all services
make status             # Show running service status
make logs               # Tail all service logs
```

### Individual Services

```bash
make backend            # Go backend (sqlite dev db) on http://localhost:5478
make frontend           # Vite dev on http://localhost:5173
make clsi-dev           # CLSI LaTeX service on port 3013
```

### Frontend

```bash
cd frontend
pnpm dev                # Start dev server
pnpm build              # Production build
pnpm typecheck          # TypeScript check
pnpm lint               # ESLint
pnpm test               # Vitest (single run)
pnpm test -- foo        # Run tests matching "foo"
pnpm test:e2e           # Playwright (requires backend + MinIO)
```

### Backend (Go)

```bash
cd server
go test ./internal/...   # 单元测试（含覆盖率）
go test -tags=integration ./internal/store/ -run TestMigrateUp  # PG 迁移集成（需 TEST_PG_DSN）

# Code quality
gofmt -l .              # 格式检查（输出须为空）
go vet ./...
golangci-lint run       # CI 使用

# 数据库迁移（golang-migrate，SQL 在 server/migrations/）
# 启动时自动执行；新增迁移添加 NNNN_name.up.sql / .down.sql
```

### Combined

```bash
make check-backend      # gofmt + go vet + 编译检查
make check-frontend     # typecheck + lint
make test-backend       # go test（后端）
make test-frontend      # vitest（前端）
make build-server       # API-only 二进制（不构建或打包前端）
make seed               # 种子数据（e2e/dev，幂等）
make parity             # 双跑对比（迁移期验收工具）
```

## End-to-End (Playwright)

E2E tests live in `frontend/tests/e2e/` and are organised by domain (`auth/`, `projects/`, `uploading/`, `writing/`, `settings/`, `navigation/`, `error-states/`). The shared helpers in `helpers.ts` expose:

- `loginAs(page, user?, pass?)` — walk the real `/auth` form
- `logout(page)` — clear tokens + redirect
- `seedAuthTokens(page)` — fast in-memory login (no API call)
- `uploadFixture(page, name)` — `setInputFiles` against `tests/e2e/fixtures/{name}`
- `apiMock(page, pattern, status, body)` — uniform `page.route` factory
- `e2eEnv(name, fallback)` — read env vars exported by the seed script

### Local run

```bash
# 1. Bring up backend + MinIO + frontend
cd server && DATABASE_URL='sqlite:///./e2e-go.db' LATEX_CLSI_ENABLED=false go run ./cmd/papersage &
docker run -d --rm -p 9000:9000 -p 9001:9001 \
  -e MINIO_ROOT_USER=ci-access -e MINIO_ROOT_PASSWORD=ci-secret \
  minio/minio:latest server /data
docker run --rm --network host minio/mc:latest \
  sh -c "MC_HOST_minio=http://ci-access:ci-secret@localhost:9000 \
         mc mb --ignore-existing minio/papersage"

cd server && DATABASE_URL='sqlite:///./e2e-go.db' \
  LATEX_CLSI_ENABLED=false S3_ENDPOINT=http://localhost:9000 \
  S3_ACCESS_KEY=ci-access S3_SECRET_KEY=ci-secret S3_BUCKET=papersage \
  S3_REGION=us-east-1 EMAIL_PROVIDER=smtp SMTP_HOST=localhost \
  go run ./cmd/seed

cd frontend && pnpm dev --port 5173 &

# 2. Run the suite
cd frontend && pnpm test:e2e

# 3. Or a single spec
cd frontend && pnpm exec playwright test auth/auth-flow.spec.ts
```

`playwright.config.ts` auto-detects `CI=1` and switches to:
- `retries: 1`, `workers: 2`
- `reporter: ['list', 'github', 'html', 'json']`
- longer `expect.timeout`

### CI

`.github/workflows/e2e.yml` runs the same suite on every push to `main` and every PR. The job brings up the backend, frontend, and MinIO as service containers / background processes, seeds the e2e DB, then runs `pnpm test:e2e`. Artifacts (`playwright-report/`, `test-results/`, `backend.log`, `frontend.log`) are uploaded for 7 days.

## Configuration

Backend reads env vars via `server/internal/config`（变量名与旧 Python 版 1:1，PORT 默认 5478）:

- **Database**: `DATABASE_URL`（三方言：`postgres://...` / `mysql://...`（含 TiDB Cloud，建议 `?tls=true`；`tidb://` 等价）/ SQLite 开发用 `sqlite:///./x.db`；迁移按方言自动选择 `migrations/{postgres,mysql}/`）
- **Redis**: `redis://localhost:6379/0` — falls back to in-memory
- **S3**: Cloudflare R2 (`S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`)
- **LaTeX**: `LATEX_CLSI_BASE_URL=http://localhost:3013`, `LATEX_CLSI_ENABLED`
- **Agent（AI 助手）**: 优先用 per-user 设置里的 provider；服务端兜底用
  `AGENT_API_KEY` / `AGENT_BASE_URL` / `AGENT_MODEL`（OpenAI 兼容；DeepSeek 走 BaseURL）。
  嵌入用 `EMBED_API_KEY` / `EMBED_BASE_URL` / `EMBED_MODEL`，未配置则退回哈希嵌入（仅保证管线可用）。
  两者都未配置时，创建 Run 的请求返回 400 + 引导文案，面板展示「去设置」入口。
- **Email**: Resend (`RESEND_API_KEY`) or SMTP fallback
- **Auth**: `SECRET_KEY`, `ACCESS_TOKEN_EXPIRE_MINUTES=30`, `REFRESH_TOKEN_EXPIRE_DAYS=7`

完整工程规范见 `docs/backend/GO-BEST-PRACTICES.md`（不造轮子选库清单、设计模式对照、覆盖率要求）。

## OpenSpec Workflow

This project uses OpenSpec for change management. Changes are defined in `openspec/changes/` (completed ones move to `openspec/changes/archive/`). Use the `/opsx:propose`, `/opsx:apply`, and `/opsx:archive` skills for managing changes.

Recent change archived: `2026-09-05-migrate-backend-to-go`（Python→Go 后端迁移，双跑验收后已删除 Python 侧）— 见 `openspec/changes/archive/`。

## Agent skills

### Issue tracker

Issues tracked via GitHub Issues using `gh` CLI, organized by Milestone and (when the token has `project` scope) GitHub Project. Use `area:*`, `role:product|backend|frontend|test|ops`, and `priority:P0|P1|P2` labels. See `docs/agents/issue-tracker.md` and `docs/agents/delivery-workflow.md`.

### Delivery workflow

Product-style delivery: freeze scope in OpenSpec, mirror workstreams as GitHub Issues under a Milestone, run backend/frontend/test/ops in parallel with disjoint file ownership, then actually launch the app and walk the user journey before claiming completion. See `docs/agents/delivery-workflow.md`.

### Triage labels

Uses default triage label vocabulary (needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix). See `docs/agents/triage-labels.md`.

### Domain docs

Project knowledge and agent workflows live under `docs/` (`docs/PROJECT.md` overview, `docs/project/` deep dives, `docs/agents/` agent recipes). See `docs/agents/domain.md`.
