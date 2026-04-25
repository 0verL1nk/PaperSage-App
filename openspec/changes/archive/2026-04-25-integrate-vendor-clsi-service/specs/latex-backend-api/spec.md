## ADDED Requirements

### Requirement: PaperSage exposes its own LaTeX compile API boundary
The backend SHALL expose PaperSage-owned LaTeX endpoints for project compilation instead of requiring clients to call vendored `clsi` routes directly.

#### Scenario: Compile through a project-scoped backend endpoint
- **WHEN** an authenticated caller submits a compile request for a project
- **THEN** the backend MUST accept that request through a PaperSage route under `/api/v1/projects/{project_id}/latex`
- **AND** it MUST translate the project data into the vendor compile request format internally

### Requirement: Project authorization is enforced for LaTeX operations
The backend SHALL enforce PaperSage project access rules before invoking vendor LaTeX capabilities or serving output artifacts.

#### Scenario: Reject compile access for unauthorized users
- **WHEN** a caller does not have access to the target project
- **THEN** the backend MUST reject compile, SyncTeX, wordcount, and output-file operations for that project
- **AND** it MUST not forward those requests to the vendored service

### Requirement: Vendor responses are normalized into backend-facing contracts
The backend SHALL normalize vendor compile results, statuses, and artifact access into PaperSage response contracts.

#### Scenario: Return compile results using backend-managed output references
- **WHEN** the vendor service returns compile status, build id, and output file metadata
- **THEN** the backend MUST return those results inside the standard PaperSage API envelope
- **AND** the returned artifact references MUST point to backend-managed output endpoints rather than raw vendor URLs

#### Scenario: Preserve meaningful compile status for clients
- **WHEN** the vendor service reports statuses such as `success`, `failure`, `timedout`, `compile-in-progress`, or `conflict`
- **THEN** the backend MUST preserve those compile semantics in its own response payload
- **AND** it MUST distinguish vendor unavailability from user-facing compile errors
