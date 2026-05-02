"""Compile job schemas for request/response validation."""

from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, Field

from models.compile_job import CompileJobStatus


class CompileJobSubmitResponse(BaseModel):
    """Response for compile job submission."""

    job_id: UUID = Field(..., description="Unique job identifier")
    status: CompileJobStatus = Field(..., description="Initial job status")


class CompileJobStatusResponse(BaseModel):
    """Response for compile job status polling."""

    job_id: UUID
    status: CompileJobStatus
    build_id: str | None = Field(None, description="CLSI build ID (available after compile starts)")
    error: str | None = Field(None, description="Error message if failed/timeout")
    logs: str | None = Field(None, description="Full compile logs")
    root_file: str
    compiler: str
    elapsed_seconds: float | None = Field(None, description="Seconds since job creation")
    created_at: datetime
    updated_at: datetime


class CompileJobListItem(BaseModel):
    """Compile job item in history listing."""

    job_id: UUID
    status: CompileJobStatus
    root_file: str
    compiler: str
    build_id: str | None = None
    error: str | None = None
    created_at: datetime
    updated_at: datetime


class CompileJobHistoryResponse(BaseModel):
    """Paginated compile job history."""

    items: list[CompileJobListItem]
    total: int
