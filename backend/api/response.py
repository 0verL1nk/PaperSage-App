"""API response formatting."""
from typing import Any, Generic, TypeVar

from pydantic import BaseModel

T = TypeVar("T")


class ApiEnvelope(BaseModel):
    """Standard API response envelope."""

    code: int = 200
    data: Any = None
    info: str = ""


class ApiResponse(BaseModel, Generic[T]):  # noqa: UP046
    """Generic API response."""

    code: int = 200
    data: T | None = None
    info: str = ""


def api_success(data: T, info: str = "") -> ApiResponse[T]:  # noqa: UP047
    """Create a success response."""
    return ApiResponse(code=200, data=data, info=info)


def success_response(data: Any, info: str = "") -> dict:
    """Create a success response."""
    return {
        "code": 200,
        "data": data,
        "info": info,
    }


def error_response(code: int, info: str) -> dict:
    """Create an error response."""
    return {
        "code": code,
        "data": None,
        "info": info,
    }
