"""API dependencies."""

from collections.abc import AsyncGenerator

from sqlalchemy.ext.asyncio import AsyncSession

from core.database import async_session_factory
from core.redis import RedisClient


async def get_db() -> AsyncGenerator[AsyncSession, None]:
    """Get database session."""
    async with async_session_factory() as session:
        try:
            yield session
            await session.commit()
        except Exception:
            await session.rollback()
            raise


async def get_redis_dep() -> RedisClient:
    """Get Redis client."""
    from core.redis import redis_client

    await redis_client.connect()
    return redis_client
