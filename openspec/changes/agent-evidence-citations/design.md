## Context

`AgentMessage` history includes an optional `runId`. During a live stream the store knows the created Run ID, but currently discards it when `finish` is reduced. The RAG hit already includes a human-readable source name, but the evidence record and endpoint do not preserve or return it.

## Decisions

1. Persist the retrieved source name on each evidence record with a nullable-safe default and backfill from `rag_chunks` where possible. Return it as `sourceName` from the existing endpoint.
2. Preserve the Run ID on the completed assistant message. For live messages, attach the store's active Run ID on `finish` and `abort`; for history, map `runId` from the message DTO.
3. Fetch evidence with TanStack Query keyed by Run ID. This avoids duplicating evidence in chat state, supports request deduplication, and reloads evidence for history after refresh.
4. Render a compact `参考材料 N 条` disclosure beneath the assistant response. Each item shows the source name and exact excerpt. Do not show UUIDs, hashes, or internal chunk IDs; do not label a chunk ordinal as a physical page number.
5. Empty evidence remains hidden. Loading, request failure, and a successful empty response are distinct; retry is available on failure. Evidence is read-only.
6. Preserve owner isolation by relying on the existing authenticated endpoint. Do not add evidence IDs or caller-controlled project IDs to a new API contract.

## Risks

- Historical messages with no Run ID cannot display evidence and should render no empty placeholder.
- Tool output may still include raw IDs for debugging; the evidence disclosure is the user-facing verification surface, not a support-quality or citation-validity score.
- Older evidence may lack a source name when its source has already been removed; show a neutral `项目资料` fallback rather than an internal ID.
- Very long excerpts must wrap within the panel and not widen the editor layout.
