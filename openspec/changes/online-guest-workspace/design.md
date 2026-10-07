# Design: browser-local guest workspace

## Product decision

The guest experience is hosted on the real PaperSage website, but its persistent project data stays in that browser profile. “Online guest” means the public site can be used without signing up; it does not mean the server creates an anonymous user. Guest data is not recoverable on another device or after browser storage is cleared. The user chooses when to transfer work to an account.

This borrows the low-friction, bounded-capability entry pattern from Figma FigJam open sessions, while accounting for PaperSage's single-user research projects and browser-local data. [Figma: Invite visitors to an open session](https://help.figma.com/hc/en-us/articles/4410786053911-Invite-visitors-to-an-open-session).

## Local data and session model

- Store a random local guest profile and all editable project content in IndexedDB. Use stable UUIDs and a versioned schema for projects, writing state, folders, document metadata/blobs, and LaTeX source files.
- Keep only the guest-mode flag/profile ID in browser session storage or the existing session store; never mint a fake bearer token and never call `/auth/guest`.
- Route project, writing-state, document, and LaTeX source APIs through a local adapter only while this browser profile is a guest. Authenticated sessions continue to use the existing Go APIs.
- Reloading or opening another tab in the same browser profile restores guest work. A different browser/device does not. “Clear local data” is explicit, warns that the data cannot be recovered, and requires confirmation.
- Use browser quota APIs when available for an estimate; handle IndexedDB unavailable/full and write errors with actionable messages. Do not promise a fixed size accepted by every browser.
- Never send local files, drafts, or project metadata to the API while in guest mode. AI generation and server LaTeX compilation require sign-in; the UI must explain that boundary before the action.

## Account import

- Login and registration remain ordinary account flows. After an authenticated session exists, detect a local guest bundle and offer a preview: project names/scenarios, file counts, total bytes, and the account receiving the data.
- Import only after explicit user confirmation. Create each project through the existing project API, then write its preparation/phase state, folders, documents, and LaTeX files through existing authenticated APIs.
- Use a stable bundle ID and local IDs to store per-account import mappings. Persist each successful mapping before continuing so retries resume without duplicate cloud projects or files.
- Keep all source rows/blobs locally until the entire selected import succeeds. On partial failure, retain everything, show which item failed, and offer retry. Clear local data only after an explicit final confirmation, or keep the local copy and report that it is now backed up.
- Never merge data into a different existing account silently. The user sees the current account and can cancel without changing either copy.

## Capability boundaries

| Capability | Local guest | Authenticated account |
|---|---|---|
| Create projects and choose scenario | IndexedDB | Existing API |
| Prepare/review state and stage changes | IndexedDB | Existing API |
| Attach documents and organize folders | IndexedDB blobs/metadata | Existing object-storage API |
| Edit LaTeX source | IndexedDB | Existing LaTeX file API |
| AI generation / server compile | Sign-in required; never silently upload local work | Existing configured providers |
| Cross-device access and durable backups | Unavailable until import | Account storage |

## Responsive and accessible behavior

- Preserve the current product design language and animated auth background; honor `prefers-reduced-motion`.
- At 320px and 390px mobile widths, make sign-in/register/guest entry and local save status reachable with one-column flow, vertical page scroll, and at least 44px touch targets. Dialogs and import progress stay inside the viewport and scroll internally when needed.
- At tablet and desktop widths, use available space without horizontal overflow. Keep project tabs, editor, and import preview readable.
- Keyboard flow must reach primary actions and every import choice; expose current stage and errors with labels/live regions.
- Automated route-mock E2E checks 320×640, 390×844, 768×1024, 1024×768, and 1440×900. Also test reload persistence, IndexedDB file blobs, quota failure, partial import retry, and no network calls in guest mode.

## Security and privacy

- Guest data is origin-scoped browser data. Treat same-device users as sharing that profile; show a warning not to store sensitive material on shared/public devices.
- A guest flag never grants backend authorization. Every request in guest mode for a network-only capability is blocked client-side with an explicit sign-in-required response; no guest bearer token is attached.
- Import uses the user's authenticated session and the same ownership checks as ordinary project creation. Bundle contents are validated before upload; errors do not delete local source data.
- Registration credentials and account tokens stay in the current auth flow; do not copy credentials into the guest bundle.

## Rollout and cleanup

1. Implement the IndexedDB contract and local API adapter before showing the public entry.
2. Add explicit import and retry behavior; keep guest and authenticated sessions distinguishable in the session store.
3. Remove server guest tables/endpoints/quotas and their migrations before applying this change to a shared release branch; retain workbench migrations.
4. Validate current and historical database fresh/handoff migrations, frontend unit/E2E, and production frontend build.
5. Document browser-specific data retention and server-only feature limits before release.

## Final import choice

- After a complete import, keep the local copy by default. Offer a separate delete action with a second confirmation that explains the browser copy cannot be restored from PaperSage.
