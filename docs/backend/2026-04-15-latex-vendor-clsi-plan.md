# PaperSage Vendor CLSI Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Vendor Overleaf `clsi` into PaperSage as the full-featured LaTeX compilation service, support local host-mode development through `make install`, and expose stable FastAPI endpoints that the frontend can use without coupling to vendor APIs.

**Architecture:** Keep Overleaf `clsi` as an isolated vendor service under `vendor/overleaf`, preserving its multi-file compile, output artifact, wordcount, and SyncTeX capabilities. Run it locally in host mode with `LocalCommandRunner` and TeX Live installed on the developer machine, while production deploys the same vendor service separately. FastAPI becomes the system boundary: it maps PaperSage project files into `clsi` resources, forwards compile and output requests, and hides vendor-specific details from the frontend.

**Tech Stack:** FastAPI, Python 3.12+, HTTPX, Node.js 24, npm 11, Overleaf CLSI, TeX Live, `latexmk`, Makefile, shell scripts, Homebrew, `apt`, macOS, Ubuntu/Debian

---

## Scope And Assumptions

- Full `clsi` capability is kept in scope for the vendor service. Do not intentionally cut down to an MVP.
- Local development does not use Docker for the LaTeX service. Developers install TeX dependencies on the host machine.
- Production deployment of vendor `clsi` is out of this repository's runtime scope, but this repository must provide the service contract and local workflow needed to integrate with it.
- The frontend should call only PaperSage backend endpoints. It should not call the vendor service directly.
- The first implementation should preserve upstream behavior where practical and only wrap it. De-Overleaf cleanup can happen incrementally afterward.
- The working tree is already dirty. Implementation must avoid reverting unrelated user changes.

## Source Inventory To Vendor

Keep the upstream layout intact enough that `clsi` can still run with minimal patching.

```text
vendor/
  overleaf/
    package.json
    package-lock.json
    patches/
    libraries/
      fetch-utils/
      logger/
      metrics/
      o-error/
      overleaf-editor-core/
      promise-utils/
      settings/
      stream-utils/
    services/
      clsi/
```

### Why This Layout

- `services/clsi/package.json` depends on local workspace packages such as `@overleaf/settings`, `@overleaf/logger`, `@overleaf/metrics`, `@overleaf/o-error`, `@overleaf/promise-utils`, `@overleaf/stream-utils`, and `overleaf-editor-core`.
- Root `package.json` and `package-lock.json` are needed so `npm ci` resolves the workspace packages consistently.
- `patches/` must be preserved because upstream uses `patch-package` at install time.
- `services/clsi` should stay structurally close to upstream so later cleanup is incremental rather than a one-shot rewrite.

## Runtime Topology

### Local Development

```text
frontend -> PaperSage FastAPI -> vendor/overleaf/services/clsi (host mode) -> local latexmk/TeX Live
```

- `SANDBOXED_COMPILES` remains unset or `false`.
- `clsi` selects `LocalCommandRunner` and executes `latexmk` on the host machine.
- `make install` installs both app dependencies and host machine LaTeX dependencies.

### Production

```text
frontend -> PaperSage FastAPI -> separately deployed vendor clsi service
```

- PaperSage backend uses a configured base URL to call the external `clsi`.
- Service deployment, isolation, and process supervision happen outside this repository.

## Backend Integration Boundary

PaperSage backend must own these responsibilities:

- authentication and project access control
- selecting the main file for compilation
- loading multi-file project resources from PaperSage storage
- mapping PaperSage file models into `clsi` `resources[]`
- exposing backend-native API envelopes
- translating vendor errors into stable PaperSage responses

Vendor `clsi` should own these responsibilities:

- actual `latexmk` execution
- generated PDF and auxiliary artifact management
- SyncTeX generation and lookup
- wordcount
- compile stop and compile cache cleanup

## Target File Structure

```text
vendor/
  overleaf/
    package.json
    package-lock.json
    patches/
    libraries/
    services/
      clsi/

scripts/
  clsi/
    install.sh
    check.sh
    dev.sh
    env.sh

backend/
  app/
    config.py
  api/
    __init__.py
    routes/
      latex.py
  schemas/
    latex.py
  services/
    __init__.py
    latex/
      __init__.py
      clsi_client.py
      resource_builder.py
      service.py
  tests/
    test_latex_routes.py
    test_latex_service.py
    test_clsi_client.py

docs/
  backend/
    2026-04-15-latex-vendor-clsi-plan.md
    DEPLOYMENT.md
```

## API Shape To Expose From FastAPI

Keep FastAPI paths PaperSage-native and stable. The backend can proxy to upstream-style `clsi` routes internally.

### Compile

- `POST /api/v1/projects/{project_id}/latex/compile`

Request:

```json
{
  "root_resource_path": "main.tex",
  "compiler": "xelatex",
  "timeout": 60,
  "draft": false,
  "stop_on_first_error": false
}
```

Response envelope payload should include:

- compile status
- build id
- output file list
- backend-owned output URLs
- raw vendor diagnostics when useful

### Output Files

- `GET /api/v1/projects/{project_id}/latex/builds/{build_id}/output/{path}`
- `GET /api/v1/projects/{project_id}/latex/builds/{build_id}/output.zip`

### Compile Control

- `POST /api/v1/projects/{project_id}/latex/compile/stop`
- `DELETE /api/v1/projects/{project_id}/latex/cache`

### Wordcount

- `GET /api/v1/projects/{project_id}/latex/wordcount?file=main.tex`

### SyncTeX

- `GET /api/v1/projects/{project_id}/latex/sync/code`
- `GET /api/v1/projects/{project_id}/latex/sync/pdf`

## Cross-Cutting Decisions

### Vendor Isolation

- Do not mix vendor code into `backend/`.
- Treat `vendor/overleaf` as third-party code with a narrow patch surface.
- Any PaperSage-specific behavior should live in shell scripts, env files, and FastAPI wrappers before modifying vendor internals.

### Local Install Strategy

- Use `scripts/clsi/install.sh` as the single cross-platform host dependency installer.
- Support:
  - Ubuntu/Debian via `apt`
  - macOS via Homebrew
- Installer responsibilities:
  - validate `node`, `npm`, `uv`, and package manager availability
  - install system packages required by host-mode `clsi`
  - install vendor Node dependencies with `npm ci`
  - create local compile/cache/output directories

### macOS And Ubuntu Package Policy

Ubuntu/Debian baseline:

- `nodejs`
- `npm`
- `latexmk`
- `texlive-latex-extra`
- `texlive-fonts-recommended`
- `texlive-fonts-extra`
- `texlive-xetex`
- `texlive-luatex`
- `texlive-bibtex-extra`
- `biber`
- `ghostscript`
- `poppler-utils`
- `qpdf`
- `inkscape`
- `python3-pygments`

macOS baseline:

- `node`
- `mactex-no-gui` or equivalent TeX distribution
- `ghostscript`
- `poppler`
- `qpdf`
- `inkscape`
- `pygments`

Notes:

- The implementation should verify actual command availability, not assume package names map one-to-one across hosts.
- If `mactex-no-gui` proves too heavy for CI or onboarding, split the script into `install` and `doctor` phases so developers can choose package provisioning while still getting deterministic checks.

### Backend Resource Mapping

- Resource builder should be a focused service that converts PaperSage project files into vendor `resources[]`.
- It must preserve nested paths because LaTeX templates often use multi-file trees and relative imports.
- Binary assets should be passed by URL only if PaperSage storage guarantees stable download URLs; otherwise fetch and inline them before calling `clsi`.

### Error Handling

- Vendor HTTP errors must be mapped into the PaperSage API envelope.
- Preserve upstream compile statuses such as `success`, `failure`, `error`, `timedout`, `compile-in-progress`, `conflict`, and `missing-updates`.
- Distinguish:
  - user compile errors
  - vendor availability errors
  - backend resource assembly errors

### Security

- FastAPI must validate that the current user can access the project before forwarding any compile request.
- Output file proxy endpoints must enforce the same project access checks.
- Local host-mode `clsi` is a development-only trust boundary. Do not assume local mode is safe enough for production.

## Recommended Execution Order

1. Vendor and boot the service first.
2. Stabilize local install and doctor scripts.
3. Add FastAPI client and proxy endpoints.
4. Add backend tests for compile and output proxy behavior.
5. Update backend deployment docs and local run instructions.
6. Only then start incremental de-Overleaf cleanup.

---

### Task 1: Vendor The Upstream CLSI Tree

**Files:**
- Create: `vendor/overleaf/package.json`
- Create: `vendor/overleaf/package-lock.json`
- Create: `vendor/overleaf/patches/*`
- Create: `vendor/overleaf/libraries/*`
- Create: `vendor/overleaf/services/clsi/*`
- Create: `vendor/overleaf/README.md`

- [ ] **Step 1: Copy the minimal runtime workspace from `.ref/overleaf`**

Copy only the required root files, workspace libraries, and `services/clsi`, preserving directory structure exactly.

- [ ] **Step 2: Record provenance in `vendor/overleaf/README.md`**

Document:
- source path in `.ref`
- upstream commit or snapshot identifier if available
- which subtrees were intentionally included
- policy that cleanup happens by wrapping first, editing vendor code second

- [ ] **Step 3: Install Node workspace dependencies**

Run: `cd vendor/overleaf && npm ci`

Expected: workspace install completes and applies `patch-package`.

- [ ] **Step 4: Smoke-test raw service startup**

Run:

```bash
cd vendor/overleaf
LISTEN_ADDRESS=127.0.0.1 DOWNLOAD_HOST=http://127.0.0.1:3013 node services/clsi/app.js
```

Expected: service binds successfully, even if compile commands fail until TeX Live is installed.

- [ ] **Step 5: Commit vendor import**

```bash
git add vendor/overleaf
git commit -m "chore: vendor overleaf clsi service"
```

### Task 2: Add Cross-Platform Local Install And Doctor Scripts

**Files:**
- Modify: `Makefile`
- Create: `scripts/clsi/install.sh`
- Create: `scripts/clsi/check.sh`
- Create: `scripts/clsi/dev.sh`
- Create: `scripts/clsi/env.sh`

- [ ] **Step 1: Add `make` entry points**

Add:
- `make install-clsi`
- `make check-clsi`
- `make clsi-dev`

Keep `make install` delegating to the new target so developers can bootstrap everything from one command.

- [ ] **Step 2: Implement `scripts/clsi/install.sh`**

Responsibilities:
- detect OS
- install or instruct for host packages
- run `npm ci` under `vendor/overleaf`
- create writable directories for compiles, output, cache, and uploads

- [ ] **Step 3: Implement `scripts/clsi/check.sh`**

Verify:
- `node`
- `npm`
- `latexmk`
- selected TeX engines
- `biber`
- `qpdf`
- `inkscape`

Expected output should be explicit about missing commands and next actions.

- [ ] **Step 4: Implement `scripts/clsi/env.sh` and `scripts/clsi/dev.sh`**

Set local env such as:
- `LISTEN_ADDRESS`
- `DOWNLOAD_HOST`
- `CLSI_COMPILES_PATH`
- `CLSI_OUTPUT_PATH`
- `CLSI_CACHE_PATH`
- `CLSI_UPLOAD_PATH`

Then start `node services/clsi/app.js` in host mode.

- [ ] **Step 5: Verify local workflow**

Run:

```bash
make install-clsi
make check-clsi
make clsi-dev
```

Expected: host prerequisites validated and local service starts without Docker.

- [ ] **Step 6: Commit local tooling**

```bash
git add Makefile scripts/clsi
git commit -m "build: add local clsi install and dev scripts"
```

### Task 3: Add Backend Configuration For Vendor CLSI

**Files:**
- Modify: `backend/app/config.py`
- Create: `backend/services/__init__.py`
- Create: `backend/services/latex/__init__.py`

- [ ] **Step 1: Extend settings with vendor service configuration**

Add fields for:
- `latex_clsi_base_url`
- `latex_clsi_timeout_seconds`
- `latex_clsi_enabled`
- optional local dev defaults pointing to `http://127.0.0.1:3013`

- [ ] **Step 2: Keep configuration neutral to local or production topology**

Do not encode assumptions about Docker or host mode into backend logic. The backend only needs a reachable base URL.

- [ ] **Step 3: Add config coverage**

Create or extend tests so missing required settings fail clearly only when the feature is enabled.

- [ ] **Step 4: Commit configuration changes**

```bash
git add backend/app/config.py backend/services/__init__.py backend/services/latex/__init__.py
git commit -m "feat: add latex service configuration"
```

### Task 4: Implement The Backend CLSI Client

**Files:**
- Create: `backend/services/latex/clsi_client.py`
- Create: `backend/tests/test_clsi_client.py`

- [ ] **Step 1: Write failing tests for the HTTP client**

Cover:
- compile request serialization
- output file URL forwarding
- stop compile
- clear cache
- wordcount
- sync endpoints
- vendor error mapping

- [ ] **Step 2: Implement a small typed client around vendor endpoints**

The client should wrap upstream routes such as:
- `POST /project/{project_id}/compile`
- `POST /project/{project_id}/compile/stop`
- `DELETE /project/{project_id}`
- `GET /project/{project_id}/wordcount`
- `GET /project/{project_id}/sync/code`
- `GET /project/{project_id}/sync/pdf`

- [ ] **Step 3: Keep the client transport-focused**

Do not let it know about PaperSage ORM models. It should accept plain IDs, request DTOs, and resource arrays.

- [ ] **Step 4: Run tests**

Run: `cd backend && uv run pytest tests/test_clsi_client.py -v`

Expected: PASS

- [ ] **Step 5: Commit the client**

```bash
git add backend/services/latex/clsi_client.py backend/tests/test_clsi_client.py
git commit -m "feat: add clsi http client"
```

### Task 5: Implement Project Resource Assembly For Multi-File LaTeX

**Files:**
- Create: `backend/services/latex/resource_builder.py`
- Create: `backend/services/latex/service.py`
- Create: `backend/tests/test_latex_service.py`

- [ ] **Step 1: Write failing tests for resource assembly**

Cover:
- nested file trees
- `main.tex` selection
- binary asset handling
- stable relative paths
- empty project and missing root file failures

- [ ] **Step 2: Implement `resource_builder.py`**

It should:
- load all project LaTeX resources
- preserve relative paths
- emit vendor-compatible `resources[]`
- decide whether content is sent inline or via URL

- [ ] **Step 3: Implement `service.py`**

This service should:
- validate project access
- call `resource_builder`
- call `clsi_client`
- normalize vendor response into backend-native payloads

- [ ] **Step 4: Run tests**

Run: `cd backend && uv run pytest tests/test_latex_service.py -v`

Expected: PASS

- [ ] **Step 5: Commit the service layer**

```bash
git add backend/services/latex/resource_builder.py backend/services/latex/service.py backend/tests/test_latex_service.py
git commit -m "feat: add latex resource assembly service"
```

### Task 6: Expose FastAPI Routes For Compile, Output, SyncTeX, And Wordcount

**Files:**
- Create: `backend/schemas/latex.py`
- Create: `backend/api/routes/latex.py`
- Modify: `backend/api/__init__.py`
- Create: `backend/tests/test_latex_routes.py`

- [ ] **Step 1: Write failing route tests**

Cover:
- authenticated compile
- unauthorized access rejection
- output file proxy
- output zip proxy
- stop compile
- cache clear
- wordcount
- sync code/pdf

- [ ] **Step 2: Implement request and response schemas**

Define typed schemas for:
- compile request
- compile response payload
- wordcount response
- sync code response
- sync pdf response

- [ ] **Step 3: Implement the route module**

Responsibilities:
- bind auth dependency
- call service layer
- return standard `ApiResponse`
- stream or proxy output artifacts safely

- [ ] **Step 4: Register router**

Mount routes under a PaperSage path such as `/api/v1/projects/{project_id}/latex/...`.

- [ ] **Step 5: Run tests**

Run: `cd backend && uv run pytest tests/test_latex_routes.py -v`

Expected: PASS

- [ ] **Step 6: Commit routes**

```bash
git add backend/schemas/latex.py backend/api/routes/latex.py backend/api/__init__.py backend/tests/test_latex_routes.py
git commit -m "feat: add latex compile api routes"
```

### Task 7: Document Local Development And Production Integration

**Files:**
- Modify: `docs/backend/DEPLOYMENT.md`
- Modify: `Makefile`
- Modify: `backend/.env.example`

- [ ] **Step 1: Document local development setup**

Explain:
- `make install`
- `make install-clsi`
- `make check-clsi`
- `make clsi-dev`
- expected ports and directories

- [ ] **Step 2: Document production wiring**

Explain:
- backend env pointing at external `clsi`
- operational expectation that production service is deployed separately
- backend remains the only service the frontend talks to

- [ ] **Step 3: Add `.env.example` variables**

Document:
- `LATEX_CLSI_ENABLED`
- `LATEX_CLSI_BASE_URL`
- `LATEX_CLSI_TIMEOUT_SECONDS`

- [ ] **Step 4: Commit docs**

```bash
git add docs/backend/DEPLOYMENT.md backend/.env.example Makefile
git commit -m "docs: add latex clsi integration setup"
```

### Task 8: Add End-To-End Verification For A Multi-File Template

**Files:**
- Create: `backend/tests/fixtures/latex_project/`
- Modify: `backend/tests/test_latex_routes.py`

- [ ] **Step 1: Add a real multi-file fixture**

Include:
- `main.tex`
- one included section file
- one bibliography file
- one image if practical

- [ ] **Step 2: Write an integration-style test**

The test should:
- assemble the project resources
- call the route or service
- assert vendor response contains `output.pdf`

- [ ] **Step 3: Run the targeted test with local `clsi` available**

Run:

```bash
cd backend && uv run pytest tests/test_latex_routes.py -k latex -v
```

Expected: PASS with a real PDF output path present in the response payload.

- [ ] **Step 4: Commit verification assets**

```bash
git add backend/tests/fixtures/latex_project backend/tests/test_latex_routes.py
git commit -m "test: verify latex compile with multi-file fixture"
```

## Verification Checklist

- `cd vendor/overleaf && npm ci`
- `make install-clsi`
- `make check-clsi`
- `make clsi-dev`
- `cd backend && uv run pytest tests/test_clsi_client.py -v`
- `cd backend && uv run pytest tests/test_latex_service.py -v`
- `cd backend && uv run pytest tests/test_latex_routes.py -v`

## Deferred Cleanup After Integration

These items are intentionally deferred until after the service works end-to-end:

- remove unused vendored Overleaf code gradually
- rename vendor-facing modules away from Overleaf terminology where useful
- replace workspace dependencies with PaperSage-owned equivalents one by one
- decide whether output artifacts should remain proxied or move to storage-backed URLs
- add frontend-specific LaTeX UX on top of the backend contract
