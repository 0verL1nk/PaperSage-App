## ADDED Requirements

### Requirement: Browser-local guest workspace
The deployed website SHALL allow a visitor to use a persistent guest workspace without creating a server identity.

#### Scenario: Visitor starts a guest workspace
- **WHEN** a visitor chooses the guest entry on the public site
- **THEN** the browser SHALL create or restore a local guest profile and open a real editable workspace
- **AND** guest project content SHALL be stored in origin-scoped browser storage
- **AND** no anonymous user row, guest token, or guest-session API request SHALL be created

#### Scenario: Visitor reloads or changes device
- **WHEN** the same browser profile reloads the website
- **THEN** its guest projects, writing state, and locally stored files SHALL be restored
- **WHEN** the visitor uses another browser/device or clears site data
- **THEN** the local workspace SHALL not be represented as cloud-backed or recoverable

### Requirement: Guest network privacy and capability boundaries
Guest content SHALL remain in browser storage until the user explicitly imports it into an authenticated account.

#### Scenario: Guest edits local content
- **WHEN** a guest creates or edits a project, writing state, document, folder, or LaTeX source
- **THEN** the operation SHALL persist locally and SHALL NOT send that content to the backend

#### Scenario: Guest requests an account-backed capability
- **WHEN** a guest requests server-side AI generation, LaTeX compilation, or another capability that requires an account
- **THEN** the system SHALL explain that sign-in is required
- **AND** the system SHALL NOT silently upload guest content or attach a fabricated guest bearer token

#### Scenario: Browser storage is unavailable or full
- **WHEN** IndexedDB cannot be opened or a local write exceeds browser storage limits
- **THEN** the system SHALL preserve existing local data and show a clear, actionable error

### Requirement: Explicit guest-to-account import
The system SHALL provide an explicit, resumable import of local guest work after account authentication.

#### Scenario: User reviews and confirms import
- **WHEN** an authenticated account is available and local guest data exists
- **THEN** the system SHALL show the destination account and an inventory of the selected local data
- **AND** the system SHALL upload only after the user confirms

#### Scenario: Import succeeds
- **WHEN** every selected project, writing state, folder, document, and LaTeX source has been imported
- **THEN** the cloud copy SHALL be owned by the authenticated account
- **AND** the local copy SHALL remain until the user explicitly chooses to delete it

#### Scenario: Import partially fails
- **WHEN** one or more items fail during import
- **THEN** the system SHALL retain all local source data and successful import mappings
- **AND** a retry SHALL resume without creating duplicate projects or files

#### Scenario: User cancels or changes account
- **WHEN** the user cancels import or authenticates into a different account
- **THEN** local guest data SHALL remain unchanged and no import SHALL occur without a new confirmation

### Requirement: Responsive guest experience
Guest entry, account forms, local workspace, and import dialog SHALL remain usable on mobile, tablet, and desktop.

#### Scenario: Visitor uses a phone
- **WHEN** the viewport is 320px or 390px wide
- **THEN** primary actions SHALL remain reachable, touch targets SHALL be at least 44px, and content SHALL not overflow horizontally
- **AND** page and dialog content SHALL remain vertically scrollable without fixed navigation covering the active controls
