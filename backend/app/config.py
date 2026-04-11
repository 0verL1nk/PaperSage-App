"""Application configuration using Pydantic Settings."""
from functools import lru_cache

from pydantic import field_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Application settings loaded from environment variables."""

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        case_sensitive=False,
    )

    # Database
    database_url: str = "postgresql+asyncpg://papersage:papersage_dev_password@localhost:5432/papersage"

    # Redis
    redis_url: str = "redis://:papersage_dev_password@localhost:6379/0"

    # Security
    secret_key: str = "dev-secret-key-change-in-production"
    access_token_expire_minutes: int = 30
    refresh_token_expire_days: int = 7

    # App
    debug: bool = False
    allowed_hosts: str = "localhost,127.0.0.1"

    # Server
    host: str = "0.0.0.0"
    port: int = 8000
    reload: bool = False

    @field_validator("allowed_hosts", mode="after")
    @classmethod
    def parse_allowed_hosts(cls, v: list[str] | str) -> list[str]:
        """Parse comma-separated hosts to list."""
        if isinstance(v, list):
            return v
        if isinstance(v, str):
            return [h.strip() for h in v.split(",") if h.strip()]
        return ["localhost", "127.0.0.1"]


@lru_cache
def get_settings() -> Settings:
    """Get cached settings instance."""
    return Settings()
