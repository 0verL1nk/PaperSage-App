"""LaTeX file management service."""

from __future__ import annotations

import logging
import uuid
from datetime import UTC, datetime

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from models.latex_file import LatexFile
from models.project import Project
from services.storage.s3_client import S3Storage

logger = logging.getLogger(__name__)

ALLOWED_EXTENSIONS = (".tex", ".bib", ".cls", ".sty", ".bst")


def _validate_path(file_path: str) -> str:
    """Validate and normalize a file path.

    Raises ValueError for paths with directory traversal or disallowed extensions.
    """
    if ".." in file_path:
        raise ValueError("Path traversal is not allowed")

    normalized = file_path.lstrip("/")

    if not any(normalized.endswith(ext) for ext in ALLOWED_EXTENSIONS):
        raise ValueError(
            f"File extension not allowed. Must be one of: {', '.join(ALLOWED_EXTENSIONS)}"
        )

    return normalized


async def _verify_project_access(
    project_id: uuid.UUID,
    user_id: uuid.UUID,
    db: AsyncSession,
) -> Project:
    """Verify user owns the project."""
    result = await db.execute(
        select(Project).where(
            Project.id == project_id,
            Project.owner_id == user_id,
            Project.is_deleted == False,  # noqa: E712
        )
    )
    project = result.scalar_one_or_none()
    if not project:
        raise ValueError("Project not found")
    return project


class LatexFileService:
    """Service for managing LaTeX files within a project."""

    def __init__(self, storage: S3Storage | None = None) -> None:
        self._storage = storage or S3Storage()

    async def list_files(
        self,
        project_id: uuid.UUID,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> list[LatexFile]:
        """List all files in a project."""
        await _verify_project_access(project_id, user_id, db)

        result = await db.execute(
            select(LatexFile)
            .where(LatexFile.project_id == project_id)
            .order_by(LatexFile.file_path)
        )
        return list(result.scalars().all())

    async def get_file_content(
        self,
        project_id: uuid.UUID,
        file_path: str,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> str:
        """Get file content from S3."""
        await _verify_project_access(project_id, user_id, db)

        result = await db.execute(
            select(LatexFile).where(
                LatexFile.project_id == project_id,
                LatexFile.file_path == file_path,
            )
        )
        latex_file = result.scalar_one_or_none()
        if not latex_file:
            raise ValueError("File not found")

        content = self._storage.get_object(latex_file.s3_key or "")
        return content.decode("utf-8")

    async def create_file(
        self,
        project_id: uuid.UUID,
        path: str,
        content: str,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> LatexFile:
        """Create a new LaTeX file."""
        project = await _verify_project_access(project_id, user_id, db)
        validated_path = _validate_path(path)

        # Check for duplicate
        existing = await db.execute(
            select(LatexFile).where(
                LatexFile.project_id == project.id,
                LatexFile.file_path == validated_path,
            )
        )
        if existing.scalar_one_or_none():
            raise ValueError(f"File already exists: {validated_path}")

        s3_key = S3Storage.build_key(str(project.id), validated_path)
        self._storage.put_object(s3_key, content)

        now = datetime.now(UTC)
        latex_file = LatexFile(
            project_id=project.id,
            file_path=validated_path,
            content_type="text/plain",
            size_bytes=len(content.encode("utf-8")),
            s3_key=s3_key,
            last_saved_at=now,
        )
        db.add(latex_file)
        await db.commit()
        await db.refresh(latex_file)

        return latex_file

    async def update_file(
        self,
        project_id: uuid.UUID,
        file_path: str,
        content: str,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> LatexFile:
        """Update an existing LaTeX file."""
        await _verify_project_access(project_id, user_id, db)

        result = await db.execute(
            select(LatexFile).where(
                LatexFile.project_id == project_id,
                LatexFile.file_path == file_path,
            )
        )
        latex_file = result.scalar_one_or_none()
        if not latex_file:
            raise ValueError("File not found")

        self._storage.put_object(latex_file.s3_key or "", content)

        latex_file.size_bytes = len(content.encode("utf-8"))
        latex_file.last_saved_at = datetime.now(UTC)
        await db.commit()
        await db.refresh(latex_file)

        return latex_file

    async def delete_file(
        self,
        project_id: uuid.UUID,
        file_path: str,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> bool:
        """Delete a LaTeX file."""
        await _verify_project_access(project_id, user_id, db)

        result = await db.execute(
            select(LatexFile).where(
                LatexFile.project_id == project_id,
                LatexFile.file_path == file_path,
            )
        )
        latex_file = result.scalar_one_or_none()
        if not latex_file:
            raise ValueError("File not found")

        if latex_file.s3_key:
            self._storage.delete_object(latex_file.s3_key)

        await db.delete(latex_file)
        await db.commit()

        return True
