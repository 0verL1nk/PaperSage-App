# backend/core

## OVERVIEW

Low-level security, token, database, and Redis utilities. Exports bcrypt hashing, dual-token JWT pattern, async SQLAlchemy, and lazy Redis connection.

## WHERE TO LOOK

| Symbol | Type | Location | Role |
|--------|------|----------|------|
| `hash_password` | Function | `security.py` | bcrypt password hashing |
| `verify_password` | Function | `security.py` | password verification |
| `create_access_token` | Function | `token.py` | JWT access token (short-lived) |
| `create_refresh_token` | Function | `token.py` | JWT refresh token (long-lived) |
| `verify_token` | Function | `token.py` | token verification with type check |
| ~~`get_db`~~ | ~~Generator~~ | ~~`database.py`~~ | Moved to `api/dependencies.py` (2026-06); `database.py` now only owns the engine and session factory |
| `get_redis` | Function | `redis.py` | FastAPI dependency for Redis |
| `Base` | Class | `base.py` | SQLAlchemy DeclarativeBase |
| `TimestampMixin` | Mixin | `base.py` | created_at/updated_at fields |
| `UUIDMixin` | Mixin | `base.py` | UUID primary key |

## KEY PATTERNS

### Dual-Token JWT

Access and refresh tokens carry a `type` field to distinguish them. Verify rejects invalid types.

```python
payload = jwt.decode(token, settings.secret_key)
if payload.get("type") != token_type:
    raise JWTError("Invalid token type")
```

### Lazy Redis Connection

Redis connects on first operation, not at import. Use `get_redis()` as FastAPI dependency.

```python
async def ping(self) -> bool:
    if self._client is None:
        await self.connect()
    return await self._client.ping()
```

### Conditional Database Pool

SQLite uses NullPool (no connection sharing). PostgreSQL uses standard pool with 20 size + 30 overflow.

```python
is_sqlite = settings.database_url.startswith("sqlite")
if is_sqlite:
    engine_kwargs["poolclass"] = NullPool
else:
    engine_kwargs.update({"pool_size": 20, "max_overflow": 30})
```

## ANTI-PATTERNS

- **Silently swallow verify_password returns False**: Returns `False` on exception, never raises
- **No get_current_user here**: Defined in `api/routes/auth.py` and `api/routes/account.py`, not core
- **Redis client is singleton**: `redis_client` module-level instance, shared across requests