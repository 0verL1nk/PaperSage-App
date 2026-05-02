"""LaTeX file CRUD API routes."""

from __future__ import annotations

import urllib.parse
import uuid

from fastapi import APIRouter, Depends, HTTPException, Query, Request, status
from fastapi.responses import Response
from sqlalchemy.ext.asyncio import AsyncSession

from api.dependencies import get_db
from api.response import ApiResponse, api_success
from api.routes.auth import get_current_user
from models import User
from schemas.latex_file import (
    LatexFileContentResponse,
    LatexFileCreateRequest,
    LatexFileListResponse,
    LatexFileMoveRequest,
    LatexFileRenameRequest,
    LatexFileResponse,
    LatexFileUpdateRequest,
)
from services.latex.file_service import LatexFileService

router = APIRouter()


def _get_service() -> LatexFileService:
    return LatexFileService()


@router.get("/", response_model=ApiResponse[LatexFileListResponse])
async def list_files(
    project_id: uuid.UUID,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[LatexFileListResponse]:
    """List all LaTeX files in a project."""
    service = _get_service()
    try:
        files = await service.list_files(project_id, current_user.id, db)
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(e)) from None

    return api_success(
        LatexFileListResponse(
            files=[LatexFileResponse.model_validate(f) for f in files],
        )
    )


@router.get("/{file_path:path}", response_model=None)
async def get_file_content(
    project_id: uuid.UUID,
    file_path: str,
    raw: bool = Query(False, description="Return raw binary content"),
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> Response | ApiResponse[LatexFileContentResponse]:
    """Get content of a LaTeX file. Returns JSON by default for text, raw bytes if raw=true or file is binary."""
    service = _get_service()
    try:
        content, content_type = await service.get_file_raw(
            project_id, file_path, current_user.id, db
        )

        # Determine if we should return raw binary or JSON
        is_text = any(
            file_path.lower().endswith(ext)
            for ext in [".tex", ".bib", ".cls", ".sty", ".bst", ".txt", ".md"]
        )

        if raw or not is_text:
            filename = file_path.split("/")[-1]
            encoded_filename = urllib.parse.quote(filename)
            return Response(
                content=content,
                media_type=content_type,
                headers={
                    "Content-Disposition": f"inline; filename=\"{encoded_filename}\"; filename*=UTF-8''{encoded_filename}"
                },
            )

        # Default: return JSON envelope for the editor
        return api_success(
            LatexFileContentResponse(path=file_path, content=content.decode("utf-8"))
        )
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(e)) from None
    except UnicodeDecodeError:
        # Fallback for files that failed to decode but were thought to be text
        content, content_type = await service.get_file_raw(
            project_id, file_path, current_user.id, db
        )
        return Response(content=content, media_type=content_type)


@router.post("/", response_model=ApiResponse[LatexFileResponse])
async def create_file(
    project_id: uuid.UUID,
    request: Request,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[LatexFileResponse]:
    """Create a new LaTeX file. Supports JSON body (for text) or Multipart (for binary)."""
    service = _get_service()
    content_type = request.headers.get("content-type", "")

    try:
        if "multipart/form-data" in content_type:
            form = await request.form()
            upload = form.get("file")
            file_path = form.get("path") or (upload.filename if upload else None) or "unnamed"
            if upload:
                data = await upload.read()
                ct = upload.content_type
                latex_file = await service.create_file_binary(
                    project_id, file_path, data, ct, current_user.id, db
                )
            else:
                raise HTTPException(status_code=400, detail="No file provided in multipart")
        else:
            body = await request.json()
            parsed = LatexFileCreateRequest.model_validate(body)
            latex_file = await service.create_file(
                project_id, parsed.path, parsed.content, current_user.id, db
            )

        return api_success(LatexFileResponse.model_validate(latex_file))
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(e)) from None


@router.put("/{file_path:path}", response_model=ApiResponse[LatexFileResponse])
async def update_file(
    project_id: uuid.UUID,
    file_path: str,
    body: LatexFileUpdateRequest,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[LatexFileResponse]:
    """Update content of a LaTeX file."""
    service = _get_service()
    try:
        latex_file = await service.update_file(
            project_id,
            file_path,
            body.content,
            current_user.id,
            db,
        )
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(e)) from None

    return api_success(LatexFileResponse.model_validate(latex_file))


@router.delete("/{file_path:path}", response_model=ApiResponse[dict])
async def delete_file(
    project_id: uuid.UUID,
    file_path: str,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[dict]:
    """Delete a LaTeX file."""
    service = _get_service()
    try:
        await service.delete_file(project_id, file_path, current_user.id, db)
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(e)) from None

    return api_success({"deleted": True})


@router.post("/{file_path:path}/rename", response_model=ApiResponse[LatexFileResponse])
async def rename_file(
    project_id: uuid.UUID,
    file_path: str,
    body: LatexFileRenameRequest,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[LatexFileResponse]:
    """Rename a LaTeX file."""
    service = _get_service()
    try:
        latex_file = await service.rename_file(
            project_id, file_path, body.new_path, current_user.id, db
        )
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(e)) from None
    return api_success(LatexFileResponse.model_validate(latex_file))


@router.post("/{file_path:path}/move", response_model=ApiResponse[LatexFileResponse])
async def move_file(
    project_id: uuid.UUID,
    file_path: str,
    body: LatexFileMoveRequest,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[LatexFileResponse]:
    """Move a LaTeX file to a new directory."""
    service = _get_service()
    try:
        latex_file = await service.move_file(
            project_id, file_path, body.new_dir, current_user.id, db
        )
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(e)) from None
    return api_success(LatexFileResponse.model_validate(latex_file))
