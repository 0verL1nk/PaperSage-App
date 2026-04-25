"""Application configuration using Pydantic Settings."""

from functools import lru_cache

from pydantic import field_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class EmailSettings:
    """Email service configuration loaded from environment variables."""

    def __init__(self) -> None:
        import os

        self.provider: str = os.getenv("EMAIL_PROVIDER", "resend")
        self.from_email: str = os.getenv("FROM_EMAIL", "noreply@papersage.dev")
        self.from_name: str = os.getenv("FROM_NAME", "PaperSage")

        # Resend specific
        self.resend_api_key: str | None = os.getenv("RESEND_API_KEY")
        self.resend_domain: str | None = os.getenv("RESEND_DOMAIN")

        # Generic SMTP specific
        self.smtp_host: str | None = os.getenv("SMTP_HOST")
        self.smtp_port: int = int(os.getenv("SMTP_PORT", "587"))
        self.smtp_user: str | None = os.getenv("SMTP_USER")
        self.smtp_password: str | None = os.getenv("SMTP_PASSWORD")
        self.smtp_tls: bool = os.getenv("SMTP_TLS", "true").lower() == "true"

    def validate_config(self) -> None:
        """Validate that required config exists for the chosen provider."""
        if self.provider == "resend" and not self.resend_api_key:
            raise ValueError("RESEND_API_KEY is required when provider is 'resend'")
        elif self.provider == "smtp" and not self.smtp_host:
            raise ValueError("SMTP_HOST is required when provider is 'smtp'")


class Settings(BaseSettings):
    """Application settings loaded from environment variables."""

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        case_sensitive=False,
    )

    # Database
    database_url: str = (
        "postgresql+asyncpg://papersage:papersage_dev_password@localhost:5432/papersage"
    )

    # Redis (leave empty to use in-memory cache)
    redis_url: str = ""

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

    # LaTeX CLSI
    latex_clsi_enabled: bool = True
    latex_clsi_base_url: str = "http://localhost:3013"
    latex_clsi_timeout_seconds: int = 120

    # S3 Storage (Cloudflare R2 / S3-compatible)
    s3_endpoint: str = "https://account_id.r2.cloudflarestorage.com"
    s3_access_key: str = ""
    s3_secret_key: str = ""
    s3_bucket: str = "papersage"
    s3_region: str = "auto"

    # Email
    email: EmailSettings = EmailSettings()

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
