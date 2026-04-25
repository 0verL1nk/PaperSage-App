## ADDED Requirements

### Requirement: Source locations can synchronize to PDF positions
The system SHALL support SyncTeX source-to-PDF lookup for compiled LaTeX projects through the vendored service.

#### Scenario: Map code coordinates to PDF coordinates
- **WHEN** a caller provides a project identifier, source file path, line, and column for a compiled project
- **THEN** the backend MUST forward that lookup to the vendored SyncTeX code-to-PDF capability
- **AND** the response MUST return PDF position data associated with the compiled output

### Requirement: PDF positions can synchronize to source locations
The system SHALL support SyncTeX PDF-to-source lookup for compiled LaTeX projects through the vendored service.

#### Scenario: Map PDF coordinates to source coordinates
- **WHEN** a caller provides a project identifier, PDF page number, and PDF coordinates for a compiled project
- **THEN** the backend MUST forward that lookup to the vendored SyncTeX PDF-to-code capability
- **AND** the response MUST return source file and position data associated with the compiled project resources
