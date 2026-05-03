"""LaTeX files API route tests."""

import pytest

from schemas.latex_file import FolderDeleteRequest, FolderDeleteResponse, FolderRenameRequest, FolderRenameResponse
from services.latex.file_service import BLOCKED_EXTENSIONS, _validate_path


class TestFolderDeleteSchemas:
    def test_folder_delete_request(self):
        req = FolderDeleteRequest(folder_path="sections")
        assert req.folder_path == "sections"

    def test_folder_delete_request_strips_trailing_slash(self):
        req = FolderDeleteRequest(folder_path="sections/")
        assert req.folder_path == "sections"

    def test_folder_delete_request_rejects_empty(self):
        with pytest.raises(Exception):
            FolderDeleteRequest(folder_path="")

    def test_folder_delete_response(self):
        resp = FolderDeleteResponse(deleted_count=3, folder_path="sections")
        assert resp.deleted_count == 3
        assert resp.folder_path == "sections"


class TestFolderRenameSchemas:
    def test_rename_request(self):
        req = FolderRenameRequest(folder_path="sections", new_folder="chapters")
        assert req.folder_path == "sections"
        assert req.new_folder == "chapters"

    def test_rename_request_rejects_empty_folder_path(self):
        with pytest.raises(Exception):
            FolderRenameRequest(folder_path="", new_folder="chapters")

    def test_rename_request_rejects_empty_new_folder(self):
        with pytest.raises(Exception):
            FolderRenameRequest(folder_path="sections", new_folder="")

    def test_rename_response(self):
        resp = FolderRenameResponse(renamed_count=3, old_folder="sections", new_folder="chapters")
        assert resp.renamed_count == 3
        assert resp.old_folder == "sections"
        assert resp.new_folder == "chapters"


class TestMoveAndRenameSchemas:
    def test_rename_schema(self):
        from schemas.latex_file import LatexFileRenameRequest

        req = LatexFileRenameRequest(new_path="sections/intro_v2.tex")
        assert req.new_path == "sections/intro_v2.tex"

    def test_rename_schema_rejects_empty(self):
        from schemas.latex_file import LatexFileRenameRequest

        with pytest.raises(Exception):
            LatexFileRenameRequest(new_path="")

    def test_move_schema(self):
        from schemas.latex_file import LatexFileMoveRequest

        req = LatexFileMoveRequest(new_dir="chapters")
        assert req.new_dir == "chapters"

    def test_move_schema_accepts_empty_as_root(self):
        from schemas.latex_file import LatexFileMoveRequest

        req = LatexFileMoveRequest(new_dir="")
        assert req.new_dir == ""


class TestPathValidation:
    def test_valid_tex_path(self):
        assert _validate_path("main.tex") == "main.tex"

    def test_valid_nested_path(self):
        assert _validate_path("chapters/intro.tex") == "chapters/intro.tex"

    def test_valid_bib_path(self):
        assert _validate_path("refs.bib") == "refs.bib"

    def test_valid_cls_path(self):
        assert _validate_path("template.cls") == "template.cls"

    def test_valid_sty_path(self):
        assert _validate_path("custom.sty") == "custom.sty"

    def test_valid_bst_path(self):
        assert _validate_path("style.bst") == "style.bst"

    def test_valid_txt_path(self):
        assert _validate_path("notes.txt") == "notes.txt"

    def test_valid_json_path(self):
        assert _validate_path("config.json") == "config.json"

    def test_valid_md_path(self):
        assert _validate_path("readme.md") == "readme.md"

    def test_valid_pdf_path(self):
        assert _validate_path("output.pdf") == "output.pdf"

    def test_valid_png_path(self):
        assert _validate_path("image.png") == "image.png"

    def test_valid_zip_path(self):
        assert _validate_path("archive.zip") == "archive.zip"

    def test_reject_traversal(self):
        with pytest.raises(ValueError, match="traversal"):
            _validate_path("../etc/passwd")

    def test_reject_traversal_complex(self):
        with pytest.raises(ValueError, match="traversal"):
            _validate_path("foo/../../bar.tex")

    def test_reject_exe_extension(self):
        with pytest.raises(ValueError, match="not allowed"):
            _validate_path("malware.exe")

    def test_strips_leading_slash(self):
        assert _validate_path("/main.tex") == "main.tex"


class TestLatexFilesRouter:
    def test_router_imports(self):
        from api.routes import latex_files

        assert latex_files.router is not None

    def test_schemas_imports(self):
        from schemas.latex_file import (
            FolderDeleteRequest,
            FolderDeleteResponse,
            FolderRenameRequest,
            FolderRenameResponse,
            LatexFileContentResponse,
            LatexFileCreateRequest,
            LatexFileListResponse,
            LatexFileMoveRequest,
            LatexFileRenameRequest,
            LatexFileResponse,
            LatexFileUpdateRequest,
        )

        assert LatexFileCreateRequest.model_fields is not None
        assert LatexFileUpdateRequest.model_fields is not None
        assert LatexFileResponse.model_fields is not None
        assert LatexFileListResponse.model_fields is not None
        assert LatexFileContentResponse.model_fields is not None
        assert LatexFileRenameRequest.model_fields is not None
        assert LatexFileMoveRequest.model_fields is not None
        assert FolderDeleteRequest.model_fields is not None
        assert FolderDeleteResponse.model_fields is not None
        assert FolderRenameRequest.model_fields is not None
        assert FolderRenameResponse.model_fields is not None

    def test_blocked_extensions(self):
        assert ".exe" in BLOCKED_EXTENSIONS
        assert ".dll" in BLOCKED_EXTENSIONS
        assert ".so" in BLOCKED_EXTENSIONS
