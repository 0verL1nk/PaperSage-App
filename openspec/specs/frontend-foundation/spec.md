## MODIFIED Requirements

### Requirement: Frontend source tree is organized for feature migration
The frontend application SHALL define an initial source layout that separates application bootstrap concerns from shared utilities and future feature modules, and it SHALL provide dedicated extension points for the runtime infrastructure required by later migration phases.

#### Scenario: Source directories are prepared
- **WHEN** a developer inspects `frontend/src`
- **THEN** the source tree MUST include dedicated locations for `app`, `routes`, `shared`, and `features`
- **AND** the `shared` tree MUST include stable locations for `api`, `components`, `config`, `hooks`, `lib`, and `stores`
- **AND** the `features` tree MUST include the domain folders needed by the first migration milestone
- **AND** those locations MUST be suitable for future migration work without a second structural rewrite
