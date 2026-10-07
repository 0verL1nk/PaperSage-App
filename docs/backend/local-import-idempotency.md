# Local-to-account import idempotency

The browser-local guest workspace imports one item at a time after sign-up or login. Checkpointing remains useful for progress, but each create request also carries a durable server idempotency key so a lost response or a browser restart cannot create a second cloud resource.

## Request contract

Import create calls send both headers:

- `X-PaperSage-Import-Bundle-Id`: stable UUID for one local import bundle.
- `X-PaperSage-Import-Local-Id`: stable UUID for one local item within that bundle.

The server infers the resource type from the endpoint. Both headers omitted means the existing ordinary create behavior. Supplying only one header, an invalid UUID, or the nil UUID returns validation error. The server computes the content fingerprint; clients do not submit it.

Covered create endpoints:

- `POST /api/v1/projects/`
- `POST /api/v1/folders/`
- `POST /api/v1/documents/upload` (multipart; staged and uploaded documents)
- `POST /api/v1/projects/:project_id/latex-files` (JSON and multipart forms)

The receipt scope is `(owner_id, bundle_id, resource_type, local_id)`. Same key and same normalized content returns the existing resource through the endpoint's usual response envelope and ID. Reusing that key with changed content returns HTTP 409. The import is per-item; there is no all-or-nothing transaction across a bundle. Update, delete, and staged-document confirmation endpoints do not consume import receipts.

## Persistence and failure behavior

`import_receipts` stores the unique scoped key, SHA-256 fingerprint, resource type and resource ID. The receipt and resource row commit in one database transaction. The unique index arbitrates concurrent retries in PostgreSQL, MySQL/TiDB, and SQLite; a retry reads the winning receipt after the first transaction commits. If the referenced resource was subsequently deleted, the same key returns 409 and the client must use a new local item ID for a deliberate recreation.

Document and LaTeX uploads write to deterministic keys derived from project/bundle/local IDs before committing the resource and receipt. If the database transaction fails, the handler attempts to delete the object. If the process stops after storage accepts the object but before database commit, a retry writes the same deterministic key and then commits one resource. This is item-level retry safety across the DB/object-store boundary, not atomic multi-item bundle import.

Fingerprints include the normalized resource fields that affect creation. Project fingerprints cover name, description, group and scenario; folder fingerprints cover name, project and parent; document fingerprints cover project/folder, name, MIME type, remark, staged/uploaded status and file bytes; LaTeX fingerprints cover project, normalized file path, content type and file bytes.
