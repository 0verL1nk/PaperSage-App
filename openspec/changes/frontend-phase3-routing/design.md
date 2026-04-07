## Context

Phase 1 已完成依赖安装和目录骨架，Phase 2 已完成 API 客户端和全局状态管理。当前 React 应用仅有基础 Provider 层，缺少路由系统和页面布局。用户无法通过 URL 导航到具体功能模块。

## Goals / Non-Goals

**Goals:**
- 建立基于 `@tanstack/react-router` 的路由系统
- 实现根路由配置和全局布局
- 实现主站布局（顶部导航 + 侧边栏 + 内容区）
- 实现项目工作区布局（顶部 tab 导航）
- 实现路由守卫，未登录用户重定向到 `/auth`
- 创建占位页面以支持路由可达性验证

**Non-Goals:**
- Auth 模块的业务逻辑（登录/注册功能）- 属于 Phase 4
- 项目列表、回收站等业务页面 - 属于 Phase 5
- 上传、Coding 等功能页面 - 属于 Phase 6-7

## Decisions

### 1. 路由结构
采用扁平化 `/app` 前缀结构：
- `/auth` - 认证页面（登录/注册）
- `/app/documents` - 文档列表
- `/app/trash` - 回收站
- `/app/profile` - 用户设置
- `/project/:projectId` - 项目工作区根
- `/project/:projectId/uploading` - 上传模块
- `/project/:projectId/coding` - Coding 模块
- `/project/:projectId/classifying` - 分类模块
- `/project/:projectId/logic-chain` - 逻辑链模块
- `/project/:projectId/generating` - 生成模块

**Alternative considered**: 保持原 Vue 的 `/main` 前缀
**Decision**: 使用 `/app` 前缀，更简洁且便于后续扩展

### 2. 路由守卫实现
- 使用 `@tanstack/react-router` 的 `beforeLoad` 钩子实现路由守卫
- 从 session-store 读取登录状态
- 未登录时调用 `router意Navigate({ to: '/auth' })`

### 3. 布局组件位置
- 主站布局: `src/features/layout/MainLayout.tsx`
- 工作区布局: `src/features/layout/WorkspaceLayout.tsx`
- 根路由: `src/routes/__root.tsx`

### 4. 占位页面策略
为未实现的业务页面创建基础占位组件，确保路由可达：
- `/app/profile` 占位
- `/project/:projectId/classifying` 占位
- `/project/:projectId/logic-chain` 占位
- `/project/:projectId/generating` 占位

## Risks / Trade-offs

- [Risk] 路由守卫与 session-store 的时序问题
  - **Mitigation**: 在 App 初始化时优先恢复 session，确保守卫检查时状态已加载

- [Risk] 工作区布局需要 projectId 参数
  - **Mitigation**: 使用路由参数 `$:projectId` 捕获，通过 useParams 获取