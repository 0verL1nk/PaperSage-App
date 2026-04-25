"""LaTeX file CRUD API routes."""

from __future__ import annotations

import uuid

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.ext.asyncio import AsyncSession

from api.dependencies import get_db
from api.response import ApiResponse, api_success
from api.routes.auth import get_current_user
from models import User
from schemas.latex_file import (
    LatexFileContentResponse,
    LatexFileCreateRequest,
    LatexFileListResponse,
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

    return api_success(LatexFileListResponse(
        files=[LatexFileResponse.model_validate(f) for f in files],
    ))


@router.get("/{file_path:path}", response_model=ApiResponse[LatexFileContentResponse])
async def get_file_content(
    project_id: uuid.UUID,
    file_path: str,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[LatexFileContentResponse]:
    """Get content of a LaTeX file."""
    service = _get_service()
    try:
        content = await service.get_file_content(project_id, file_path, current_user.id, db)
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(e)) from None

    return api_success(LatexFileContentResponse(path=file_path, content=content))


@router.post("/", response_model=ApiResponse[LatexFileResponse])
async def create_file(
    project_id: uuid.UUID,
    body: LatexFileCreateRequest,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[LatexFileResponse]:
    """Create a new LaTeX file."""
    service = _get_service()
    try:
        latex_file = await service.create_file(
            project_id, body.path, body.content, current_user.id, db,
        )
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(e)) from None

    return api_success(LatexFileResponse.model_validate(latex_file))


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
            project_id, file_path, body.content, current_user.id, db,
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
