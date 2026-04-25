## ADDED Requirements

### Requirement: Project card navigation is functional

When a user clicks a project card in the project list, the application SHALL navigate to the project workspace.

#### Scenario: User clicks project card from project list

- **WHEN** user clicks a project card in the project list page
- **THEN** the application SHALL navigate to `/project/$projectId`
- **AND** the URL SHALL contain the correct `projectId` parameter

#### Scenario: User clicks project card with valid session

- **WHEN** user has a valid session token and clicks a project card
- **THEN** navigation SHALL succeed without additional auth challenges

---

### Requirement: Auth route redirects logged-in users to valid page

When a logged-in user visits the auth page, they SHALL be redirected to the projects list, not to an incomplete page.

#### Scenario: Logged-in user visits /auth

- **WHEN** a user with a valid session token navigates to `/auth`
- **THEN** the application SHALL redirect to `/app/projects`
- **AND** the user SHALL NOT see the auth login form

#### Scenario: Logged-in user redirected to projects

- **WHEN** redirect from `/auth` to `/app/projects` occurs
- **THEN** the projects list page SHALL render correctly with user's projects

---

### Requirement: Sidebar quick-create button opens project dialog

When a user clicks the + button in the sidebar, the application SHALL show the CreateProjectDialog.

#### Scenario: User clicks sidebar + button

- **WHEN** user clicks the + button in the sidebar
- **THEN** the CreateProjectDialog SHALL appear
- **AND** the user SHALL be able to create a new project from the dialog

#### Scenario: Dialog closes after successful project creation

- **WHEN** user successfully creates a project from the dialog
- **THEN** the dialog SHALL close automatically
- **AND** the project list SHALL refresh to show the new project

---

### Requirement: Workspace layout uses framework routing

The workspace layout component SHALL use TanStack Router's `useParams` hook instead of manual `window.location` parsing.

#### Scenario: Project workspace loads with correct project ID

- **WHEN** user navigates to `/project/$projectId`
- **THEN** the WorkspaceLayout SHALL extract `projectId` from route params via `useParams`
- **AND** `window.location` SHALL NOT be accessed for routing purposes

---

### Requirement: Logout request uses consistent API path

The logout request SHALL use the same API path prefix as other auth endpoints.

#### Scenario: User initiates logout

- **WHEN** user clicks logout button
- **THEN** the logout request SHALL be sent to `/api/v1/auth/logout`
- **AND** the request SHALL be allowed by CORS configuration
- **AND** the refresh token SHALL be revoked from Redis

---

### Requirement: Create project dialog has no duplicate event handlers

The create project form submit handler SHALL only prevent the default event behavior once.

#### Scenario: User submits create project form

- **WHEN** user fills in the project form and submits
- **THEN** `e.preventDefault()` SHALL be called exactly once per submission
- **AND** the form SHALL be processed correctly without errors
