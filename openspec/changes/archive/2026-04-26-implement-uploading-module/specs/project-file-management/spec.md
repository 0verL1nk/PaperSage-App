## ADDED Requirements

### Requirement: List folders
The system SHALL allow users to view the folder hierarchy within a specific project.

#### Scenario: User navigates to the Uploading tab
- **GIVEN** a project with existing folders and documents
- **WHEN** the user opens `/project/$projectId/uploading`
- **THEN** the system displays the folder tree in the left pane
- **AND** the system displays root-level documents (folder_id IS NULL) in the center pane

#### Scenario: User selects a folder
- **WHEN** the user clicks on a folder in the folder tree
- **THEN** the center pane shows only the documents within that folder
- **AND** the folder is visually highlighted as active

### Requirement: Create folders
The system SHALL allow users to create folders within a project to organize documents.

#### Scenario: Create root-level folder
- **WHEN** the user clicks the "+" button in the folder tree header
- **THEN** a dialog appears asking for the folder name
- **AND** on submit, the folder is created with `parent_id = NULL` under the current project

#### Scenario: Create subfolder
- **WHEN** the user right-clicks a folder and selects "New Subfolder"
- **THEN** a dialog appears asking for the folder name
- **AND** on submit, the folder is created with `parent_id` set to the parent folder

#### Scenario: Folder name validation
- **WHEN** the user submits an empty folder name
- **THEN** the system shows a validation error "文件夹名称不能为空"

### Requirement: Delete folders
The system SHALL allow users to delete folders, moving contained documents to the project root.

#### Scenario: Delete a folder with documents
- **WHEN** the user deletes a folder that contains documents
- **THEN** a confirmation dialog appears
- **AND** on confirm, the folder is deleted
- **AND** all documents previously in that folder have their `folder_id` set to NULL (moved to root)

#### Scenario: Delete a folder with subfolders
- **WHEN** the user deletes a folder that has child folders
- **THEN** a confirmation dialog warns about nested content
- **AND** on confirm, the folder and all child folders are deleted (CASCADE)
- **AND** all documents from all deleted folders are moved to root

### Requirement: Upload documents
The system SHALL allow users to upload PDF, DOC, DOCX, and TXT files into the selected folder (or project root).

#### Scenario: Upload via drag and drop
- **WHEN** the user drags supported files onto the dropzone
- **THEN** the system starts uploading each file with a progress indicator
- **AND** on completion, the documents appear in the file list

#### Scenario: Upload via file selector
- **WHEN** the user clicks the "Upload" button and selects files
- **THEN** the system validates file type (PDF, DOC, DOCX, TXT only)
- **AND** rejects unsupported file types with an error message

#### Scenario: Upload exceeds size limit
- **WHEN** the user attempts to upload a file larger than 50MB
- **THEN** the system rejects the file with an error "文件大小不能超过 50MB"

#### Scenario: Upload with remark
- **WHEN** the user uploads a file with an optional remark
- **THEN** the remark is stored with the document record

### Requirement: List documents
The system SHALL display documents for the selected folder or project root.

#### Scenario: Documents in root
- **WHEN** no folder is selected (or "All Files" is selected)
- **THEN** the file list shows all documents with `folder_id = NULL`

#### Scenario: Documents in a folder
- **WHEN** a folder is selected in the tree
- **THEN** the file list shows only documents in that folder

#### Scenario: Empty folder
- **WHEN** the selected folder has no documents
- **THEN** the center pane shows an empty state with a dropzone prompt

### Requirement: Manage document properties and location
The system SHALL allow users to select multiple documents and perform operations.

#### Scenario: Select documents
- **WHEN** the user clicks the checkbox on a document row
- **THEN** the document is added to the selection (tracked in Zustand store)

#### Scenario: Select all documents
- **WHEN** the user clicks the header checkbox
- **THEN** all visible documents are selected

#### Scenario: Move document to another folder
- **WHEN** the user clicks "Move to..." on a document
- **THEN** a dialog shows a folder picker with all project folders
- **AND** on confirm, the document's `folder_id` is updated

#### Scenario: Edit document remark
- **WHEN** the user clicks "Edit Remark" on a document
- **THEN** a dialog shows a textarea pre-filled with the current remark
- **AND** on save, the remark is updated

#### Scenario: Delete documents
- **WHEN** the user clicks "Delete" on one or more selected documents
- **THEN** a confirmation dialog appears
- **AND** on confirm, the documents are permanently removed from the database and S3 storage

### Requirement: File type validation on backend
The system SHALL validate uploaded file types on the backend.

#### Scenario: Supported file types
- **WHEN** a file with MIME type `application/pdf`, `application/msword`, `application/vnd.openxmlformats-officedocument.wordprocessingml.document`, or `text/plain` is uploaded
- **THEN** the file is accepted and stored in S3

#### Scenario: Unsupported file type
- **WHEN** a file with an unsupported MIME type is uploaded
- **THEN** the backend returns HTTP 400 with an error message listing supported types

### Requirement: S3 storage isolation
The system SHALL store uploaded documents in a separate S3 namespace from LaTeX files.

#### Scenario: Document S3 key format
- **WHEN** a document is uploaded
- **THEN** the S3 key follows the pattern `projects/{projectId}/documents/{uuid}/{filename}`
- **AND** the UUID prevents filename collisions between uploads with the same name
