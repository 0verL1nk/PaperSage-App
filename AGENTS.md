# PROJECT KNOWLEDGE BASE

**Generated:** 2026-04-11
**Commit:** 43e7b51 feat: implement environment configuration and session management
**Branch:** (current)

## OVERVIEW

PaperSage - Full-stack academic paper management app. React 19 + TypeScript frontend (Vite), Python FastAPI backend.

## STRUCTURE
```
./
├── frontend/                    # React 19 + TypeScript
│   └── src/
│       ├── app/               # TanStack Router, shell.tsx (659 lines)
│       ├── features/         # auth, workspace, projects, uploading
│       ├── components/ui/   # shadcn/ui (19 components)
│       └── shared/           # api client, stores, hooks, config
├── backend/                    # Python FastAPI
│   ├── api/routes/            # auth, account, projects, documents
│   ├── core/                  # security, token, database, redis
│   ├── models/                # SQLAlchemy ORM
│   ├── schemas/               # Pydantic DTOs
│   └── app/                   # FastAPI app (non-standard layer)
├── docs/                       # Project docs
└── openspec/                   # Change management
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Frontend routing | `frontend/src/app/router.tsx` | TanStack Router |
| API client | `frontend/src/shared/api/client.ts` | Axios + interceptors |
| Auth flow | `frontend/src/features/auth/` | Login/register |
| Backend routes | `backend/api/routes/` | Auth, projects |
| JWT tokens | `backend/core/token.py` | Dual-token pattern |
| DB setup | `backend/core/database.py` | Async SQLAlchemy |

## CODE MAP
| Symbol | Type | Location | Refs | Role |
|--------|------|----------|------|------|
| `shell.tsx` | Component | `frontend/src/app/` | 659 lines | Main app shell |
| `client.ts` | API Client | `frontend/src/shared/api/` | All FE requests | Axios wrapper |
| `token.py` | JWT Module | `backend/core/` | auth routes | Token management |
| `security.py` | Auth Module | `backend/core/` | account routes | Password hashing |

## CONVENTIONS

- **Dual entry points**: Backend has both `/backend/main.py` and `/backend/app/main.py` (redundant)
- **Double password hash**: Frontend SHA-256 → Backend bcrypt
- **Form-based auth**: Backend uses `Form(...)` instead of JSON
- **API envelope**: `{code: 200, data: T, info: ""}`
- **Token types**: `access` (short) + `refresh` (long-lived) with `type` field

## ANTI-PATTERNS (THIS PROJECT)

- **`get_current_user` was a route-file dependency**: now lives in `backend/api/dependencies.py`; routes import from there, not from each other (2026-06 fix)
- **`get_db` is duplicated** in `backend/core/database.py` and `backend/api/dependencies.py` (identical bodies) — known cleanup target, currently harmless because no caller imports from `core.database`
- **Port mismatch**: Makefile has 8000, CLAUDE.md says 5478
- **Test = type check**: Makefile `test` target runs `ty check`, not pytest
- **`app/` package**: Extra layer not in standard layout

## UNIQUE STYLES

- **19 shadcn/ui components** in `frontend/src/components/ui`
- **Lazy Redis connection**: Connects on first operation
- **Conditional DB pool**: NullPool for SQLite, standard pool for PostgreSQL
- **Zod schemas**: `loginSchema`, `registerSchema` with pattern validation
- **OpenSpec workflow**: Changes in `openspec/changes/archive/`

## COMMANDS
```bash
# Frontend
cd frontend && pnpm dev      # http://localhost:5173
cd frontend && pnpm build

# Backend  
cd backend && uv run uvicorn main:app --reload --port 5478

# Test
cd frontend && pnpm test
cd backend && pytest
```

## NOTES

- No `.github/workflows` CI configured
- Only 1 file >300 lines: `shell.tsx` (659 lines)
- Project uses async SQLAlchemy 2.0, not sync
- JWT contains `type: "access"|"refresh"` to distinguish token type