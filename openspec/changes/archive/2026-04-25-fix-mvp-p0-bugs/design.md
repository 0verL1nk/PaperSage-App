## Context

PaperSage MVP 存在多个 P0 级 UX 断点，在前后端探索阶段已全部识别。这些是纯 bug 修复，不需要架构变更或新功能。

**当前状态**:
- 用户登录后看到项目列表，但无法点击进入任何项目（ProjectCard onClick 是空 stub）
- 已登录用户访问 /auth 被重定向到 UnderConstruction 的 /app/documents 页面
- logout 请求路径与其他 auth 端点不一致，存在 CORS 风险
- WorkspaceLayout 用 `window.location` 而非框架的 `useParams`
- 侧边栏 + 按钮无功能
- 后端有重复代码和违反错误处理原则的实现

## Goals / Non-Goals

**Goals:**
- 修复 3 个 P0 阻断性问题（ProjectCard 导航、/auth redirect、logout CORS）
- 修复 6 个 P1/P2 体验问题
- 清理后端冗余代码（account.py 合并）
- 修复数据库字段类型错误（deleted_at）

**Non-Goals:**
- 不实现新的业务功能
- 不改变 API 契约或数据模型
- 不做性能优化（除了错误修复附带的质量提升）
- 不添加新的测试覆盖（仅修复对应 bug 的测试）

## Decisions

### 1. ProjectCard 导航修复

**选项**: navigate() vs Link 组件

**决策**: 使用 `navigate()` 函数
**理由**: ProjectListPage 已有 TanStack Router 的 `useNavigate` hook，ProjectCard 作为点击处理器而非 `<a>` 标签，用 imperative navigate 更自然。

```tsx
// ProjectListPage.tsx - ProjectCard onClick
onClick={() => navigate({ to: '/project/$projectId', params: { projectId: project.id } })}
```

### 2. /auth redirect 修复

**选项**: 改 redirect 目标 vs 删除 redirect

**决策**: 改 redirect 目标为 `/app/projects`
**理由**: `/app/documents` 是 Phase 3 内容，MVP 阶段 projects 才是用户登录后的正确着陆页。

```tsx
// router.tsx:74 - authRoute.beforeLoad
if (hasSessionToken()) {
  throw redirect({ to: '/app/projects' }) // 改前: '/app/documents'
}
```

### 3. Auth API 统一（删除 account.py，统一 v1 前缀）

**当前状态**:
- `account.py`: `/api/account/login`, `/api/account/register`
- `auth.py`: `/api/v1/auth/logout`, `/api/v1/auth/refresh`

**决策**: 删除 `account.py`，将 login/register 合并到 `auth.py` 下的 `/api/v1/auth/` 路径
**理由**: account.py 与 auth.py 完全重复，统一到 `/api/v1/auth/*` 符合 API versioning 规范。

**前端改动**: `auth-api.ts` 中 login/register 调用路径同步改为 `/api/v1/auth/` 前缀。

### 4. WorkspaceLayout useParams

**决策**: 改用 `useParams` hook
**理由**: TanStack Router 提供了类型安全的 `useParams`，避免字符串解析错误。

```tsx
// WorkspaceLayout.tsx
import { useParams } from '@tanstack/react-router'
const { projectId } = useParams({ from: '/project/$projectId' })
```

### 5. 侧边栏 + 按钮

**选项**: Dialog 提升到 MainLayout vs 状态提升 vs 使用全局状态

**决策**: 将 CreateProjectDialog 提升到 MainLayout 层
**理由**: MainLayout 是所有 app 子路由的父级，Dialog 提升到这里后 ProjectListPage 只需触发一个 state change 而不需要传递 Dialog 组件。

### 6. account.py 合并

**决策**: 删除 `account.py`，功能已由 `auth.py` 覆盖
**理由**: 两者完全重复，保留增加维护成本和困惑。

### 7. verify_password 异常处理

**决策**: 捕获异常，记录 debug 日志，返回 False
**理由**: 密码验证失败原因（数据库错误、密码被篡改等）不应透露给攻击者，但内部应留有日志以便排查。

### 8. deleted_at 类型修复

**决策**: Alembic migration 将 `deleted_at` 从 String 改为 DateTime
**理由**: DateTime 才能做范围查询和排序，String 存储是历史遗留错误。

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| logout CORS 问题实际不存在 | 先验证，不盲目改 |
| 修改 router redirect 影响其他 flow | 只改 authRoute 的 beforeLoad，隔离影响 |
| Alembic migration 在生产环境失败 | 先在 dev 验证，确认字段内容可转换 |

## Open Questions

1. `/app/documents` 未来实现后，redirect 逻辑是否需要再次调整？
2. 侧边栏 + 按钮是否需要快捷分组管理，还是仅创建项目就够了？
