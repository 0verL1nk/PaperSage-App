## MODIFIED Requirements

### Requirement: Run evidence is inspectable in the AI conversation
The system SHALL let an authenticated owner inspect evidence in live and historical messages, navigate to a verified source locator when available, and understand version, exclusion and unavailability without presenting internal IDs as citations or implying that retrieval proves a claim.

#### Scenario: Evidence exists for a completed Run
- **WHEN** the owner views an assistant message with readable persisted evidence
- **THEN** an expandable list SHALL show source names, exact excerpts and revision/applicability status
- **AND** internal UUIDs, hashes and chunk ordinals SHALL NOT be used as user-facing citations

#### Scenario: Evidence is empty or unavailable
- **WHEN** evidence is loading, empty, failed, legacy or no longer readable
- **THEN** the UI SHALL distinguish those states, offer retry for transient failure and avoid displaying protected excerpts after permission loss

#### Scenario: User does not own the Run
- **WHEN** a user requests evidence for another user's Run
- **THEN** the existing not-found behavior SHALL remain and no evidence details SHALL be shown

#### Scenario: A verified locator exists
- **WHEN** the owner opens an evidence entry with a verified PDF physical page or text/paragraph range
- **THEN** the UI SHALL open the matching immutable source revision at that locator and label the coordinate kind accurately

#### Scenario: Locator cannot be verified
- **WHEN** evidence lacks a valid original-file locator
- **THEN** the UI SHALL provide the readable file/excerpt and an explicit positioning limitation, without invented page numbers or fake highlights
