## ADDED Requirements

### Requirement: Application uses @tanstack/react-router for routing
The frontend application SHALL use @tanstack/react-router as the routing solution, providing type-safe navigation between different views.

#### Scenario: Router is configured
- **WHEN** a developer inspects the router configuration
- **THEN** the application MUST use `@tanstack/react-router` for route management
- **AND** routes MUST be defined with proper TypeScript types

### Requirement: Root route provides global layout
The application SHALL define a root route that wraps all other routes with global providers and layout components.

#### Scenario: Root route contains global elements
- **WHEN** the application mounts at `/`
- **THEN** the root route MUST provide global toast/notification container
- **AND** the root route MUST provide global loading overlay
- **AND** the root route MUST render the child route via `<Outlet />`

### Requirement: Auth route is accessible without authentication
The auth route SHALL be publicly accessible and not require authentication.

#### Scenario: Auth route is public
- **WHEN** an unauthenticated user navigates to `/auth`
- **THEN** the route MUST render without redirect

### Requirement: Protected routes redirect unauthenticated users
The application SHALL protect routes that require authentication by redirecting to `/auth` when no valid session exists.

#### Scenario: Unauthenticated user accesses protected route
- **WHEN** an unauthenticated user navigates to `/app/documents`
- **THEN** the application MUST redirect to `/auth`

#### Scenario: Authenticated user accesses auth route
- **WHEN** an authenticated user navigates to `/auth`
- **THEN** the application SHOULD redirect to `/app/documents`

### Requirement: Application route structure follows /app prefix convention
The application SHALL use `/app` prefix for main application routes.

#### Scenario: Main routes use /app prefix
- **WHEN** a developer inspects the route definitions
- **THEN** document list MUST be at `/app/documents`
- **AND** trash list MUST be at `/app/trash`
- **AND** profile MUST be at `/app/profile`

### Requirement: Project routes use /project/:projectId pattern
The application SHALL organize project-specific routes under `/project/:projectId`.

#### Scenario: Project routes are nested
- **WHEN** a developer inspects the route definitions
- **THEN** project root MUST be at `/project/:projectId`
- **AND** uploading MUST be at `/project/:projectId/uploading`
- **AND** coding MUST be at `/project/:projectId/coding`
- **AND** classifying MUST be at `/project/:projectId/classifying`
- **AND** logic-chain MUST be at `/project/:projectId/logic-chain`
- **AND** generating MUST be at `/project/:projectId/generating`