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
    # Startup - create tables
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    yield
    # Shutdown


settings = get_settings()

app = FastAPI(
    title="PaperSage API",
    description="PaperSage Backend API",
    version="0.1.0",
    docs_url="/docs",
    redoc_url="/redoc",
    lifespan=lifespan,
    redirect_slashes=False,
)

# CORS middleware - allow all origins for development
app.add_middleware(
    CORSMiddleware,
    allow_origin_regex=r"http://localhost:[0-9]+",
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
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
app.include_router(api_router, prefix="/api")


@app.get("/health")
async def health_check() -> dict:
    """Health check endpoint."""
    return {"status": "healthy", "version": "0.1.0"}
