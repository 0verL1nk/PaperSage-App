"""LaTeX file models."""

import uuid
from datetime import UTC, datetime
from typing import TYPE_CHECKING

from sqlalchemy import DateTime, ForeignKey, String, UniqueConstraint
from sqlalchemy.orm import Mapped, mapped_column, relationship

from core.base import Base, TimestampMixin, UUID, UUIDMixin

if TYPE_CHECKING:
    from models.project import Project


class LatexFile(Base, UUIDMixin, TimestampMixin):
    """LaTeX file metadata model."""

    __tablename__ = "latex_files"
    __table_args__ = (
        UniqueConstraint("project_id", "file_path", name="uq_project_file_path"),
    )

    project_id: Mapped[uuid.UUID] = mapped_column(
        UUID(),
        ForeignKey("projects.id", ondelete="CASCADE"),
        nullable=False,
    )
    file_path: Mapped[str] = mapped_column(String(500), nullable=False)
    content_type: Mapped[str | None] = mapped_column(String(100), nullable=True)
    size_bytes: Mapped[int] = mapped_column(default=0)
    s3_key: Mapped[str | None] = mapped_column(String(500), nullable=True)
    last_saved_at: Mapped[datetime | None] = mapped_column(
        DateTime(timezone=True),
        default=lambda: datetime.now(UTC),
        nullable=True,
    )

    # Relationships
    project: Mapped["Project"] = relationship("Project", back_populates="latex_files")

    def __repr__(self) -> str:
        return f"<LatexFile(id={self.id}, path={self.file_path})>"
