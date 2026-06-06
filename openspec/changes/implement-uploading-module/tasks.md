## 1. Project Infrastructure

- [x] 1.1 Create the API services (`src/services/files-api.ts` and `src/services/folders-api.ts`) with TanStack queries/mutations.
- [x] 1.2 Define Zod schemas and TypeScript interfaces for file/folder models in `src/types`.
- [x] 1.3 Create the Zustand store (`src/store/upload-store.ts`) for selection and progress management.

## 2. Layout & Shell Components

- [x] 2.1 Set up `src/views/uploading/UploadingPage.tsx` skeleton and route integration.
- [x] 2.2 Create three-pane layout shell (Folder Tree | File List | Document Preview).
- [x] 2.3 Implement the mobile-responsive behaviors for these panes.

## 3. Left Pane: Folder Tree

- [x] 3.1 Fetch and render folder hierarchy.
- [x] 3.2 Add create folder and delete folder actions.
- [x] 3.3 Link folder selection to the Zustand store state.

## 4. Center Pane: File Table & Batch Actions

- [x] 4.1 Implement File Table view mapped to the selected folder.
- [x] 4.2 Implement row selection (linking to `selectedFileIds` in the store).
- [x] 4.3 Create the batch action toolbar (Move, Edit Remarks, Delete).
- [x] 4.4 Build the Move File Dialog component.
- [x] 4.5 Build the Edit Remarks Dialog component.

## 5. Upload Functionality

- [x] 5.1 Implement the drag-and-drop / file selector component.
- [x] 5.2 Wire up the multipart/form-data upload API.
- [x] 5.3 Connect Axios progress events to the `uploadProgress` Zustand store.
- [x] 5.4 Render upload progress indicators in the UI.

## 6. Right Pane: Document Preview

- [x] 6.1 Implement `PdfViewer` component using `pdfjs-dist`.
- [x] 6.2 Implement `TxtViewer` component.
- [x] 6.3 Link the preview pane to the currently active/selected file in the table.

## 7. Testing & Polish

- [ ] 7.1 Verify API mocks / write Vitest unit tests for the upload store and core hooks.
- [ ] 7.2 Run full linting (`pnpm lint`) and typechecking (`pnpm typecheck`).
