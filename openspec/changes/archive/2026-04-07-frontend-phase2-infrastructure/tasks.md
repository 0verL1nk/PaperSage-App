## 1. Environment And Shared Contracts

- [x] 1.1 Add `frontend/.env.example` with the documented `VITE_API_BASE_URL` placeholder
- [x] 1.2 Implement `frontend/src/shared/config/env.ts` for runtime env parsing and mode helpers
- [x] 1.3 Add shared API contract types in `frontend/src/shared/api/contracts.ts`
- [x] 1.4 Add normalized frontend error types in `frontend/src/shared/api/errors.ts`

## 2. API Client

- [x] 2.1 Implement the shared Axios instance in `frontend/src/shared/api/client.ts`
- [x] 2.2 Add backend envelope parsing for `{ code, data, info }` success and failure responses
- [x] 2.3 Add `Bearer <accessToken>` injection for the `Authorization` header
- [x] 2.4 Add shared helpers for `multipart/form-data` and blob download requests
- [x] 2.5 Expose client utilities from stable shared import paths

## 3. Session And Loading State

- [x] 3.1 Implement `frontend/src/shared/stores/session-store.ts` with browser-safe `accessToken` / `refreshToken` persistence
- [x] 3.2 Implement `frontend/src/shared/stores/ui-store.ts` with request-count-based loading state
- [x] 3.3 Add shared helpers for restoring session state during app boot
- [x] 3.4 Add shared helpers for incrementing and decrementing tracked loading operations

## 4. Provider And UI Wiring

- [x] 4.1 Add `frontend/src/shared/components/loading-overlay.tsx`
- [x] 4.2 Add `frontend/src/shared/hooks/use-auth-guard.ts` as the shared auth gate entry point
- [x] 4.3 Expand `frontend/src/app/providers.tsx` to compose Query, Router, toast, and loading overlay support
- [x] 4.4 Update app bootstrap files so the frontend renders a Phase 2 smoke route or placeholder shell through the provider stack
- [x] 4.5 Ensure shared API, store, and guard modules are importable from route files and feature hooks without local bootstrap code

## 5. Verification

- [x] 5.1 Verify the Phase 2 provider stack renders without business APIs wired
- [x] 5.2 Run `pnpm typecheck` in `frontend`
- [x] 5.3 Run `pnpm build` in `frontend`
- [x] 5.4 Update `docs/2026-03-27-frontend-migration-todo.md` to reflect completed Phase 2 items once implementation lands
