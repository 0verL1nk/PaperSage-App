# 在线本地访客工作区与账号导入

## Why

PaperSage needs a no-registration path on its deployed website. Visitor work should not create an anonymous server account or upload drafts and source files behind the user's back. Store a real, editable workspace in the browser and let the user explicitly import it into an account after registration or login.

## What Changes

- Replace server-side guest identities and guest endpoints with a durable browser-local guest workspace backed by IndexedDB.
- Route supported project, writing-state, document, and LaTeX file operations to the local store while in guest mode; do not send those records to the server.
- Clearly disclose local-only retention and browser-storage limits. Server AI generation and compilation require an account and must be labeled as such.
- After account authentication, show a reviewable import action. Import projects and their writing state/files into the authenticated account, resume safely after partial failure, and retain local data until the complete import is confirmed.
- Keep login/register dynamic background and responsive improvements. Cover authentication, local guest landing, guest workspace, and account import across mobile, tablet, and desktop.

## Capabilities

### New Capabilities

- `browser-local-guest-workspace`: browser-only identity and persistent editable guest data.
- `guest-workspace-account-import`: explicit, retryable transfer of local guest projects into an authenticated account.

### Modified Capabilities

- `writing-workbench`: project scenario and three-phase writing state work in both authenticated and local guest modes.

## Impact

- Frontend: IndexedDB persistence, local API adapter, session/auth routing, import review/progress/retry UI, responsive E2E.
- Backend: remove anonymous guest user/session/quota tables, routes, cleanup worker, and guest-only account quotas; retain normal authenticated APIs and the writing-workbench schema.
- Documentation: replace server guest-account contract with local retention, import, privacy, and capability boundaries.
