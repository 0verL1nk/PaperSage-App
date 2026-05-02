"""LaTeX file schemas for request/response validation."""

from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, Field


class LatexFileCreateRequest(BaseModel):
    """Create LaTeX file request."""

    path: str = Field(..., min_length=1, max_length=500, description="File path")
    content: str = Field("", description="File content")


class LatexFileUpdateRequest(BaseModel):
    """Update LaTeX file request."""

    content: str = Field(..., min_length=0, description="File content")


class LatexFileRenameRequest(BaseModel):
    """Rename a LaTeX file (change its path)."""

    new_path: str = Field(..., min_length=1, max_length=500, description="New file path")


class LatexFileMoveRequest(BaseModel):
    """Move a LaTeX file to a new directory."""

    new_dir: str = Field("", max_length=500, description="Target directory path (empty string for root)")


class LatexFileResponse(BaseModel):
    """LaTeX file metadata response."""

    id: UUID
    file_path: str
    content_type: str | None
    size_bytes: int
    last_saved_at: datetime | None
    created_at: datetime
    updated_at: datetime

    model_config = {"from_attributes": True}


class LatexFileListResponse(BaseModel):
    """List of LaTeX files response."""

    files: list[LatexFileResponse]


class LatexFileContentResponse(BaseModel):
    """LaTeX file content response."""

    path: str
    content: str
