## Why

用户注册登录后，第一个交互页面是项目列表。但当前前后端 Projects 模块都是空壳/占位，无法完成完整的 MVP 闭环。其他功能（文档上传、编码标注、AI 辅助）都依赖于 Projects 模块作为入口。

## What Changes

1. 后端实现 Projects CRUD API
   - Projects 模型定义（SQLAlchemy）
   - Groups 分组 API
   - 软删除 + 回收站基础

2. 后端实现 Trash API
   - 回收站列表
   - 恢复/永久删除

3. 前端实现 Projects Feature
   - 项目列表页（ProjectListPage）
   - 创建项目对话框（CreateProjectDialog）
   - 项目卡片组件
   - 项目分组筛选

4. 前端实现 Trash Feature
   - 回收站页面
   - 恢复/删除操作

## Capabilities

### New Capabilities

- `projects-api`: 后端 Projects CRUD API，包括项目创建、列表、详情、更新、删除
- `groups-api`: 后端项目分组管理 API
- `trash-api`: 后端回收站 API
- `projects-frontend`: 前端项目模块 UI
- `trash-frontend`: 前端回收站 UI

### Modified Capabilities

- 无（现有需求规格已在 docs/project/02-requirements.md 中定义）

## Impact

- **后端**：`backend/api/routes/projects.py` 从占位变为完整实现
- **前端**：`frontend/src/features/projects/` 从空壳变为完整实现
- **数据库**：新增 projects、project_groups 表（已有 alembic）