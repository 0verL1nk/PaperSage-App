"""Documents routes for uploading and managing reference files."""

import urllib.parse
import uuid

from fastapi import APIRouter, Depends, Form, HTTPException, UploadFile, status
from fastapi.responses import Response
from sqlalchemy.ext.asyncio import AsyncSession

from api.dependencies import get_db
from api.response import ApiResponse, api_success
from api.routes.auth import get_current_user
from models.user import User
from schemas import DocumentMoveRequest, DocumentRemarkRequest, DocumentResponse
from services.documents.document_service import DocumentService

router = APIRouter()

_document_service = DocumentService()


@router.post("/upload", response_model=ApiResponse[DocumentResponse])
async def upload_document(
    projectId: uuid.UUID = Form(...),  # noqa: N803
    file: UploadFile = ...,
    folderId: uuid.UUID | None = Form(None),  # noqa: N803
    remark: str = Form(""),
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[DocumentResponse]:
    """Upload a document (PDF, DOCX, TXT) to a project."""
    content = await file.read()
    try:
        doc = await _document_service.upload(
            project_id=projectId,
            user_id=current_user.id,
            file_name=file.filename or "unnamed",
            file_content=content,
            content_type=file.content_type or "application/octet-stream",
            folder_id=folderId,
            remark=remark or None,
            db=db,
        )
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(e)) from e
    return api_success(DocumentResponse.model_validate(doc))


@router.get("/{document_id}/preview")
async def preview_document(
    document_id: uuid.UUID,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> Response:
    """Stream document content for preview."""
    try:
        content, content_type, file_name = await _document_service.get_preview(
            document_id, current_user.id, db
        )
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(e)) from e
    # Use quote to handle non-ASCII characters in filename
    encoded_filename = urllib.parse.quote(file_name)
    return Response(
        content=content,
        media_type=content_type,
        headers={
            "Content-Disposition": f"inline; filename=\"{encoded_filename}\"; filename*=UTF-8''{encoded_filename}"
        },
    )


@router.put("/{document_id}/move", response_model=ApiResponse[DocumentResponse])
async def move_document(
    document_id: uuid.UUID,
    data: DocumentMoveRequest,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[DocumentResponse]:
    """Move a document to another folder."""
    try:
        doc = await _document_service.move(document_id, data.folder_id, current_user.id, db)
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(e)) from e
    return api_success(DocumentResponse.model_validate(doc))


@router.put("/{document_id}/remark", response_model=ApiResponse[DocumentResponse])
async def update_remark(
    document_id: uuid.UUID,
    data: DocumentRemarkRequest,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[DocumentResponse]:
    """Update document remark."""
    try:
        doc = await _document_service.update_remark(document_id, data.remark, current_user.id, db)
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(e)) from e
    return api_success(DocumentResponse.model_validate(doc))


@router.delete("/{document_id}")
async def delete_document(
    document_id: uuid.UUID,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[dict]:
    """Delete a document permanently."""
    try:
        await _document_service.delete(document_id, current_user.id, db)
    except ValueError as e:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(e)) from e
    return api_success({"id": str(document_id), "deleted": True})
