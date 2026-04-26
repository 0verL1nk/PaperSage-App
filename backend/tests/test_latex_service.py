"""Tests for the LaTeX service and resource builder."""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from services.latex.clsi_client import (
    ClsiClient,
    ClsiCompileResult,
    ClsiOutputFile,
    ClsiSyncCodeResult,
    ClsiSyncPdfResult,
    ClsiWordcountResult,
)
from services.latex.resource_builder import (
    ProjectFile,
    build_resources,
    _is_binary_path,
)
from services.latex.service import LaTeXService, LaTeXServiceError


class TestResourceBuilder:
    def test_text_files_use_content(self) -> None:
        files = [ProjectFile(path="main.tex", content="\\documentclass{article}")]
        resources, root = build_resources(files, "main.tex")
        assert len(resources) == 1
        assert resources[0].content == "\\documentclass{article}"
        assert resources[0].url is None
        assert root == "main.tex"

    def test_binary_files_use_url(self) -> None:
        files = [ProjectFile(path="fig.png", url="http://example.com/fig.png")]
        resources, _ = build_resources(files)
        assert resources[0].content is None
        assert resources[0].url == "http://example.com/fig.png"

    def test_nested_paths_preserved(self) -> None:
        files = [
            ProjectFile(path="chapters/intro.tex", content="intro"),
            ProjectFile(path="images/fig.png", url="http://x.com/fig.png"),
        ]
        resources, _ = build_resources(files)
        assert resources[0].path == "chapters/intro.tex"
        assert resources[1].path == "images/fig.png"

    def test_root_file_validation(self) -> None:
        files = [ProjectFile(path="report.tex", content="content")]
        resources, root = build_resources(files, "main.tex")
        assert root == "report.tex"

    def test_binary_extension_detection(self) -> None:
        assert _is_binary_path("photo.png")
        assert _is_binary_path("photo.JPG")
        assert _is_binary_path("font.ttf")
        assert not _is_binary_path("main.tex")
        assert _is_binary_path("data.bib")  # bib files use URL refs

    def test_empty_files_list(self) -> None:
        resources, root = build_resources([])
        assert resources == []
        assert root == "main.tex"

    def test_explicit_binary_flag(self) -> None:
        files = [ProjectFile(path="data.csv", content="a,b,c", is_binary=True)]
        resources, _ = build_resources(files)
        assert resources[0].url == ""
        assert resources[0].content is None


class TestLaTeXService:
    @pytest.fixture
    def mock_client(self) -> AsyncMock:
        return AsyncMock(spec=ClsiClient)

    @pytest.fixture
    def service(self, mock_client: AsyncMock) -> LaTeXService:
        return LaTeXService(client=mock_client)

    @pytest.mark.asyncio
    async def test_compile_project_success(
        self,
        service: LaTeXService,
        mock_client: AsyncMock,
    ) -> None:
        expected = ClsiCompileResult(
            status="success",
            build_id="build-1",
            output_files=[ClsiOutputFile(path="output.pdf", url="/out.pdf")],
        )
        mock_client.compile.return_value = expected

        files = [ProjectFile(path="main.tex", content="hello")]
        result = await service.compile_project("proj-1", files, compiler="xelatex")

        assert result.status == "success"
        assert result.build_id == "build-1"
        mock_client.compile.assert_called_once()

    @pytest.mark.asyncio
    async def test_compile_project_empty_files(self, service: LaTeXService) -> None:
        with pytest.raises(LaTeXServiceError) as exc_info:
            await service.compile_project("proj-1", [])
        assert exc_info.value.status_code == 400

    @pytest.mark.asyncio
    async def test_compile_disabled(
        self,
        service: LaTeXService,
        mock_client: AsyncMock,
    ) -> None:
        with patch(
            "services.latex.service.get_settings",
            return_value=MagicMock(latex_clsi_enabled=False),
        ):
            with pytest.raises(LaTeXServiceError) as exc_info:
                await service.compile_project("proj-1", [ProjectFile(path="m.tex", content="x")])
            assert exc_info.value.status_code == 503

    @pytest.mark.asyncio
    async def test_stop_compile(self, service: LaTeXService, mock_client: AsyncMock) -> None:
        mock_client.stop_compile.return_value = True
        result = await service.stop_compile("proj-1")
        assert result is True

    @pytest.mark.asyncio
    async def test_clear_cache(self, service: LaTeXService, mock_client: AsyncMock) -> None:
        mock_client.clear_cache.return_value = True
        result = await service.clear_cache("proj-1")
        assert result is True

    @pytest.mark.asyncio
    async def test_get_wordcount(self, service: LaTeXService, mock_client: AsyncMock) -> None:
        expected = ClsiWordcountResult(texcount={"textWords": 42})
        mock_client.wordcount.return_value = expected
        result = await service.get_wordcount("proj-1", file="intro.tex")
        assert result.texcount["textWords"] == 42

    @pytest.mark.asyncio
    async def test_sync_code_to_pdf(self, service: LaTeXService, mock_client: AsyncMock) -> None:
        expected = ClsiSyncCodeResult(pdf=[])
        mock_client.sync_from_code.return_value = expected
        result = await service.sync_code_to_pdf("proj-1", "main.tex", 10, 5)
        assert isinstance(result, ClsiSyncCodeResult)

    @pytest.mark.asyncio
    async def test_sync_pdf_to_code(self, service: LaTeXService, mock_client: AsyncMock) -> None:
        expected = ClsiSyncPdfResult(code=[])
        mock_client.sync_from_pdf.return_value = expected
        result = await service.sync_pdf_to_code("proj-1", page=1, h=100.0, v=200.0)
        assert isinstance(result, ClsiSyncPdfResult)
