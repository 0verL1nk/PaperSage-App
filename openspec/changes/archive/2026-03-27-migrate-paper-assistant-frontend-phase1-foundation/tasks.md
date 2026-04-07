## 1. Foundation dependencies and scripts

- [x] 1.1 Audit the scaffolded `frontend/package.json` and remove starter-only assumptions that do not fit the migration stack
- [x] 1.2 Add the required runtime dependencies for routing, server state, local state, forms, validation, icons, API access, hashing, and PDF support
- [x] 1.3 Add the required development dependencies for Tailwind CSS and any missing frontend tooling
- [x] 1.4 Add or normalize frontend scripts for `dev`, `build`, `preview`, `lint`, and `typecheck`

## 2. Styling and design system setup

- [x] 2.1 Initialize Tailwind CSS for the Vite frontend project
- [x] 2.2 Initialize shadcn/ui for the same project using the project's `pnpm` workflow
- [x] 2.3 Generate the baseline reusable UI components needed for the foundation phase
- [x] 2.4 Define centralized visual tokens and global theme styles for surfaces, text hierarchy, accents, and interaction states

## 3. Application structure and starter shell

- [x] 3.1 Replace the default Vite starter screen and starter assets with a project-specific foundation screen
- [x] 3.2 Create the initial `src/app`, `src/routes`, `src/shared`, and `src/features` directory layout
- [x] 3.3 Add placeholder bootstrap files for the future app providers and shared application entry structure
- [x] 3.4 Add a branded visual shell that demonstrates the intended product tone instead of a generic scaffold

## 4. Verification and readiness

- [x] 4.1 Run frontend dependency installation with `pnpm` and confirm the lockfile is consistent
- [x] 4.2 Run frontend type checking and fix any setup issues
- [x] 4.3 Run a production frontend build and fix any setup issues
- [x] 4.4 Review the resulting foundation files to confirm the frontend is ready for the next migration change
