"""Document model for uploaded research references."""

import uuid
from typing import TYPE_CHECKING

from sqlalchemy import BigInteger, ForeignKey, String, Text
from sqlalchemy.orm import Mapped, mapped_column, relationship

from core.base import UUID, Base, TimestampMixin, UUIDMixin

if TYPE_CHECKING:
    from models.folder import Folder
    from models.project import Project
    from models.user import User


class Document(Base, UUIDMixin, TimestampMixin):
    """Document model for uploaded reference files (PDF, DOCX, TXT)."""

    __tablename__ = "documents"

    name: Mapped[str] = mapped_column(String(255), nullable=False)
    type: Mapped[str] = mapped_column(String(100), nullable=False)
    size: Mapped[int] = mapped_column(BigInteger, default=0)
    status: Mapped[str] = mapped_column(String(20), default="uploaded")
    s3_key: Mapped[str] = mapped_column(String(500), nullable=False)
    remark: Mapped[str | None] = mapped_column(Text, nullable=True)
    project_id: Mapped[uuid.UUID] = mapped_column(
        UUID(),
        ForeignKey("projects.id", ondelete="CASCADE"),
        nullable=False,
    )
    folder_id: Mapped[uuid.UUID | None] = mapped_column(
        UUID(),
        ForeignKey("folders.id", ondelete="SET NULL"),
        nullable=True,
    )
    owner_id: Mapped[uuid.UUID] = mapped_column(
        UUID(),
        ForeignKey("users.id", ondelete="CASCADE"),
        nullable=False,
    )

    # Relationships
    project: Mapped["Project"] = relationship("Project")
    folder: Mapped["Folder | None"] = relationship("Folder")
    owner: Mapped["User"] = relationship("User")

    def __repr__(self) -> str:
        return f"<Document(id={self.id}, name={self.name})>"
