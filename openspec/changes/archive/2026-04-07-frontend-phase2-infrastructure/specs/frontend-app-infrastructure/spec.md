## ADDED Requirements

### Requirement: Frontend environment configuration is centralized
The frontend application SHALL expose a shared environment module that resolves runtime configuration for API access and application mode checks from one place.

#### Scenario: API base URL is available to shared code
- **WHEN** shared API or provider code imports the frontend environment module
- **THEN** it MUST be able to read the configured `VITE_API_BASE_URL`
- **AND** it MUST not require feature modules to access `import.meta.env` directly

#### Scenario: Missing required runtime configuration fails clearly
- **WHEN** the frontend starts without a required environment value for the current mode
- **THEN** the environment module MUST fail with a clear developer-facing error
- **AND** the error MUST identify which variable is missing

### Requirement: Frontend API client standardizes backend access
The frontend application SHALL provide a shared API client that standardizes request construction and backend envelope handling for later feature modules.

#### Scenario: JSON responses use the backend envelope contract
- **WHEN** a feature performs a JSON request through the shared API client
- **THEN** the client MUST parse the backend envelope `{ code, data, info }`
- **AND** it MUST return typed `data` for successful responses

#### Scenario: Authorization header uses bearer access token format
- **WHEN** an access token is available and a protected request is sent through the shared API client
- **THEN** the client MUST inject `Bearer <accessToken>` into the `Authorization` header
- **AND** feature modules MUST not construct authorization headers locally

#### Scenario: Multipart and blob requests are supported centrally
- **WHEN** a feature sends `multipart/form-data` or requests a blob through the shared API client
- **THEN** the client MUST provide a shared path for those request types
- **AND** feature modules MUST not reimplement multipart or blob transport behavior locally

#### Scenario: Transport and backend failures are normalized
- **WHEN** the backend returns a failure envelope or the request fails at the transport layer
- **THEN** the shared API client MUST return a normalized error shape
- **AND** the normalized error MUST preserve enough detail for UI feedback and debugging

### Requirement: Frontend session and loading state are shared
The frontend application SHALL provide shared Zustand stores for persisted auth token state and request-count-based loading state.

#### Scenario: Access and refresh tokens are restored on application boot
- **WHEN** the application starts in a browser with stored auth tokens
- **THEN** the session store MUST restore `accessToken` and `refreshToken` into runtime state
- **AND** shared API and auth-guard code MUST be able to read the access token from one place

#### Scenario: Global loading state supports concurrent requests
- **WHEN** multiple tracked requests overlap in time
- **THEN** the loading store MUST keep a request count rather than a single boolean
- **AND** the global loading state MUST remain active until all tracked requests complete

### Requirement: Provider composition exposes app-wide infrastructure
The frontend application SHALL compose shared providers so the app can boot with routing, server state, toast feedback, and loading feedback enabled from one entry point.

#### Scenario: Shared providers boot without business features
- **WHEN** the frontend renders its Phase 2 smoke route or placeholder shell
- **THEN** the provider layer MUST initialize Query, Router, toast, and loading overlay behavior without requiring migrated business pages
- **AND** the app MUST remain renderable before Phase 3 route content is complete

#### Scenario: Shared infrastructure is reusable from routes and hooks
- **WHEN** route files or feature hooks consume app infrastructure
- **THEN** they MUST be able to import the shared providers, auth guard, stores, and API client from stable shared paths
- **AND** they MUST not need page-local bootstrap code to access those capabilities
