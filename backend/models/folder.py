"""Folder model for document organization."""

import uuid
from typing import TYPE_CHECKING

from sqlalchemy import ForeignKey, String
from sqlalchemy.orm import Mapped, mapped_column, relationship

from core.base import UUID, Base, TimestampMixin, UUIDMixin

if TYPE_CHECKING:
    from models.project import Project
    from models.user import User


class Folder(Base, UUIDMixin, TimestampMixin):
    """Folder model for organizing documents within a project."""

    __tablename__ = "folders"

    name: Mapped[str] = mapped_column(String(100), nullable=False)
    parent_id: Mapped[uuid.UUID | None] = mapped_column(
        UUID(),
        ForeignKey("folders.id", ondelete="CASCADE"),
        nullable=True,
    )
    project_id: Mapped[uuid.UUID] = mapped_column(
        UUID(),
        ForeignKey("projects.id", ondelete="CASCADE"),
        nullable=False,
    )
    owner_id: Mapped[uuid.UUID] = mapped_column(
        UUID(),
        ForeignKey("users.id", ondelete="CASCADE"),
        nullable=False,
    )

    # Relationships
    project: Mapped["Project"] = relationship("Project")
    owner: Mapped["User"] = relationship("User")
    children: Mapped[list["Folder"]] = relationship(
        "Folder",
        back_populates="parent",
        cascade="all, delete-orphan",
    )
    parent: Mapped["Folder | None"] = relationship(
        "Folder",
        back_populates="children",
        remote_side="Folder.id",
    )

    def __repr__(self) -> str:
        return f"<Folder(id={self.id}, name={self.name})>"
