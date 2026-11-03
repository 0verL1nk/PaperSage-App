## ADDED Requirements

### Requirement: Browser compilation binds its actual input snapshot
The system SHALL bind each browser compilation to an immutable manifest of file revisions or draft content digests, selected engine and resource profile; it SHALL distinguish draft preview, saved-version result and an outdated result without silently substituting newer file content during execution.

#### Scenario: Compile unsaved local edits
- **WHEN** the user compiles a local draft that differs from saved content
- **THEN** the output SHALL be labeled as a draft preview with its input digest and SHALL NOT be claimed as the latest saved-version artifact

#### Scenario: Edit while compilation is running
- **WHEN** project content changes after the input manifest was fixed
- **THEN** the compile SHALL finish against its original inputs or report failure, and its result SHALL be marked outdated relative to the changed project

#### Scenario: Publish saved-version artifacts
- **WHEN** authenticated browser output for a saved manifest is published
- **THEN** the backend SHALL verify project ownership and manifest identity, persist idempotently, and mark it latest only when the manifest still matches current saved inputs
