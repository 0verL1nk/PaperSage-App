"""User settings schemas for request/response validation."""

import uuid
from typing import Any, Literal

from pydantic import BaseModel, Field, model_validator

# Supported setting categories
VALID_CATEGORIES = ("ui", "editor", "agent")

# Centralized default values for each category
DEFAULT_SETTINGS: dict[str, dict[str, Any]] = {
    "ui": {
        "theme": "system",
        "language": "zh-CN",
        "sidebar_collapsed": False,
    },
    "editor": {
        "font_size": 14,
        "tab_size": 2,
        "auto_save": True,
        "auto_save_interval": 30,
        "word_wrap": True,
    },
    "agent": {
        "providers": [],
        "active_provider_id": "",
        "temperature": 0.7,
        "writing_style": "academic",
        "memory_enabled": True,
    },
}


# --- Per-category Pydantic validation schemas ---


class UISettings(BaseModel):
    """UI preference settings."""

    theme: Literal["system", "light", "dark"] = "system"
    language: str = Field(default="zh-CN", pattern=r"^[a-z]{2,3}-[A-Z]{2,4}$")
    sidebar_collapsed: bool = False


class EditorSettings(BaseModel):
    """Editor behavior settings."""

    font_size: int = Field(default=14, ge=8, le=32)
    tab_size: int = Field(default=2, ge=1, le=8)
    auto_save: bool = True
    auto_save_interval: int = Field(default=30, ge=5, le=300)
    word_wrap: bool = True


class ProviderConfig(BaseModel):
    """A single AI model provider configuration."""

    id: str = Field(default="", max_length=64)
    name: str = Field(default="", max_length=100)
    type: Literal["openai_compatible", "anthropic"] = "openai_compatible"
    base_url: str = Field(default="", max_length=500)
    api_key: str = Field(default="", max_length=500)
    models: list[str] = Field(default_factory=list)
    active_model: str = Field(default="")


class AgentSettings(BaseModel):
    """AI assistant settings."""

    providers: list[ProviderConfig] = Field(default_factory=list)
    active_provider_id: str = Field(default="")
    temperature: float = Field(default=0.7, ge=0.0, le=2.0)
    writing_style: str = Field(default="academic", min_length=1, max_length=50)
    memory_enabled: bool = True

    @model_validator(mode="after")
    def assign_provider_ids(self) -> "AgentSettings":
        """Generate IDs for providers that don't have one."""
        for provider in self.providers:
            if not provider.id:
                provider.id = uuid.uuid4().hex[:12]
        return self


# Map category name to its Pydantic schema class
CATEGORY_SCHEMAS: dict[str, type[BaseModel]] = {
    "ui": UISettings,
    "editor": EditorSettings,
    "agent": AgentSettings,
}


# --- Request / Response schemas ---


class SettingsUpdateRequest(BaseModel):
    """Partial update body for PATCH (any valid fields for the category)."""

    settings: dict[str, Any] = Field(default_factory=dict)


class SettingsResponse(BaseModel):
    """Response for a single category of settings."""

    category: str
    settings: dict[str, Any]


class AllSettingsResponse(BaseModel):
    """Response for all categories of settings."""

    settings: dict[str, dict[str, Any]]
