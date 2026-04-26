"""Folder management service."""

from __future__ import annotations

import uuid

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from models.folder import Folder
from models.project import Project


async def verify_project_access(
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


class FolderService:
    """Service for managing folders within a project."""

    async def list_by_project(
        self,
        project_id: uuid.UUID,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> list[Folder]:
        """List all folders in a project."""
        await verify_project_access(project_id, user_id, db)
        result = await db.execute(
            select(Folder).where(Folder.project_id == project_id).order_by(Folder.name)
        )
        return list(result.scalars().all())

    async def create(
        self,
        project_id: uuid.UUID,
        user_id: uuid.UUID,
        name: str,
        parent_id: uuid.UUID | None,
        db: AsyncSession,
    ) -> Folder:
        """Create a new folder."""
        await verify_project_access(project_id, user_id, db)

        if parent_id:
            parent = await db.execute(
                select(Folder).where(
                    Folder.id == parent_id,
                    Folder.project_id == project_id,
                )
            )
            if not parent.scalar_one_or_none():
                raise ValueError("Parent folder not found")

        folder = Folder(
            name=name,
            parent_id=parent_id,
            project_id=project_id,
            owner_id=user_id,
        )
        db.add(folder)
        await db.commit()
        await db.refresh(folder)
        return folder

    async def delete(
        self,
        folder_id: uuid.UUID,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> bool:
        """Delete a folder. Documents move to project root (SET NULL)."""
        result = await db.execute(
            select(Folder).where(Folder.id == folder_id, Folder.owner_id == user_id)
        )
        folder = result.scalar_one_or_none()
        if not folder:
            raise ValueError("Folder not found")

        await db.delete(folder)
        await db.commit()
        return True
