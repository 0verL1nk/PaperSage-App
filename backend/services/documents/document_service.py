"""Document management service with S3 storage."""

from __future__ import annotations

import logging
import uuid
from pathlib import PurePosixPath

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from models.document import Document
from services.documents.folder_service import verify_project_access
from services.storage.s3_client import S3Storage

logger = logging.getLogger(__name__)

ALLOWED_MIME_TYPES = {
    "application/pdf",
    "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    "text/plain",
}

MAX_FILE_SIZE_BYTES = 50 * 1024 * 1024  # 50 MB


class DocumentService:
    """Service for managing uploaded reference documents."""

    def __init__(self, storage: S3Storage | None = None) -> None:
        self._storage = storage or S3Storage()

    async def list_by_project(
        self,
        project_id: uuid.UUID,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> list[Document]:
        """List all documents in a project."""
        await verify_project_access(project_id, user_id, db)
        result = await db.execute(
            select(Document)
            .where(Document.project_id == project_id)
            .order_by(Document.created_at.desc())
        )
        return list(result.scalars().all())

    async def list_by_folder(
        self,
        folder_id: uuid.UUID,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> list[Document]:
        """List all documents in a specific folder."""
        result = await db.execute(
            select(Document).where(Document.folder_id == folder_id, Document.owner_id == user_id)
        )
        docs = list(result.scalars().all())
        if docs:
            await verify_project_access(docs[0].project_id, user_id, db)
        return docs

    async def upload(
        self,
        project_id: uuid.UUID,
        user_id: uuid.UUID,
        file_name: str,
        file_content: bytes,
        content_type: str,
        folder_id: uuid.UUID | None,
        remark: str | None,
        db: AsyncSession,
    ) -> Document:
        """Upload a document to a project."""
        await verify_project_access(project_id, user_id, db)

        if content_type not in ALLOWED_MIME_TYPES:
            raise ValueError(
                f"Unsupported file type: {content_type}. "
                f"Allowed: {', '.join(sorted(ALLOWED_MIME_TYPES))}"
            )

        if len(file_content) > MAX_FILE_SIZE_BYTES:
            raise ValueError("文件大小不能超过 50MB")

        if folder_id:
            from models.folder import Folder

            folder_result = await db.execute(
                select(Folder).where(Folder.id == folder_id, Folder.project_id == project_id)
            )
            if not folder_result.scalar_one_or_none():
                raise ValueError("Folder not found")

        file_uuid = uuid.uuid4()
        s3_key = str(
            PurePosixPath("projects") / str(project_id) / "documents" / str(file_uuid) / file_name
        )
        self._storage.put_object(s3_key, file_content)

        doc = Document(
            name=file_name,
            type=content_type,
            size=len(file_content),
            status="uploaded",
            s3_key=s3_key,
            remark=remark,
            project_id=project_id,
            folder_id=folder_id,
            owner_id=user_id,
        )
        db.add(doc)
        await db.commit()
        await db.refresh(doc)
        return doc

    async def get_preview(
        self,
        document_id: uuid.UUID,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> tuple[bytes, str, str]:
        """Get document content for preview. Returns (content_bytes, content_type, file_name)."""
        result = await db.execute(
            select(Document).where(Document.id == document_id, Document.owner_id == user_id)
        )
        doc = result.scalar_one_or_none()
        if not doc:
            raise ValueError("Document not found")

        content = self._storage.get_object(doc.s3_key)
        return content, doc.type, doc.name

    async def move(
        self,
        document_id: uuid.UUID,
        folder_id: uuid.UUID | None,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> Document:
        """Move a document to another folder."""
        result = await db.execute(
            select(Document).where(Document.id == document_id, Document.owner_id == user_id)
        )
        doc = result.scalar_one_or_none()
        if not doc:
            raise ValueError("Document not found")

        if folder_id:
            from models.folder import Folder

            folder_result = await db.execute(
                select(Folder).where(Folder.id == folder_id, Folder.project_id == doc.project_id)
            )
            if not folder_result.scalar_one_or_none():
                raise ValueError("Target folder not found")

        doc.folder_id = folder_id
        await db.commit()
        await db.refresh(doc)
        return doc

    async def update_remark(
        self,
        document_id: uuid.UUID,
        remark: str,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> Document:
        """Update document remark."""
        result = await db.execute(
            select(Document).where(Document.id == document_id, Document.owner_id == user_id)
        )
        doc = result.scalar_one_or_none()
        if not doc:
            raise ValueError("Document not found")

        doc.remark = remark or None
        await db.commit()
        await db.refresh(doc)
        return doc

    async def delete(
        self,
        document_id: uuid.UUID,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> bool:
        """Delete a document from database and S3."""
        result = await db.execute(
            select(Document).where(Document.id == document_id, Document.owner_id == user_id)
        )
        doc = result.scalar_one_or_none()
        if not doc:
            raise ValueError("Document not found")

        try:
            self._storage.delete_object(doc.s3_key)
        except Exception:
            logger.warning("Failed to delete S3 object: %s", doc.s3_key)

        await db.delete(doc)
        await db.commit()
        return True
