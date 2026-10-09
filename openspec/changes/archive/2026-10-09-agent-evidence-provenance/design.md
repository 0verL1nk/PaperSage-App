## Context

RAG hits are produced by `rag.Store.Search` and consumed inside the `search_references` sub-agent. Evidence must belong to the root assistant Run because that is the user-facing turn; child Runs remain execution/audit detail. The existing `AgentEvidence` model already has fields for source identity, source version, chunk identity, locator, and quote.

## Decisions

1. `source_version` is the lowercase SHA-256 hex digest of the exact chunk text returned by retrieval. `chunk_id` remains the stable index identity (`source_type:source_id:chunk_index`). Together, these identify the exact retrieved excerpt even if a later re-index replaces the content at that chunk position.
2. `locator` records the chunk index when it can be parsed from the stable chunk ID. It does not claim a physical PDF page or paragraph location that the index does not retain.
3. Evidence writes are idempotent on `(run_id, chunk_id, source_version)`. A unique database index enforces this across MySQL, PostgreSQL, and SQLite-backed tests.
4. A retrieval result is not returned to the model if evidence persistence fails. This prevents the agent from making claims using provenance that the application failed to record.
5. `GET /agent/runs/:run_id/evidence` authorizes through the Run's conversation and current user, returns deterministic order, and returns 404 for absent or unauthorized Runs.
6. This slice does not add source revocation/staleness processing, wiki source dependency tracking, physical document locations, or a frontend citation renderer.

## Data flow

```text
rag_search → hits → persist evidence on root Run → return evidence IDs + excerpts to sub-agent
                                                ↘ GET Run evidence (owner checked)
```

## Risks

- A chunk digest versions only the retrieved excerpt, not the entire original document. The API and docs must describe that scope explicitly until source-level immutable versions are implemented.
- Quotes duplicate chunk text in the primary database. Current chunks are bounded; a future retention policy may move long excerpts to object storage.
