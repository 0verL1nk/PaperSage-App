"""Project models."""
import uuid
from typing import TYPE_CHECKING

from sqlalchemy import ForeignKey, String, Text
from sqlalchemy.orm import Mapped, mapped_column, relationship

from core.base import Base, TimestampMixin, UUID, UUIDMixin

if TYPE_CHECKING:
    from models.user import User


class ProjectGroup(Base, UUIDMixin, TimestampMixin):
    """Project group model."""

    __tablename__ = "project_groups"

    name: Mapped[str] = mapped_column(String(50), nullable=False)
    owner_id: Mapped[uuid.UUID] = mapped_column(
        UUID(),
        ForeignKey("users.id", ondelete="CASCADE"),
        nullable=False,
    )
    sort_order: Mapped[int] = mapped_column(default=0)

    # Relationships
    owner: Mapped["User"] = relationship("User", back_populates="project_groups")
    projects: Mapped[list["Project"]] = relationship(
        "Project", back_populates="group", cascade="all, delete-orphan"
    )


class Project(Base, UUIDMixin, TimestampMixin):
    """Project model."""

    __tablename__ = "projects"

    name: Mapped[str] = mapped_column(String(100), nullable=False)
    description: Mapped[str | None] = mapped_column(Text, nullable=True)
    owner_id: Mapped[uuid.UUID] = mapped_column(
        UUID(),
        ForeignKey("users.id", ondelete="CASCADE"),
        nullable=False,
    )
    group_id: Mapped[uuid.UUID | None] = mapped_column(
        UUID(),
        ForeignKey("project_groups.id", ondelete="SET NULL"),
        nullable=True,
    )
    status: Mapped[str] = mapped_column(String(20), default="active")
    is_deleted: Mapped[bool] = mapped_column(default=False)
    deleted_at: Mapped[str | None] = mapped_column(String(50), nullable=True)

    # Relationships
    owner: Mapped["User"] = relationship("User", back_populates="projects")
    group: Mapped["ProjectGroup | None"] = relationship(
        "ProjectGroup", back_populates="projects"
    )

    def __repr__(self) -> str:
        return f"<Project(id={self.id}, name={self.name})>"