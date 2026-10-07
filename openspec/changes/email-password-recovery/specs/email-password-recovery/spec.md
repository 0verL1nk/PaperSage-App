## ADDED Requirements

### Requirement: Email-based password recovery
The system SHALL let users request a password reset through a previously verified email address without revealing whether an account exists.

#### Scenario: Request for an eligible account
- **WHEN** a client submits a registered, verified email address
- **THEN** the system returns the same generic success response used for unknown or ineligible addresses
- **AND** sends a short-lived password-reset link only to the verified address

#### Scenario: Request for an unknown or unverified address
- **WHEN** a client submits an unknown address or an address not verified on an account
- **THEN** the system returns the same generic success response
- **AND** sends no email

#### Scenario: Reset token is valid
- **WHEN** a user submits a valid, unexpired reset token and a policy-compliant new password
- **THEN** the system changes the password exactly once
- **AND** invalidates other reset tokens and all existing sessions
- **AND** requires the user to sign in with the new password

#### Scenario: Reset token is invalid, expired, or already used
- **WHEN** a user submits an invalid, expired, or consumed reset token
- **THEN** the system leaves the password and sessions unchanged
- **AND** returns a generic invalid-or-expired-link response

#### Scenario: Reset requests are abusive
- **WHEN** a client exceeds the configured in-process request limits
- **THEN** the system does not send another email
- **AND** retains the generic request response to prevent account enumeration
