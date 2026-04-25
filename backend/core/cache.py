"""Cache store abstraction with Redis and in-memory implementations."""

from __future__ import annotations

import asyncio
import time
from typing import Protocol, runtime_checkable

from app.config import get_settings

settings = get_settings()

# ─── Protocol ─────────────────────────────────────────────────


@runtime_checkable
class CacheStore(Protocol):
    """Interface for key-value cache with TTL support."""

    async def connect(self) -> None: ...
    async def disconnect(self) -> None: ...
    async def ping(self) -> bool: ...
    async def get(self, key: str) -> str | None: ...
    async def set(self, key: str, value: str, ex: int | None = None) -> None: ...
    async def delete(self, *keys: str) -> int: ...


# ─── In-memory implementation ────────────────────────────────


class MemoryCacheStore:
    """Thread-safe in-memory cache with TTL support.

    Uses lazy expiry: expired entries are cleaned up on access.
    Suitable for single-process development without Redis dependency.
    """

    def __init__(self) -> None:
        self._data: dict[str, tuple[str, float | None]] = {}
        self._lock = asyncio.Lock()

    async def connect(self) -> None:
        pass

    async def disconnect(self) -> None:
        async with self._lock:
            self._data.clear()

    async def ping(self) -> bool:
        return True

    async def get(self, key: str) -> str | None:
        async with self._lock:
            entry = self._data.get(key)
            if entry is None:
                return None
            value, expiry = entry
            if expiry is not None and time.monotonic() > expiry:
                del self._data[key]
                return None
            return value

    async def set(self, key: str, value: str, ex: int | None = None) -> None:
        async with self._lock:
            expiry = time.monotonic() + ex if ex else None
            self._data[key] = (value, expiry)

    async def delete(self, *keys: str) -> int:
        count = 0
        async with self._lock:
            for key in keys:
                if key in self._data:
                    del self._data[key]
                    count += 1
        return count


# ─── Redis implementation ────────────────────────────────────


class RedisCacheStore:
    """Redis-backed cache store using redis-py async client."""

    def __init__(self, url: str) -> None:
        self._url = url
        self._client: "redis.asyncio.Redis | None" = None

    async def connect(self) -> None:
        if self._client is None:
            from redis import asyncio as redis

            self._client = redis.from_url(
                self._url,
                encoding="utf-8",
                decode_responses=True,
                socket_connect_timeout=5,
                socket_timeout=5,
            )

    async def disconnect(self) -> None:
        if self._client:
            await self._client.close()
            self._client = None

    async def ping(self) -> bool:
        if self._client:
            return await self._client.ping()
        return False

    async def get(self, key: str) -> str | None:
        if self._client:
            return await self._client.get(key)
        return None

    async def set(self, key: str, value: str, ex: int | None = None) -> None:
        if self._client:
            await self._client.set(key, value, ex=ex)

    async def delete(self, *keys: str) -> int:
        if self._client:
            return await self._client.delete(*keys)
        return 0


# ─── Factory ─────────────────────────────────────────────────


def create_cache_store() -> CacheStore:
    """Create cache store based on configuration.

    If redis_url is set, use Redis. Otherwise, use in-memory store.
    """
    url = settings.redis_url
    if url:
        return RedisCacheStore(url)
    return MemoryCacheStore()


# Module-level singleton
cache_store = create_cache_store()


# ─── Convenience accessors ───────────────────────────────────


async def get_cache() -> CacheStore:
    """Get connected cache store (FastAPI dependency helper)."""
    await cache_store.connect()
    return cache_store


async def cache_get(key: str) -> str | None:
    return await cache_store.get(key)


async def cache_set(key: str, value: str, ttl: int | None = None) -> None:
    await cache_store.set(key, value, ex=ttl)


async def cache_delete(key: str) -> int:
    return await cache_store.delete(key)


# ─── Refresh token helpers ───────────────────────────────────

REFRESH_TOKEN_PREFIX = "refresh_token:"


async def store_refresh_token(token: str, user_id: str, ttl_seconds: int) -> None:
    key = f"{REFRESH_TOKEN_PREFIX}{token}"
    await cache_store.set(key, user_id, ex=ttl_seconds)


async def get_refresh_token_user(token: str) -> str | None:
    key = f"{REFRESH_TOKEN_PREFIX}{token}"
    return await cache_store.get(key)


async def revoke_refresh_token(token: str) -> int:
    key = f"{REFRESH_TOKEN_PREFIX}{token}"
    return await cache_store.delete(key)


async def is_refresh_token_valid(token: str) -> bool:
    return await get_refresh_token_user(token) is not None
