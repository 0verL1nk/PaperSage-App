## MODIFIED Requirements

### Requirement: Persist exact retrieval evidence for an agent Run
The system SHALL persist each retrieved chunk before exposing it to the model, retaining source identity, stable chunk identity, exact excerpt and excerpt digest, and additionally binding new evidence to an immutable source revision, extraction and verifiable locator. Existing excerpt hashes MUST NOT be reinterpreted as whole-source versions.

#### Scenario: A search result is used by the agent
- **WHEN** `rag_search` returns a project chunk
- **THEN** the root Run evidence SHALL identify its exact source revision, extraction, locator and quote before the model receives it
- **AND** only currently authorized and analysis-eligible sources SHALL be used

#### Scenario: The same excerpt is retrieved more than once in a Run
- **WHEN** a Run retrieves the same excerpt from the same source revision and extraction again
- **THEN** it SHALL reuse the canonical evidence record; different source revisions SHALL NOT be merged merely because their text matches

#### Scenario: Evidence persistence fails
- **WHEN** evidence cannot be durably recorded
- **THEN** the retrieval tool SHALL fail without exposing the unrecorded excerpt to the model

#### Scenario: Legacy evidence has only an excerpt hash
- **WHEN** historical evidence cannot be bound to a verified source revision or page
- **THEN** it SHALL retain its original hash semantics and be marked legacy/unverified, without fabricated source versions or page numbers

### Requirement: Retrieve evidence through an owned Run
The system SHALL check both Run/project ownership and current source read permissions when returning evidence, including historical messages and resumable events; analysis eligibility and read permission SHALL remain distinct.

#### Scenario: The Run owner requests evidence
- **WHEN** the owner requests evidence whose source remains readable
- **THEN** records SHALL be returned deterministically with human-readable source metadata, exact excerpts, version and locator status

#### Scenario: A user requests another user's evidence
- **WHEN** the requester does not own the Run's project
- **THEN** the response SHALL be not found and SHALL disclose no evidence metadata or quotes

#### Scenario: Source read permission is lost
- **WHEN** a historical or live result depends on a source no longer readable by the requester
- **THEN** protected evidence and dependent content SHALL be replaced by an unavailable state rather than leaked through cache, tools, history or SSE replay

#### Scenario: Source excluded from future analysis
- **WHEN** the owner may still read a source but has excluded it from analysis
- **THEN** authorized history MAY show it as excluded, while new context assembly and proposal application SHALL reject it as an active dependency
