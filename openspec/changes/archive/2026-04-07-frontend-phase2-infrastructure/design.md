## Context

The React frontend now has a usable Phase 1 design baseline, but runtime infrastructure is still missing. There is no centralized env parsing, no shared API client contract, no persisted session/loading stores, and no provider composition that can host Query, Router, toast, and loading overlay behavior together.

This change spans `src/app`, `src/shared/api`, `src/shared/config`, `src/shared/hooks`, `src/shared/stores`, and shared UI feedback. It also has migration constraints inherited from the backend contract: the backend uses the envelope `{ code, data, info }`, auth requests use `multipart/form-data`, the new frontend should send `Authorization: Bearer <accessToken>`, and file preview endpoints may return blobs.

## Goals / Non-Goals

**Goals:**
- Establish one typed env entry point for API base URL and runtime mode helpers.
- Establish one shared API client abstraction that handles the backend response envelope, auth header injection, multipart payloads, blob downloads, and normalized errors.
- Establish one persisted session store for `accessToken` / `refreshToken` and one request-count-based loading store using Zustand.
- Wire the provider layer so the app can host Query, Router, toast, and global loading feedback without each feature rebuilding those concerns.
- Keep the infrastructure usable before any business routes are fully migrated.

**Non-Goals:**
- Implement business features such as auth forms, project list pages, uploading flows, or coding UI.
- Finalize all route files from Phase 3 onward.
- Introduce SSR, i18n runtime, or a backend contract rewrite.
- Replace the current Phase 1 preview page with production navigation in this change.

## Decisions

### Decision: Introduce a dedicated `frontend-app-infrastructure` capability
Phase 2 changes add new runtime behavior, not just refactoring details, so they should be captured as a first-class capability instead of being buried inside `frontend-foundation`.

Alternatives considered:
- Extend only `frontend-foundation`.
  Rejected because env parsing, API access, state, and providers are concrete product behaviors with their own acceptance criteria.

### Decision: Use a shared Axios-based client in `src/shared/api/client.ts`
The current dependency set already includes Axios, and Phase 2 requires standardized support for JSON, `multipart/form-data`, and blob downloads. A wrapped Axios instance keeps those concerns centralized while allowing request/response interceptors for auth and error normalization.

Alternatives considered:
- Use raw `fetch`.
  Rejected because multipart/blob ergonomics and centralized interceptor behavior would require more custom plumbing immediately.

### Decision: Normalize backend failures into one shared error shape
Features should not parse `{ code, data, info }` or transport failures individually. The client should convert backend envelope failures, HTTP failures, and unexpected exceptions into one predictable error object that can be rendered by toasts, page states, or route loaders.

Alternatives considered:
- Let each feature parse backend errors ad hoc.
  Rejected because it would recreate the coupling and inconsistency Phase 2 is meant to eliminate.

### Decision: Persist access and refresh tokens separately but keep fetched collections in Query
Auth tokens and login restoration belong in Zustand because they outlive a single component tree and need localStorage persistence. The frontend should store `accessToken` and `refreshToken` separately so refresh flow can be introduced later without redesigning the session model. API-backed collections remain in TanStack Query to avoid duplicated caches and manual invalidation complexity.

Alternatives considered:
- Put all API-backed data in Zustand.
  Rejected because it weakens cache semantics and creates unnecessary synchronization work.

### Decision: Track global loading with a counter store, not a boolean
The legacy app relied on request-count semantics, and multiple concurrent requests are expected once routed pages land. A counter-based store prevents race conditions where one request finishes early and incorrectly hides the loading overlay.

Alternatives considered:
- Use a single boolean `isLoading`.
  Rejected because it cannot represent overlapping network operations safely.

### Decision: Keep Phase 2 routable without depending on full Phase 3 screens
Providers and shared infra should compile and render with a smoke route or placeholder shell so the team can verify the stack before building auth and main/project route trees.

Alternatives considered:
- Delay provider wiring until route migration starts.
  Rejected because it would push infrastructure mistakes into later feature work and make failures harder to isolate.

## Risks / Trade-offs

- [Auth contract mismatch] → Keep explicit multipart helpers, switch request injection to `Bearer <accessToken>`, and verify the login response fields before auth UI work starts.
- [Error normalization hides backend detail] → Keep normalized error objects structured enough to preserve original `code`, HTTP status, and message fields for debugging.
- [Loading overlay becomes noisy] → Use request counting plus a single shared overlay entry point so later features opt into one consistent behavior.
- [Router integration may shift once real route files exist] → Keep provider composition modular so RouterProvider can move from preview wiring to route-tree wiring without changing shared API/state code.
- [LocalStorage persistence can break in non-browser contexts] → Gate persistence access behind browser-safe checks and keep store defaults functional without stored state.

## Migration Plan

1. Add `env.ts`, `.env.example`, and runtime helpers so the app has one source of truth for backend origin configuration.
2. Add API contracts, normalized error types, and the shared Axios client with auth, multipart, and blob helpers.
3. Add `session-store` and `ui-store`, including access / refresh token restore and request counter behavior.
4. Add `loading-overlay` and `use-auth-guard` primitives, then expand the provider layer to host Query, Router, toast, and overlay wiring.
5. Replace the current direct shell bootstrap with a smoke route or equivalent placeholder that proves the provider stack renders without business APIs.

Rollback strategy:
- Revert the change directory implementation commit(s) and restore the Phase 1 preview shell if Phase 2 integration destabilizes app boot.
- Because this change is frontend-only and pre-release, rollback is code rollback rather than data migration.

## Open Questions

- Should `VITE_API_BASE_URL` be required in all environments, or can development default to the legacy local backend origin when omitted?
- Should the global loading overlay be driven automatically by API client interceptors, manually by feature hooks, or by a hybrid approach?
- Should the session store persist only `accessToken` / `refreshToken`, or also retain a minimal user snapshot once auth routes are implemented?
