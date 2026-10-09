# agent-evidence-citations Specification

## Purpose
This capability helps authenticated project owners inspect the source material used by AI responses.
## Requirements
### Requirement: Run evidence is inspectable in the AI conversation
The system SHALL let an authenticated project owner inspect the persisted evidence associated with an assistant Run from both live and historical assistant messages.

#### Scenario: Evidence exists for a completed Run
- **WHEN** the owner views an assistant message whose Run has persisted evidence
- **THEN** the system shows an expandable list with a human-readable source name and the exact retrieved excerpt
- **AND** it does not expose internal UUIDs, excerpt hashes, or chunk identifiers as user-facing citations

#### Scenario: Evidence is empty or unavailable
- **WHEN** evidence is loading, empty, or cannot be retrieved
- **THEN** the system distinguishes loading from request failure, hides the evidence disclosure for a successful empty result, and offers retry after failure

#### Scenario: User does not own the Run
- **WHEN** evidence retrieval is requested for a Run the current user does not own
- **THEN** the server returns the existing not-found response and the UI does not expose evidence details

