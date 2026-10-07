## 1. Browser-local guest workspace

- [x] 1.1 Versioned IndexedDB schema and typed local persistence for project/scenario, writing state, folders, document blobs, and LaTeX source files.
- [x] 1.2 Local guest session restore across reloads without guest bearer tokens or server guest API calls.
- [x] 1.3 Route supported project, writing-state, document, and LaTeX source operations to the local adapter while guest mode is active.
- [x] 1.4 Show actionable IndexedDB unavailable, blocked, and quota errors; do not discard existing local data on import failure.
- [x] 1.5 Explicitly gate server AI and compilation capabilities with an account-required explanation; never upload guest drafts implicitly.

## 2. Account import

- [x] 2.1 After ordinary login/registration, show a preview of local projects, scenarios, documents, LaTeX files, and destination account.
- [x] 2.2 Require explicit confirmation before uploading local guest data.
- [x] 2.3 Import projects, writing state, folders, documents, and LaTeX source through authenticated APIs.
- [x] 2.4 Save stable account-scoped mappings and resume after partial failures without duplicates.
- [x] 2.5 Keep local source data until full import succeeds; offer explicit local-copy deletion only after confirmation.
- [x] 2.6 Cover complete import, account-scoped resume, interrupted-item retry, and trash-state preservation with service tests; UI always displays the authenticated destination account before confirmation.

## 3. Remove server-side guest implementation

- [x] 3.1 Remove guest user/session schema, migrations, config, API endpoints, token/middleware branches, cleanup worker, and guest-only quotas.
- [x] 3.2 Preserve writing-workbench migrations and database handoff fixes; run SQLite unit tests and PostgreSQL/MySQL fresh/handoff integration tests.

## 4. Responsive design and accessibility

- [x] 4.1 Keep dynamic login/register background and support reduced motion.
- [x] 4.2 Check auth and guest entry at 320×640, 390×844, 768×1024, 1024×768, and 1440×900 for overflow, scroll, main-action reachability, and 44px targets.
- [x] 4.3 Check local guest prepare/write/review/library, account-import dialog, and workspace navigation at the same viewports, including keyboard and fixed-nav coverage.
- [x] 4.4 Show device-local retention/privacy warning and accessible storage/import status.

## 5. Verification and docs

- [x] 5.1 Frontend unit/type/lint/build and browser-local persistence tests pass.
- [x] 5.2 E2E confirms no network calls for guest project content and reload persistence; service tests confirm complete import, checkpointed retry, and trash preservation.
- [x] 5.3 Backend vet/unit and isolated PostgreSQL/MySQL migration checks pass after server guest removal.
- [x] 5.4 Update `docs/PROJECT.md`, guest storage/import operations docs, and handoff with verified behavior and remaining provider/deployment boundaries.
- [x] 5.5 Run `openspec validate online-guest-workspace`.
