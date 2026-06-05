# `backend/api/`

FastAPI app surface: route registration, dependencies, response envelope.

## OVERVIEW

```
backend/api/
├── __init__.py        # api_router assembly — single import point for app.main
├── dependencies.py    # FastAPI Depends providers (get_db, get_cache_dep, get_current_user, security)
├── response.py        # ApiResponse envelope + api_success / error_response helpers
└── routes/            # One file per domain: auth, projects, documents, folders, latex, latex_files, settings
```

## WHERE TO LOOK

| Symbol | Type | Location | Role |
|--------|------|----------|------|
| `api_router` | APIRouter | `__init__.py` | Mounts every domain router; mounted once in `app/main.py` at `/api/v1` |
| `get_db` | AsyncGenerator | `dependencies.py` | Yields an `AsyncSession`, commits on success, rolls back on exception |
| `get_cache_dep` | Coroutine | `dependencies.py` | Returns the singleton `CacheStore` (Redis or in-memory fallback) |
| `get_current_user` | Coroutine | `dependencies.py` | Resolves a `User` from `Authorization: Bearer <accessToken>`; 401 on any failure |
| `security` | HTTPBearer | `dependencies.py` | The bearer-token extractor used by `get_current_user` |
| `ApiResponse` / `api_success` | Pydantic + helper | `response.py` | Standard `{code, data, info}` envelope; every route returns one |

## KEY PATTERNS

### Shared dependencies live in `dependencies.py`, not in route files

Routes import shared FastAPI dependencies from `api.dependencies`, **never** from other route files. Historical violation: `get_current_user` was defined in `api/routes/auth.py` and re-imported by 6 other route files. Fixed 2026-06 — see ANTI-PATTERNS below for the full migration.

Rationale:
- One canonical home for cross-route concerns
- Override target for tests is a single function object (`app.dependency_overrides[get_current_user] = ...`)
- Adding a new shared dep doesn't touch any route file (just `dependencies.py` + the files that need it)

### Response envelope — every route returns `ApiResponse[T]`

```python
from api.response import ApiResponse, api_success

@router.get("/items", response_model=ApiResponse[list[ItemResponse]])
async def list_items() -> ApiResponse[list[ItemResponse]]:
    return api_success([ItemResponse(...)])
```

Errors are **not** returned as `ApiResponse` with non-200 `code`; they raise `HTTPException`. The global handlers in `app/exceptions.py` convert `HTTPException` and `RequestValidationError` into the envelope shape (see `docs/backend/RELIABILITY.md`).

### Auth in protected routes

```python
from api.dependencies import get_current_user
from models import User

@router.post("/items")
async def create_item(
    body: ItemCreate,
    current_user: User = Depends(get_current_user),  # noqa: B008
) -> ApiResponse[ItemResponse]:
    ...
```

`get_current_user` handles all 401 paths: missing header, invalid token, bad `sub`, user not found, user inactive. Routes don't need to repeat this.

## CONVENTIONS

- New shared dep → add to `dependencies.py` with full type hints; do not put FastAPI `Depends` providers inside route files
- New domain → create `routes/<domain>.py`; register in `__init__.py`; pick a clear `prefix` and `tags`
- New route → `response_model=ApiResponse[T]`; return `api_success(data)`; raise `HTTPException` for errors
- Don't import across route files; if two routes need a shared helper, move it to `dependencies.py` or a new module
- Test override target is always the function in `dependencies.py`, not the route's local variable

## ANTI-PATTERNS

- ❌ **Reaching into other route files for shared code**. If you find yourself writing `from api.routes.X import helper`, the helper belongs in `dependencies.py`
- ❌ **Defining FastAPI dependencies in route files**. This was the original `get_current_user` smell. If a `Depends` is used by more than one route file, it must live in `dependencies.py`
- ❌ **Bypassing the envelope**. Don't `return {"foo": bar}` from a route — use `api_success(...)`. Otherwise the global error handlers and frontend envelope parsing break
- ❌ **`get_db` is duplicated** in `core/database.py` and `dependencies.py` (identical bodies). No caller imports the `core.database` version, so this is currently harmless, but a future cleanup target
- ❌ **Inline JWT verification in route handlers**. `get_current_user` already does this; the `/auth/refresh` route inlines it only because it uses a *refresh* token instead of an access token — that's a legitimate exception, not a pattern to copy

## TESTING

### Unit tests

`tests/test_<domain>_api.py` covers: schema import, route signature, `Depends` wiring, `api_router.routes` registration. See `tests/test_settings_api.py` for a reference structure.

### Integration tests (httpx + ASGI)

See `docs/project/09-testing.md` §3.5 for the full pattern. Key points for `api.dependencies` work:
- Override `api.dependencies.get_db` and `api.dependencies.get_current_user` in the `client` fixture
- The override key must be the **same function object** that the route `Depends` references — never alias or re-define
- To exercise 401 in a single test, clear just the `get_current_user` override and reuse the `db_session` fixture
