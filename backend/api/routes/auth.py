"""Auth routes for token refresh (frontend compatibility)."""
import uuid

from fastapi import APIRouter, Depends, Form, HTTPException, status
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from pydantic import BaseModel
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from api.dependencies import get_db
from api.response import ApiResponse, api_success
from app.config import get_settings
from core.redis import get_refresh_token_user, revoke_refresh_token, store_refresh_token
from core.security import hash_password, verify_password
from core.token import create_access_token, create_refresh_token, verify_token
from models import User
from schemas import TokenResponse, UserResponse

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


@router.post("/register", response_model=ApiResponse[UserResponse])
async def register(
    account: str = Form(...),
    password: str = Form(...),
    email: str | None = Form(None),
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[UserResponse]:
    """Register a new user account."""
    # Check if username exists
    result = await db.execute(select(User).where(User.username == account))
    existing_user = result.scalar_one_or_none()

    if existing_user:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Username already exists",
        )

    # Hash the password (frontend already sent SHA-256, we hash again with bcrypt)
    hashed = hash_password(password)

    # Create user
    user = User(
        username=account,
        password_hash=hashed,
        email=email,
    )

    db.add(user)
    await db.commit()
    await db.refresh(user)

    return api_success(UserResponse.model_validate(user))


@router.post("/login", response_model=ApiResponse[TokenResponse])
async def login(
    account: str = Form(...),
    password: str = Form(...),
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[TokenResponse]:
    """Login with username and password."""
    # Find user
    result = await db.execute(select(User).where(User.username == account))
    user = result.scalar_one_or_none()

    if not user or not user.is_active:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid credentials",
        )

    # Verify password (frontend sent SHA-256, verify against bcrypt hash)
    if not verify_password(password, user.password_hash):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid credentials",
        )

    # Generate tokens
    access_token = create_access_token(data={"sub": str(user.id)})
    refresh_token = create_refresh_token(data={"sub": str(user.id)})

    # Store refresh token in Redis with 7-day expiry
    ttl_seconds = settings.refresh_token_expire_days * 24 * 60 * 60
    await store_refresh_token(refresh_token, str(user.id), ttl_seconds)

    return api_success(
        TokenResponse(
            accessToken=access_token,
            refreshToken=refresh_token,
        )
    )


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
