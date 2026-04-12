"""Tests for email service module."""
import pytest

from core.email.base import EmailBackend
from core.email.service import EmailService


class MockEmailBackend(EmailBackend):
    """Mock email backend for testing."""

    def __init__(self) -> None:
        self.sent_emails: list[tuple[str, str, str]] = []
        self.should_fail = False

    async def send(self, to: str, subject: str, body: str) -> bool:
        if self.should_fail:
            return False
        self.sent_emails.append((to, subject, body))
        return True


class TestEmailService:
    """Tests for EmailService class."""

    @pytest.fixture
    def mock_backend(self) -> MockEmailBackend:
        return MockEmailBackend()

    @pytest.fixture
    def service(self, mock_backend: MockEmailBackend) -> EmailService:
        return EmailService(backend=mock_backend)

    @pytest.mark.asyncio
    async def test_send_basic_email(self, service: EmailService, mock_backend: MockEmailBackend) -> None:
        """Test sending a basic email."""
        result = await service.send(
            to="user@example.com",
            subject="Test Subject",
            body="Test Body",
        )

        assert result is True
        assert len(mock_backend.sent_emails) == 1
        to, subject, body = mock_backend.sent_emails[0]
        assert to == "user@example.com"
        assert subject == "Test Subject"
        assert body == "Test Body"

    @pytest.mark.asyncio
    async def test_send_template_email(self, service: EmailService, mock_backend: MockEmailBackend) -> None:
        """Test sending a templated email."""
        result = await service.send_template(
            to="user@example.com",
            subject="Welcome, {{username}}!",
            template="Hello {{username}}, welcome to {{app_name}}.",
            variables={"username": "John", "app_name": "PaperSage"},
        )

        assert result is True
        assert len(mock_backend.sent_emails) == 1
        to, subject, body = mock_backend.sent_emails[0]
        assert subject == "Welcome, John!"
        assert body == "Hello John, welcome to PaperSage."

    @pytest.mark.asyncio
    async def test_send_template_missing_variable(self, service: EmailService, mock_backend: MockEmailBackend) -> None:
        """Test that missing template variable returns False."""
        result = await service.send_template(
            to="user@example.com",
            subject="Hello {{username}}",
            template="Hello {{username}}",
            variables={},  # Missing 'username'
        )

        assert result is False
        assert len(mock_backend.sent_emails) == 0

    @pytest.mark.asyncio
    async def test_send_failure(self, mock_backend: MockEmailBackend) -> None:
        """Test handling of send failure."""
        mock_backend.should_fail = True
        service = EmailService(backend=mock_backend)

        result = await service.send(
            to="user@example.com",
            subject="Test",
            body="Body",
        )

        assert result is False
        assert len(mock_backend.sent_emails) == 0

    def test_render_template(self, service: EmailService) -> None:
        """Test template rendering."""
        result = service._render_template(
            "Hello {{name}}, your code is {{code}}",
            {"name": "Alice", "code": "12345"},
        )
        assert result == "Hello Alice, your code is 12345"

    def test_render_template_missing_var(self, service: EmailService) -> None:
        """Test that missing variable raises KeyError."""
        with pytest.raises(KeyError):
            service._render_template(
                "Hello {{name}}",
                {},  # Missing 'name'
            )
