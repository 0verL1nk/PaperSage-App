"""CLSI HTTP client for communicating with the vendored Overleaf CLSI service."""

from __future__ import annotations

from typing import Any

import httpx
from pydantic import BaseModel, Field

from app.config import get_settings

# Compile status constants
STATUS_SUCCESS = "success"
STATUS_FAILURE = "failure"
STATUS_COMPILED_IN_PROGRESS = "compile-in-progress"
STATUS_CONFLICT = "conflict"
STATUS_TIMEDOUT = "timedout"
STATUS_ERROR = "error"
STATUS_TERMINATED = "terminated"
STATUS_UNAVAILABLE = "unavailable"


class ClsiResource(BaseModel):
    """A single resource in a CLSI compile request."""

    path: str
    content: str | None = None
    url: str | None = None


class ClsiOutputFile(BaseModel):
    """Output file metadata from a CLSI compile response."""

    path: str
    url: str
    type: str = ""
    build: str = ""
    size: int = 0


class ClsiCompileResult(BaseModel):
    """Parsed result from a CLSI compile response."""

    status: str
    build_id: str | None = None
    output_files: list[ClsiOutputFile] = Field(default_factory=list)
    logs: str | None = None
    error: str | None = None
    output_url_prefix: str = ""


class ClsiSyncPosition(BaseModel):
    """A single position in SyncTeX response."""

    file: str | None = None
    line: int | None = None
    column: int | None = None
    page: int | None = None
    h: float | None = None
    v: float | None = None
    width: float | None = None
    height: float | None = None


class ClsiSyncCodeResult(BaseModel):
    """Result from SyncTeX code-to-PDF lookup."""

    pdf: list[ClsiSyncPosition] = Field(default_factory=list)


class ClsiSyncPdfResult(BaseModel):
    """Result from SyncTeX PDF-to-code lookup."""

    code: list[ClsiSyncPosition] = Field(default_factory=list)


class ClsiWordcountResult(BaseModel):
    """Result from CLSI wordcount."""

    texcount: dict[str, Any] = Field(default_factory=dict)


class ClsiClient:
    """Async HTTP client for the vendored CLSI service."""

    def __init__(self, base_url: str | None = None, timeout: int | None = None) -> None:
        settings = get_settings()
        self._base_url = (base_url or settings.latex_clsi_base_url).rstrip("/")
        self._timeout = timeout or settings.latex_clsi_timeout_seconds

    def _client(self) -> httpx.AsyncClient:
        return httpx.AsyncClient(
            base_url=self._base_url,
            timeout=httpx.Timeout(self._timeout, connect=10.0),
        )

    async def compile(
        self,
        project_id: str,
        resources: list[ClsiResource],
        root_resource_path: str = "main.tex",
        compiler: str = "pdflatex",
        sync_type: str = "full",
    ) -> ClsiCompileResult:
        """Submit a compile request to CLSI."""
        body: dict[str, Any] = {
            "compile": {
                "options": {
                    "compiler": compiler,
                    "syncType": sync_type,
                },
                "rootResourcePath": root_resource_path,
                "resources": [r.model_dump(exclude_none=True) for r in resources],
            },
        }

        async with self._client() as client:
            resp = await client.post(f"/project/{project_id}/compile", json=body)

        return _parse_compile_response(resp.status_code, resp.json())

    async def stop_compile(self, project_id: str) -> bool:
        """Stop a running compile for the given project."""
        async with self._client() as client:
            resp = await client.post(f"/project/{project_id}/compile/stop")
        return resp.status_code == 204

    async def clear_cache(self, project_id: str) -> bool:
        """Clear the compile cache for the given project."""
        async with self._client() as client:
            resp = await client.delete(f"/project/{project_id}")
        return resp.status_code == 204

    async def wordcount(
        self,
        project_id: str,
        file: str = "main.tex",
        image: str | None = None,
    ) -> ClsiWordcountResult:
        """Get wordcount for a file in the project."""
        params: dict[str, str] = {"file": file}
        if image:
            params["image"] = image

        async with self._client() as client:
            resp = await client.get(f"/project/{project_id}/wordcount", params=params)

        resp.raise_for_status()
        data = resp.json()
        return ClsiWordcountResult(texcount=data.get("texcount", {}))

    async def sync_from_code(
        self,
        project_id: str,
        file: str,
        line: int,
        column: int,
    ) -> ClsiSyncCodeResult:
        """SyncTeX source-to-PDF lookup."""
        params = {"file": file, "line": str(line), "column": str(column)}

        async with self._client() as client:
            resp = await client.get(f"/project/{project_id}/sync/code", params=params)

        resp.raise_for_status()
        data = resp.json()
        return ClsiSyncCodeResult(
            pdf=[ClsiSyncPosition(**p) for p in data.get("pdf", [])],
        )

    async def sync_from_pdf(
        self,
        project_id: str,
        page: int,
        h: float,
        v: float,
    ) -> ClsiSyncPdfResult:
        """SyncTeX PDF-to-source lookup."""
        params = {"page": str(page), "h": str(h), "v": str(v)}

        async with self._client() as client:
            resp = await client.get(f"/project/{project_id}/sync/pdf", params=params)

        resp.raise_for_status()
        data = resp.json()
        return ClsiSyncPdfResult(
            code=[ClsiSyncPosition(**p) for p in data.get("code", [])],
        )

    async def get_output_zip(
        self,
        project_id: str,
        build_id: str,
    ) -> httpx.Response:
        """Get output ZIP archive from CLSI."""
        async with self._client() as client:
            resp = await client.get(
                f"/project/{project_id}/build/{build_id}/output/output.zip",
            )
        return resp


def _parse_compile_response(status_code: int, data: dict[str, Any]) -> ClsiCompileResult:
    """Parse a CLSI compile response into a structured result."""
    compile_data = data.get("compile", {})
    raw_files = compile_data.get("outputFiles", [])

    output_files = [
        ClsiOutputFile(
            path=f.get("path", ""),
            url=f.get("url", ""),
            type=f.get("type", ""),
            build=f.get("build", ""),
            size=f.get("size", 0),
        )
        for f in raw_files
    ]

    return ClsiCompileResult(
        status=compile_data.get("status", STATUS_ERROR),
        build_id=compile_data.get("buildId"),
        output_files=output_files,
        error=compile_data.get("error"),
        output_url_prefix=compile_data.get("outputUrlPrefix", ""),
    )
