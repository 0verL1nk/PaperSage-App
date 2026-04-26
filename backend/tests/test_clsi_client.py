"""Tests for the CLSI client."""

from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest

from services.latex.clsi_client import (
    STATUS_SUCCESS,
    ClsiClient,
    ClsiCompileResult,
    ClsiResource,
    ClsiSyncCodeResult,
    ClsiSyncPdfResult,
    ClsiWordcountResult,
)


@pytest.fixture
def client() -> ClsiClient:
    return ClsiClient(base_url="http://localhost:3013", timeout=30)


def _mock_response(status_code: int, json_data: dict) -> httpx.Response:
    resp = MagicMock(spec=httpx.Response)
    resp.status_code = status_code
    resp.json.return_value = json_data
    resp.raise_for_status = MagicMock()
    return resp


class TestClsiResource:
    def test_resource_with_content(self) -> None:
        r = ClsiResource(path="main.tex", content="\\documentclass{article}")
        dumped = r.model_dump(exclude_none=True)
        assert dumped == {"path": "main.tex", "content": "\\documentclass{article}"}

    def test_resource_with_url(self) -> None:
        r = ClsiResource(path="images/fig.png", url="http://example.com/fig.png")
        dumped = r.model_dump(exclude_none=True)
        assert "content" not in dumped
        assert dumped["url"] == "http://example.com/fig.png"


class TestCompile:
    @pytest.mark.asyncio
    async def test_compile_success(self, client: ClsiClient) -> None:
        mock_resp = _mock_response(
            200,
            {
                "compile": {
                    "status": "success",
                    "buildId": "build-abc123",
                    "outputFiles": [
                        {
                            "path": "output.pdf",
                            "url": "/output.pdf",
                            "type": "pdf",
                            "build": "build-abc123",
                            "size": 12345,
                        },
                    ],
                    "outputUrlPrefix": "/build/build-abc123/output",
                },
            },
        )

        mock_client = AsyncMock()
        mock_client.post.return_value = mock_resp
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        with patch.object(httpx, "AsyncClient", return_value=mock_client):
            result = await client.compile(
                project_id="proj-1",
                resources=[ClsiResource(path="main.tex", content="hello")],
                root_resource_path="main.tex",
                compiler="pdflatex",
            )

        assert isinstance(result, ClsiCompileResult)
        assert result.status == STATUS_SUCCESS
        assert result.build_id == "build-abc123"
        assert len(result.output_files) == 1
        assert result.output_files[0].path == "output.pdf"

    @pytest.mark.asyncio
    async def test_compile_request_format(self, client: ClsiClient) -> None:
        mock_resp = _mock_response(200, {"compile": {"status": "success", "outputFiles": []}})
        mock_client = AsyncMock()
        mock_client.post.return_value = mock_resp
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        with patch.object(httpx, "AsyncClient", return_value=mock_client):
            await client.compile(
                project_id="proj-1",
                resources=[ClsiResource(path="main.tex", content="hello")],
                compiler="xelatex",
            )

        call_args = mock_client.post.call_args
        body = call_args.kwargs.get("json", call_args[1].get("json"))
        assert body["compile"]["options"]["compiler"] == "xelatex"
        assert body["compile"]["rootResourcePath"] == "main.tex"
        assert len(body["compile"]["resources"]) == 1

    @pytest.mark.asyncio
    async def test_compile_failure_status(self, client: ClsiClient) -> None:
        mock_resp = _mock_response(
            200,
            {
                "compile": {
                    "status": "failure",
                    "outputFiles": [],
                    "error": "Compilation failed",
                },
            },
        )
        mock_client = AsyncMock()
        mock_client.post.return_value = mock_resp
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        with patch.object(httpx, "AsyncClient", return_value=mock_client):
            result = await client.compile("proj-1", [])

        assert result.status == "failure"
        assert result.error == "Compilation failed"


class TestStopAndClear:
    @pytest.mark.asyncio
    async def test_stop_compile(self, client: ClsiClient) -> None:
        mock_resp = _mock_response(204, {})
        mock_client = AsyncMock()
        mock_client.post.return_value = mock_resp
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        with patch.object(httpx, "AsyncClient", return_value=mock_client):
            result = await client.stop_compile("proj-1")

        assert result is True
        mock_client.post.assert_called_once_with("/project/proj-1/compile/stop")

    @pytest.mark.asyncio
    async def test_clear_cache(self, client: ClsiClient) -> None:
        mock_resp = _mock_response(204, {})
        mock_client = AsyncMock()
        mock_client.delete.return_value = mock_resp
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        with patch.object(httpx, "AsyncClient", return_value=mock_client):
            result = await client.clear_cache("proj-1")

        assert result is True
        mock_client.delete.assert_called_once_with("/project/proj-1")


class TestSyncTeX:
    @pytest.mark.asyncio
    async def test_sync_from_code(self, client: ClsiClient) -> None:
        mock_resp = _mock_response(
            200,
            {
                "pdf": [{"page": 1, "h": 100.0, "v": 200.0, "width": 50.0, "height": 10.0}],
            },
        )
        mock_client = AsyncMock()
        mock_client.get.return_value = mock_resp
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        with patch.object(httpx, "AsyncClient", return_value=mock_client):
            result = await client.sync_from_code("proj-1", "main.tex", 10, 5)

        assert isinstance(result, ClsiSyncCodeResult)
        assert len(result.pdf) == 1
        assert result.pdf[0].page == 1

    @pytest.mark.asyncio
    async def test_sync_from_pdf(self, client: ClsiClient) -> None:
        mock_resp = _mock_response(
            200,
            {
                "code": [{"file": "main.tex", "line": 42, "column": 1}],
            },
        )
        mock_client = AsyncMock()
        mock_client.get.return_value = mock_resp
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        with patch.object(httpx, "AsyncClient", return_value=mock_client):
            result = await client.sync_from_pdf("proj-1", page=1, h=100.0, v=200.0)

        assert isinstance(result, ClsiSyncPdfResult)
        assert len(result.code) == 1
        assert result.code[0].file == "main.tex"
        assert result.code[0].line == 42


class TestWordcount:
    @pytest.mark.asyncio
    async def test_wordcount(self, client: ClsiClient) -> None:
        mock_resp = _mock_response(
            200,
            {
                "texcount": {"textWords": 100, "headers": 10, "mathDisplay": 5},
            },
        )
        mock_client = AsyncMock()
        mock_client.get.return_value = mock_resp
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        with patch.object(httpx, "AsyncClient", return_value=mock_client):
            result = await client.wordcount("proj-1", file="main.tex")

        assert isinstance(result, ClsiWordcountResult)
        assert result.texcount["textWords"] == 100
