"""Document schemas for request/response validation."""

from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, Field


class DocumentResponse(BaseModel):
    """Document response schema."""

    model_config = {"from_attributes": True, "populate_by_name": True, "ser_json_alias": True}

    id: UUID
    name: str
    type: str
    size: int
    status: str
    project_id: UUID = Field(alias="projectId")
    folder_id: UUID | None = Field(alias="folderId")
    owner_id: UUID = Field(alias="ownerId")
    s3_key: str = Field(alias="s3Key")
    remark: str | None
    created_at: datetime = Field(alias="createdAt")
    updated_at: datetime = Field(alias="updatedAt")


class DocumentMoveRequest(BaseModel):
    """Move document to another folder."""

    model_config = {"populate_by_name": True}

    folder_id: UUID | None = Field(
        None, alias="folderId", description="Target folder ID (null to move to project root)"
    )


class DocumentRemarkRequest(BaseModel):
    """Update document remark."""

    remark: str = Field("", max_length=2000, description="Document remark")
