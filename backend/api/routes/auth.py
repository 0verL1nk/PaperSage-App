"""Auth routes for token refresh (frontend compatibility)."""
import uuid

from fastapi import APIRouter, Depends, HTTPException, status
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from pydantic import BaseModel
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from api.dependencies import get_db
from api.response import ApiResponse, api_success
from app.config import get_settings
from core.redis import get_refresh_token_user, revoke_refresh_token, store_refresh_token
from core.token import create_access_token, create_refresh_token, verify_token
from models import User
from schemas import TokenResponse

settings = get_settings()

router = APIRouter()
security = HTTPBearer(auto_error=False)


class RefreshRequest(BaseModel):
    """Refresh token request schema."""

    refresh_token: str


async def get_current_user(
    credentials: HTTPAuthorizationCredentials | None = Depends(security),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> User:
    """Get current authenticated user."""
    if not credentials:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Not authenticated",
        )

    try:
        payload = verify_token(credentials.credentials, "access")
        user_id: str = payload.get("sub")
        if not user_id:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Invalid token",
            )
    except ValueError:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid token",
        ) from None

    # Convert string to UUID for database query
    user_uuid = uuid.UUID(user_id)
    result = await db.execute(select(User).where(User.id == user_uuid))
    user = result.scalar_one_or_none()

    if not user or not user.is_active:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="User not found or inactive",
        )

    return user


@router.post("/refresh", response_model=ApiResponse[TokenResponse])
async def refresh_token(request: RefreshRequest, db: AsyncSession = Depends(get_db)) -> ApiResponse[TokenResponse]:  # noqa: B008
    """Refresh access token using refresh token."""
    try:
        payload = verify_token(request.refresh_token, "refresh")
        user_id: str = payload.get("sub")
        if not user_id:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Invalid refresh token",
            )
    except ValueError:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid or expired refresh token",
        ) from None

    # Check if refresh token exists in Redis (not revoked)
    stored_user_id = await get_refresh_token_user(request.refresh_token)
    if stored_user_id is None:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Refresh token has been revoked",
        )

    # Convert string to UUID for database query
    user_uuid = uuid.UUID(user_id)

    # Verify user still exists and is active
    result = await db.execute(select(User).where(User.id == user_uuid))
    user = result.scalar_one_or_none()

    if not user or not user.is_active:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="User not found or inactive",
        )

    # Generate new tokens
    access_token = create_access_token(data={"sub": str(user.id)})
    new_refresh_token = create_refresh_token(data={"sub": str(user.id)})

    # Update refresh token in Redis
    ttl_seconds = settings.refresh_token_expire_days * 24 * 60 * 60
    await store_refresh_token(new_refresh_token, str(user.id), ttl_seconds)

    return api_success(
        TokenResponse(
            accessToken=access_token,
            refreshToken=new_refresh_token,
        )
    )


class LogoutRequest(BaseModel):
    """Logout request schema."""

    refresh_token: str


@router.post("/logout")
async def logout(request: LogoutRequest) -> ApiResponse[None]:
    """Logout endpoint. Revokes the refresh token in Redis."""
    try:
        payload = verify_token(request.refresh_token, "refresh")
        if payload is None:
            return api_success(None)
    except ValueError:
        pass  # Token invalid, already unusable

    # Revoke refresh token in Redis
    await revoke_refresh_token(request.refresh_token)
    return api_success(None)
