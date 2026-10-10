## Context and verified baseline

- SwiftLaTeX exposes browser WebAssembly engine workers for pdfTeX and XeTeX. Its XeTeX runtime emits XDV; a separate xdvipdfmx-based stage is needed to produce PDF.
- The upstream XeTeX engine source in SwiftLaTeX is old. The current TeX Live source mirror reports XeTeX 0.999998, so the intended fork is SwiftLaTeX with a deliberate XeTeX source/toolchain refresh, not a fork of TeX Live itself.
- Current PaperSage compiler choices are pdfLaTeX, XeLaTeX, and LuaLaTeX. The SwiftLaTeX baseline covers the first two; this research has not established a maintained, compatible LuaTeX WebAssembly engine.
- SwiftLaTeX's existing resource fetching can block inside the worker and does not provide the user-facing download lifecycle or durable browser cache required here. The integration needs an explicit resource manager and progress protocol.
- The current Go service owns compile-job persistence. Browser code must not receive database or object-store credentials; it sends the completed artifacts to an authenticated Go endpoint, which stores objects and changes the job state.

## Architecture

    Writing UI
      -> Compile coordinator (multi-pass, BibTeX/indexer orchestration)
          -> Engine worker adapter (one compiler invocation per pass)
          -> Resource manager (manifest, progress, Cache API/IndexedDB)
          -> PDF finalizer (including XDV -> PDF where required)
      -> authenticated Go completion endpoint
          -> S3-compatible object storage (Cloudflare R2)
          -> database compile-job status and artifact references

Compilation happens in the browser. The Go backend creates/updates compile-job records and stores completed artifacts. A read-only resource gateway maps SwiftLaTeX Kpathsea requests to a versioned TeX Live package catalog, retrieves and verifies a package archive on demand, and returns only the requested file. It never runs TeX or compiles a project. Browser IndexedDB is the durable per-user cache. Cloudflare's HTTP cache may reuse immutable responses; R2 is an optional shared cache tier for requested package archives, never a requirement to pre-upload the full resource inventory. `latex_render` is not part of the target execution path.

Treat a resource catalog as a release artifact, not a per-run cache dump: generate and qualify it once for an engine/resource revision, publish it only after acceptance, and retain only profiles still referenced by active deployments. Store the single catalog object as deterministic gzip at the immutable profile key; the Go gateway transparently reads both legacy plain-JSON profiles and compressed profiles. Package/font bytes remain on-demand, so publishing a profile does not upload the hundreds of megabytes represented by the catalog.

### Engine adapters

Define a stable frontend contract around engine workers: initialize, compile one pass, return output files/diagnostics, and emit structured progress. Keep engine-specific filesystems and message formats behind adapters. Keep compilation in a worker so the editor remains interactive. The coordinator owns reruns and tool chaining; it must not silently claim that one engine invocation resolves references that require additional passes.

Start with pdfTeX and XeTeX based on the refreshed private SwiftLaTeX fork. Keep the newer XeTeX core (`0.999998`) and retain an auditable build recipe. A browser run of a maintained alternative, WasmTex, reached XDV but failed finalization on the HIT template's EPS input; it also depends on a cross-origin TeX Live mirror that failed the browser CORS check. Do not replace the engine stack with WasmTex unless those delivery and conversion gaps are resolved and the exact template passes. Its newer xdvipdfmx/resource orchestration remains a reference for the Swift fork. Treat LuaTeX as a separate feasibility gate: identify source revision, Emscripten compatibility, required Lua/runtime/font dependencies, output behavior, and a reproducible smoke build before promising support. Other TeX engines can be added through the same adapter contract after a separate compatibility decision.

XeLaTeX projects containing EPS need an explicit browser-side EPS-to-PDF conversion step before the XDV finalizer. The finalizer alone cannot render PostScript. Keep the original project files intact, add converted PDF companions to the worker filesystem, and allow normal graphics extension selection to prefer those companions. The converter must run in a worker, be version-pinned, and have its own license/source record. A converted sample created by a host tool is useful for isolating xdvipdfmx but is not browser acceptance evidence.

### Resource catalog, delivery, and caching

Provide an explicit environment setup view in WritingPage. The user selects an engine profile and starts initialization; the app downloads the engine and a measured base package/font set, verifies hashes, and generates engine formats locally when possible. Show queued/downloaded/verified/unpacked/cached/failed states, byte progress when the transport exposes it, total bytes, and cache reuse. Keep pause/retry/clear-cache and quota recovery actions visible.

Use a versioned TeX Live profile and a generated Kpathsea resource catalog. Each lookup key preserves engine, numeric Kpathsea format, and requested filename, and resolves to the canonical TeX Live source path, package archive, package revision, archive checksum, file checksum, and license provenance. Generate the catalog from the exact TeX Live inventory using Kpathsea-compatible rules; do not infer lookup results from extensions, search-order heuristics, or filename-specific exceptions. On a cache miss, the same-origin gateway fetches the catalogued package archive from the configured TeX Live repository, verifies its pinned archive checksum, extracts only the catalogued file, verifies the file checksum, and returns the `fileid`/`fontid` contract expected by the WASM engine. It must bound archive size, request time, and concurrency, and never accept a client-provided upstream URL. This avoids browser CORS and never compiles on the server.

Do not bulk-upload the generated object inventory to R2. Persist verified complete resources in the user's IndexedDB cache. The gateway and Cloudflare's HTTP cache may reuse immutable per-resource responses. If an R2 shared cache is enabled later, populate it lazily from a verified package archive after a real request; browser compilation must remain correct when the shared cache is empty or evicts content. Keep compiler output artifacts in R2 under the existing authenticated upload flow.

The browser UI reports each requested resource, transfer state, cache hit, and failure. Persist each complete response in IndexedDB even if compilation later fails; do not wait for a successful compile before saving. Show honest counts/stages when no reliable total is known. Handle quota errors, eviction, corrupt entries, and cache schema upgrades; never claim offline readiness until required resources are verified locally. The tested CorTeX static snapshot is useful for smoke testing but is not a qualified production catalog: Chromium found a missing standard resource (`l3bitset.sty`) despite serving nearby LaTeX files. Do not ship it as the compatibility source.

Large downloads are resumable across reloads: persist fixed-size chunks and a download record (manifest version, URL, validators, total length, completed byte ranges, and expected final hash). Resume only against the same immutable resource version with `Range`/`If-Range`; if the server ignores the range, validator changes, or the final checksum fails, restart that resource safely and report why. A source must pass CORS checks for range requests and resume headers before it is offered for initialization.

### Result publishing

Send only the final artifact set and required metadata to the Go backend. The backend authenticates and authorizes the project/job, enforces size and type limits, uploads objects, then updates the compile-job record with terminal status and object references. Make retries idempotent so a repeated completion request does not create duplicate objects or inconsistent status. Keep temporary/partial compile files local unless explicitly needed for diagnostics.

If a requested engine or project feature is unsupported in the browser, report the unsupported capability and keep the job failed/retryable. Do not submit that compile to CLSI or another server-side TeX runtime. The existing CLSI integration may remain operational during migration but is not a browser fallback or a dependency of this design.

## Security and compatibility

- Run untrusted TeX only in a Web Worker/WASM sandbox with bounded memory, execution time, input size, and output size.
- Never expose R2 credentials to the browser. Use the existing authenticated backend path or narrowly scoped, short-lived upload authorization if direct upload is later justified.
- Pin engine/resource manifests and include their versions in cache keys and compile metadata.
- Keep the existing CLSI deployment independent during migration; do not add new browser-to-CLSI fallback behavior.
- Record upstream licenses and source provenance for engines, packages, fonts, and generated artifacts before distributing them.

## Open questions / gates

1. Can current XeTeX and xdvipdfmx be built reproducibly with the selected Emscripten version and produce PDFs for representative Unicode/OpenType projects?
2. Is a LuaTeX WASM port practical with the required Lua, font, and filesystem integrations, and what subset can be supported?
3. Which existing compile-job API semantics and database columns can be reused for browser-originated jobs?
4. Generate and qualify the version-pinned Kpathsea resource catalog and on-demand package archive gateway, including package/font licensing, archive and file checksums, duplicate-name search precedence, and cache-miss behavior.
5. What resource limits and unsupported-capability signals should be product-configurable?
