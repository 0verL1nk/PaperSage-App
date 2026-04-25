## ADDED Requirements

### Requirement: Local development bootstrap supports Ubuntu/Debian and macOS
The development workflow SHALL provide a documented and scriptable host-mode bootstrap process for running the vendored LaTeX service on Ubuntu/Debian and macOS without requiring Docker.

#### Scenario: Install local prerequisites on Ubuntu/Debian
- **WHEN** a developer runs the local installation workflow on Ubuntu or Debian
- **THEN** the workflow MUST install or validate the system packages required for host-mode `clsi`, including Node, TeX Live tooling, and supporting command-line utilities
- **AND** it MUST install the vendored Node workspace dependencies needed to run `clsi`

#### Scenario: Install local prerequisites on macOS
- **WHEN** a developer runs the local installation workflow on macOS
- **THEN** the workflow MUST install or validate the Homebrew and TeX distribution dependencies required for host-mode `clsi`
- **AND** it MUST install the vendored Node workspace dependencies needed to run `clsi`

### Requirement: Local development includes validation and startup workflows
The development workflow SHALL provide explicit validation and startup commands for the host-mode vendor service.

#### Scenario: Validate a developer machine before starting the service
- **WHEN** a developer runs the validation workflow after installation
- **THEN** the workflow MUST check for required commands such as `node`, `npm`, `latexmk`, and the supported TeX engines
- **AND** it MUST fail with actionable output when required tooling is unavailable

#### Scenario: Start the vendored service without Docker
- **WHEN** a developer runs the local service startup workflow
- **THEN** the vendored `clsi` service MUST start in host mode without enabling `SANDBOXED_COMPILES`
- **AND** it MUST use local writable directories for compile, output, cache, and upload state
