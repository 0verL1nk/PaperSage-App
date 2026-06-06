## Why

The PaperSage platform is designed to manage research documents within specific projects. Currently, the project workspace only shows placeholder pages. Implementing the Uploading module (Phase 6) is critical as it serves as the entry point for users to upload, organize, and preview their reference documents (PDFs, TXT, etc.) inside a project, enabling the rest of the research workflow (coding, classifying, etc.).

## What Changes

- Create `UploadingPage` with a three-pane layout: Folder Tree (left), File List & Batch Actions (center), and Document Preview (right).
- Implement file and folder listing using TanStack Query hooks.
- Add document uploading capabilities with support for `multipart/form-data`, file type restrictions (PDF, DOC, DOCX, TXT), and upload progress indicators.
- Implement batch operations for documents including selection, moving, editing remarks, and deletion.
- Integrate a PDF viewer (using `pdfjs-dist`) in the right-hand preview panel.

## Capabilities

### New Capabilities
- `project-file-management`: Managing folders and documents (upload, move, delete, remark) within a specific project context.
- `document-preview`: Previewing PDF and text documents directly within the frontend application without downloading.

### Modified Capabilities

## Impact

- **Frontend**: Adds significant new UI components to `src/views/uploading`, integrates `pdfjs-dist` for previewing, and adds new API hooks in `src/services/` for file and folder management.
- **Dependencies**: Relies heavily on `@tanstack/react-query` and `zustand` (for selection and upload progress state).
