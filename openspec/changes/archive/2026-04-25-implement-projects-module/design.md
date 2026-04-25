## Context

当前 PaperSage 项目 MVP 阶段：
- 前端：auth 已完成，projects/trash/workspace 等 feature 都是空壳
- 后端：account.py 已完成，projects.py/documents.py 都是占位返回 501
- 用户注册登录后无法创建/查看项目，MVP 闭环不完整

需求规格已在 `docs/project/02-requirements.md` 和 `docs/project/06-database.md` 定义。

## Goals / Non-Goals

**Goals:**
- 后端实现 Projects CRUD API（创建、列表、详情、更新、删除）
- 后端实现 Groups 分组管理 API
- 后端实现 Trash 回收站 API
- 前端实现 Projects 列表页 + 创建项目功能
- 前端实现 Trash 回收站页面

**Non-Goals:**
- 不实现 Workspace 完整布局（后续变更）
- 不实现文档上传/编码标注（后续变更）
- 不实现 AI 功能（后续变更）
- 不实现分类/标签功能（后续变更）

## Decisions

### 1. 后端架构：Service + Repository 模式

**选择：** 使用 `api/routes → services → repositories` 分层

**理由：** 
- 符合后端现有 account.py 模式（直接在 route 中调用 db）
- MVP 阶段保持简单，复杂度可接受
- 后续可重构为 service layer

**备选：** 纯 repository 模式 — MVP 不需要复杂业务逻辑

### 2. 数据模型：复用已有设计

**选择：** 使用 `docs/project/06-database.md` 中的 projects、project_groups 表结构

**理由：**
- 已有完整设计，无需重新设计
- 与前端 schema 对应

### 3. 前端状态管理：TanStack Query

**选择：** 使用 TanStack Query 管理 projects API 状态

**理由：**
- 项目已有 TanStack Query 配置
- 自动缓存、轮询、错误处理适合列表页

### 4. 前端组件：复用现有 shadcn/ui

**选择：** 使用 Card、Dialog、Dropdown、Input 等现有组件

**理由：**
- 项目已配置 shadcn/ui
- 减少样式工作量

## Risks / Trade-offs

- [风险] 后端数据库未初始化 →  Mitigation: 先创建 migration，用户本地运行
- [风险] 前端路由需要调整 →  Mitigation: 已有 /app/documents 路由，只需完善组件
- [风险] 软删除后回收站清理逻辑 →  MVP 先实现手动删除，auto-clean 后续

## Migration Plan

1. 创建数据库 migration（projects、project_groups 表）
2. 实现后端 Projects API
3. 实现后端 Groups API
4. 实现后端 Trash API
5. 前端创建 Projects feature 组件
6. 前端创建 Trash feature 组件
7. 测试前后端联调

## Open Questions

- 是否需要项目搜索功能？（需求规格中 PROJ-007 是 P1）
- 分组是否支持多层级？（当前设计是单层）