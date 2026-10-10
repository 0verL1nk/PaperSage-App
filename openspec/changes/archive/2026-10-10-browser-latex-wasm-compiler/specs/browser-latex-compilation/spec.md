## ADDED Requirements

### Requirement: Initialize browser compilation resources
The system SHALL let a user initialize a selected browser LaTeX engine and persist its verified base resources locally before compiling a project.

#### Scenario: First-time initialization
- **WHEN** a user opens the LaTeX environment setup and starts initialization for an engine
- **THEN** the UI shows engine/resource download, verification, format generation, and cache progress until the profile is ready or a recoverable error is shown

#### Scenario: Reuse an initialized environment
- **WHEN** the user returns with a valid cached engine profile
- **THEN** the UI reports the profile as ready and compilation reuses its local resources without downloading them again

#### Scenario: Browser storage is insufficient
- **WHEN** initialization cannot persist resources because of quota or storage failure
- **THEN** the UI explains that the engine is not ready, preserves retry/clear-cache actions, and does not claim offline readiness

#### Scenario: Resume initialization after leaving
- **WHEN** a user closes or reloads the page during a resource download and later returns
- **THEN** the setup resumes from persisted byte ranges using HTTP Range and validates the completed resource before marking it cached

#### Scenario: Resource source cannot resume
- **WHEN** a configured source does not support byte ranges or cannot provide the required CORS response headers
- **THEN** the resource is not marked resumable and the UI selects a source that meets the resume contract before starting large downloads

### Requirement: Compile LaTeX projects in browser workers
The system SHALL run supported WebAssembly LaTeX engine invocations in a browser worker and SHALL keep the editor responsive while compilation runs.

#### Scenario: Run one engine pass
- **WHEN** a user starts a compile with a supported browser engine
- **THEN** the worker runs one engine pass and returns generated files, diagnostics, and a terminal status to the compile coordinator

#### Scenario: Browser engine is unavailable
- **WHEN** the selected engine or required capability is unavailable in the browser
- **THEN** the system explains the unsupported capability, does not submit the source to a server-side TeX compiler, and lets the user retry after selecting a supported browser profile or changing the project

### Requirement: Show resource loading progress
The system SHALL expose engine, package, and font loading lifecycle events to the writing UI.

#### Scenario: Download a missing resource
- **WHEN** compilation needs a resource absent from the local cache
- **THEN** the UI shows that resource loading is in progress and reports byte progress when the transport provides it

#### Scenario: Reuse a cached resource
- **WHEN** a required immutable resource is present and valid in browser storage
- **THEN** the resource manager reports a cache hit and avoids downloading that resource again

#### Scenario: Distinguish local and shared cache hits
- **WHEN** a required resource is served from the gateway's shared cache but is absent from browser storage
- **THEN** the UI reports a shared-cache load and SHALL NOT claim the browser has a reusable local copy

#### Scenario: Recover a corrupt local resource
- **WHEN** a cached resource's bytes do not match its verified SHA-256
- **THEN** the browser removes the corrupt entry and fetches the catalogued resource again, or reports an actionable failure if it is offline

#### Scenario: Recover from browser storage quota
- **WHEN** IndexedDB rejects a cache write because storage is full
- **THEN** the resource manager evicts older complete entries, preserves partial resumable downloads, retries the write, and reports when the write still cannot be persisted

#### Scenario: Fetch an uncached package through the same-origin gateway
- **WHEN** compilation requests a package absent from local storage
- **THEN** the browser fetches it through the same-origin resource gateway, which retrieves the catalogued TeX Live package archive, verifies the pinned archive and file checksums, and returns the requested file for browser verification and IndexedDB caching without requiring prefilled R2 resource objects

### Requirement: Orchestrate multi-pass compilation
The system SHALL let the frontend coordinator schedule additional engine passes and auxiliary tools when the project requires them.

#### Scenario: Resolve bibliography references
- **WHEN** the first engine pass indicates that bibliography processing is needed
- **THEN** the coordinator runs the configured bibliography tool and schedules a subsequent engine pass before declaring the compile complete

#### Scenario: Convert XeTeX output
- **WHEN** the XeTeX adapter returns XDV output
- **THEN** the coordinator runs a compatible PDF finalizer and only publishes a PDF after conversion succeeds

### Requirement: Publish completed artifacts through the backend
The system SHALL send completed browser compilation artifacts to an authenticated backend endpoint, and SHALL NOT expose object storage credentials to the browser.

#### Scenario: Persist successful browser compile
- **WHEN** an authorized browser compile submits a successful artifact set
- **THEN** the backend stores artifacts in configured S3-compatible object storage and updates the corresponding database job with terminal status and artifact references

#### Scenario: Retry completion after a transient failure
- **WHEN** the client retries a completion request for the same compile job
- **THEN** the backend handles the request idempotently and returns the canonical job and artifact state
