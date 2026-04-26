"""LaTeX files API route tests."""

import pytest

from services.latex.file_service import BLOCKED_EXTENSIONS, _validate_path


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
            LatexFileContentResponse,
            LatexFileCreateRequest,
            LatexFileListResponse,
            LatexFileResponse,
            LatexFileUpdateRequest,
        )

        assert LatexFileCreateRequest.model_fields is not None
        assert LatexFileUpdateRequest.model_fields is not None
        assert LatexFileResponse.model_fields is not None
        assert LatexFileListResponse.model_fields is not None
        assert LatexFileContentResponse.model_fields is not None

    def test_blocked_extensions(self):
        assert ".exe" in BLOCKED_EXTENSIONS
        assert ".dll" in BLOCKED_EXTENSIONS
        assert ".so" in BLOCKED_EXTENSIONS
