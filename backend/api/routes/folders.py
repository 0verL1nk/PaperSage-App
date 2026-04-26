"""Folders routes."""

import uuid

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.ext.asyncio import AsyncSession

from api.dependencies import get_db
from api.response import ApiResponse, api_success
from api.routes.auth import get_current_user
from models.user import User
from schemas import FolderCreate, FolderResponse
from services.documents.document_service import DocumentService
from services.documents.folder_service import FolderService

router = APIRouter()

_folder_service = FolderService()
_document_service = DocumentService()


@router.post("/", response_model=ApiResponse[FolderResponse])
async def create_folder(
    data: FolderCreate,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[FolderResponse]:
    """Create a new folder."""
    try:
        folder = await _folder_service.create(
            project_id=data.project_id,
            user_id=current_user.id,
            name=data.name,
            parent_id=data.parent_id,
            db=db,
        )
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(e)) from e
    return api_success(FolderResponse.model_validate(folder))


@router.delete("/{folder_id}")
async def delete_folder(
    folder_id: uuid.UUID,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[dict]:
    """Delete a folder. Documents move to project root."""
    try:
        await _folder_service.delete(folder_id, current_user.id, db)
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(e)) from e
    return api_success({"id": str(folder_id), "deleted": True})


@router.get("/{folder_id}/documents", response_model=ApiResponse[list[dict]])
async def list_folder_documents(
    folder_id: uuid.UUID,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[list[dict]]:
    """List documents in a folder."""
    from schemas import DocumentResponse

    try:
        docs = await _document_service.list_by_folder(folder_id, current_user.id, db)
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(e)) from e
    return api_success([DocumentResponse.model_validate(d).model_dump() for d in docs])
