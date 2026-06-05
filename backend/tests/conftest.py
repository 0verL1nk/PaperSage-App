"""Shared test fixtures."""

from __future__ import annotations

import uuid
from collections.abc import AsyncGenerator

import pytest
import pytest_asyncio
from httpx import ASGITransport, AsyncClient
from sqlalchemy.ext.asyncio import (
    AsyncSession,
    async_sessionmaker,
    create_async_engine,
)

from app.config import get_settings
from app.main import app
from core.base import Base
from models import User  # noqa: F401  — ensures UserSettings FK target is registered
from models.user import User as UserModel


@pytest.fixture(autouse=True)
def _mock_s3_settings(monkeypatch):
    """Provide valid S3 env vars so boto3.client() does not fail on init.

    CI has no real S3 credentials; the default placeholder endpoint
    'https://account_id.r2.cloudflarestorage.com' causes ValueError.
    """
    monkeypatch.setenv("S3_ENDPOINT", "http://localhost:9000")
    monkeypatch.setenv("S3_ACCESS_KEY", "test-access-key")
    monkeypatch.setenv("S3_SECRET_KEY", "test-secret-key")
    monkeypatch.setenv("S3_BUCKET", "test-bucket")
    monkeypatch.setenv("S3_REGION", "us-east-1")
    get_settings.cache_clear()


# ─── Integration-test fixtures (httpx + in-memory SQLite) ───


@pytest_asyncio.fixture
async def db_session() -> AsyncGenerator[AsyncSession, None]:
    """Per-test async session over a fresh in-memory SQLite database."""
    engine = create_async_engine(
        "sqlite+aiosqlite:///:memory:",
        connect_args={"check_same_thread": False},
    )
    factory = async_sessionmaker(engine, expire_on_commit=False)

    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)

    async with factory() as session:
        try:
            yield session
        finally:
            pass

    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)
    await engine.dispose()


@pytest.fixture
def authed_user() -> UserModel:
    """A plain User instance used to satisfy the get_current_user override.

    The override bypasses the DB lookup, so the user does not need to be
    persisted. The id is fixed so tests can assert on it.
    """
    return UserModel(
        id=uuid.UUID("11111111-1111-1111-1111-111111111111"),
        username="tester",
        password_hash="not-a-real-hash",
        is_active=True,
    )


@pytest_asyncio.fixture
async def client(
    db_session: AsyncSession, authed_user: UserModel
) -> AsyncGenerator[AsyncClient, None]:
    """httpx AsyncClient with get_db and get_current_user overridden.

    Lifespan is disabled so we don't run alembic migrations or connect
    to the cache/compile-queue subsystems during tests.
    """

    async def _override_get_db() -> AsyncGenerator[AsyncSession, None]:
        try:
            yield db_session
            await db_session.commit()
        except Exception:
            await db_session.rollback()
            raise

    async def _override_get_current_user() -> UserModel:
        return authed_user

    # Import here so the overrides match the function objects the routes
    # captured at import time.
    from api.dependencies import get_db
    from api.routes.auth import get_current_user

    app.dependency_overrides[get_db] = _override_get_db
    app.dependency_overrides[get_current_user] = _override_get_current_user

    # httpx 0.28 does not run ASGI lifespan events unless wrapped with
    # asgi-lifespan's LifespanManager, so we don't need an explicit opt-out
    # to avoid alembic migrations or cache/compile-queue init in tests.
    transport = ASGITransport(app=app)
    async with AsyncClient(
        transport=transport,
        base_url="http://test",
        headers={"Authorization": "Bearer test"},
    ) as ac:
        yield ac

    app.dependency_overrides.clear()
