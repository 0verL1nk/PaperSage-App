## Context

The repository already contains a scaffolded Vite React TypeScript application at `frontend/`, while the source Vue application lives at `../PaperAssistantFront`. Planning documents exist for the broader migration, but the first implementation slice needs a tighter technical boundary so work can start without pulling feature migration, routing, and backend integration into the same step.

This first change is intentionally foundational. It establishes the base frontend stack, shared app structure, design system, and developer workflow required for later phases such as auth, project lists, uploading, and coding. The main constraints are:

- package management must use `pnpm`
- the target stack must include Tailwind CSS, shadcn/ui, and Radix UI
- the migrated frontend should look intentional, not like a default starter
- later feature phases will depend on whatever app structure and UI tokens are created here

## Goals / Non-Goals

**Goals:**
- Convert the generated Vite starter into a usable frontend foundation for the migration.
- Install and configure the required shared libraries for routing, server state, local state, forms, validation, icons, API access, and PDF support.
- Establish a directory layout that separates `app`, `routes`, `shared`, and `features`.
- Initialize Tailwind CSS and shadcn/ui in a way that supports Radix-based components and future reuse.
- Introduce an initial visual system with semantic tokens, stronger typography, and a branded landing shell instead of starter content.
- Add baseline scripts and environment conventions so future phases can build and typecheck consistently.

**Non-Goals:**
- Implement auth, documents, trash, uploading, coding, or any backend-driven feature flow.
- Finalize the complete route tree for the migrated app.
- Port Vue components one-for-one.
- Solve PDF rendering details beyond installing the dependency and reserving the structure for later phases.

## Decisions

### 1. Keep the change scoped to the migration foundation

This change stops before business features. That keeps the first implementation small enough to verify quickly and prevents routing, data fetching, and UI library setup from being entangled with feature migration defects.

Alternative considered:
- Start auth and project-list work in the same change. Rejected because it would make failure modes ambiguous: a broken page could come from setup, routing, API shape, or UI composition.

### 2. Use the existing `frontend/` Vite app instead of recreating it

The user already scaffolded the project with `pnpm create vite@latest . -- --template react-ts`. Reusing it avoids churn in generated files and keeps the first change focused on turning the starter into the intended foundation.

Alternative considered:
- Recreate the project from scratch. Rejected because it adds noise without adding value.

### 3. Establish a domain-ready source layout immediately

The source tree will be reorganized into `src/app`, `src/routes`, `src/shared`, and `src/features`. This creates a stable place for future providers, route definitions, shared utilities, and feature slices, and avoids a second structural migration later.

Alternative considered:
- Keep everything under a flat `src/` until features are implemented. Rejected because later moves would create unnecessary churn across every file import.

### 4. Initialize shadcn/ui as the primary UI source layer

shadcn/ui is chosen as the source-code UI layer because later work needs flexibility, Radix-based accessibility primitives, and a reusable design system rather than a large opaque component dependency. Tailwind tokens and component source files also make it easier to preserve a consistent visual language.

Alternative considered:
- Use only raw Radix primitives and hand-build the UI layer. Rejected because it would slow down future feature work and duplicate a problem already solved by shadcn/ui.

### 5. Create a visual foundation during the setup phase

The default Vite starter will be replaced with a branded shell and semantic theme tokens in the setup phase. This avoids the common failure mode where engineering establishes technical foundations but leaves a visually generic UI that later becomes expensive to rework.

Alternative considered:
- Leave visual work for a later polish phase. Rejected because layout, spacing, token naming, and component choices are part of the foundation, not cosmetic afterthoughts.

### 6. Add future-facing dependencies now, but delay feature coupling

TanStack Query, TanStack Router, Zustand, React Hook Form, Zod, Lucide React, axios, md5, and pdfjs-dist will be added now so the foundation already matches the migration target stack. Their first usage can stay minimal in this change.

Alternative considered:
- Install each dependency only when a specific feature needs it. Rejected because the foundation phase is specifically responsible for stack convergence and developer reproducibility.

## Risks / Trade-offs

- [shadcn initialization differences across versions] → Use the CLI to inspect and initialize the project rather than assuming an older setup flow.
- [Too much scaffolding with too little behavior] → Keep the foundation visible by replacing the starter page with a realistic shell and by adding the actual project directory layout.
- [Visual baseline becomes inconsistent later] → Define semantic tokens and reusable UI primitives now so later phases extend them rather than improvising page-level styles.
- [Stack installed but unused] → Accept this in the foundation phase because the change is explicitly about stack convergence and not only runtime behavior.
- [Future route structure may evolve] → Only create the minimal app shell and source structure here; defer business route details to later capability changes.

## Migration Plan

1. Audit the scaffolded `frontend/` app and remove starter-specific content.
2. Install the required dependencies and ensure scripts cover build, lint, preview, and typecheck.
3. Initialize Tailwind CSS and shadcn/ui for the Vite project.
4. Establish shared theme tokens, global styles, and a branded starter shell.
5. Create the initial source folders and placeholder files required by later phases.
6. Run build and typecheck verification before advancing to feature work.

Rollback strategy:
- If setup fails, revert the `frontend/` changes in this change set only; no backend or data migration is involved.

## Open Questions

- Which shadcn preset and base color should be considered the long-term default for this project?
- Should the first branded shell already include the future app providers and route wiring, or should those land in the next change after the design system is stable?
- Is a dedicated font choice desired now, or should the foundation stay on system fonts until the first feature pages land?
