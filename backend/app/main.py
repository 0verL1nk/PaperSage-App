"""FastAPI application entry point."""

from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException
from fastapi.exceptions import RequestValidationError
from fastapi.middleware.cors import CORSMiddleware

from api import api_router
from app.config import get_settings
from app.exceptions import (
    generic_exception_handler,
    http_exception_handler,
    validation_exception_handler,
)
from core.base import Base
from core.database import engine


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan events."""
    # Auto-run pending alembic migrations on startup
    from logging import getLogger

    from alembic.config import Config

    from alembic import command

    logger = getLogger("app.main")
    try:
        alembic_cfg = Config("alembic.ini")
        command.upgrade(alembic_cfg, "head")
        logger.info("Database migrations applied")
    except Exception:
        logger.warning("Alembic migration skipped (alembic.ini not found or failed)", exc_info=True)

    # Create any new tables not covered by migrations
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)

    # Connect cache store
    from core.cache import RedisCacheStore, cache_store

    if isinstance(cache_store, RedisCacheStore):
        await cache_store.connect()
        await cache_store.ping()
        print("✓ Cache: Redis connected")
    else:
        await cache_store.connect()
        print("✓ Cache: in-memory store active")

    # Recover orphaned compile jobs from previous session
    from services.latex.compile_queue import get_compile_queue

    queue = get_compile_queue()
    await queue.recover_on_startup()
    print("✓ Compile queue: recovered pending jobs")

    yield
    # Shutdown
    await cache_store.disconnect()


settings = get_settings()

app = FastAPI(
    title="PaperSage API",
    description="PaperSage Backend API",
    version="0.1.0",
    docs_url="/docs",
    redoc_url="/redoc",
    lifespan=lifespan,
)

# CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=False,
    allow_methods=["*"],
    allow_headers=["*"],
    expose_headers=["Content-Disposition"],
)

# Exception handlers
app.add_exception_handler(
    RequestValidationError,
    validation_exception_handler,  # type: ignore[arg-type]  # ty: ignore[invalid-argument-type]
)
app.add_exception_handler(
    HTTPException,
    http_exception_handler,  # type: ignore[arg-type]  # ty: ignore[invalid-argument-type]
)
app.add_exception_handler(
    Exception,
    generic_exception_handler,
)

# Include API router
app.include_router(api_router, prefix="/api/v1")


@app.get("/health")
async def health_check() -> dict:
    """Health check endpoint."""
    return {"status": "healthy", "version": "0.1.0"}
