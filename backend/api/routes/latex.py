"""LaTeX compilation API routes."""

from __future__ import annotations

import re
import uuid
from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, Query, Request, status
from fastapi.responses import Response
from sqlalchemy.ext.asyncio import AsyncSession

from api.dependencies import get_db
from api.response import ApiResponse, api_success
from api.routes.auth import get_current_user
from models import User
from schemas.compile_job import (
    CompileJobHistoryResponse,
    CompileJobListItem,
    CompileJobStatusResponse,
    CompileJobSubmitResponse,
)
from schemas.latex import (
    VALID_COMPILERS,
    LatexCompileRequest,
    LatexSyncCodeRequest,
    LatexSyncCodeResponse,
    LatexSyncPdfRequest,
    LatexSyncPdfResponse,
    LatexSyncPosition,
    LatexWordcountResponse,
)
from services.latex.compile_queue import CompileQueueService, get_compile_queue
from services.latex.resource_builder import ProjectFile
from services.latex.service import LaTeXService, LaTeXServiceError

router = APIRouter()

_RANGE_RE = re.compile(r"^bytes=(\d*)-(\d*)$")

CompileQueue = Annotated[CompileQueueService, Depends(get_compile_queue)]


def _get_service() -> LaTeXService:
    return LaTeXService()


def _project_id_str(project_id: uuid.UUID) -> str:
    return str(project_id).replace("-", "")


async def _load_project_files_from_storage(
    project_id: uuid.UUID,
    db: AsyncSession,
) -> list[ProjectFile]:
    """Load all project files from storage (with cache)."""
    from services.latex.file_service import LatexFileService

    file_svc = LatexFileService()
    pairs = await file_svc.load_all_contents(project_id, db)
    return [ProjectFile(path=path, content=content) for path, content in pairs]


@router.post(
    "/compile",
    status_code=status.HTTP_202_ACCEPTED,
    response_model=ApiResponse[CompileJobSubmitResponse],
)
async def compile_project(
    project_id: uuid.UUID,
    body: LatexCompileRequest,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
    queue: CompileQueueService = Depends(get_compile_queue),  # noqa: B008
) -> ApiResponse[CompileJobSubmitResponse]:
    """Submit an async compile job. Returns job_id immediately."""
    if body.compiler not in VALID_COMPILERS:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Invalid compiler. Must be one of: {', '.join(VALID_COMPILERS)}",
        )

    service = _get_service()
    await service.verify_project_access(project_id, current_user, db)

    job = await queue.submit(
        project_id=project_id,
        user_id=current_user.id,
        db=db,
        root_file=body.root_file,
        compiler=body.compiler,
    )
    return api_success(
        CompileJobSubmitResponse(job_id=job.id, status=job.status),
    )


@router.delete("/compile")
async def stop_compile(
    project_id: uuid.UUID,
    job_id: uuid.UUID | None = Query(None, description="Compile job ID to cancel"),
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
    queue: CompileQueueService = Depends(get_compile_queue),  # noqa: B008
) -> ApiResponse[dict]:
    """Stop/cancel a compilation. Optionally cancel a specific job by job_id."""
    service = _get_service()
    await service.verify_project_access(project_id, current_user, db)

    if job_id:
        job = await queue.cancel(job_id, current_user.id, db)
        if not job:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Compile job not found",
            )
        return api_success({"cancelled": True, "job_id": str(job_id)})

    await service.stop_compile(_project_id_str(project_id))
    return api_success({"stopped": True})


@router.get(
    "/compile/history",
    response_model=ApiResponse[CompileJobHistoryResponse],
)
async def get_compile_history(
    project_id: uuid.UUID,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
    queue: CompileQueueService = Depends(get_compile_queue),  # noqa: B008
) -> ApiResponse[CompileJobHistoryResponse]:
    """Get compile history for a project."""
    service = _get_service()
    await service.verify_project_access(project_id, current_user, db)

    items, total = await queue.get_history(project_id, current_user.id, db)
    return api_success(
        CompileJobHistoryResponse(
            items=[
                CompileJobListItem(
                    job_id=j.id,
                    status=j.status,
                    root_file=j.root_file,
                    compiler=j.compiler,
                    build_id=j.build_id,
                    error=j.error,
                    created_at=j.created_at,
                    updated_at=j.updated_at,
                )
                for j in items
            ],
            total=total,
        )
    )


@router.get(
    "/compile/{job_id}/status",
    response_model=ApiResponse[CompileJobStatusResponse],
)
async def get_compile_status(
    project_id: uuid.UUID,
    job_id: uuid.UUID,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
    queue: CompileQueueService = Depends(get_compile_queue),  # noqa: B008
) -> ApiResponse[CompileJobStatusResponse]:
    """Poll compile job status."""
    service = _get_service()
    await service.verify_project_access(project_id, current_user, db)

    job = await queue.get_status(job_id, current_user.id, db)
    if not job:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Compile job not found",
        )

    elapsed = (job.updated_at - job.created_at).total_seconds()
    return api_success(
        CompileJobStatusResponse(
            job_id=job.id,
            status=job.status,
            build_id=job.build_id,
            error=job.error,
            logs=job.logs,
            root_file=job.root_file,
            compiler=job.compiler,
            elapsed_seconds=elapsed,
            created_at=job.created_at,
            updated_at=job.updated_at,
        )
    )


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
        _project_id_str(project_id),
        body.file,
        body.line,
        body.column,
    )
    return api_success(
        LatexSyncCodeResponse(
            pdf=[LatexSyncPosition(**p.model_dump()) for p in result.pdf],
        )
    )


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
        _project_id_str(project_id),
        body.page,
        body.h,
        body.v,
    )
    return api_success(
        LatexSyncPdfResponse(
            code=[LatexSyncPosition(**p.model_dump()) for p in result.code],
        )
    )


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
    try:
        clsi_resp = await service.get_output_zip(_project_id_str(project_id), build_id)
    except Exception as e:
        raise LaTeXServiceError(f"无法获取编译输出: {e}", status_code=502) from e
    return Response(
        content=clsi_resp.content,
        status_code=clsi_resp.status_code,
        media_type="application/zip",
    )


@router.api_route("/output/{build_id}/pdf", methods=["GET", "HEAD"])
async def get_output_pdf(
    project_id: uuid.UUID,
    build_id: str,
    request: Request,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> Response:
    """Proxy compiled PDF from CLSI with Range and HEAD request support."""
    service = _get_service()
    await service.verify_project_access(project_id, current_user, db)
    try:
        pdf_bytes = await service.get_output_pdf(
            _project_id_str(project_id),
            build_id,
        )
    except LaTeXServiceError:
        raise
    except Exception as e:
        raise LaTeXServiceError(
            f"无法获取编译输出: {e}",
            status_code=502,
        ) from e

    total_size = len(pdf_bytes)

    # HEAD request — return metadata only, no body
    if request.method == "HEAD":
        return Response(
            content=b"",
            status_code=200,
            media_type="application/pdf",
            headers={
                "Content-Disposition": "inline",
                "Accept-Ranges": "bytes",
                "Content-Length": str(total_size),
            },
        )

    # Parse Range header
    range_header = request.headers.get("range", "")
    match = _RANGE_RE.match(range_header)

    if not match:
        # Full response
        return Response(
            content=pdf_bytes,
            status_code=200,
            media_type="application/pdf",
            headers={
                "Content-Disposition": "inline",
                "Accept-Ranges": "bytes",
                "Content-Length": str(total_size),
            },
        )

    start_str, end_str = match.group(1), match.group(2)
    start = int(start_str) if start_str else 0
    end = int(end_str) if end_str else total_size - 1
    end = min(end, total_size - 1)

    if start > end or start >= total_size:
        raise HTTPException(
            status_code=status.HTTP_416_REQUESTED_RANGE_NOT_SATISFIABLE,
            detail="Requested range not satisfiable",
        )

    chunk = pdf_bytes[start : end + 1]
    return Response(
        content=chunk,
        status_code=206,
        media_type="application/pdf",
        headers={
            "Content-Range": f"bytes {start}-{end}/{total_size}",
            "Accept-Ranges": "bytes",
            "Content-Length": str(len(chunk)),
        },
    )
