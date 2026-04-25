"""Tests for LaTeX API route authentication and schema validation."""

import pytest

from schemas.latex import (
    LatexCompileRequest,
    LatexFileInput,
    LatexSyncCodeRequest,
    LatexSyncPdfRequest,
    VALID_COMPILERS,
)


class TestLatexSchemas:
    def test_compile_request_defaults(self) -> None:
        req = LatexCompileRequest(
            files=[LatexFileInput(path="main.tex", content="\\documentclass{article}")],
        )
        assert req.root_file == "main.tex"
        assert req.compiler == "pdflatex"
        assert len(req.files) == 1

    def test_compile_request_with_xelatex(self) -> None:
        req = LatexCompileRequest(
            root_file="report.tex",
            compiler="xelatex",
            files=[LatexFileInput(path="report.tex", content="content")],
        )
        assert req.compiler == "xelatex"
        assert req.root_file == "report.tex"

    def test_file_input_binary(self) -> None:
        f = LatexFileInput(path="fig.png", url="http://example.com/fig.png", is_binary=True)
        assert f.is_binary is True
        assert f.url == "http://example.com/fig.png"

    def test_valid_compilers(self) -> None:
        assert "pdflatex" in VALID_COMPILERS
        assert "xelatex" in VALID_COMPILERS
        assert "lualatex" in VALID_COMPILERS

    def test_sync_code_request_validation(self) -> None:
        from pydantic import ValidationError

        with pytest.raises(ValidationError):
            LatexSyncCodeRequest(file="main.tex", line=-1, column=0)

    def test_sync_pdf_request_validation(self) -> None:
        from pydantic import ValidationError

        with pytest.raises(ValidationError):
            LatexSyncPdfRequest(page=0, h=1.0, v=1.0)

    def test_compile_request_requires_files(self) -> None:
        from pydantic import ValidationError

        with pytest.raises(ValidationError):
            LatexCompileRequest()


class TestLatexRouteImports:
    def test_latex_router_imports(self) -> None:
        from api.routes import latex

        assert latex.router is not None
        assert hasattr(latex, "compile_project")
        assert hasattr(latex, "stop_compile")
        assert hasattr(latex, "clear_cache")
        assert hasattr(latex, "get_wordcount")
        assert hasattr(latex, "sync_code_to_pdf")
        assert hasattr(latex, "sync_pdf_to_code")
        assert hasattr(latex, "get_output_zip")

    def test_latex_router_registered(self) -> None:
        from api import api_router

        latex_routes = [r.path for r in api_router.routes]
        assert any("/compile" in str(r) for r in latex_routes)
