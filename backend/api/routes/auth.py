"""Auth routes for token refresh (frontend compatibility)."""
import uuid

from fastapi import APIRouter, Depends, HTTPException, status
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from pydantic import BaseModel
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from api.dependencies import get_db
from api.response import ApiResponse, api_success
from core.token import create_access_token, create_refresh_token, verify_token
from models import User
from schemas import TokenResponse

router = APIRouter()
security = HTTPBearer(auto_error=False)


class RefreshRequest(BaseModel):
    """Refresh token request schema."""

    refreshToken: str


async def get_current_user(
    credentials: HTTPAuthorizationCredentials | None = Depends(security),
    db: AsyncSession = Depends(get_db),
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
        )

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
async def refresh_token(request: RefreshRequest, db: AsyncSession = Depends(get_db)) -> ApiResponse[TokenResponse]:
    """Refresh access token using refresh token."""
    try:
        payload = verify_token(request.refreshToken, "refresh")
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

    return api_success(
        TokenResponse(
            accessToken=access_token,
            refreshToken=new_refresh_token,
        )
    )


@router.post("/logout")
async def logout() -> ApiResponse[None]:
    """Logout endpoint. JWT tokens are stateless, so we just acknowledge the request."""
    return api_success(None)
