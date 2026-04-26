## ADDED Requirements

### Requirement: Preview PDF documents
The system SHALL allow users to preview PDF documents in the right-hand pane of the uploading interface.

#### Scenario: User selects a PDF document
- **WHEN** the user clicks on a PDF file row in the file table
- **THEN** the right-hand pane renders the PDF content using the `react-pdf` viewer
- **AND** all pages are rendered in a scrollable vertical layout

#### Scenario: PDF with many pages
- **WHEN** the user previews a PDF with 50+ pages
- **THEN** the system renders pages on demand via pdf.js Web Worker
- **AND** scrolling performance remains smooth

#### Scenario: PDF preview zoom
- **WHEN** the user adjusts the zoom control in the preview header
- **THEN** the PDF re-renders at the new scale
- **AND** rendering is debounced (400ms) to avoid excessive re-renders during continuous zoom adjustment

#### Scenario: PDF preview source
- **WHEN** a PDF document is selected for preview
- **THEN** the frontend fetches the preview URL from `GET /api/v1/documents/{id}/preview`
- **AND** the backend returns a streaming response from S3 (not a presigned URL)
- **AND** the PDF viewer uses the existing `range-transport.ts` for HTTP Range requests

### Requirement: Preview text documents
The system SHALL allow users to preview plain text documents (.txt) in the right-hand pane.

#### Scenario: User selects a text document
- **WHEN** the user clicks on a TXT file row in the file table
- **THEN** the right-hand pane renders the text content in a monospace font
- **AND** long text is wrapped within the pane with vertical scrolling

### Requirement: DOCX document preview
The system SHALL allow users to preview DOCX documents in the right-hand pane using browser-side rendering.

#### Scenario: User selects a DOCX document
- **WHEN** the user clicks on a DOCX file row in the file table
- **THEN** the right-hand pane renders the DOCX content using `docx-preview` (`renderAsync`)
- **AND** the document is rendered with pagination, fonts, styles, and images preserved
- **AND** long documents scroll vertically within the pane

#### Scenario: DOCX preview source
- **WHEN** a DOCX document is selected for preview
- **THEN** the frontend fetches the file blob from `GET /api/v1/documents/{id}/preview`
- **AND** passes the blob to `docx-preview`'s `renderAsync(blob, container)` for rendering

### Requirement: Empty preview state
The system SHALL show a placeholder when no document is selected.

#### Scenario: No document selected
- **WHEN** the user has not clicked on any document
- **THEN** the right-hand pane shows "Select a document to preview" message

#### Scenario: Document deselected
- **WHEN** the user clears the selection or switches folders
- **THEN** the preview pane returns to the empty state
