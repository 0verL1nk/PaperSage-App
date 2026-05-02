"""User settings API routes."""

from __future__ import annotations

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.ext.asyncio import AsyncSession

from api.dependencies import get_db
from api.response import ApiResponse, api_success
from api.routes.auth import get_current_user
from models import User
from schemas.user_settings import (
    VALID_CATEGORIES,
    AllSettingsResponse,
    SettingsResponse,
    SettingsUpdateRequest,
)
from services.settings_service import SettingsService, SettingsServiceError

router = APIRouter()


def _svc(db: AsyncSession) -> SettingsService:
    return SettingsService(db)


@router.get(
    "",
    response_model=ApiResponse[AllSettingsResponse],
)
async def get_all_settings(
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[AllSettingsResponse]:
    """Return all categories of settings for the current user."""
    svc = _svc(db)
    data = await svc.get_all_settings(current_user.id)
    return api_success(AllSettingsResponse(settings=data))


@router.get(
    "/{category}",
    response_model=ApiResponse[SettingsResponse],
)
async def get_category_settings(
    category: str,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[SettingsResponse]:
    """Return settings for a single category."""
    _check_category(category)
    svc = _svc(db)
    data = await svc.get_category(current_user.id, category)
    return api_success(SettingsResponse(category=category, settings=data))


@router.patch(
    "/{category}",
    response_model=ApiResponse[SettingsResponse],
)
async def patch_category_settings(
    category: str,
    body: SettingsUpdateRequest,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[SettingsResponse]:
    """Merge-update a single category (only changed fields)."""
    _check_category(category)
    svc = _svc(db)
    try:
        data = await svc.update_category(current_user.id, category, body.settings)
    except SettingsServiceError as exc:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            detail=str(exc),
        ) from exc
    return api_success(SettingsResponse(category=category, settings=data))


@router.put(
    "/{category}",
    response_model=ApiResponse[SettingsResponse],
)
async def put_category_settings(
    category: str,
    body: SettingsUpdateRequest,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[SettingsResponse]:
    """Full-replace a single category."""
    _check_category(category)
    svc = _svc(db)
    try:
        data = await svc.replace_category(current_user.id, category, body.settings)
    except SettingsServiceError as exc:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            detail=str(exc),
        ) from exc
    return api_success(SettingsResponse(category=category, settings=data))


@router.delete(
    "/{category}",
    response_model=ApiResponse[SettingsResponse],
)
async def reset_category_settings(
    category: str,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[SettingsResponse]:
    """Reset a category to default values."""
    _check_category(category)
    svc = _svc(db)
    await svc.reset_category(current_user.id, category)
    data = await svc.get_category(current_user.id, category)
    return api_success(SettingsResponse(category=category, settings=data))


def _check_category(category: str) -> None:
    if category not in VALID_CATEGORIES:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=f"Unknown settings category: {category}",
        )
