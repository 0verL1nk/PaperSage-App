## 1. P0 Bug Fixes (MVP Blocking)

- [x] 1.1 Fix ProjectCard onClick in ProjectListPage.tsx - add navigate to /project/$projectId
- [x] 1.2 Fix /auth redirect in router.tsx - change from /app/documents to /app/projects

## 2. Auth API 统一 (删除 account.py，统一 v1 前缀)

- [x] 2.1 Merge account.py login/register into auth.py under /api/v1/auth/login and /api/v1/auth/register
- [x] 2.2 Delete backend/api/routes/account.py (fully redundant)
- [x] 2.3 Update frontend auth-api.ts: change login path from /api/account/login to /api/v1/auth/login
- [x] 2.4 Update frontend auth-api.ts: change register path from /api/account/register to /api/v1/auth/register
- [x] 2.5 Update frontend auth-api.test.ts: update expected URLs in test assertions

## 3. P1 Bug Fixes (UX Improvements)

- [x] 3.1 Fix WorkspaceLayout.tsx - replace window.location.pathname with useParams hook
- [x] 3.2 Add CreateProjectDialog to MainLayout sidebar - + button opens dialog
- [x] 3.3 Fix CreateProjectDialog.tsx:29 - remove duplicate e.preventDefault() call

## 4. P2 Bug Fixes (Code Quality)

- [x] 4.1 Fix verify_password in security.py - log exception before returning False (don't silently swallow)
- [x] 4.2 Create Alembic migration - change deleted_at from String(50) to DateTime in project model

## 5. Testing

- [x] 5.1 Add unit test for ProjectCard navigation (mock navigate, verify called with correct params)
- [x] 5.2 Update auth tests for new /api/v1/auth paths
