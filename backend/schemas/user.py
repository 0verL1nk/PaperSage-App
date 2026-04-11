"""User schemas for request/response validation."""
from uuid import UUID

from pydantic import BaseModel, Field


class LoginRequest(BaseModel):
    """Login request schema."""

    account: str = Field(..., min_length=1, max_length=50, description="Username")
    password: str = Field(..., min_length=1, description="Password (SHA-256 hashed)")


class RegisterRequest(BaseModel):
    """Register request schema."""

    account: str = Field(..., min_length=3, max_length=50, description="Username")
    password: str = Field(..., min_length=6, description="Password (SHA-256 hashed)")
    email: str | None = Field(None, description="Email (optional)")


class RefreshTokenRequest(BaseModel):
    """Refresh token request schema."""

    refresh_token: str = Field(..., description="Refresh token")


class TokenResponse(BaseModel):
    """Token response schema."""

    accessToken: str = Field(..., description="JWT access token")
    refreshToken: str = Field(..., description="JWT refresh token")
    tokenType: str = Field(default="bearer", description="Token type")

    model_config = {"populate_by_name": True}


class UserResponse(BaseModel):
    """User response schema."""

    id: UUID
    username: str
    email: str | None
    is_active: bool
    is_superuser: bool

    model_config = {"from_attributes": True}