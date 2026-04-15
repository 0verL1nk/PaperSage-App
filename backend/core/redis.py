"""Redis connection and cache utilities."""

from typing import NotRequired, TypedDict

from redis import asyncio as redis

from app.config import get_settings

settings = get_settings()


class RedisClient:
    """Redis client wrapper."""

    def __init__(self) -> None:
        self._client: redis.Redis | None = None

    async def connect(self) -> None:
        """Connect to Redis."""
        if self._client is None:
            self._client = redis.from_url(
                settings.redis_url,
                encoding="utf-8",
                decode_responses=True,
            )

    async def disconnect(self) -> None:
        """Disconnect from Redis."""
        if self._client:
            await self._client.close()
            self._client = None

    async def ping(self) -> bool:
        """Check Redis connection."""
        if self._client is None:
            await self.connect()
        return await self._client.ping()  # type: ignore[union-attr]

    async def get(self, key: str) -> str | None:
        """Get value by key."""
        if self._client is None:
            await self.connect()
        return await self._client.get(key)  # type: ignore[union-attr]

    async def set(
        self,
        key: str,
        value: str,
        ex: int | None = None,
    ) -> bool:
        """Set key-value pair with optional expiry."""
        if self._client is None:
            await self.connect()
        return await self._client.set(key, value, ex=ex)  # type: ignore[union-attr]

    async def delete(self, *keys: str) -> int:
        """Delete keys."""
        if self._client is None:
            await self.connect()
        return await self._client.delete(*keys)  # type: ignore[union-attr]

    @property
    def client(self) -> redis.Redis:
        """Get raw Redis client."""
        if self._client is None:
            raise RuntimeError("Redis not connected. Call connect() first.")
        return self._client


redis_client = RedisClient()


async def get_redis() -> RedisClient:
    """Get Redis client dependency."""
    await redis_client.connect()
    return redis_client


class CacheOptions(TypedDict):
    """Cache options."""

    ttl: NotRequired[int]
    nx: NotRequired[bool]
    xx: NotRequired[bool]


async def cache_get(key: str) -> str | None:
    """Get cached value."""
    return await redis_client.get(key)


async def cache_set(
    key: str,
    value: str,
    ttl: int | None = None,
) -> bool:
    """Set cached value with optional TTL."""
    return await redis_client.set(key, value, ex=ttl)


async def cache_delete(key: str) -> int:
    """Delete cached value."""
    return await redis_client.delete(key)


# Refresh token storage keys
REFRESH_TOKEN_PREFIX = "refresh_token:"


async def store_refresh_token(token: str, user_id: str, ttl_seconds: int) -> bool:
    """Store refresh token in Redis with expiry."""
    key = f"{REFRESH_TOKEN_PREFIX}{token}"
    return await redis_client.set(key, user_id, ex=ttl_seconds)


async def get_refresh_token_user(token: str) -> str | None:
    """Get user_id for a refresh token."""
    key = f"{REFRESH_TOKEN_PREFIX}{token}"
    return await redis_client.get(key)


async def revoke_refresh_token(token: str) -> int:
    """Revoke a refresh token (add to blacklist)."""
    key = f"{REFRESH_TOKEN_PREFIX}{token}"
    return await redis_client.delete(key)


async def is_refresh_token_valid(token: str) -> bool:
    """Check if a refresh token is valid (exists in Redis)."""
    user_id = await get_refresh_token_user(token)
    return user_id is not None
