## ADDED Requirements

### Requirement: Main layout provides header component
The main layout SHALL include a header component at the top of the page.

#### Scenario: Header is rendered
- **WHEN** the user visits any `/app/*` route
- **THEN** the header MUST be visible at the top of the viewport
- **AND** the header SHOULD contain application branding or title

### Requirement: Main layout provides sidebar navigation
The main layout SHALL include a sidebar with navigation links.

#### Scenario: Sidebar navigation is available
- **WHEN** the user visits any `/app/*` route
- **THEN** the sidebar MUST be visible on the left side
- **AND** the sidebar MUST contain links to `/app/documents`, `/app/trash`, `/app/profile`

### Requirement: Main layout provides content area
The main layout SHALL provide a content area where page content is rendered.

#### Scenario: Content area renders page content
- **WHEN** the user visits any `/app/*` route
- **THEN** the main content MUST be rendered in the content area
- **AND** the content area MUST be visually distinct from header and sidebar

### Requirement: Main layout is responsive
The main layout SHALL adapt to different screen sizes.

#### Scenario: Layout adapts to desktop
- **WHEN** the viewport width is >= 1024px
- **THEN** the sidebar MUST be visible by default

#### Scenario: Layout adapts to mobile
- **WHEN** the viewport width is < 1024px
- **THEN** the sidebar MAY be hidden behind a toggle