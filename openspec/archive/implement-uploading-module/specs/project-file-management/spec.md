## ADDED Requirements

### Requirement: List folders and documents
The system SHALL allow users to view a list of folders and documents within a specific project.

#### Scenario: User navigates to the Uploading tab
- **WHEN** the user opens `/project/$projectId/uploading`
- **THEN** the system displays the root folder's documents in the file table
- **THEN** the system displays the folder tree in the left pane

### Requirement: Upload documents
The system SHALL allow users to upload PDF, DOC, DOCX, and TXT files into the selected folder.

#### Scenario: Successful document upload
- **WHEN** the user selects files and initiates upload
- **THEN** the system displays upload progress
- **THEN** the system adds the uploaded documents to the file table upon success

### Requirement: Manage document properties and location
The system SHALL allow users to select multiple documents and perform bulk operations like moving to a different folder, deleting, or editing remarks.

#### Scenario: Bulk deleting documents
- **WHEN** the user selects one or more documents and clicks the delete action
- **THEN** the system permanently removes the documents from the view after confirmation
