## ADDED Requirements

### Requirement: Workspace layout provides back navigation
The workspace layout SHALL provide a way to navigate back to the main application.

#### Scenario: Back button is available
- **WHEN** the user visits any `/project/:projectId/*` route
- **THEN** a back button or link MUST be visible
- **AND** clicking it MUST navigate to `/app/documents`

### Requirement: Workspace layout provides home navigation
The workspace layout SHALL provide a way to navigate to the project home.

#### Scenario: Home button is available
- **WHEN** the user visits any `/project/:projectId/*` route
- **THEN** a home button or link to the project root MUST be visible

### Requirement: Workspace layout provides tab navigation
The workspace layout SHALL include a tab navigation for switching between project modules.

#### Scenario: Tab navigation is available
- **WHEN** the user visits any `/project/:projectId/*` route
- **THEN** tab navigation MUST be visible at the top
- **AND** tabs MUST include links to: uploading, coding, classifying, logic-chain, generating

### Requirement: Workspace layout uses projectId parameter
The workspace layout SHALL use the projectId from route parameters.

#### Scenario: Layout receives projectId
- **WHEN** the user visits `/project/:projectId/uploading`
- **THEN** the layout MUST have access to the projectId value
- **AND** navigation elements SHOULD reflect the current project