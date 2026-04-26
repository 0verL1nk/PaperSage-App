"""Folder schemas for request/response validation."""

from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, Field


class FolderCreate(BaseModel):
    """Create folder request schema."""

    model_config = {"populate_by_name": True}

    name: str = Field(..., min_length=1, max_length=100, description="Folder name")
    project_id: UUID = Field(..., alias="projectId", description="Project ID")
    parent_id: UUID | None = Field(None, alias="parentId", description="Parent folder ID (null for root)")


class FolderResponse(BaseModel):
    """Folder response schema."""

    model_config = {"from_attributes": True, "populate_by_name": True, "ser_json_alias": True}

    id: UUID
    name: str
    parent_id: UUID | None = Field(alias="parentId")
    project_id: UUID = Field(alias="projectId")
    owner_id: UUID = Field(alias="ownerId")
    created_at: datetime = Field(alias="createdAt")
    updated_at: datetime = Field(alias="updatedAt")
