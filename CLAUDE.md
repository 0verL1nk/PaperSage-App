# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**PaperSage** - Full-stack academic paper management app with LaTeX editing and compilation. React 19 + TypeScript frontend, Go (gin + GORM) backend shipped as a **single binary with the embedded frontend**, vendor Overleaf CLSI for LaTeX rendering.

## CI Status

| Workflow | Status |
|---|---|
| Server CI (gofmt + vet + golangci-lint + tidy 漂移 + go test -race -shuffle, coverage >=80% + govulncheck) | ![Server CI](https://github.com/0verL1nk/GraduateProject/actions/workflows/server-ci.yml/badge.svg) |
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
    ├── projects/       # Project list, writing page (LaTeX editor + PDF preview, file tree)
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

### Backend (Go — gin + GORM, single binary)

```
server/
├── cmd/
│   ├── papersage/     # 主程序入口（装配依赖 + 优雅停机）
│   └── seed/          # E2E/开发种子数据（幂等）
├── internal/
│   ├── config/        # env 配置（变量名与旧 Python 版 1:1）
│   ├── httpapi/       # gin 装配、{code,data,info} 信封、错误映射、CORS、SPA 静态服务
│   ├── auth/          # JWT 双 token + bcrypt + AuthRequired 中间件
│   ├── projects/      # 项目/分组/回收站域
│   ├── documents/     # 文档上传/文件夹域（camelCase DTO）
│   ├── latex/         # latex_files + CLSI 客户端 + 异步编译队列
│   ├── settings/      # 用户设置域（分类 JSONB + 默认值合并）
│   ├── store/         # GORM 模型 ×8 + golang-migrate（Alembic 交接）
│   ├── cache/         # Redis / 内存双实现
│   ├── storage/       # S3 客户端（aws-sdk-go-v2）
│   ├── dto/           # JSONTime（Python isoformat 字节级兼容）
│   └── testutil/      # 跨域契约测试装配
├── migrations/        # golang-migrate SQL（embed）
├── web/               # 前端构建产物（go:embed，构建时填充）
├── scripts/           # parity_check.py（双跑对比，迁移期工具）
└── Dockerfile         # 三阶段 → distroless 单镜像
```

**Key patterns:**
- gin + 按域分包（域内 handler+service；store 集中放模型防 import 环）
- 构造函数依赖注入（无全局单例）；测试用 testutil 内存替身
- JWE 加密双 token（A256KW+A256GCM，载荷不明文）；refresh 注册表落库持久化，真轮换（旧 token 用后即废）且重启不丢登录态
- API envelope: `{code, data, info}`；路由级 404/405 是裸 `{"detail": ...}`（对齐 FastAPI 实测行为）
- Form-based auth；双重口令哈希：前端 SHA-256 → 后端 bcrypt（存量用户可直接登录）
- 测试覆盖率门槛 ≥80%（server-ci 强制）
- 单二进制交付：`make build-server`；SPA fallback / assets immutable 缓存
- 迁移：golang-migrate + Alembic 交接（存量 PG 库启动时自动接管，schema 不变）

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
go test ./internal/... ./web/...   # 单元测试（含覆盖率）
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
make check              # gofmt/vet + typecheck + lint (all)
make test               # go test + vitest (all)
make build-server       # 单二进制（前端 embed）
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

- **Database**: `DATABASE_URL`（Go 驱动格式 `postgres://...`；旧 SQLAlchemy 格式 `postgresql+asyncpg://` 自动转换；SQLite 用 `sqlite:///./x.db`）
- **Redis**: `redis://localhost:6379/0` — falls back to in-memory
- **S3**: Cloudflare R2 (`S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`)
- **LaTeX**: `LATEX_CLSI_BASE_URL=http://localhost:3013`, `LATEX_CLSI_ENABLED`
- **Email**: Resend (`RESEND_API_KEY`) or SMTP fallback
- **Auth**: `SECRET_KEY`, `ACCESS_TOKEN_EXPIRE_MINUTES=30`, `REFRESH_TOKEN_EXPIRE_DAYS=7`

完整工程规范见 `docs/backend/GO-BEST-PRACTICES.md`（不造轮子选库清单、设计模式对照、覆盖率要求）。

## OpenSpec Workflow

This project uses OpenSpec for change management. Changes are defined in `openspec/changes/` (completed ones move to `openspec/changes/archive/`). Use the `/opsx:propose`, `/opsx:apply`, and `/opsx:archive` skills for managing changes.

Recent change archived: `2026-09-05-migrate-backend-to-go`（Python→Go 后端迁移，双跑验收后已删除 Python 侧）— 见 `openspec/changes/archive/`。

## Agent skills

### Issue tracker

Issues tracked via GitHub Issues using `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Uses default triage label vocabulary (needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix). See `docs/agents/triage-labels.md`.

### Domain docs

Project knowledge and agent workflows live under `docs/` (`docs/PROJECT.md` overview, `docs/project/` deep dives, `docs/agents/` agent recipes). See `docs/agents/domain.md`.
