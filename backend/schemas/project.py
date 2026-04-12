"""Project schemas for request/response validation."""
from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, Field


class ProjectGroupCreate(BaseModel):
    """Create project group request schema."""

    name: str = Field(..., min_length=1, max_length=50, description="Group name")


class ProjectGroupUpdate(BaseModel):
    """Update project group request schema."""

    name: str | None = Field(None, min_length=1, max_length=50, description="Group name")
    sort_order: int | None = Field(None, description="Sort order")


class ProjectGroupResponse(BaseModel):
    """Project group response schema."""

    id: UUID
    name: str
    owner_id: UUID
    sort_order: int
    created_at: datetime

    model_config = {"from_attributes": True}


class ProjectCreate(BaseModel):
    """Create project request schema."""

    name: str = Field(..., min_length=1, max_length=100, description="Project name")
    description: str | None = Field(None, max_length=1000, description="Project description")
    group_id: UUID | None = Field(None, description="Project group ID")


class ProjectUpdate(BaseModel):
    """Update project request schema."""

    name: str | None = Field(None, min_length=1, max_length=100, description="Project name")
    description: str | None = Field(None, max_length=1000, description="Project description")
    group_id: UUID | None = Field(None, description="Project group ID")
    status: str | None = Field(None, description="Project status")


class ProjectResponse(BaseModel):
    """Project response schema."""

    id: UUID
    name: str
    description: str | None
    owner_id: UUID
    group_id: UUID | None
    status: str
    is_deleted: bool
    deleted_at: datetime | None
    created_at: datetime
    updated_at: datetime

    model_config = {"from_attributes": True}


class ProjectWithGroupResponse(ProjectResponse):
    """Project response with group info."""

    group: ProjectGroupResponse | None = None

    model_config = {"from_attributes": True}


class TrashItemResponse(BaseModel):
    """Trash item response schema."""

    type: str  # "project" or "document"
    id: UUID
    name: str
    deleted_at: datetime
    original_data: dict | None = None

    model_config = {"from_attributes": True}