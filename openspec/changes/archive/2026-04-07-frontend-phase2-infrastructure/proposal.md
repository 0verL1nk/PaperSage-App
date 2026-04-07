## Why

Phase 1 has established the React, Tailwind, and shadcn foundation, but the frontend still lacks the runtime infrastructure needed to build real product flows. Phase 2 is needed now so later auth, documents, trash, uploading, and coding work can share one environment contract, one API client, one session model, and one provider stack instead of reintroducing ad-hoc plumbing in each feature.

## What Changes

- Add frontend environment parsing for API base URL and runtime mode helpers.
- Add a shared API client that standardizes the backend envelope `{ code, data, info }`, `Bearer` access-token injection, multipart requests, blob downloads, and normalized error handling.
- Add global Zustand stores for access-token / refresh-token persistence and request-count-based loading UI.
- Expand the provider layer so the application bootstraps Query, Router, toast, and loading overlay behavior from one place.
- Add minimal shared support files such as `.env.example`, loading overlay, and auth-guard hook entry points needed to unblock the next migration phases.

## Capabilities

### New Capabilities
- `frontend-app-infrastructure`: Shared runtime infrastructure for env parsing, API access, session/loading state, and app-wide providers in the React frontend.

### Modified Capabilities
- `frontend-foundation`: Extend the frontend foundation requirements so the prepared source tree includes shared config, hooks, stores, and provider integration points that are actually wired for Phase 2.

## Impact

- Affected code: `frontend/src/app/*`, `frontend/src/shared/{api,config,hooks,stores,components}/*`, `frontend/.env.example`, `frontend/package.json`
- Affected systems: React app bootstrap, API integration layer, session persistence, loading feedback, route/provider composition
- Dependencies: TanStack Query, TanStack Router, Zustand, Axios, localStorage, Vite env handling
