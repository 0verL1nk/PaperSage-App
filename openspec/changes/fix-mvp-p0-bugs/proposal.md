## Why

当前 MVP 存在 3 个 P0 级阻断性问题，导致用户登录后无法进入项目工作区，且 /auth 路由 redirect 指向未完成的 UnderConstruction 页面。这些是纯代码缺陷修复，不需要新的需求规格。

## What Changes

1. **ProjectCard onClick 导航修复**
   - `frontend/src/views/projects/ProjectListPage.tsx` 中 ProjectCard 的 onClick 是空 stub
   - 修复为 `navigate({ to: '/project/$projectId', params: { projectId } })`

2. **/auth 已登录用户 redirect 目标修复**
   - `frontend/src/app/router.tsx:74` 中 `authRoute.beforeLoad` redirect 到 `/app/documents`
   - `/app/documents` 是 UnderConstruction 占位页，用户体验极差
   - 修复为 redirect 到 `/app/projects`

3. **logout endpoint 路径一致性**
   - 登录用 `/api/account/login`，登出用 `/api/v1/auth/logout`，路径前缀不一致
   - CORS 是全局中间件（allow_methods=["*"]），实际无 CORS 问题
   - 但为代码一致性，建议统一为 `/api/v1/auth/*`

4. **WorkspaceLayout 路由参数获取方式**
   - `frontend/src/views/layout/WorkspaceLayout.tsx` 使用 `window.location.pathname` 解析 projectId
   - 改为 TanStack Router 的 `useParams()` hook

5. **侧边栏 + 按钮功能**
   - `MainLayout.tsx` 中侧边栏的 + 按钮无 onClick
   - 应打开 CreateProjectDialog（需实现 Dialog 状态共享或提升到 Layout 层）

6. **CreateProjectDialog double preventDefault bug**
   - `frontend/src/components/domain/CreateProjectDialog.tsx:29` 有连续两个 `e.preventDefault()`
   - 删除重复调用

7. **删除冗余 account.py + 统一 Auth API v1 前缀**
   - `backend/api/routes/account.py` 与 `auth.py` 功能完全重复
   - 将 account.py 中的 login/register 合并到 auth.py，路径统一为 `/api/v1/auth/login` 和 `/api/v1/auth/register`
   - 前端 `auth-api.ts` 对应改为 `/api/v1/auth/login` 和 `/api/v1/auth/register`
   - 前端 `auth-api.test.ts` 测试断言中的 URL 同步更新

8. **verify_password 静默吞异常**
   - `backend/core/security.py` 中任何验证异常都静默返回 False
   - 违反"禁止静默吞掉异常"原则，应记录日志

9. **deleted_at 字段类型错误**
   - `backend/models/project.py` 中 `deleted_at` 是 `String(50)` 而非 `DateTime`
   - 需 Alembic 数据库迁移修复

## Capabilities

### New Capabilities

- `mvp-ux-fixes`: 修复 MVP 阶段遗留的 UX 断点问题，包含导航、一致性、错误处理修复

### Modified Capabilities

- 无需求规格变更（均为实现层 bug 修复）

## Impact

- **前端**: `router.tsx`, `ProjectListPage.tsx`, `WorkspaceLayout.tsx`, `MainLayout.tsx`, `CreateProjectDialog.tsx`, `auth-api.ts`, `auth-api.test.ts`
- **后端**: `auth.py`, `account.py`（合并）, `security.py`, `models/project.py`
- **数据库**: `alembic` migration 修复 `deleted_at` 类型
- **测试**: 需补充 ProjectCard 导航的单元测试，更新 auth 路径相关测试
