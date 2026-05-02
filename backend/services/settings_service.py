"""User settings service — CRUD with defaults merging."""

from __future__ import annotations

import logging
from typing import Any
from uuid import UUID

from pydantic import ValidationError
from sqlalchemy import delete, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession

from models.user_settings import UserSettings
from schemas.user_settings import (
    CATEGORY_SCHEMAS,
    DEFAULT_SETTINGS,
    VALID_CATEGORIES,
)

logger = logging.getLogger(__name__)


class SettingsServiceError(Exception):
    """Settings service error."""


class SettingsService:
    """Manage per-user settings with category-based JSONB storage."""

    def __init__(self, db: AsyncSession) -> None:
        self._db = db

    async def get_all_settings(self, user_id: UUID) -> dict[str, dict[str, Any]]:
        """Read all categories, merging with defaults."""
        result = await self._db.execute(
            select(UserSettings).where(UserSettings.user_id == user_id)
        )
        rows = {row.category: row.settings for row in result.scalars().all()}

        merged: dict[str, dict[str, Any]] = {}
        for cat in DEFAULT_SETTINGS:
            merged[cat] = {**DEFAULT_SETTINGS[cat], **rows.get(cat, {})}
        return merged

    async def get_category(
        self, user_id: UUID, category: str
    ) -> dict[str, Any] | None:
        """Read a single category, merged with defaults."""
        if category not in VALID_CATEGORIES:
            return None

        result = await self._db.execute(
            select(UserSettings).where(
                UserSettings.user_id == user_id,
                UserSettings.category == category,
            )
        )
        row = result.scalar_one_or_none()
        stored = row.settings if row else {}
        return {**DEFAULT_SETTINGS[category], **stored}

    async def update_category(
        self, user_id: UUID, category: str, partial: dict[str, Any]
    ) -> dict[str, Any]:
        """Merge-update a category: validate partial, then upsert merged result."""
        self._validate_category(category)
        current = await self.get_category(user_id, category)
        merged = {**current, **partial}
        validated = self._validate_settings(category, merged)
        await self._upsert(user_id, category, validated)
        return await self.get_category(user_id, category)

    async def replace_category(
        self, user_id: UUID, category: str, settings: dict[str, Any]
    ) -> dict[str, Any]:
        """Full-replace a category: validate, then upsert."""
        self._validate_category(category)
        validated = self._validate_settings(category, settings)
        await self._upsert(user_id, category, validated)
        return await self.get_category(user_id, category)

    async def reset_category(self, user_id: UUID, category: str) -> None:
        """Delete stored row for a category (reads will return defaults)."""
        self._validate_category(category)
        await self._db.execute(
            delete(UserSettings).where(
                UserSettings.user_id == user_id,
                UserSettings.category == category,
            )
        )

    # --- Private helpers ---

    def _validate_category(self, category: str) -> None:
        if category not in VALID_CATEGORIES:
            raise SettingsServiceError(f"Unknown category: {category}")

    def _validate_settings(self, category: str, data: dict[str, Any]) -> dict:
        schema_cls = CATEGORY_SCHEMAS.get(category)
        if not schema_cls:
            return data
        try:
            instance = schema_cls(**data)
            return instance.model_dump()
        except ValidationError as exc:
            raise SettingsServiceError(str(exc)) from exc

    async def _upsert(
        self, user_id: UUID, category: str, settings: dict
    ) -> None:
        stmt = insert(UserSettings).values(
            user_id=user_id,
            category=category,
            settings=settings,
        )
        stmt = stmt.on_conflict_do_update(
            index_elements=["user_id", "category"],
            set_={"settings": stmt.excluded.settings},
        )
        await self._db.execute(stmt)
