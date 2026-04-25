"""LaTeX compilation API routes."""

from __future__ import annotations

import uuid

from fastapi import APIRouter, Depends, HTTPException, status
from fastapi.responses import Response
from sqlalchemy.ext.asyncio import AsyncSession

from api.dependencies import get_db
from api.response import ApiResponse, api_success
from api.routes.auth import get_current_user
from models import User
from schemas.latex import (
    LatexCompileRequest,
    LatexCompileResponse,
    LatexOutputFileResponse,
    LatexSyncCodeRequest,
    LatexSyncCodeResponse,
    LatexSyncPdfRequest,
    LatexSyncPdfResponse,
    LatexSyncPosition,
    LatexWordcountResponse,
    VALID_COMPILERS,
)
from services.latex.resource_builder import ProjectFile
from services.latex.service import LaTeXService, LaTeXServiceError

router = APIRouter()


def _get_service() -> LaTeXService:
    return LaTeXService()


def _project_id_str(project_id: uuid.UUID) -> str:
    return str(project_id).replace("-", "")


@router.post("/compile", response_model=ApiResponse[LatexCompileResponse])
async def compile_project(
    project_id: uuid.UUID,
    body: LatexCompileRequest,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[LatexCompileResponse]:
    """Compile a LaTeX project."""
    if body.compiler not in VALID_COMPILERS:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Invalid compiler. Must be one of: {', '.join(VALID_COMPILERS)}",
        )

    service = _get_service()
    await service.verify_project_access(project_id, current_user, db)

    files = [
        ProjectFile(
            path=f.path,
            content=f.content,
            url=f.url,
            is_binary=f.is_binary,
        )
        for f in body.files
    ]

    try:
        result = await service.compile_project(
            project_id=_project_id_str(project_id),
            files=files,
            root_file=body.root_file,
            compiler=body.compiler,
        )
    except LaTeXServiceError as e:
        raise HTTPException(status_code=e.status_code, detail=e.message) from None

    response = LatexCompileResponse(
        status=result.status,
        build_id=result.build_id,
        output_files=[
            LatexOutputFileResponse(
                path=f.path, url=f.url, type=f.type, build=f.build, size=f.size,
            )
            for f in result.output_files
        ],
        error=result.error,
    )
    return api_success(response)


@router.delete("/compile")
async def stop_compile(
    project_id: uuid.UUID,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[dict]:
    """Stop a running compilation."""
    service = _get_service()
    await service.verify_project_access(project_id, current_user, db)
    await service.stop_compile(_project_id_str(project_id))
    return api_success({"stopped": True})


@router.delete("/cache")
async def clear_cache(
    project_id: uuid.UUID,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[dict]:
    """Clear compilation cache for a project."""
    service = _get_service()
    await service.verify_project_access(project_id, current_user, db)
    await service.clear_cache(_project_id_str(project_id))
    return api_success({"cleared": True})


@router.get("/wordcount", response_model=ApiResponse[LatexWordcountResponse])
async def get_wordcount(
    project_id: uuid.UUID,
    file: str = "main.tex",
    image: str | None = None,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[LatexWordcountResponse]:
    """Get word count for a file in the project."""
    service = _get_service()
    await service.verify_project_access(project_id, current_user, db)
    result = await service.get_wordcount(_project_id_str(project_id), file, image)
    return api_success(LatexWordcountResponse(texcount=result.texcount))


@router.post("/sync/code", response_model=ApiResponse[LatexSyncCodeResponse])
async def sync_code_to_pdf(
    project_id: uuid.UUID,
    body: LatexSyncCodeRequest,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[LatexSyncCodeResponse]:
    """SyncTeX source-to-PDF lookup."""
    service = _get_service()
    await service.verify_project_access(project_id, current_user, db)
    result = await service.sync_code_to_pdf(
        _project_id_str(project_id), body.file, body.line, body.column,
    )
    return api_success(LatexSyncCodeResponse(
        pdf=[LatexSyncPosition(**p.model_dump()) for p in result.pdf],
    ))


@router.post("/sync/pdf", response_model=ApiResponse[LatexSyncPdfResponse])
async def sync_pdf_to_code(
    project_id: uuid.UUID,
    body: LatexSyncPdfRequest,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[LatexSyncPdfResponse]:
    """SyncTeX PDF-to-source lookup."""
    service = _get_service()
    await service.verify_project_access(project_id, current_user, db)
    result = await service.sync_pdf_to_code(
        _project_id_str(project_id), body.page, body.h, body.v,
    )
    return api_success(LatexSyncPdfResponse(
        code=[LatexSyncPosition(**p.model_dump()) for p in result.code],
    ))


# NOTE: output.zip must come before {file_path:path} to avoid route shadowing
@router.get("/output/{build_id}/output.zip")
async def get_output_zip(
    project_id: uuid.UUID,
    build_id: str,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> Response:
    """Proxy output ZIP archive from CLSI."""
    service = _get_service()
    await service.verify_project_access(project_id, current_user, db)
    clsi_resp = await service.get_output_zip(_project_id_str(project_id), build_id)
    return Response(
        content=clsi_resp.content,
        status_code=clsi_resp.status_code,
        media_type="application/zip",
    )
