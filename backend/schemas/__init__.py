"""Schemas package."""

from schemas.compile_job import (
    CompileJobHistoryResponse,
    CompileJobListItem,
    CompileJobStatusResponse,
    CompileJobSubmitResponse,
)
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
from schemas.user_settings import (
    AllSettingsResponse,
    SettingsResponse,
    SettingsUpdateRequest,
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
    "CompileJobSubmitResponse",
    "CompileJobStatusResponse",
    "CompileJobListItem",
    "CompileJobHistoryResponse",
    "SettingsUpdateRequest",
    "SettingsResponse",
    "AllSettingsResponse",
]
