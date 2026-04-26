"""Schemas package."""

from schemas.document import DocumentMoveRequest, DocumentRemarkRequest, DocumentResponse
from schemas.folder import FolderCreate, FolderResponse
from schemas.project import (
    ProjectCreate,
    ProjectGroupCreate,
    ProjectGroupResponse,
    ProjectGroupUpdate,
    ProjectResponse,
    ProjectUpdate,
    ProjectWithGroupResponse,
    TrashItemResponse,
)
from schemas.user import (
    LoginRequest,
    RefreshTokenRequest,
    RegisterRequest,
    TokenResponse,
    UserResponse,
)

__all__ = [
    "LoginRequest",
    "RefreshTokenRequest",
    "RegisterRequest",
    "TokenResponse",
    "UserResponse",
    "ProjectCreate",
    "ProjectUpdate",
    "ProjectResponse",
    "ProjectWithGroupResponse",
    "ProjectGroupCreate",
    "ProjectGroupUpdate",
    "ProjectGroupResponse",
    "TrashItemResponse",
    "FolderCreate",
    "FolderResponse",
    "DocumentResponse",
    "DocumentMoveRequest",
    "DocumentRemarkRequest",
]
