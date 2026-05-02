"""Tests for settings API routes (schema validation and route structure)."""

from __future__ import annotations

import pytest

from schemas.user_settings import (
    AllSettingsResponse,
    SettingsResponse,
    SettingsUpdateRequest,
    VALID_CATEGORIES,
)


class TestSettingsSchemas:
    """Request/response schema tests."""

    def test_settings_update_request_empty(self) -> None:
        req = SettingsUpdateRequest()
        assert req.settings == {}

    def test_settings_update_request_with_data(self) -> None:
        req = SettingsUpdateRequest(settings={"theme": "dark"})
        assert req.settings == {"theme": "dark"}

    def test_settings_response(self) -> None:
        resp = SettingsResponse(category="ui", settings={"theme": "dark"})
        assert resp.category == "ui"
        assert resp.settings["theme"] == "dark"

    def test_all_settings_response(self) -> None:
        resp = AllSettingsResponse(
            settings={
                "ui": {"theme": "system"},
                "editor": {"font_size": 14},
                "agent": {"model_name": "deepseek"},
            }
        )
        assert len(resp.settings) == 3
        assert resp.settings["ui"]["theme"] == "system"


class TestValidCategories:
    """Category validation tests."""

    def test_valid_categories_tuple(self) -> None:
        assert VALID_CATEGORIES == ("ui", "editor", "agent")

    def test_category_count(self) -> None:
        assert len(VALID_CATEGORIES) == 3


class TestRouteStructure:
    """Verify the router is properly defined."""

    def test_router_exists(self) -> None:
        from api.routes.settings import router
        assert router is not None

    def test_router_has_expected_routes(self) -> None:
        from api.routes.settings import router
        routes = [r.path for r in router.routes]
        assert "" in routes  # GET /settings (empty path relative to prefix)
        assert "/{category}" in routes
