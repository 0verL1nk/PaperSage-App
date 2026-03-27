# Paper Assistant Frontend Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild `../PaperAssistantFront` inside `./frontend` as a Vite + React + TypeScript application while preserving the usable business flows and keeping the existing backend API contract.

**Architecture:** Build a single-page application with TanStack Router for route structure, TanStack Query for server state, Zustand for lightweight client state, and React Hook Form + Zod for forms. Use Tailwind CSS + shadcn/ui + Radix UI + Lucide React for the design system, and migrate legacy Vue modules feature-by-feature instead of attempting a full mechanical rewrite.

**Tech Stack:** Vite, React, TypeScript, Tailwind CSS, shadcn/ui, Radix UI, TanStack Query, TanStack Router, Zustand, React Hook Form, Zod, Lucide React, pnpm, PDF.js

---

## Scope And Assumptions

- The migration target is `/home/ling/GraduateProject/frontend`.
- Package management is `pnpm`.
- Backend API paths and payload formats stay compatible with the old frontend in `../PaperAssistantFront`.
- First migration milestone covers the flows that currently have real behavior in the Vue app:
  - auth
  - project list
  - create project
  - create group
  - delete project
  - trash list and restore/delete
  - project workspace shell
  - uploading view
  - coding view with PDF preview and highlights
- The legacy routes `/main/labels` and `/main/more` are not migrated in the first milestone because they have no matching router targets in the Vue app.
- `classifying`, `logic-chain`, and `generating` currently appear to be placeholders in the Vue app, so the React version should carry them over as explicit placeholder routes first, not invent new behavior.
- Prefer Chinese-first UI content for the first pass. Keep the code ready for i18n later, but do not let i18n block migration.

## Source Feature Inventory

### Confirmed Legacy Routes

- `/auth`
- `/main/documents`
- `/main/trash`
- `/main/profile`
- `/project/:id/uploading`
- `/project/:id/coding`
- `/project/:id/classifying`
- `/project/:id/logic-chain`
- `/project/:id/generating`

### Confirmed Backend Contracts

- Auth uses `multipart/form-data` and MD5 password hashing.
- Token is stored in local storage and sent as raw `Authorization` header value.
- Core resource areas:
  - account
  - project
  - project trash
  - document upload/folder/file
  - file preview URL
  - file highlights

### Important Legacy Behavior To Preserve

- Global loading overlay with request-count semantics.
- Auth guard around main and project routes.
- Project grouping and search in the documents page.
- Uploading workflow with folder tree, batch selection, move, remark, delete, upload progress, preview panel.
- Coding workflow with opened-file tabs, PDF rendering, and highlight panel.

## Recommended Migration Strategy

### Recommended Approach: Domain-First Rebuild

Rebuild by business domain, not by page screenshot copying:

1. Create the React shell and shared infrastructure.
2. Migrate auth and the main project list first.
3. Migrate the project workspace shell.
4. Migrate uploading and coding as two focused feature slices.
5. Carry placeholder tabs over only after the real flows are stable.

This is the safest path because the source Vue app is uneven in quality and contains some dead links and duplicated infrastructure. A domain-first rebuild preserves behavior without importing those problems into the new codebase.

### Rejected Approach: File-For-File Port

Do not mirror the Vue file structure one-to-one. That would bring over accidental complexity like mixed UI concerns, duplicated env/i18n setup, and request-layer coupling to global UI state.

### Rejected Approach: Big Bang Full Rewrite Before Any Verification

Do not wait until every page is migrated before verifying the app. Each milestone should be runnable and testable inside `frontend`.

## Target File Structure

```text
frontend/
  components.json
  index.html
  package.json
  pnpm-lock.yaml
  postcss.config.js
  tailwind.config.ts
  tsconfig.json
  vite.config.ts
  src/
    main.tsx
    routeTree.gen.ts
    app/
      router.tsx
      providers.tsx
    routes/
      __root.tsx
      auth.tsx
      main/
        documents.tsx
        trash.tsx
        profile.tsx
      project/
        $projectId.tsx
        $projectId.uploading.tsx
        $projectId.coding.tsx
        $projectId.classifying.tsx
        $projectId.logic-chain.tsx
        $projectId.generating.tsx
    shared/
      api/
        client.ts
        contracts.ts
        errors.ts
      config/
        env.ts
      hooks/
        use-auth-guard.ts
      lib/
        cn.ts
        md5.ts
      stores/
        ui-store.ts
        session-store.ts
      components/
        app-shell.tsx
        loading-overlay.tsx
        page-state.tsx
    features/
      auth/
        api.ts
        schema.ts
        hooks.ts
        components/
          auth-form.tsx
      projects/
        api.ts
        schema.ts
        hooks.ts
        components/
          project-list.tsx
          project-card.tsx
          new-project-dialog.tsx
          new-group-dialog.tsx
      trash/
        api.ts
        hooks.ts
      workspace/
        components/
          workspace-layout.tsx
          workspace-tabs.tsx
      uploading/
        api.ts
        schema.ts
        hooks.ts
        store.ts
        components/
          folder-tree.tsx
          file-table.tsx
          file-preview-sheet.tsx
          upload-toolbar.tsx
      coding/
        api.ts
        hooks.ts
        store.ts
        components/
          pdf-viewer.tsx
          file-tabs.tsx
          highlights-panel.tsx
```

## Cross-Cutting Decisions

### Routing

- Use TanStack Router route files under `src/routes`.
- Put auth-free pages at root level and protect `main/*` and `project/*` via route loaders or a shared auth wrapper.
- Keep route names aligned with the legacy paths so deep-link compatibility stays obvious.

### Server State

- Use TanStack Query for all API-backed data:
  - auth mutation
  - projects list
  - trash list
  - folders list
  - files list
  - file URL
  - highlights
- Use query invalidation instead of manually mirroring API state across many local stores.

### Client State

- Use Zustand only for UI-local state that should outlive a single component tree:
  - global loading overlay counter
  - auth session token
  - opened coding tabs
  - uploading selection state when shared across toolbar/table/tree
- Do not put fetched collections in Zustand if TanStack Query already owns them.

### Forms

- Use React Hook Form + Zod for:
  - login/register
  - create project
  - create group
  - remark form
  - move-file dialog if validation is needed

### Design System

- Initialize shadcn/ui on the Vite app using a Radix-based preset.
- Use shadcn components for dialog, sheet, form controls, tabs, table, toast, tooltip, alert, skeleton, separator, dropdown menu, scroll area.
- Use Lucide React for icons. Do not port the custom iconfont unless a concrete icon is missing and materially blocks migration.
- Treat visual quality as a first-class requirement, not post-processing.
- Build a coherent visual language early:
  - clear color system and semantic tokens
  - intentional spacing and typography scale
  - polished desktop and mobile layout behavior
  - restrained but meaningful motion
  - clean empty, loading, and error states
- Avoid a plain scaffold look. The React version should feel like a deliberate product UI, not a default admin template.

### Networking

- Build a typed API client wrapper on top of `fetch` or `axios`, but standardize request/response handling in one place.
- Preserve legacy compatibility details:
  - multipart login/register
  - token header format
  - blob download for PDF preview
  - backend response envelope `{ code, data, info }`

### PDF And Highlights

- Keep PDF.js as the rendering base.
- Do not embed the full legacy iframe-based viewer unchanged if a cleaner React wrapper is practical.
- First pass can still use the existing PDF.js web assets if that shortens migration risk.
- Highlight fetching and right-panel display should remain separate from the rendering engine.

## Delivery Phases

### Task 1: Bootstrap The React App

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/index.html`
- Create: `frontend/vite.config.ts`
- Create: `frontend/tsconfig.json`
- Create: `frontend/postcss.config.js`
- Create: `frontend/tailwind.config.ts`
- Create: `frontend/src/main.tsx`
- Create: `frontend/src/app/providers.tsx`
- Create: `frontend/src/app/router.tsx`

- [ ] **Step 1: Scaffold the frontend with pnpm**

Run:

```bash
cd /home/ling/GraduateProject/frontend
pnpm create vite@latest . -- --template react-ts
```

Expected: Vite React TypeScript starter is generated in `frontend/`.

- [ ] **Step 2: Add core dependencies**

Run:

```bash
cd /home/ling/GraduateProject/frontend
pnpm add @tanstack/react-query @tanstack/react-router zustand react-hook-form zod @hookform/resolvers lucide-react axios md5 pdfjs-dist
pnpm add -D tailwindcss postcss autoprefixer @types/node
```

Expected: dependency installation succeeds and `package.json` reflects the requested stack.

- [ ] **Step 3: Add shared app providers**

Implement:
- QueryClient provider
- Router provider
- future toast/provider hooks if needed

- [ ] **Step 4: Verify the clean app starts**

Run:

```bash
cd /home/ling/GraduateProject/frontend
pnpm dev
```

Expected: local Vite dev server starts without type errors.

### Task 2: Initialize Tailwind And shadcn/ui

**Files:**
- Modify: `frontend/package.json`
- Create: `frontend/components.json`
- Modify: `frontend/src/index.css`
- Create or Modify: `frontend/src/lib/utils.ts`
- Create: `frontend/src/components/ui/*`

- [ ] **Step 1: Initialize Tailwind**

Run:

```bash
cd /home/ling/GraduateProject/frontend
pnpm dlx tailwindcss init -p
```

Expected: Tailwind config and PostCSS config exist.

- [ ] **Step 2: Initialize shadcn/ui with a Radix-based Vite preset**

Run:

```bash
cd /home/ling/GraduateProject/frontend
pnpm dlx shadcn@latest init
```

Expected: `components.json` and baseline shadcn files are created for the Vite project.

- [ ] **Step 3: Add base UI primitives**

Run:

```bash
cd /home/ling/GraduateProject/frontend
pnpm dlx shadcn@latest add button card dialog dropdown-menu form input textarea tabs table sheet tooltip alert badge skeleton separator scroll-area sonner
```

Expected: base UI source files are generated under the configured UI directory.

- [ ] **Step 4: Verify styling pipeline**

Run:

```bash
cd /home/ling/GraduateProject/frontend
pnpm build
```

Expected: production build succeeds with Tailwind and shadcn configured.

### Task 3: Build Shared Infrastructure

**Files:**
- Create: `frontend/src/shared/api/client.ts`
- Create: `frontend/src/shared/api/contracts.ts`
- Create: `frontend/src/shared/api/errors.ts`
- Create: `frontend/src/shared/config/env.ts`
- Create: `frontend/src/shared/stores/ui-store.ts`
- Create: `frontend/src/shared/stores/session-store.ts`
- Create: `frontend/src/shared/components/loading-overlay.tsx`
- Create: `frontend/src/shared/hooks/use-auth-guard.ts`

- [ ] **Step 1: Implement env parsing**

Include:
- `VITE_API_BASE_URL`
- app mode helpers
- clear failure on missing required env

- [ ] **Step 2: Implement API client**

Support:
- request envelope parsing
- auth header injection
- multipart payloads
- blob downloads
- consistent error normalization

- [ ] **Step 3: Implement session and loading stores**

Keep:
- token persistence in local storage
- global loading counter semantics from the legacy app

- [ ] **Step 4: Add app-wide loading and error presentation**

Include:
- loading overlay
- toast pipeline
- query error defaults

- [ ] **Step 5: Verify infra with a smoke route**

Run:

```bash
cd /home/ling/GraduateProject/frontend
pnpm build
```

Expected: shared infra compiles before feature migration begins.

### Task 4: Implement Routing And Layout Shell

**Files:**
- Create: `frontend/src/routes/__root.tsx`
- Create: `frontend/src/routes/auth.tsx`
- Create: `frontend/src/routes/main/documents.tsx`
- Create: `frontend/src/routes/main/trash.tsx`
- Create: `frontend/src/routes/main/profile.tsx`
- Create: `frontend/src/routes/project/$projectId.tsx`
- Create: `frontend/src/routes/project/$projectId.uploading.tsx`
- Create: `frontend/src/routes/project/$projectId.coding.tsx`
- Create: `frontend/src/routes/project/$projectId.classifying.tsx`
- Create: `frontend/src/routes/project/$projectId.logic-chain.tsx`
- Create: `frontend/src/routes/project/$projectId.generating.tsx`
- Create: `frontend/src/features/workspace/components/workspace-layout.tsx`
- Create: `frontend/src/shared/components/app-shell.tsx`

- [ ] **Step 1: Recreate route tree**

Map the React routes to the confirmed legacy URLs.

- [ ] **Step 2: Add authenticated route protection**

Redirect unauthenticated users to `/auth`.

- [ ] **Step 3: Create the main shell**

Include:
- top header area
- left navigation for documents/trash/profile
- content outlet

- [ ] **Step 4: Create the project workspace shell**

Include:
- back/home controls
- tab navigation for uploading/coding/classifying/logic-chain/generating

- [ ] **Step 5: Verify deep-link navigation**

Run:

```bash
cd /home/ling/GraduateProject/frontend
pnpm dev
```

Expected: manual navigation to each route renders the correct shell without crashing.

### Task 5: Migrate Auth

**Files:**
- Create: `frontend/src/features/auth/api.ts`
- Create: `frontend/src/features/auth/schema.ts`
- Create: `frontend/src/features/auth/hooks.ts`
- Create: `frontend/src/features/auth/components/auth-form.tsx`
- Modify: `frontend/src/routes/auth.tsx`

- [ ] **Step 1: Model login and register schemas**

Use Zod to validate:
- username required
- password required
- password complexity
- confirm password on register

- [ ] **Step 2: Implement auth API mutations**

Preserve:
- `multipart/form-data`
- MD5 password hashing
- token storage after login

- [ ] **Step 3: Build the auth page**

Use:
- shadcn form primitives
- form-level error display
- mode switch between login and register

- [ ] **Step 4: Verify auth redirect flow**

Expected:
- no token -> redirect to `/auth`
- valid login -> redirect to `/main/documents`

### Task 6: Migrate Project List And Trash

**Files:**
- Create: `frontend/src/features/projects/api.ts`
- Create: `frontend/src/features/projects/schema.ts`
- Create: `frontend/src/features/projects/hooks.ts`
- Create: `frontend/src/features/projects/components/project-list.tsx`
- Create: `frontend/src/features/projects/components/project-card.tsx`
- Create: `frontend/src/features/projects/components/new-project-dialog.tsx`
- Create: `frontend/src/features/projects/components/new-group-dialog.tsx`
- Create: `frontend/src/features/trash/api.ts`
- Create: `frontend/src/features/trash/hooks.ts`
- Modify: `frontend/src/routes/main/documents.tsx`
- Modify: `frontend/src/routes/main/trash.tsx`

- [ ] **Step 1: Implement project and trash query hooks**

Cover:
- list projects
- create project
- create group
- delete project
- list trash
- restore trash item
- permanently delete trash item

- [ ] **Step 2: Build the documents screen**

Include:
- grouped project tabs
- search input
- create project dialog
- create group dialog
- project card actions

- [ ] **Step 3: Build the trash screen**

Include:
- restore
- permanent delete
- empty state

- [ ] **Step 4: Verify list invalidation**

Expected:
- create/delete/restore actions refresh the correct lists

### Task 7: Migrate Uploading View

**Files:**
- Create: `frontend/src/features/uploading/api.ts`
- Create: `frontend/src/features/uploading/schema.ts`
- Create: `frontend/src/features/uploading/hooks.ts`
- Create: `frontend/src/features/uploading/store.ts`
- Create: `frontend/src/features/uploading/components/folder-tree.tsx`
- Create: `frontend/src/features/uploading/components/file-table.tsx`
- Create: `frontend/src/features/uploading/components/file-preview-sheet.tsx`
- Create: `frontend/src/features/uploading/components/upload-toolbar.tsx`
- Modify: `frontend/src/routes/project/$projectId.uploading.tsx`

- [ ] **Step 1: Implement folder and file queries**

Cover:
- folder list
- file list
- file status polling if still needed

- [ ] **Step 2: Implement upload mutation**

Support:
- file type restrictions
- progress reporting
- folder target

- [ ] **Step 3: Implement batch actions**

Support:
- select all
- delete
- move
- remark

- [ ] **Step 4: Implement preview side panel**

Support:
- PDF preview
- text preview
- loading and error states

- [ ] **Step 5: Verify end-to-end uploading workflow**

Expected:
- upload succeeds
- row selection works
- preview opens
- move/remark/delete mutations invalidate correctly

### Task 8: Migrate Coding View

**Files:**
- Create: `frontend/src/features/coding/api.ts`
- Create: `frontend/src/features/coding/hooks.ts`
- Create: `frontend/src/features/coding/store.ts`
- Create: `frontend/src/features/coding/components/file-tabs.tsx`
- Create: `frontend/src/features/coding/components/pdf-viewer.tsx`
- Create: `frontend/src/features/coding/components/highlights-panel.tsx`
- Modify: `frontend/src/routes/project/$projectId.coding.tsx`

- [ ] **Step 1: Implement opened-file tab state**

Persist:
- opened files
- active file

- [ ] **Step 2: Implement file URL and highlights fetching**

Support:
- blob PDF fetch
- highlights JSON fetch
- loading progress state

- [ ] **Step 3: Implement the coding workspace UI**

Include:
- tools/file selection panel
- tab strip
- PDF viewer
- highlights side panel

- [ ] **Step 4: Verify file-switch behavior**

Expected:
- tab open/close works
- active file switch clears stale highlight data
- PDF reloads correctly

### Task 9: Carry Placeholder Project Tabs

**Files:**
- Modify: `frontend/src/routes/project/$projectId.classifying.tsx`
- Modify: `frontend/src/routes/project/$projectId.logic-chain.tsx`
- Modify: `frontend/src/routes/project/$projectId.generating.tsx`

- [ ] **Step 1: Add explicit placeholder pages**

Each page should clearly state:
- feature not migrated yet or pending backend capability
- project context is preserved

- [ ] **Step 2: Verify route parity**

Expected:
- all legacy project tabs resolve in the React app

### Task 10: Quality Gates And Cutover

**Files:**
- Modify: `frontend/package.json`
- Create: `frontend/.env.example`
- Create: `frontend/README.md`
- Create: `frontend/src/test/*`

- [ ] **Step 1: Add basic quality scripts**

Add scripts for:
- `pnpm dev`
- `pnpm build`
- `pnpm typecheck`
- `pnpm lint`

- [ ] **Step 2: Add minimum automated tests**

Prefer:
- Vitest + React Testing Library for auth form, route guards, and one query-backed page

- [ ] **Step 3: Write frontend setup docs**

Include:
- required env vars
- dev commands
- backend dependency assumptions

- [ ] **Step 4: Run release checks**

Run:

```bash
cd /home/ling/GraduateProject/frontend
pnpm install
pnpm typecheck
pnpm build
```

Expected: the migrated frontend is reproducibly installable and buildable.

## Risks And Watchpoints

- The legacy auth flow hashes passwords with MD5 on the client. Preserve it for compatibility now, but flag it for later backend cleanup.
- The old Vue app has inconsistent i18n setup. Do not port that implementation directly.
- The old request layer mixes networking and global UI state. Keep the new API client pure and let UI state subscribe around it.
- The old project contains debug logging and dead routes. Treat them as defects to avoid, not behavior to preserve.
- PDF rendering is the highest-risk feature slice. Verify it in isolation before integrating polish work.
- If visual work is postponed until the end, the app will likely regress into a generic scaffold. Apply the design system and layout polish from the shell phase onward.

## Definition Of Done

- `frontend/` exists as an independent pnpm-managed React app.
- Auth, documents, trash, uploading, and coding flows are migrated.
- Project workspace routes exist for all legacy tabs.
- Backend API compatibility is preserved for the migrated flows.
- The app builds locally with `pnpm build`.
- Setup and scope are documented inside the repository.

## Suggested Execution Order

1. Bootstrap app and design system.
2. Shared infra and route shell.
3. Auth.
4. Documents and trash.
5. Project workspace shell.
6. Uploading.
7. Coding.
8. Placeholder tabs.
9. Quality gates and cutover.
