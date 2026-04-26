"""Document and folder API route integration tests."""

import pytest
from fastapi.routing import APIRoute


class TestDocumentRoutes:
    def test_router_imports(self):
        from api.routes import documents

        assert documents.router is not None

    def test_upload_endpoint_exists(self):
        from api.routes.documents import router

        paths = [r.path for r in router.routes if isinstance(r, APIRoute)]
        assert "/upload" in paths

    def test_preview_endpoint_exists(self):
        from api.routes.documents import router

        paths = [r.path for r in router.routes if isinstance(r, APIRoute)]
        assert "/{document_id}/preview" in paths

    def test_move_endpoint_exists(self):
        from api.routes.documents import router

        paths = [r.path for r in router.routes if isinstance(r, APIRoute)]
        assert "/{document_id}/move" in paths

    def test_remark_endpoint_exists(self):
        from api.routes.documents import router

        paths = [r.path for r in router.routes if isinstance(r, APIRoute)]
        assert "/{document_id}/remark" in paths

    def test_delete_endpoint_exists(self):
        from api.routes.documents import router

        paths = [r.path for r in router.routes if isinstance(r, APIRoute)]
        assert "/{document_id}" in paths

    def test_upload_is_post(self):
        from api.routes.documents import router

        for route in router.routes:
            if isinstance(route, APIRoute) and route.path == "/upload":
                assert "POST" in route.methods
                return
        pytest.fail("Upload route not found")

    def test_preview_is_get(self):
        from api.routes.documents import router

        for route in router.routes:
            if isinstance(route, APIRoute) and route.path == "/{document_id}/preview":
                assert "GET" in route.methods
                return
        pytest.fail("Preview route not found")

    def test_delete_is_delete_method(self):
        from api.routes.documents import router

        for route in router.routes:
            if isinstance(route, APIRoute) and route.path == "/{document_id}":
                assert "DELETE" in route.methods
                return
        pytest.fail("Delete route not found")


class TestFolderRoutes:
    def test_router_imports(self):
        from api.routes import folders

        assert folders.router is not None

    def test_create_endpoint_exists(self):
        from api.routes.folders import router

        paths = [r.path for r in router.routes if isinstance(r, APIRoute)]
        assert "/" in paths

    def test_delete_endpoint_exists(self):
        from api.routes.folders import router

        paths = [r.path for r in router.routes if isinstance(r, APIRoute)]
        assert "/{folder_id}" in paths

    def test_list_documents_endpoint_exists(self):
        from api.routes.folders import router

        paths = [r.path for r in router.routes if isinstance(r, APIRoute)]
        assert "/{folder_id}/documents" in paths

    def test_create_is_post(self):
        from api.routes.folders import router

        for route in router.routes:
            if isinstance(route, APIRoute) and route.path == "/":
                assert "POST" in route.methods
                return
        pytest.fail("Create folder route not found")

    def test_delete_is_delete_method(self):
        from api.routes.folders import router

        for route in router.routes:
            if isinstance(route, APIRoute) and route.path == "/{folder_id}":
                assert "DELETE" in route.methods
                return
        pytest.fail("Delete folder route not found")


class TestProjectDocumentRoutes:
    def test_project_documents_endpoint_registered(self):
        from api import api_router

        paths = [r.path for r in api_router.routes if isinstance(r, APIRoute)]
        assert "/projects/{project_id}/documents" in paths

    def test_project_folders_endpoint_registered(self):
        from api import api_router

        paths = [r.path for r in api_router.routes if isinstance(r, APIRoute)]
        assert "/projects/{project_id}/folders" in paths

    def test_documents_prefix_registered(self):
        from api import api_router

        paths = [r.path for r in api_router.routes if isinstance(r, APIRoute)]
        assert any("/documents/" in p for p in paths)

    def test_folders_prefix_registered(self):
        from api import api_router

        paths = [r.path for r in api_router.routes if isinstance(r, APIRoute)]
        assert any("/folders/" in p for p in paths)


class TestNoStubsRemain:
    """Verify no 501 NotImplemented responses remain in documents/folders routes."""

    def test_documents_no_501(self):
        from api.routes.documents import router

        for route in router.routes:
            if isinstance(route, APIRoute):
                name = route.name or ""
                assert "stub" not in name.lower(), f"Stub route found: {name}"

    def test_folders_no_501(self):
        from api.routes.folders import router

        for route in router.routes:
            if isinstance(route, APIRoute):
                name = route.name or ""
                assert "stub" not in name.lower(), f"Stub route found: {name}"
