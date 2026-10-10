## Why

PaperSage currently sends LaTeX jobs to the server-side CLSI service. This couples compilation to a server runtime and makes package availability and compiler load a backend concern. SwiftLaTeX demonstrates that pdfTeX and XeTeX can run in WebAssembly in a browser. PaperSage should move compilation into the browser while keeping project persistence and object storage behind the existing Go backend. The target architecture does not depend on `latex_render` or retain it as a compile fallback.

The goal is a responsive writing flow: users see engine and resource download progress, downloaded resources can be reused from browser storage, and a successful compile is uploaded to Cloudflare S3-compatible storage with the database job state updated by the backend.

## What Changes

- Add a browser-side LaTeX compilation capability with pluggable WebAssembly engine adapters.
- Support PaperSage's current pdfLaTeX, XeLaTeX, and LuaLaTeX choices, with a source/build investigation for each engine before committing to a delivery path.
- Make the UI orchestrate engine invocations and auxiliary tools (for example BibTeX and XDV-to-PDF conversion); an engine invocation itself compiles one pass.
- Report engine, package, and font loading progress and persist reusable assets in browser-managed storage with versioned cache keys.
- Publish final artifacts and completion state through the Go backend; the backend uploads to configured S3-compatible storage and updates the database.
- During migration, the existing CLSI route may remain operational until browser compilation replaces it; it is not a target fallback for browser compile failures.

## Capabilities

### New Capabilities
- **browser-latex-compilation**: Compile LaTeX projects in the browser through pluggable WebAssembly engines, with resource progress and local caching.
- **latex-compile-result-publishing**: Publish browser-produced artifacts through the backend and persist job completion and artifact references.

### Modified Capabilities
- **async-compile-queue**: Add browser-originated compile completion; unsupported browser projects receive an actionable error instead of being routed to a server compiler.

## Impact

- Frontend: adds worker-based compilation, engine selection/orchestration, visible asset progress, browser cache management, and result upload initiation.
- Backend: accepts and validates compile completion/artifacts, writes objects to configured S3-compatible storage, and updates existing compile-job records.
- Database: reuse the current compile-job lifecycle where possible; any schema change must be proposed explicitly after inspecting its current contract.
- CLSI: existing deployment can remain during migration, but browser compilation must not depend on it and unsupported projects must not fall back to it.
- Build/dependencies: requires reproducible WASM engine builds and versioned engine/resource artifacts; no generated engine binaries are included until licensing and build provenance are recorded.
