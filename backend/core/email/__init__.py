"""Email service module.

Provides a universal email abstraction layer with multiple backend support.

Usage:
    from core.email import get_email_service

    service = get_email_service()
    await service.send("user@example.com", "Subject", "Body")

    # Or with template
    await service.send_template(
        to="user@example.com",
        subject="Welcome, {{username}}!",
        template="Hello {{username}}, ...",
        variables={"username": "John"},
    )
"""
from core.email.base import EmailBackend
from core.email.service import EmailService

__all__ = [
    "EmailBackend",
    "EmailService",
    "get_email_service",
    "get_backend",
]


def get_backend() -> EmailBackend:
    """Get email backend based on configuration.

    Returns:
        Configured email backend instance.
    """
    from app.config import get_settings
    from core.email.resend import ResendBackend
    from core.email.smtp import SMTPBackend

    settings = get_settings()

    if settings.email.provider == "resend":
        return ResendBackend(
            api_key=settings.email.resend_api_key or "",
            domain=settings.email.resend_domain or "",
            from_email=settings.email.from_email,
            from_name=settings.email.from_name,
        )
    else:
        return SMTPBackend(
            host=settings.email.smtp_host or "",
            port=settings.email.smtp_port,
            user=settings.email.smtp_user,
            password=settings.email.smtp_password,
            from_email=settings.email.from_email,
            from_name=settings.email.from_name,
            use_tls=settings.email.smtp_tls,
        )


def get_email_service() -> EmailService:
    """Get configured email service instance.

    Returns:
        EmailService with configured backend.
    """
    return EmailService(backend=get_backend())
