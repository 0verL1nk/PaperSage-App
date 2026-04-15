"""Email service with template support."""

import logging
import re
from typing import Any

from core.email.base import EmailBackend

logger = logging.getLogger(__name__)

# Simple template placeholder pattern: {{variable_name}}
TEMPLATE_PATTERN = re.compile(r"\{\{(\w+)\}\}")


class EmailService:
    """Email service that wraps backend and provides additional features."""

    def __init__(self, backend: EmailBackend) -> None:
        """Initialize email service.

        Args:
            backend: Email backend implementation.
        """
        self._backend = backend

    async def send(self, to: str, subject: str, body: str) -> bool:
        """Send a plain text email.

        Args:
            to: Recipient email address.
            subject: Email subject.
            body: Email body content.

        Returns:
            True if sent successfully, False otherwise.
        """
        return await self._backend.send(to, subject, body)

    async def send_template(
        self,
        to: str,
        subject: str,
        template: str,
        variables: dict[str, Any],
    ) -> bool:
        """Send an email using a template with variable substitution.

        Placeholders in template use {{variable_name}} syntax.

        Args:
            to: Recipient email address.
            subject: Email subject (can also use template variables).
            template: Email body template with {{variable}} placeholders.
            variables: Dictionary of variable names to values.

        Returns:
            True if sent successfully, False otherwise.

        Example:
            await service.send_template(
                to="user@example.com",
                subject="Welcome, {{username}}!",
                template="Hello {{username}}, welcome to {{app_name}}.",
                variables={"username": "John", "app_name": "PaperSage"},
            )
        """
        try:
            # Substitute placeholders in subject and body
            rendered_subject = self._render_template(subject, variables)
            rendered_body = self._render_template(template, variables)
            return await self.send(to, rendered_subject, rendered_body)
        except KeyError as e:
            logger.error("Missing template variable: %s", e)
            return False

    def _render_template(self, template: str, variables: dict[str, Any]) -> str:
        """Render a template by replacing placeholders with variables.

        Args:
            template: Template string with {{variable}} placeholders.
            variables: Variable values.

        Returns:
            Rendered string with placeholders replaced.

        Raises:
            KeyError: If a placeholder has no corresponding variable.
        """

        def replacer(match: re.Match[str]) -> str:
            var_name = match.group(1)
            if var_name not in variables:
                raise KeyError(var_name)
            return str(variables[var_name])

        return TEMPLATE_PATTERN.sub(replacer, template)
