## ADDED Requirements

### Requirement: Email Backend Interface

The system SHALL provide a universal `EmailBackend` protocol that defines the contract for all email backends.

#### Scenario: Backend interface contract
- **WHEN** any email backend is implemented
- **THEN** it MUST implement `async def send(to: str, subject: str, body: str) -> bool` method

### Requirement: Resend Backend Support

The system SHALL provide a `ResendBackend` implementation that sends emails via Resend API.

#### Scenario: Successful email sending via Resend
- **WHEN** `ResendBackend.send()` is called with valid parameters
- **THEN** email SHALL be sent via Resend API
- **AND** function SHALL return `True`

#### Scenario: Resend API failure
- **WHEN** `ResendBackend.send()` is called but Resend API returns error
- **THEN** function SHALL return `False`
- **AND** error details SHALL be logged

### Requirement: Generic SMTP Backend Support

The system SHALL provide a `SMTPBackend` implementation that sends emails via standard SMTP protocol.

#### Scenario: Successful email sending via SMTP
- **WHEN** `SMTPBackend.send()` is called with valid SMTP configuration
- **THEN** email SHALL be sent via SMTP
- **AND** function SHALL return `True`

#### Scenario: SMTP connection failure
- **WHEN** `SMTPBackend.send()` is called but SMTP connection fails
- **THEN** function SHALL return `False`
- **AND** error details SHALL be logged

### Requirement: Email Service Wrapper

The system SHALL provide an `EmailService` class that wraps backend and provides simplified interface.

#### Scenario: Send email with default backend
- **WHEN** `EmailService.send()` is called
- **THEN** email SHALL be sent using the configured default backend

#### Scenario: Template-based email sending
- **WHEN** `EmailService.send_template()` is called with template and variables
- **THEN** template placeholders SHALL be replaced with corresponding values
- **AND** resulting email SHALL be sent

### Requirement: Backend Selection via Configuration

The system SHALL select the appropriate backend based on configuration.

#### Scenario: Use Resend backend when configured
- **WHEN** `email.provider` is set to `"resend"`
- **THEN** `get_backend()` SHALL return `ResendBackend` instance

#### Scenario: Use SMTP backend when configured
- **WHEN** `email.provider` is set to `"smtp"`
- **THEN** `get_backend()` SHALL return `SMTPBackend` instance

### Requirement: Environment-based Configuration

The system SHALL load email configuration from environment variables.

#### Scenario: Load Resend configuration
- **WHEN** environment contains `EMAIL_PROVIDER=resend`, `RESEND_API_KEY`, `RESEND_DOMAIN`, `FROM_EMAIL`
- **THEN** Resend backend SHALL be configured with these values

#### Scenario: Load SMTP configuration
- **WHEN** environment contains `EMAIL_PROVIDER=smtp`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `FROM_EMAIL`
- **THEN** SMTP backend SHALL be configured with these values
