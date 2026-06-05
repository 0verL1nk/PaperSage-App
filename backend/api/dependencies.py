"""API dependencies — FastAPI Depends providers.

This module is the single home for cross-route FastAPI dependencies. Routes
import from here; they do NOT reach into other route files for shared
dependencies (see `backend/api/AGENTS.md` for the rationale).
"""

from __future__ import annotations

import uuid
from collections.abc import AsyncGenerator

from fastapi import Depends, HTTPException, status
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from core.cache import CacheStore, get_cache
from core.database import async_session_factory
from core.token import verify_token
from models import User

security = HTTPBearer(auto_error=False)


async def get_db() -> AsyncGenerator[AsyncSession, None]:
    """Get database session."""
    async with async_session_factory() as session:
        try:
            yield session
            await session.commit()
        except Exception:
            await session.rollback()
            raise


async def get_cache_dep() -> CacheStore:
    """Get cache store."""
    return await get_cache()


async def get_current_user(
    credentials: HTTPAuthorizationCredentials | None = Depends(security),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> User:
    """Resolve the authenticated user from a bearer access token.

    Raises 401 for:
    - missing Authorization header
    - invalid/expired access token
    - token's `sub` not a UUID
    - user not found or marked inactive

    Use as `current_user: User = Depends(get_current_user)` in protected
    route signatures. See `backend/api/AGENTS.md` for the override pattern
    used in tests.
    """
    if not credentials:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Not authenticated",
        )

    try:
        payload = verify_token(credentials.credentials, "access")
        user_id: str = payload.get("sub")
        if not user_id:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Invalid token",
            )
    except ValueError:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid token",
        ) from None

    # Convert string to UUID for database query
    user_uuid = uuid.UUID(user_id)
    result = await db.execute(select(User).where(User.id == user_uuid))
    user = result.scalar_one_or_none()

    if not user or not user.is_active:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="User not found or inactive",
        )

    return user
