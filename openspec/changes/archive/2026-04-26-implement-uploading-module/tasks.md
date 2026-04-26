## Phase 1: Backend Models, Schemas, and Migration

- [x] 1.1 Create `backend/models/folder.py` — Folder model with self-referencing parent_id, project_id FK, owner_id FK
- [x] 1.2 Create `backend/models/document.py` — Document model with project_id FK, folder_id FK (SET NULL), s3_key, status, remark
- [x] 1.3 Create `backend/schemas/folder.py` — FolderCreate, FolderResponse Pydantic schemas
- [x] 1.4 Create `backend/schemas/document.py` — DocumentUploadResponse, DocumentMoveRequest, DocumentRemarkRequest schemas
- [x] 1.5 Update `backend/models/__init__.py` — export Folder and Document
- [x] 1.6 Update `backend/schemas/__init__.py` — export new schemas
- [x] 1.7 Run `alembic revision --autogenerate -m "add_folders_and_documents_tables"` and verify migration
- [x] 1.8 Run `alembic upgrade head` to apply migration

## Phase 2: Backend Service Layer and Routes

- [x] 2.1 Create `backend/services/documents/__init__.py`
- [x] 2.2 Create `backend/services/documents/folder_service.py` — list_by_project, create, delete
- [x] 2.3 Create `backend/services/documents/document_service.py` — list_by_project, list_by_folder, upload, get_preview_url, move, update_remark, delete; include ALLOWED_MIME_TYPES and MAX_FILE_SIZE_BYTES constants
- [x] 2.4 Create `backend/api/routes/folders.py` — POST /, DELETE /{id}, GET /{id}/documents
- [x] 2.5 Rewrite `backend/api/routes/documents.py` — POST /upload, GET /{id}/preview, PUT /{id}/move, PUT /{id}/remark, DELETE /{id}
- [x] 2.6 Update `backend/api/routes/projects.py` — add GET /{projectId}/documents and GET /{projectId}/folders
- [x] 2.7 Update `backend/api/__init__.py` — register folders router
- [x] 2.8 Verify all endpoints return 200 (not 501)

## Phase 3: Frontend Dependencies

- [x] 3.1 Install `@tanstack/react-table`: `pnpm add @tanstack/react-table`
- [x] 3.2 Install `react-dropzone`: `pnpm add react-dropzone`
- [x] 3.3 Install `docx-preview`: `pnpm add docx-preview`
- [x] 3.4 Install shadcn components: `npx shadcn@latest add checkbox select`

## Phase 4: Frontend Layout and Data Table

- [x] 4.1 Create `frontend/src/views/uploading/constants.ts` — ACCEPTED_FILE_TYPES, MAX_FILE_SIZE_BYTES, PANEL_SIZES, MOBILE_BREAKPOINT
- [x] 4.2 Create `frontend/src/views/uploading/UploadingPage.tsx` — 两栏布局（表格 + 预览面板），使用 react-resizable-panels，移动端检测
- [x] 4.3 Create `frontend/src/views/uploading/columns.tsx` — @tanstack/react-table 列定义：select checkbox, name, size, type, remark, date, actions
- [x] 4.4 Create `frontend/src/views/uploading/DataTable.tsx` — shadcn + @tanstack/react-table 数据表格，含排序、筛选、分页、行选择
- [x] 4.5 Create `frontend/src/views/uploading/FilterBar.tsx` — 顶部筛选：文件类型标签（全部/PDF/DOC/TXT）、文件夹下拉选择器、搜索框
- [x] 4.6 Create `frontend/src/views/uploading/BatchActionsBar.tsx` — 选中文件时显示的浮动批量操作栏（移动、备注、删除）
- [x] 4.7 Update `frontend/src/app/router.tsx` — replace Uploading placeholder with UploadingPage
- [x] 4.8 Verify: navigate to `/project/{id}/uploading`, see data table with real data, sorting, filtering

## Phase 5: Frontend Upload, Dialogs, and Preview

- [x] 5.1 Create `frontend/src/views/uploading/UploadDropzone.tsx` — react-dropzone + shadcn Card 自定义 UI，文件类型验证，进度条
- [x] 5.2 Create `frontend/src/views/uploading/DocumentPreviewPanel.tsx` — react-pdf for PDF, text viewer for TXT, metadata for unsupported, zoom controls, empty state
- [x] 5.3 Create `frontend/src/views/uploading/MoveDocumentDialog.tsx` — folder picker dialog (shadcn Select)
- [x] 5.4 Create `frontend/src/views/uploading/EditRemarkDialog.tsx` — textarea dialog
- [x] 5.5 Create `frontend/src/views/uploading/CreateFolderDialog.tsx` — name input dialog
- [x] 5.6 Verify: full upload flow (drag → progress → table → click → preview)

## Phase 6: Testing and Polish

- [x] 6.1 Backend model/schema tests: `backend/tests/test_document_models.py`
- [x] 6.2 Backend folder service tests: `backend/tests/test_folder_service.py`
- [x] 6.3 Backend document service tests: `backend/tests/test_document_service.py`
- [x] 6.4 Backend route integration tests: `backend/tests/test_document_routes.py`
- [x] 6.5 Run `cd frontend && pnpm typecheck && pnpm lint`
- [x] 6.6 Run `cd backend && ruff check .`
- [x] 6.7 Manual E2E: `make dev` → upload → filter → select → batch delete → preview
- [x] 6.8 Mobile responsive test: < 1024px, preview uses Sheet, table full width
