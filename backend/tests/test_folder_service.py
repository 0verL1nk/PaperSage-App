"""Folder service and model tests."""

import pytest


class TestFolderModel:
    def test_imports(self):
        from models.folder import Folder

        assert Folder.__tablename__ == "folders"

    def test_schema_imports(self):
        from schemas import FolderCreate, FolderResponse

        assert FolderCreate.model_fields is not None
        assert FolderResponse.model_fields is not None


class TestFolderCreateSchema:
    def test_valid_folder(self):
        from schemas import FolderCreate

        folder = FolderCreate(name="Papers", project_id="00000000-0000-0000-0000-000000000001")
        assert folder.name == "Papers"
        assert folder.parent_id is None

    def test_with_parent(self):
        from schemas import FolderCreate

        folder = FolderCreate(
            name="Subfolder",
            project_id="00000000-0000-0000-0000-000000000001",
            parent_id="00000000-0000-0000-0000-000000000002",
        )
        assert folder.parent_id is not None

    def test_reject_empty_name(self):
        from pydantic import ValidationError

        from schemas import FolderCreate

        with pytest.raises(ValidationError):
            FolderCreate(name="", project_id="00000000-0000-0000-0000-000000000001")

    def test_reject_missing_project(self):
        from pydantic import ValidationError

        from schemas import FolderCreate

        with pytest.raises(ValidationError):
            FolderCreate(name="Orphan")


class TestFolderResponseSchema:
    def test_from_attributes(self):
        from schemas import FolderResponse

        assert "from_attributes" in str(FolderResponse.model_config)
        field_names = set(FolderResponse.model_fields)
        expected = {"id", "name", "parent_id", "project_id", "owner_id", "created_at", "updated_at"}
        assert expected == field_names


class TestFolderServiceImport:
    def test_service_imports(self):
        from services.documents.folder_service import FolderService, verify_project_access

        assert FolderService is not None
        assert verify_project_access is not None

    def test_service_has_methods(self):
        from services.documents.folder_service import FolderService

        svc = FolderService()
        assert hasattr(svc, "list_by_project")
        assert hasattr(svc, "create")
        assert hasattr(svc, "delete")
