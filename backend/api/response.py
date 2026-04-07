"""API response formatting."""
from typing import Any

from pydantic import BaseModel


class ApiEnvelope[T](BaseModel):
    """Standard API response envelope."""

    code: int = 200
    data: T | None = None
    info: str = ""


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
