## Why

The agent architecture already defines `agent_evidence` and `agent_wiki_sources`, but the retrieval runtime does not persist evidence identities or source versions. Retrieval output currently contains only a transient ordinal, source name, and text, so later runs and users cannot reliably trace a claim to the exact material excerpt that produced it.

## What Changes

- Persist each RAG result used by a Run with its source type/ID, stable chunk ID, SHA-256 version of the exact excerpt, locator, and quote.
- Make persistence idempotent for the same Run, chunk, and excerpt version.
- Add an authenticated Run-scoped evidence query endpoint with ownership checks.
- Include the persisted evidence identity in retrieval tool output so the agent can refer to evidence without inventing IDs.

## Capabilities

### New Capabilities
- **agent-evidence-provenance**: Persist and retrieve exact source excerpts used by an agent Run.

### Modified Capabilities
- None.

## Impact

- Backend: `server/internal/agent` retrieval tool, evidence persistence, and Run-scoped API.
- Database: unique index on existing `agent_evidence` identity columns; no new table.
- Frontend: no change in this slice; the endpoint enables a later citation UI.
- Documentation: update the AI writing design and handoff status after implementation.
