"""Integration tests for /api/v1/settings routes.

Uses httpx.AsyncClient with the FastAPI app via ASGITransport, with
get_db and get_current_user overridden so we can drive the full HTTP
stack against a fresh in-memory SQLite database per test.

These tests cover the routes that test_settings_api.py cannot:
- actual request/response envelopes
- DB persistence across requests
- auth gate (401)
- validation errors (422)
- 404 for unknown categories
"""

from __future__ import annotations

import pytest
from httpx import ASGITransport, AsyncClient

from api.dependencies import get_db
from app.main import app

AUTHED_USER_ID = "11111111-1111-1111-1111-111111111111"


def _unwrap(response_json: dict) -> dict:
    """Strip the ApiResponse envelope and return data."""
    assert "code" in response_json and "data" in response_json
    assert response_json["code"] == 200, f"Expected success, got {response_json}"
    return response_json["data"]


@pytest.mark.asyncio
class TestGetAllSettings:
    async def test_returns_envelope_with_all_categories(self, client: AsyncClient) -> None:
        r = await client.get("/api/v1/settings")
        assert r.status_code == 200
        body = r.json()
        assert body["code"] == 200
        data = body["data"]["settings"]
        assert set(data.keys()) == {"ui", "editor", "agent"}
        # Defaults for a fresh user
        assert data["ui"]["theme"] == "system"
        assert data["editor"]["font_size"] == 14
        assert data["agent"]["writing_style"] == "academic"


@pytest.mark.asyncio
class TestGetCategory:
    async def test_returns_ui_defaults_when_no_row(self, client: AsyncClient) -> None:
        r = await client.get("/api/v1/settings/ui")
        assert r.status_code == 200
        data = _unwrap(r.json())
        assert data["category"] == "ui"
        assert data["settings"]["theme"] == "system"
        assert data["settings"]["language"] == "zh-CN"

    async def test_returns_editor_defaults_when_no_row(self, client: AsyncClient) -> None:
        r = await client.get("/api/v1/settings/editor")
        assert r.status_code == 200
        data = _unwrap(r.json())
        assert data["settings"]["auto_save"] is True

    async def test_unknown_category_returns_404(self, client: AsyncClient) -> None:
        r = await client.get("/api/v1/settings/nonexistent")
        assert r.status_code == 404


@pytest.mark.asyncio
class TestPatchCategory:
    async def test_merges_partial_update(self, client: AsyncClient) -> None:
        r = await client.patch(
            "/api/v1/settings/ui",
            json={"settings": {"theme": "dark"}},
        )
        assert r.status_code == 200
        data = _unwrap(r.json())
        # New value
        assert data["settings"]["theme"] == "dark"
        # Defaults preserved
        assert data["settings"]["language"] == "zh-CN"
        assert data["settings"]["sidebar_collapsed"] is False

    async def test_persists_across_gets(self, client: AsyncClient) -> None:
        await client.patch(
            "/api/v1/settings/ui",
            json={"settings": {"theme": "dark"}},
        )
        r = await client.get("/api/v1/settings/ui")
        data = _unwrap(r.json())
        assert data["settings"]["theme"] == "dark"

    async def test_subsequent_patch_does_not_clobber(self, client: AsyncClient) -> None:
        await client.patch(
            "/api/v1/settings/ui",
            json={"settings": {"theme": "dark"}},
        )
        await client.patch(
            "/api/v1/settings/ui",
            json={"settings": {"language": "en-US"}},
        )
        r = await client.get("/api/v1/settings/ui")
        data = _unwrap(r.json())
        assert data["settings"]["theme"] == "dark"
        assert data["settings"]["language"] == "en-US"

    async def test_invalid_value_returns_422(self, client: AsyncClient) -> None:
        r = await client.patch(
            "/api/v1/settings/ui",
            json={"settings": {"theme": "neon"}},
        )
        assert r.status_code == 422
        body = r.json()
        # Project uses a custom validation_exception_handler that wraps
        # the error in the standard ApiResponse envelope.
        assert body["code"] == 422
        assert "UISettings" in body["info"]
        assert "theme" in body["info"]


@pytest.mark.asyncio
class TestPutCategory:
    async def test_full_replacement(self, client: AsyncClient) -> None:
        full = {
            "settings": {
                "theme": "light",
                "language": "en-US",
                "sidebar_collapsed": True,
            }
        }
        r = await client.put("/api/v1/settings/ui", json=full)
        assert r.status_code == 200
        data = _unwrap(r.json())
        assert data["settings"] == full["settings"]

        # And a subsequent GET returns the same
        r2 = await client.get("/api/v1/settings/ui")
        assert _unwrap(r2.json())["settings"] == full["settings"]


@pytest.mark.asyncio
class TestDeleteCategory:
    async def test_resets_to_defaults(self, client: AsyncClient) -> None:
        # First persist a non-default value
        await client.patch(
            "/api/v1/settings/ui",
            json={"settings": {"theme": "dark"}},
        )
        # Then reset
        r = await client.delete("/api/v1/settings/ui")
        assert r.status_code == 200
        # GET returns defaults again
        r2 = await client.get("/api/v1/settings/ui")
        data = _unwrap(r2.json())
        assert data["settings"]["theme"] == "system"


@pytest.mark.asyncio
class TestAuthGate:
    async def test_unauthenticated_request_returns_401(self, db_session) -> None:
        """Without the get_current_user override, requests must 401."""

        async def _override_get_db():
            try:
                yield db_session
                await db_session.commit()
            except Exception:
                await db_session.rollback()
                raise

        app.dependency_overrides[get_db] = _override_get_db
        # Note: deliberately NOT overriding get_current_user.
        try:
            transport = ASGITransport(app=app)
            async with AsyncClient(
                transport=transport, base_url="http://test"
            ) as ac:
                r = await ac.get("/api/v1/settings")
                assert r.status_code == 401
        finally:
            app.dependency_overrides.clear()
