"""LaTeX compilation service that orchestrates CLSI operations."""

from __future__ import annotations

import logging
import uuid

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
)
from services.latex.resource_builder import ProjectFile, build_resources

logger = logging.getLogger(__name__)


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
            project_id, validated_root, compiler, len(resources),
        )

        return await self._client.compile(
            project_id=project_id,
            resources=resources,
            root_resource_path=validated_root,
            compiler=compiler,
        )

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
