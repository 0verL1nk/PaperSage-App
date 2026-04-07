## ADDED Requirements

### Requirement: Frontend foundation stack is installed and configured
The frontend application SHALL provide the agreed React migration foundation in `frontend/`, using `pnpm` as the package manager and including the required shared dependencies for routing, server state, local state, forms, validation, icons, API access, and PDF support.

#### Scenario: Foundation dependencies are present
- **WHEN** a developer inspects `frontend/package.json`
- **THEN** the file MUST define the agreed runtime and development dependencies for the migration foundation
- **AND** the file MUST expose scripts for development, build, preview, lint, and type checking

### Requirement: Frontend source tree is organized for feature migration
The frontend application SHALL define an initial source layout that separates application bootstrap concerns from shared utilities and future feature modules.

#### Scenario: Source directories are prepared
- **WHEN** a developer inspects `frontend/src`
- **THEN** the source tree MUST include dedicated locations for `app`, `routes`, `shared`, and `features`
- **AND** those locations MUST be suitable for future migration work without a second structural rewrite

### Requirement: Frontend build conventions are reproducible
The frontend foundation SHALL be buildable and type-checkable in a clean local environment without relying on the default Vite starter content.

#### Scenario: Foundation verification succeeds
- **WHEN** a developer runs the documented verification commands for the frontend foundation
- **THEN** the frontend MUST complete type checking successfully
- **AND** the frontend MUST complete a production build successfully

