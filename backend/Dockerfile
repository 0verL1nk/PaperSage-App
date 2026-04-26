# ── Stage: deps ──────────────────────────────────────────────
FROM python:3.12-slim AS deps

COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

WORKDIR /app

COPY pyproject.toml uv.lock ./

# Mount cache for uv downloads — survives rebuilds
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-install-project

# ── Stage: production ───────────────────────────────────────
FROM python:3.12-slim AS production

RUN addgroup --system appgroup && adduser --system --ingroup appgroup appuser

WORKDIR /app

COPY --from=deps --chown=appuser:appgroup /app/.venv .venv
COPY --chown=appuser:appgroup app/ app/
COPY --chown=appuser:appgroup api/ api/
COPY --chown=appuser:appgroup core/ core/
COPY --chown=appuser:appgroup models/ models/
COPY --chown=appuser:appgroup schemas/ schemas/
COPY --chown=appuser:appgroup services/ services/
COPY --chown=appuser:appgroup main.py .
COPY --chown=appuser:appgroup alembic/ alembic/
COPY --chown=appuser:appgroup alembic.ini .

ENV PATH="/app/.venv/bin:$PATH" \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONOPTIMIZE=2

USER appuser

EXPOSE 5478

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:5478/health')" || exit 1

# Workers: 2-4 x $(nproc) for I/O-bound FastAPI
CMD ["uvicorn", "app.main:app", \
     "--host", "0.0.0.0", \
     "--port", "5478", \
     "--workers", "4", \
     "--no-access-log"]
