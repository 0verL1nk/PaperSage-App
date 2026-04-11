# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**PaperSage** - A full-stack academic paper management application with a React 19 + TypeScript frontend and Python FastAPI backend.

## Architecture

### Frontend (React 19 + TypeScript)

Feature-based architecture with TanStack Router for routing.

```
frontend/src/
├── app/              # Router, providers, app-level config
├── features/         # Feature modules (auth, layout, workspace, etc.)
├── components/ui/    # shadcn/ui components
├── routes/           # TanStack Router route definitions
└── shared/           # Utilities, stores, hooks
```

**Key Patterns:**
- Routing: TanStack Router with file-based conventions
- State: TanStack Query (server) + Zustand (client)
- Forms: React Hook Form + Zod
- Auth: JWT in localStorage (`accessToken` + `refreshToken`)

### Backend (Python FastAPI)

Clean architecture with dependency injection and repository pattern.

```
backend/
├── api/routes/       # FastAPI route handlers
├── core/             # Config, security, database
├── models/           # SQLAlchemy ORM models
├── schemas/          # Pydantic DTOs
└── alembic/          # Database migrations
```

**Key Patterns:**
- Async SQLAlchemy 2.0 with repository pattern
- JWT auth with refresh token rotation
- Layered architecture: API → Service → Repository → Model

## Development Commands

### Frontend

```bash
cd frontend

# Development
pnpm dev              # Start dev server (Vite)
pnpm build            # Production build
pnpm preview          # Preview production build

# Code Quality
pnpm lint             # ESLint
pnpm typecheck        # TypeScript compiler check

# Testing
pnpm test             # Run tests once (Vitest)
pnpm test:watch       # Run tests in watch mode
pnpm test -- foo      # Run single test file matching "foo"
```

### Backend

```bash
cd backend

# Environment Setup
uv sync               # Install/sync dependencies
source .venv/bin/activate  # Activate virtualenv

# Development
uv run uvicorn main:app --reload --port 5478

# Database
alembic revision --autogenerate -m "description"
alembic upgrade head
alembic downgrade -1

# Code Quality
ruff check .          # Linting
ruff check --fix .    # Fix auto-fixable issues

# Testing
pytest                # Run all tests
pytest -x             # Stop on first failure
pytest -v             # Verbose output
pytest tests/test_auth.py  # Run specific test file
```

## API Configuration

- **Frontend dev server**: http://localhost:5173
- **Backend API**: http://localhost:5478
- **API Base Path**: `/api/v1`
- **CORS**: Configured for localhost:5173 in dev

## Auth Flow

1. Register/Login → receive `accessToken` + `refreshToken`
2. Store both in localStorage
3. Attach `accessToken` to all API requests via `Authorization: Bearer <token>`
4. On 401, use `refreshToken` to get new `accessToken`
5. On refresh failure, redirect to login

## Key Dependencies

**Frontend:**
- React 19 + React Compiler
- TanStack Router + Query
- Zustand
- Tailwind CSS 4 + shadcn/ui
- Vite 8
- Vitest

**Backend:**
- FastAPI
- SQLAlchemy 2.0 (async)
- Pydantic 2
- Alembic
- PostgreSQL + Redis
- PyJWT

## OpenSpec Workflow

This project uses OpenSpec for change management. Changes are defined in `openspec/changes/`. Use the `/opsx:propose`, `/opsx:apply`, and `/opsx:archive` skills for managing changes.
