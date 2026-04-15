"""Resend email backend implementation."""

import logging

import resend

from app.config import get_settings
from core.email.base import EmailBackend

logger = logging.getLogger(__name__)


class ResendBackend(EmailBackend):
    """Email backend using Resend API."""

    def __init__(
        self, api_key: str, domain: str, from_email: str, from_name: str = "PaperSage"
    ) -> None:
        """Initialize Resend backend.

        Args:
            api_key: Resend API key.
            domain: Resend verified domain.
            from_email: Sender email address.
            from_name: Sender display name.
        """
        self._client = resend.Emails
        self._domain = domain
        self._from_email = from_email
        self._from_name = from_name
        # Set API key for the client
        resend.api_key = api_key

    async def send(self, to: str, subject: str, body: str) -> bool:
        """Send email via Resend API.

        Args:
            to: Recipient email address.
            subject: Email subject line.
            body: Email body content.

        Returns:
            True if sent successfully, False otherwise.
        """
        try:
            params: resend.Emails.sendParams = {
                "from": f"{self._from_name} <{self._from_email}>",
                "to": to,
                "subject": subject,
                "text": body,
            }
            self._client.send(params)
            logger.info("Email sent successfully to %s via Resend", to)
            return True
        except Exception as e:
            logger.error("Failed to send email via Resend to %s: %s", to, e)
            return False


def create_resend_backend() -> ResendBackend:
    """Create Resend backend from settings."""
    settings = get_settings()
    return ResendBackend(
        api_key=settings.email.resend_api_key,
        domain=settings.email.resend_domain,
        from_email=settings.email.from_email,
        from_name=settings.email.from_name,
    )
