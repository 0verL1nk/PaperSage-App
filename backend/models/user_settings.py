"""User settings model."""

from datetime import UTC, datetime
from uuid import UUID as UUIDType

from sqlalchemy import DateTime, ForeignKey, String
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column, relationship

from core.base import Base


class UserSettings(Base):
    """Per-user settings stored by category as JSONB."""

    __tablename__ = "user_settings"

    user_id: Mapped[UUIDType] = mapped_column(
        String(32),
        ForeignKey("users.id", ondelete="CASCADE"),
        primary_key=True,
        nullable=False,
    )
    category: Mapped[str] = mapped_column(
        String(100),
        primary_key=True,
        nullable=False,
    )
    settings: Mapped[dict] = mapped_column(
        JSONB,
        nullable=False,
        default=dict,
        server_default="{}",
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True),
        default=lambda: datetime.now(UTC),
        onupdate=lambda: datetime.now(UTC),
        nullable=False,
    )

    user: Mapped["User"] = relationship("User", lazy="selectin")

    def __repr__(self) -> str:
        return f"<UserSettings(user_id={self.user_id}, category={self.category})>"
