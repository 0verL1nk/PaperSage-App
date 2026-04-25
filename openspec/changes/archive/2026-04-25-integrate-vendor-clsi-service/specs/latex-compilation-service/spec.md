## ADDED Requirements

### Requirement: Multi-file LaTeX projects compile through the vendored CLSI service
The system SHALL compile a PaperSage LaTeX project by invoking the vendored `clsi` service with a selected root file and the full project resource set, preserving nested paths and relative file references.

#### Scenario: Compile a template with nested files and bibliography
- **WHEN** a project contains `main.tex`, included section files in subdirectories, bibliography files, and image assets
- **THEN** the backend MUST assemble those resources into a `clsi` compile request without flattening their paths
- **AND** the vendored service MUST be able to produce `output.pdf` when the template is valid

#### Scenario: Compile with a selected TeX engine
- **WHEN** the caller requests `pdflatex`, `xelatex`, or `lualatex` for a valid project
- **THEN** the compile request MUST forward the selected engine to the vendored service
- **AND** the reported compile result MUST correspond to the requested engine configuration

### Requirement: Compilation outputs and control operations are preserved
The system SHALL preserve vendor support for build outputs and compile control operations so that a compiled project can expose artifacts and respond to operational commands.

#### Scenario: Access build outputs after a successful compile
- **WHEN** a compile succeeds and the vendor service returns output file metadata
- **THEN** the system MUST retain the build identifier and output file list
- **AND** it MUST make the generated output files retrievable through backend-managed endpoints

#### Scenario: Stop a running compile and clear compile cache
- **WHEN** a compile is in progress or cached build state exists for a project
- **THEN** the system MUST support forwarding stop-compile and clear-cache operations to the vendored service
- **AND** those operations MUST target the same project compilation context used for compile requests

### Requirement: Wordcount remains available for LaTeX projects
The system SHALL expose vendor-backed LaTeX wordcount capability for project files.

#### Scenario: Count words for a selected source file
- **WHEN** a caller requests wordcount for a file in a LaTeX project
- **THEN** the backend MUST invoke the vendored wordcount capability for that project context
- **AND** the response MUST include the returned wordcount result without requiring a separate compilation implementation in PaperSage
