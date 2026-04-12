"""Email backend interface definition."""
from abc import ABC, abstractmethod


class EmailBackend(ABC):
    """Abstract email backend interface.

    All email backends must implement this interface.
    """

    @abstractmethod
    async def send(self, to: str, subject: str, body: str) -> bool:
        """Send an email.

        Args:
            to: Recipient email address.
            subject: Email subject line.
            body: Email body content (plain text).

        Returns:
            True if email was sent successfully, False otherwise.
        """
        ...
