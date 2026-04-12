## Context

The PaperSage frontend is actively being migrated. The core layout and authentication layers are completed. Now, we are implementing the `Uploading` module, which is the entry point for all documents into a specific project workspace.
Currently, navigating to `/project/$projectId/uploading` displays only a placeholder. The module needs to handle file uploads, listing files/folders, and displaying a preview of selected files.

## Goals / Non-Goals

**Goals:**
- Provide a robust three-pane layout for the Uploading view: Folder Tree, File List, and Document Preview.
- Build reliable integration with the file management APIs using `@tanstack/react-query`.
- Build a generic PDF and TXT preview component.
- Implement batch actions (delete, move, edit remarks) utilizing `zustand` for managing client-side selection state.

**Non-Goals:**
- Not implementing the actual backend parsing/text extraction (that happens in other phases).
- Not modifying global project metadata settings in this view.
- Not implementing complex folder drag-and-drop operations in v1 (only basic API-driven moves via dialog).

## Decisions

- **State Management**: Use `zustand` (`useUploadStore`) for UI state like `selectedFolderId` and `selectedFileIds` instead of lifting state to the URL or complex context providers. This keeps the batch-action logic decoupled and predictable.
- **Data Fetching**: Use `@tanstack/react-query` for all GET operations (files, folders) and mutations. This handles caching, invalidation on success, and error states gracefully, matching Phase 4/5 patterns.
- **PDF Viewer**: Use `pdfjs-dist` integrated via a custom React hook/component. It provides maximum control over rendering scale and pagination without relying on heavy external PDF component libraries.

## Risks / Trade-offs

- **Risk**: Large PDF files could cause performance issues on the frontend.
  - **Mitigation**: Offload heavy PDF rendering to a Web Worker (native to pdf.js config) and implement lazy-loading/pagination inside the viewer.
- **Risk**: Uploading large files can fail silently or block UI.
  - **Mitigation**: Implement progress tracking using Axios `onUploadProgress` and visualize it via the `uploadProgress` state in the zustand store.
