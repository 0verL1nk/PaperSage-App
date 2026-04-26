"""LaTeX file management service with in-memory content cache."""

from __future__ import annotations

import hashlib
import logging
import uuid
from collections import OrderedDict
from datetime import UTC, datetime
from threading import Lock

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from models.latex_file import LatexFile
from models.project import Project
from services.storage.s3_client import S3Storage

logger = logging.getLogger(__name__)

ALLOWED_EXTENSIONS = (".tex", ".bib", ".cls", ".sty", ".bst", ".txt", ".md", ".lof", ".lot", ".toc")
BLOCKED_EXTENSIONS = (".exe", ".dll", ".so", ".dylib")

# In-memory LRU cache for file contents
_MAX_CACHE_SIZE = 512


class _ContentCache:
    """Thread-safe LRU cache keyed by s3_key."""

    def __init__(self, maxsize: int = _MAX_CACHE_SIZE) -> None:
        self._maxsize = maxsize
        self._cache: OrderedDict[str, str] = OrderedDict()
        self._lock = Lock()

    def get(self, key: str) -> str | None:
        with self._lock:
            val = self._cache.get(key)
            if val is not None:
                self._cache.move_to_end(key)
            return val

    def put(self, key: str, content: str) -> None:
        with self._lock:
            self._cache[key] = content
            self._cache.move_to_end(key)
            while len(self._cache) > self._maxsize:
                self._cache.popitem(last=False)

    def invalidate(self, key: str) -> None:
        with self._lock:
            self._cache.pop(key, None)

    def clear(self) -> None:
        with self._lock:
            self._cache.clear()


_cache = _ContentCache()


def _validate_path(file_path: str) -> str:
    """Validate and normalize a file path."""
    if ".." in file_path:
        raise ValueError("Path traversal is not allowed")

    normalized = file_path.lstrip("/")

    ext = "." + normalized.rsplit(".", 1)[-1].lower() if "." in normalized else ""
    if ext in BLOCKED_EXTENSIONS:
        raise ValueError(f"File type not allowed: {ext}")

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

    async def load_all_contents(
        self,
        project_id: uuid.UUID,
        db: AsyncSession,
    ) -> list[tuple[str, str]]:
        """Load all file (path, content) pairs for a project, using cache."""
        result = await db.execute(select(LatexFile).where(LatexFile.project_id == project_id))
        db_files = list(result.scalars().all())

        pairs: list[tuple[str, str]] = []
        for f in db_files:
            s3_key = f.s3_key or ""
            cached = _cache.get(s3_key)
            if cached is not None:
                pairs.append((f.file_path, cached))
            else:
                try:
                    content = self._storage.get_object(s3_key).decode("utf-8")
                    _cache.put(s3_key, content)
                    pairs.append((f.file_path, content))
                except Exception:
                    continue
        return pairs

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

    async def get_file_raw(
        self,
        project_id: uuid.UUID,
        file_path: str,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> tuple[bytes, str]:
        """Get raw file bytes and content type."""
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

        s3_key = latex_file.s3_key or ""
        content_type = latex_file.content_type or "application/octet-stream"

        # Check cache (only for text files, skip for binary)
        if any(file_path.endswith(ext) for ext in ALLOWED_EXTENSIONS):
            cached = _cache.get(s3_key)
            if cached is not None:
                return cached.encode("utf-8"), content_type

        content = self._storage.get_object(s3_key)
        
        # Cache text contents
        if any(file_path.endswith(ext) for ext in ALLOWED_EXTENSIONS):
            try:
                _cache.put(s3_key, content.decode("utf-8"))
            except UnicodeDecodeError:
                pass

        return content, content_type

    async def get_file_content(
        self,
        project_id: uuid.UUID,
        file_path: str,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> str:
        """Get file content as string (for text files)."""
        content, _ = await self.get_file_raw(project_id, file_path, user_id, db)
        return content.decode("utf-8")

    async def create_file(
        self,
        project_id: uuid.UUID,
        path: str,
        content: str,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> LatexFile:
        """Create a new text-based LaTeX file."""
        return await self.create_file_binary(
            project_id, path, content.encode("utf-8"), "text/plain", user_id, db
        )

    async def create_file_binary(
        self,
        project_id: uuid.UUID,
        path: str,
        content: bytes,
        content_type: str | None,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> LatexFile:
        """Create a new LaTeX file (text or binary)."""
        project = await _verify_project_access(project_id, user_id, db)
        validated_path = _validate_path(path)

        existing = await db.execute(
            select(LatexFile).where(
                LatexFile.project_id == project.id,
                LatexFile.file_path == validated_path,
            )
        )
        if existing.scalar_one_or_none():
            raise ValueError(f"File already exists: {validated_path}")

        content_hash = hashlib.md5(content).hexdigest()

        s3_key = S3Storage.build_key(str(project.id), validated_path)
        self._storage.put_object(s3_key, content)
        
        # Only cache if it's likely text
        if content_type and "text" in content_type:
            try:
                _cache.put(s3_key, content.decode("utf-8"))
            except UnicodeDecodeError:
                pass

        now = datetime.now(UTC)
        latex_file = LatexFile(
            project_id=project.id,
            file_path=validated_path,
            content_type=content_type or "application/octet-stream",
            size_bytes=len(content),
            content_hash=content_hash,
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
        """Update an existing LaTeX file with change detection."""
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

        content_bytes = content.encode("utf-8")
        new_hash = hashlib.md5(content_bytes).hexdigest()

        # SKIP upload if hash matches
        if latex_file.content_hash == new_hash:
            logger.debug("Skipping upload for %s, content unchanged", file_path)
            return latex_file

        self._storage.put_object(latex_file.s3_key or "", content_bytes)
        _cache.put(latex_file.s3_key or "", content)

        latex_file.content_hash = new_hash
        latex_file.size_bytes = len(content_bytes)
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
            _cache.invalidate(latex_file.s3_key)

        await db.delete(latex_file)
        await db.commit()

        return True
