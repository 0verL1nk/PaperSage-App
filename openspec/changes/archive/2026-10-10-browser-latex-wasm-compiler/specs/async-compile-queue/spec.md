## MODIFIED Requirements

### Requirement: Async compile API
Compile jobs SHALL support browser execution while preserving the existing asynchronous job identity and status polling. Browser-originated jobs SHALL NOT be routed to a server-side TeX compiler.

#### Scenario: Submit compilation
- **WHEN** the project editor POSTs `/api/v1/projects/{id}/latex/compile/browser` for a supported browser engine
- **THEN** the backend creates a persisted `running` job, returns its `job_id` with HTTP 202, and does not enqueue CLSI work

#### Scenario: Poll job status
- **WHEN** the client GETs `/api/v1/projects/{id}/latex/compile/{job_id}/status`
- **THEN** the backend returns that authorized job's current or terminal status and available build ID, diagnostics, and elapsed time

#### Scenario: Job not found
- **WHEN** a client polls a job ID that does not exist or does not belong to the requested project
- **THEN** the API returns HTTP 404

#### Scenario: Unauthorized access
- **WHEN** a client polls a compile job for a project the user cannot access
- **THEN** the API returns HTTP 403 without disclosing the job data

#### Scenario: Complete a browser-executed job
- **WHEN** a browser worker successfully compiles a project and submits its artifacts
- **THEN** the backend associates the result with the authorized compile job, stores the artifacts, and exposes the terminal state through the existing job polling API

#### Scenario: Browser cannot run a required capability
- **WHEN** the selected browser engine cannot run a required engine or project capability
- **THEN** the job reports an actionable unsupported-capability error through the existing job polling API and is not sent to CLSI

#### Scenario: Legacy server compile submission is disabled
- **WHEN** a client submits a compile through the legacy server-side endpoint
- **THEN** the API returns a clear deprecation response and does not enqueue CLSI work

#### Scenario: Agent requests compilation
- **WHEN** the agent invokes `compile_project` for a valid project root file
- **THEN** the current writing page starts a browser job using the user's selected browser engine and the backend does not compile the source
