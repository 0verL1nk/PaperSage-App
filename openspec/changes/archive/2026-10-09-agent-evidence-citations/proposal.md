## Why

The backend now persists exact RAG excerpts for each agent Run and exposes an owner-scoped evidence endpoint. The AI panel still hides retrieved source identity and excerpts inside raw tool JSON, so users cannot conveniently verify what material informed an answer or revisit evidence after refreshing the conversation.

## What Changes

- Show a compact, expandable “参考材料” section on assistant messages associated with a response.
- Load evidence through the authenticated Run evidence endpoint, including for historical messages.
- Present a human-readable source name and exact excerpt; distinguish empty, loading, and failed retrieval states.

## Capabilities

### New Capabilities
- **agent-evidence-citations**: Let users inspect the exact project evidence retrieved for an assistant Run.

### Modified Capabilities
- **agent-run-architecture**: Assistant message presentation includes persisted Run evidence.

## Impact

- Frontend API, chat state, AI panel, their tests, and a source-name snapshot column on persisted evidence.
- Extends the existing authenticated `GET /agent/runs/:run_id/evidence` response; no new route.
- Guest mode remains local-only and does not invoke the AI panel or evidence API.
