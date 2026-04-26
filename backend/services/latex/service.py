"""LaTeX compilation service that orchestrates CLSI operations."""

from __future__ import annotations

import logging
import re
import uuid
from collections import OrderedDict
from threading import Lock

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.config import get_settings
from models import Project, User
from services.latex.clsi_client import (
    ClsiClient,
    ClsiCompileResult,
    ClsiSyncCodeResult,
    ClsiSyncPdfResult,
    ClsiWordcountResult,
    STATUS_FAILURE,
    STATUS_ERROR,
    STATUS_TIMEDOUT,
)
from services.latex.resource_builder import ProjectFile, build_resources

logger = logging.getLogger(__name__)

_FAILED_STATUSES = {STATUS_FAILURE, STATUS_ERROR, STATUS_TIMEDOUT}

# Matches LaTeX error lines: "! Error message"
_LATEX_ERROR_RE = re.compile(r"^!\s+.+", re.MULTILINE)

# In-memory LRU cache for PDF bytes (keyed by build_id)
_MAX_PDF_CACHE = 64


class _PdfCache:
    """Thread-safe LRU cache for compiled PDF bytes."""

    def __init__(self, maxsize: int = _MAX_PDF_CACHE) -> None:
        self._maxsize = maxsize
        self._cache: OrderedDict[str, bytes] = OrderedDict()
        self._lock = Lock()

    def get(self, key: str) -> bytes | None:
        with self._lock:
            val = self._cache.get(key)
            if val is not None:
                self._cache.move_to_end(key)
            return val

    def put(self, key: str, data: bytes) -> None:
        with self._lock:
            self._cache[key] = data
            self._cache.move_to_end(key)
            while len(self._cache) > self._maxsize:
                self._cache.popitem(last=False)

    def invalidate(self, key: str) -> None:
        with self._lock:
            self._cache.pop(key, None)


_pdf_cache = _PdfCache()


class LaTeXServiceError(Exception):
    """LaTeX service error."""

    def __init__(self, message: str, status_code: int = 500) -> None:
        self.message = message
        self.status_code = status_code
        super().__init__(message)


class LaTeXService:
    """Orchestrates LaTeX compilation through CLSI."""

    def __init__(self, client: ClsiClient | None = None) -> None:
        self._client = client or ClsiClient()

    async def verify_project_access(
        self,
        project_id: uuid.UUID,
        user: User,
        db: AsyncSession,
    ) -> Project:
        """Verify user has access to the project."""
        result = await db.execute(
            select(Project).where(
                Project.id == project_id,
                Project.owner_id == user.id,
                Project.is_deleted == False,  # noqa: E712
            )
        )
        project = result.scalar_one_or_none()
        if not project:
            raise LaTeXServiceError("Project not found", status_code=404)
        return project

    async def compile_project(
        self,
        project_id: str,
        files: list[ProjectFile],
        root_file: str = "main.tex",
        compiler: str = "pdflatex",
    ) -> ClsiCompileResult:
        """Compile a project by assembling resources and sending to CLSI."""
        settings = get_settings()
        if not settings.latex_clsi_enabled:
            raise LaTeXServiceError("LaTeX compilation is disabled", status_code=503)

        resources, validated_root = build_resources(files, root_file)

        if not resources:
            raise LaTeXServiceError("No files provided for compilation", status_code=400)

        logger.info(
            "Compiling project=%s root=%s compiler=%s resources=%d",
            project_id,
            validated_root,
            compiler,
            len(resources),
        )

        result = await self._client.compile(
            project_id=project_id,
            resources=resources,
            root_resource_path=validated_root,
            compiler=compiler,
        )

        if result.status in _FAILED_STATUSES and result.build_id:
            result = await _enrich_error_from_log(self._client, project_id, result)

        return result

    async def stop_compile(self, project_id: str) -> bool:
        """Stop a running compilation."""
        return await self._client.stop_compile(project_id)

    async def clear_cache(self, project_id: str) -> bool:
        """Clear compilation cache."""
        return await self._client.clear_cache(project_id)

    async def get_wordcount(
        self,
        project_id: str,
        file: str = "main.tex",
        image: str | None = None,
    ) -> ClsiWordcountResult:
        """Get word count for a file."""
        return await self._client.wordcount(project_id, file, image)

    async def sync_code_to_pdf(
        self,
        project_id: str,
        file: str,
        line: int,
        column: int,
    ) -> ClsiSyncCodeResult:
        """SyncTeX source-to-PDF lookup."""
        return await self._client.sync_from_code(project_id, file, line, column)

    async def sync_pdf_to_code(
        self,
        project_id: str,
        page: int,
        h: float,
        v: float,
    ) -> ClsiSyncPdfResult:
        """SyncTeX PDF-to-source lookup."""
        return await self._client.sync_from_pdf(project_id, page, h, v)

    async def get_output_zip(
        self,
        project_id: str,
        build_id: str,
    ):
        """Get build output as ZIP."""
        return await self._client.get_output_zip(project_id, build_id)

    async def get_output_pdf(
        self,
        project_id: str,
        build_id: str,
    ) -> bytes:
        """Get compiled PDF bytes from build output (cached)."""
        cached = _pdf_cache.get(build_id)
        if cached is not None:
            return cached

        resp = await self._client.get_output_file(project_id, build_id, "output.pdf")
        if resp.status_code != 200:
            raise LaTeXServiceError(
                f"CLSI returned {resp.status_code}",
                status_code=502,
            )
        data = resp.content
        _pdf_cache.put(build_id, data)
        return data


_MAX_LOG_SIZE = 8192
_MAX_ERROR_SUMMARY_LINES = 30


async def _enrich_error_from_log(
    client: ClsiClient,
    project_id: str,
    result: ClsiCompileResult,
) -> ClsiCompileResult:
    """Fetch output.log and extract error lines when compilation fails."""
    try:
        resp = await client.get_output_file(project_id, result.build_id, "output.log")
        if resp.status_code != 200:
            return result

        raw_log = resp.text[:_MAX_LOG_SIZE]
        result.logs = raw_log

        error_lines = _LATEX_ERROR_RE.findall(raw_log)
        if error_lines:
            summary = "\n".join(error_lines[:_MAX_ERROR_SUMMARY_LINES])
        elif "fatal error" in raw_log.lower():
            # Fallback: grab lines mentioning fatal error
            summary = "\n".join(
                line for line in raw_log.splitlines() if "fatal error" in line.lower()
            )[:_MAX_LOG_SIZE]
        else:
            # Last resort: last 10 non-empty lines
            tail = [l for l in raw_log.splitlines() if l.strip()][-10:]
            summary = "\n".join(tail)

        result.error = summary or result.error or "Compilation failed"
    except Exception:
        logger.warning(
            "Failed to fetch output.log for project=%s build=%s",
            project_id,
            result.build_id,
            exc_info=True,
        )

    return result
