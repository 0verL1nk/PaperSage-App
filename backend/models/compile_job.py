"""Compile job models for async LaTeX compilation."""

import uuid
from enum import StrEnum
from typing import TYPE_CHECKING

from sqlalchemy import JSON, ForeignKey, String, Text
from sqlalchemy.orm import Mapped, mapped_column, relationship

from core.base import UUID, Base, TimestampMixin, UUIDMixin

if TYPE_CHECKING:
    from models.project import Project
    from models.user import User


class CompileJobStatus(StrEnum):
    """Compile job status lifecycle."""

    PENDING = "pending"
    RUNNING = "running"
    SUCCESS = "success"
    FAILED = "failed"
    TIMEOUT = "timeout"
    CANCELLED = "cancelled"

    @property
    def is_terminal(self) -> bool:
        """Check if this status is terminal (no further transitions)."""
        return self in (self.SUCCESS, self.FAILED, self.TIMEOUT, self.CANCELLED)


class CompileJob(Base, UUIDMixin, TimestampMixin):
    """Async compile job model."""

    __tablename__ = "compile_jobs"

    project_id: Mapped[uuid.UUID] = mapped_column(
        UUID(),
        ForeignKey("projects.id", ondelete="CASCADE"),
        nullable=False,
        index=True,
    )
    user_id: Mapped[uuid.UUID] = mapped_column(
        UUID(),
        ForeignKey("users.id", ondelete="CASCADE"),
        nullable=False,
        index=True,
    )
    status: Mapped[str] = mapped_column(
        String(20),
        default=CompileJobStatus.PENDING,
        nullable=False,
        index=True,
    )
    root_file: Mapped[str] = mapped_column(String(500), nullable=False, default="main.tex")
    compiler: Mapped[str] = mapped_column(String(20), nullable=False, default="pdflatex")
    build_id: Mapped[str | None] = mapped_column(String(100), nullable=True)
    error: Mapped[str | None] = mapped_column(Text, nullable=True)
    logs: Mapped[str | None] = mapped_column(Text, nullable=True)
    sync_state: Mapped[dict | None] = mapped_column(
        JSON,
        nullable=True,
        comment="Snapshot of {file_path: content_hash} from last successful compile",
    )

    # Relationships
    project: Mapped["Project"] = relationship("Project")
    user: Mapped["User"] = relationship("User")

    def __repr__(self) -> str:
        return f"<CompileJob(id={self.id}, status={self.status})>"
