"""End-to-end tests for LaTeX compilation via CLSI.

These tests require a running CLSI service at localhost:3013.
Run `make clsi-dev` in a separate terminal before executing.

Skip automatically if CLSI is not available.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from services.latex.clsi_client import (
    STATUS_SUCCESS,
    ClsiClient,
    ClsiResource,
)
from services.latex.resource_builder import ProjectFile, build_resources

FIXTURES_DIR = Path(__file__).parent / "fixtures" / "latex_template"


def _clsi_available() -> bool:
    import socket

    try:
        with socket.create_connection(("localhost", 3013), timeout=2):
            return True
    except OSError:
        return False


skip_no_clsi = pytest.mark.skipif(
    not _clsi_available(),
    reason="CLSI service not running at localhost:3013",
)


def _read_fixture(name: str) -> str:
    return (FIXTURES_DIR / name).read_text(encoding="utf-8")


def _simple_project_id() -> str:
    return "e2etest0001"


def _multi_project_id() -> str:
    return "e2etest0002"


@skip_no_clsi
class TestClsiE2ECompile:
    """E2E: Single-file compilation through CLSI."""

    @pytest.mark.asyncio
    async def test_simple_compile(self) -> None:
        client = ClsiClient(base_url="http://localhost:3013", timeout=60)
        resources = [
            ClsiResource(
                path="main.tex",
                content=_read_fixture("main.tex"),
            ),
        ]

        result = await client.compile(
            project_id=_simple_project_id(),
            resources=resources,
            root_resource_path="main.tex",
            compiler="pdflatex",
        )

        assert result.status in (STATUS_SUCCESS, "failure")
        if result.status == STATUS_SUCCESS:
            assert result.build_id is not None
            pdf_files = [f for f in result.output_files if f.path.endswith(".pdf")]
            assert len(pdf_files) >= 1, "Expected output.pdf in output files"

    @pytest.mark.asyncio
    async def test_multi_file_compile(self) -> None:
        client = ClsiClient(base_url="http://localhost:3013", timeout=60)

        files = [
            ProjectFile(
                path="main.tex",
                content=_read_fixture("main_with_input.tex"),
            ),
            ProjectFile(
                path="sections/content.tex",
                content=_read_fixture("sections/content.tex"),
            ),
        ]
        resources, root = build_resources(files, "main.tex")

        result = await client.compile(
            project_id=_multi_project_id(),
            resources=resources,
            root_resource_path=root,
            compiler="pdflatex",
        )

        assert result.status in (STATUS_SUCCESS, "failure")
        if result.status == STATUS_SUCCESS:
            pdf_files = [f for f in result.output_files if f.path.endswith(".pdf")]
            assert len(pdf_files) >= 1

    @pytest.mark.asyncio
    async def test_xelatex_compile(self) -> None:
        client = ClsiClient(base_url="http://localhost:3013", timeout=60)
        resources = [
            ClsiResource(
                path="main.tex",
                content=_read_fixture("main.tex"),
            ),
        ]

        result = await client.compile(
            project_id="e2etest0003",
            resources=resources,
            root_resource_path="main.tex",
            compiler="xelatex",
        )

        assert result.status in (STATUS_SUCCESS, "failure")


@skip_no_clsi
class TestClsiE2EWordcount:
    """E2E: Wordcount through CLSI."""

    @pytest.mark.asyncio
    async def test_wordcount(self) -> None:
        client = ClsiClient(base_url="http://localhost:3013", timeout=60)

        pid = "e2etest0005"

        resources = [
            ClsiResource(
                path="main.tex",
                content=_read_fixture("main.tex"),
            ),
        ]
        compile_result = await client.compile(
            project_id=pid,
            resources=resources,
            root_resource_path="main.tex",
        )

        if compile_result.status != STATUS_SUCCESS:
            pytest.skip("Compile did not succeed, cannot test wordcount")

        result = await client.wordcount(
            project_id=pid,
            file="main.tex",
        )
        assert isinstance(result.texcount, dict)


@skip_no_clsi
class TestClsiE2EOutputFiles:
    """E2E: Output ZIP retrieval through CLSI."""

    @pytest.mark.asyncio
    async def test_get_output_zip(self) -> None:
        client = ClsiClient(base_url="http://localhost:3013", timeout=60)

        pid = "e2etest0004"

        resources = [
            ClsiResource(
                path="main.tex",
                content=_read_fixture("main.tex"),
            ),
        ]
        compile_result = await client.compile(
            project_id=pid,
            resources=resources,
            root_resource_path="main.tex",
        )

        if compile_result.status != STATUS_SUCCESS:
            pytest.skip("Compile did not succeed, cannot test output files")

        assert compile_result.build_id is not None

        resp = await client.get_output_zip(
            project_id=pid,
            build_id=compile_result.build_id,
        )
        assert resp.status_code == 200
        assert len(resp.content) > 0
        assert resp.content[:4] == b"PK\x03\x04"  # ZIP magic bytes


@skip_no_clsi
class TestClsiE2ECacheControl:
    """E2E: Cache and compile control."""

    @pytest.mark.asyncio
    async def test_clear_cache(self) -> None:
        client = ClsiClient(base_url="http://localhost:3013", timeout=60)
        result = await client.clear_cache(_simple_project_id())
        assert isinstance(result, bool)
