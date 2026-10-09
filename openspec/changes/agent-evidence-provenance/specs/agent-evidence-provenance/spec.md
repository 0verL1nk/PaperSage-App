## ADDED Requirements

### Requirement: Persist exact retrieval evidence for an agent Run
The system SHALL persist each RAG chunk returned to an agent Run with its source type, source ID, stable chunk ID, exact excerpt, and a SHA-256 digest of that excerpt before returning it to the model.

#### Scenario: A search result is used by the agent
- **WHEN** `rag_search` returns one or more chunks for a project
- **THEN** the system SHALL persist an evidence record for each chunk on the root assistant Run
- **AND** the record SHALL identify the source and exact retrieved excerpt version
- **AND** the model-visible result SHALL include the persisted evidence ID

#### Scenario: The same excerpt is retrieved more than once in a Run
- **WHEN** a Run retrieves the same stable chunk ID with the same excerpt digest again
- **THEN** the system SHALL return the existing evidence record without creating a duplicate

#### Scenario: Evidence persistence fails
- **WHEN** the system cannot persist a retrieval result
- **THEN** the retrieval tool SHALL fail without returning the unrecorded excerpt to the model

### Requirement: Retrieve evidence through an owned Run
The system SHALL expose persisted evidence only to the owner of the Run's conversation and project.

#### Scenario: The Run owner requests evidence
- **WHEN** an authenticated owner requests evidence for an existing Run
- **THEN** the system SHALL return its evidence records in deterministic order with source identity, excerpt digest, chunk locator, and quote

#### Scenario: A user requests another user's evidence
- **WHEN** an authenticated user requests evidence for a Run they do not own
- **THEN** the system SHALL return not found and SHALL NOT disclose evidence metadata or quotes
