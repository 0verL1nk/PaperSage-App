"""Tests for SettingsService logic (schema validation, defaults merging)."""

from __future__ import annotations

import pytest

from schemas.user_settings import (
    AgentSettings,
    CATEGORY_SCHEMAS,
    DEFAULT_SETTINGS,
    EditorSettings,
    UISettings,
    VALID_CATEGORIES,
)
from services.settings_service import SettingsService, SettingsServiceError


class TestDefaultSettings:
    """DEFAULT_SETTINGS constant tests."""

    def test_all_categories_present(self) -> None:
        assert set(DEFAULT_SETTINGS.keys()) == {"ui", "editor", "agent"}

    def test_ui_defaults(self) -> None:
        ui = DEFAULT_SETTINGS["ui"]
        assert ui["theme"] == "system"
        assert ui["language"] == "zh-CN"
        assert ui["sidebar_collapsed"] is False

    def test_editor_defaults(self) -> None:
        ed = DEFAULT_SETTINGS["editor"]
        assert ed["font_size"] == 14
        assert ed["tab_size"] == 2
        assert ed["auto_save"] is True
        assert ed["word_wrap"] is True

    def test_agent_defaults(self) -> None:
        ag = DEFAULT_SETTINGS["agent"]
        assert ag["model_name"] == "deepseek"
        assert ag["temperature"] == 0.7
        assert ag["writing_style"] == "academic"
        assert ag["memory_enabled"] is True


class TestCategorySchemas:
    """Pydantic validation per category."""

    def test_valid_ui(self) -> None:
        s = UISettings(theme="dark", language="en-US", sidebar_collapsed=True)
        assert s.theme == "dark"

    def test_invalid_theme(self) -> None:
        with pytest.raises(Exception):
            UISettings(theme="blue")

    def test_valid_editor(self) -> None:
        s = EditorSettings(font_size=20, tab_size=4)
        assert s.font_size == 20

    def test_font_size_too_small(self) -> None:
        with pytest.raises(Exception):
            EditorSettings(font_size=1)

    def test_font_size_too_large(self) -> None:
        with pytest.raises(Exception):
            EditorSettings(font_size=50)

    def test_valid_agent(self) -> None:
        s = AgentSettings(temperature=1.5)
        assert s.temperature == 1.5

    def test_temperature_out_of_range(self) -> None:
        with pytest.raises(Exception):
            AgentSettings(temperature=3.0)

    def test_negative_temperature(self) -> None:
        with pytest.raises(Exception):
            AgentSettings(temperature=-0.5)

    def test_category_schemas_map_matches_valid_categories(self) -> None:
        assert set(CATEGORY_SCHEMAS.keys()) == set(VALID_CATEGORIES)

    def test_partial_update_keeps_defaults(self) -> None:
        s = EditorSettings(font_size=20)
        assert s.tab_size == 2  # default preserved
        assert s.auto_save is True


class TestSettingsServiceValidation:
    """SettingsService._validate_category and _validate_settings unit tests."""

    def setup_method(self) -> None:
        # Service without a real DB session — only testing validation methods
        self.svc = SettingsService(db=None)  # type: ignore[arg-type]

    def test_validate_unknown_category_raises(self) -> None:
        with pytest.raises(SettingsServiceError, match="Unknown category"):
            self.svc._validate_category("nonexistent")

    def test_validate_valid_category_passes(self) -> None:
        for cat in VALID_CATEGORIES:
            self.svc._validate_category(cat)  # no raise

    def test_validate_settings_valid(self) -> None:
        result = self.svc._validate_settings("ui", {"theme": "dark"})
        assert result["theme"] == "dark"
        assert result["language"] == "zh-CN"

    def test_validate_settings_invalid_field(self) -> None:
        with pytest.raises(SettingsServiceError):
            self.svc._validate_settings("editor", {"font_size": "huge"})

    def test_validate_settings_extra_fields_stripped(self) -> None:
        result = self.svc._validate_settings("ui", {"theme": "light", "language": "en-US"})
        assert "unknown_field" not in result
