# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**PaperSage** - Full-stack academic paper management app with LaTeX editing and compilation. React 19 + TypeScript frontend, Python FastAPI backend, vendor Overleaf CLSI for LaTeX rendering.

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
    ├── projects/       # Project list, writing page (LaTeX editor + PDF preview)
    └── trash/          # Trash/deleted items
```

**Key patterns:**
- Routing: TanStack Router with file-based conventions in `routes/`
- State: TanStack Query (server) + Zustand (client)
- Forms: React Hook Form + Zod validation
- HTTP: Axios with token refresh interceptor (`services/client.ts`)
- LaTeX editing: CodeMirror + Monaco editor integration
- Auth: JWT (`accessToken` + `refreshToken`) stored in localStorage

### Backend (Python FastAPI)

```
backend/
├── app/
│   ├── config.py       # Pydantic Settings (env-based configuration)
│   ├── exceptions.py   # Global exception handlers
│   └── main.py         # FastAPI app factory, CORS, lifespan, middleware
├── api/
│   ├── dependencies.py  # FastAPI Depends (get_current_user, get_db, etc.)
│   ├── response.py      # API response envelope: {code, data, info}
│   └── routes/          # auth, projects, documents, latex, latex_files
├── core/                # Database, security, token, cache, email
├── models/              # SQLAlchemy ORM (user, project, latex_file)
├── schemas/             # Pydantic request/response DTOs
├── services/
│   ├── latex/           # CLSI client, LaTeX compilation service, resource builder
│   └── storage/         # S3 client (Cloudflare R2)
├── tests/               # pytest + httpx async tests
└── main.py              # uvicorn entry point
```

**Key patterns:**
- Async SQLAlchemy 2.0 with repository-style service layer
- Dual entry: `main.py` (uvicorn CLI) → `app/main.py` (FastAPI app)
- Layered: Route → Service → Repository/Model
- JWT auth with refresh token rotation (`core/token.py`)
- API envelope: `{code: int, data: T, info: str}`
- Form-based auth (not JSON body) for login/register
- Double password hashing: Frontend SHA-256 → Backend bcrypt
- Cache: Redis (preferred) with in-memory fallback
- Storage: S3-compatible (Cloudflare R2) via boto3

### LaTeX Pipeline

The project vendors a modified Overleaf CLSI (Compile LaTeX Service Interface):
- `latex_render/` — Active CLSI fork with custom patches
- `vendor/overleaf/` — Original Overleaf source reference
- Backend `services/latex/clsi_client.py` communicates with CLSI via HTTP
- CLSI runs on port 3013, backend on 5478, frontend on 5173

## Development Commands

### Quick Start

```bash
make install            # Install all deps (backend + frontend + CLSI)
make dev                # Start all services with health checks and PID tracking
make stop               # Gracefully stop all services
make status             # Show running service status
make logs               # Tail all service logs
```

### Individual Services

```bash
make backend            # FastAPI on http://localhost:5478
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
```

### Backend

```bash
cd backend
uv sync                 # Install/sync dependencies
uv run uvicorn app.main:app --reload --port 5478

# Database migrations
alembic revision --autogenerate -m "description"
alembic upgrade head
alembic downgrade -1

# Code quality
ruff check .            # Lint
ruff check --fix .      # Auto-fix

# Testing
pytest                  # All tests
pytest -x               # Stop on first failure
pytest tests/test_auth.py  # Specific file
```

### Combined

```bash
make check              # Ruff + typecheck + lint (all)
make test               # pytest + vitest (all)
```

## Configuration

Backend uses environment-based config via `app/config.py` (Pydantic Settings). Copy `.env.example` to `.env`:

- **Database**: PostgreSQL (primary), SQLite (fallback), MySQL (optional)
- **Redis**: `redis://localhost:6379/0` — falls back to in-memory
- **S3**: Cloudflare R2 (`S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`)
- **LaTeX**: `LATEX_CLSI_BASE_URL=http://localhost:3013`, `LATEX_CLSI_ENABLED`
- **Email**: Resend (`RESEND_API_KEY`) or SMTP fallback
- **Auth**: `SECRET_KEY`, `ACCESS_TOKEN_EXPIRE_MINUTES=30`, `REFRESH_TOKEN_EXPIRE_DAYS=7`

## OpenSpec Workflow

This project uses OpenSpec for change management. Changes are defined in `openspec/changes/`. Use the `/opsx:propose`, `/opsx:apply`, and `/opsx:archive` skills for managing changes.
