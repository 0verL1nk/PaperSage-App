"""Test project creation auto-template."""

import pytest

from services.latex.file_service import _validate_path


class TestProjectTemplate:
    def test_default_template_is_valid_tex(self):
        """Verify the default template path passes validation."""
        path = _validate_path("main.tex")
        assert path == "main.tex"

    def test_file_service_imports(self):
        from services.latex.file_service import LatexFileService

        assert LatexFileService is not None

    def test_file_model_imports(self):
        from models.latex_file import LatexFile

        assert LatexFile.__tablename__ == "latex_files"
