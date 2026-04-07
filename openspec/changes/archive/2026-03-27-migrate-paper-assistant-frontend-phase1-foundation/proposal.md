## Why

The current frontend migration work has direction and planning documents, but it does not yet have an OpenSpec change that defines the first implementation slice. We need a scoped, implementation-ready change for the frontend foundation so the React migration can start on a stable base instead of mixing tooling, UI system, and feature work in one step.

## What Changes

- Establish the first implementation slice for the new `frontend/` React application.
- Add the core frontend stack required by the migration plan: Vite, React, TypeScript, TanStack Query, TanStack Router, Zustand, React Hook Form, Zod, Lucide React, Tailwind CSS, shadcn/ui, and Radix UI.
- Replace the default Vite starter presentation with a project-specific visual foundation.
- Define the initial application structure, shared providers, styling tokens, and directory layout needed for later feature migration.
- Add baseline developer scripts and environment conventions so future phases can build on a reproducible frontend foundation.

## Capabilities

### New Capabilities
- `frontend-foundation`: Establishes the React application structure, shared providers, environment conventions, and base developer tooling for the migrated frontend.
- `frontend-design-system`: Establishes the Tailwind and shadcn/ui based visual system, theme tokens, and reusable UI baseline for future feature work.

### Modified Capabilities
- None.

## Impact

- Affected code: `/home/ling/GraduateProject/frontend`
- Affected docs: migration plan and todo documents already created under `/home/ling/GraduateProject/docs`
- New dependencies: TanStack Query, TanStack Router, Zustand, React Hook Form, Zod, Lucide React, Tailwind CSS, shadcn/ui, Radix UI, axios, md5, pdfjs-dist
- Affected systems: frontend build pipeline, UI foundation, local developer workflow
