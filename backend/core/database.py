"""Database connection and session management."""

from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine
from sqlalchemy.pool import NullPool

from app.config import get_settings

settings = get_settings()

# SQLite 不支持连接池，使用 NullPool
is_sqlite = settings.database_url.startswith("sqlite")

# Create engine options based on database type
engine_kwargs = {
    "echo": settings.debug,
}

if is_sqlite:
    engine_kwargs["poolclass"] = NullPool
else:
    engine_kwargs.update(
        {
            "pool_pre_ping": True,
            "pool_size": 20,
            "max_overflow": 30,
            "pool_recycle": 3600,
        }
    )

# Create async engine
engine = create_async_engine(settings.database_url, **engine_kwargs)

# Create async session factory
async_session_factory = async_sessionmaker(
    engine,
    class_=AsyncSession,
    expire_on_commit=False,
)
