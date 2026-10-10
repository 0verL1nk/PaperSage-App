## 1. Research and contract

- [x] 1.1 Confirm SwiftLaTeX browser worker architecture and current pdfTeX/XeTeX coverage.
- [x] 1.2 Confirm XeTeX produces XDV and requires a PDF conversion stage.
- [x] 1.3 Confirm PaperSage's current compiler choices and identify CLSI as the current legacy route, not the browser target fallback.
- [x] 1.4 Record current source/build age and identify current XeTeX source as a porting candidate.
- [x] 1.5 Audit representative PaperSage projects, package/font needs, and unsupported features.
- [x] 1.6 Inspect current compile-job API/schema and define an idempotent completion contract.
- [x] 1.7 Produce reproducible baseline builds for the existing SwiftLaTeX pdfTeX and XeTeX engines.
- [x] 1.8 Build and smoke-test refreshed XeTeX plus XDV-to-PDF pipeline.
- [x] 1.9 Complete LuaTeX WASM feasibility spike and record supported subset or rejection evidence.
- [x] 1.10 Compare maintained WasmTex against the private SwiftLaTeX fork using the exact HIT template archive; record browser logs, CORS behavior, asset sizes, and unresolved gates.

## 2. Browser compilation foundation

- [x] 2.1 Define browser-job API semantics: create a persisted job without starting CLSI, report running/progress, accept one idempotent terminal completion, and preserve existing history/status response shapes.
- [x] 2.2 Define a typed engine adapter contract for initialize, one pass, filesystem read/write, cancellation, diagnostics, and structured progress events.
- [x] 2.3 Package SwiftLaTeX Emscripten outputs as versioned same-origin static assets and load them in a dedicated Web Worker; keep WASM memory and execution limits explicit.
- [x] 2.4 Integrate pdfTeX worker one-pass compilation and verify generated PDF bytes can be read from its virtual filesystem.
- [x] 2.5 Integrate XeTeX worker and a verified XDV-to-PDF converter; add browser-side EPS-to-PDF preprocessing before finalization.
- [x] 2.6 Integrate LuaTeX only if the feasibility gate produces a reproducible engine and browser PDF smoke result; otherwise show a clear unsupported-capability state.
- [x] 2.7 Add a compile coordinator for repeated TeX passes and auxiliary tools (at minimum BibTeX), with bounded passes, cancellation, and stable diagnostics.

## 3. Resources and user experience

- [x] 3.1 Inventory resources needed by representative existing PaperSage projects and define versioned manifests for engine assets, formats, base packages, and fonts with hashes and licensing/provenance.
- [x] 3.2 Add an explicit “Initialize LaTeX environment” UI where users choose a supported engine bundle, see estimated/downloaded bytes and per-resource progress, and can start/pause/resume initialization; build format files locally once when required.
- [x] 3.3 Generate a profile-versioned Kpathsea lookup catalog from the exact TeX Live inventory, preserving engine, numeric format, filename, search precedence, package provenance, archive revision/checksum, file checksum, and license. Serve a same-origin resource gateway that fetches and verifies only the requested package archive/file on cache miss and returns the `fileid`/`fontid` contract. Do not bulk-upload the resource inventory to R2; keep compilation entirely in the browser.
  - [x] 3.3.1 Implement the immutable catalog schema, same-origin resolver, exact tuple lookup, checksum/size validation, SwiftLaTeX ID headers, and backend route.
  - [x] 3.3.2 Generate and qualify the lookup inventory using pinned TeX Live Kpathsea and XeTeX Fontconfig behavior; fail generation for unresolved, conflicting, or unlicensed entries and record package archive checksums.
  - [x] 3.3.3 Implement the on-demand TeX Live archive gateway with catalog-only package allowlisting, archive checksum validation, safe member extraction, file checksum validation, bounded caching, and complete user-visible fetch errors.
  - [x] 3.3.4 Verify cold and warm resource loads with an empty R2 resource prefix; confirm cache hits avoid repeat network requests and that optional R2 caching is lazy only.
- [x] 3.4 Persist partial downloads as resumable chunks in IndexedDB, recording resource version, ETag/Last-Modified, total size, completed ranges, and expected checksum; resume with HTTP Range after reload/network loss and verify the complete resource before use.
- [x] 3.5 Persist immutable verified resources and generated formats in IndexedDB; report cache hits, support offline reuse, handle quota/eviction/corruption, and invalidate by manifest/engine version.
- [x] 3.6 Adapt WritingPage's compiler selector and compile action to browser execution, and show engine startup, resource loading, compile pass, and artifact upload as distinct progress stages.
- [x] 3.7 Surface actionable engine diagnostics and logs, cancellation, retry, and worker crash recovery; report unsupported browser capabilities without routing compilation to CLSI.
- [x] 3.8 Keep PDF preview, page navigation, download, and existing project file behavior working with the browser-generated PDF before server publication finishes.
- [x] 3.9 Reject resource profiles whose generated catalog has unresolved or ambiguous entries for required packages/fonts; check duplicate basenames, format-specific precedence, hashes, missing objects, and immutable revision matching. Do not add filename-specific extension fallbacks.
- [x] 3.10 Allow selecting a validated custom API base URL and resolve TeX resources against the currently selected backend so Vercel branch previews can use their matching Go deployment.

## 4. Publish and rollout

- [x] 4.1 Add an authenticated Go endpoint that authorizes project/job ownership, validates artifact metadata/size/type, and publishes browser artifacts to configured S3-compatible storage without exposing credentials to the browser.
- [x] 4.2 Update compile-job terminal state and PDF/log/object references atomically or idempotently after upload; make retries safe and failed uploads visible/retryable.
- [x] 4.3 Wire published browser results into existing output preview, download, and compile history APIs.
- [x] 4.4 Compare browser output/diagnostics against representative project expectations, including CJK fonts, bibliography, images, and common templates; document unsupported packages/features.
- [x] 4.5 Replace the CLSI compile submission path with browser compilation after browser acceptance; unsupported capabilities must produce actionable errors, not server compilation fallback.
- [x] 4.6 Record licenses, exact source/toolchain revisions, deterministic build recipes, resource manifest hashes, and distributed artifact provenance.
- [x] 4.7 Complete a browser-level acceptance pass for first load, progress visibility, cached second compile, cancel/retry, PDF preview/download, and persisted compile history.
