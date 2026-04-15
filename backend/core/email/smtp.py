"""Generic SMTP email backend implementation."""

import asyncio
import logging
import smtplib
from email.message import EmailMessage

from app.config import get_settings
from core.email.base import EmailBackend

logger = logging.getLogger(__name__)

SMTP_TIMEOUT = 10  # seconds


class SMTPBackend(EmailBackend):
    """Email backend using standard Python SMTP."""

    def __init__(
        self,
        host: str,
        port: int,
        user: str | None,
        password: str | None,
        from_email: str,
        from_name: str = "PaperSage",
        use_tls: bool = True,
    ) -> None:
        """Initialize SMTP backend.

        Args:
            host: SMTP server hostname.
            port: SMTP server port.
            user: SMTP authentication username.
            password: SMTP authentication password.
            from_email: Sender email address.
            from_name: Sender display name.
            use_tls: Whether to use TLS/STARTTLS.
        """
        self._host = host
        self._port = port
        self._user = user
        self._password = password
        self._from_email = from_email
        self._from_name = from_name
        self._use_tls = use_tls

    async def send(self, to: str, subject: str, body: str) -> bool:
        """Send email via SMTP.

        Args:
            to: Recipient email address.
            subject: Email subject line.
            body: Email body content.

        Returns:
            True if sent successfully, False otherwise.
        """
        try:
            msg = EmailMessage()
            msg["From"] = f"{self._from_name} <{self._from_email}>"
            msg["To"] = to
            msg["Subject"] = subject
            msg.set_content(body)

            # Run SMTP in thread pool to avoid blocking
            await asyncio.get_event_loop().run_in_executor(
                None,
                self._send_sync,
                msg,
                to,
            )
            logger.info("Email sent successfully to %s via SMTP", to)
            return True
        except Exception as e:
            logger.error("Failed to send email via SMTP to %s: %s", to, e)
            return False

    def _send_sync(self, msg: EmailMessage, to: str) -> None:
        """Synchronous SMTP send (runs in executor)."""
        with smtplib.SMTP(self._host, self._port, timeout=SMTP_TIMEOUT) as server:
            if self._use_tls:
                server.starttls()
            if self._user and self._password:
                server.login(self._user, self._password)
            server.send_message(msg, self._from_email, to)


def create_smtp_backend() -> SMTPBackend:
    """Create SMTP backend from settings."""
    settings = get_settings()
    return SMTPBackend(
        host=settings.email.smtp_host,
        port=settings.email.smtp_port,
        user=settings.email.smtp_user,
        password=settings.email.smtp_password,
        from_email=settings.email.from_email,
        from_name=settings.email.from_name,
        use_tls=settings.email.smtp_tls,
    )
