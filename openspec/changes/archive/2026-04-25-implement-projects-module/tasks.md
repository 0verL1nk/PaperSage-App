## 1. 数据库迁移

- [x] 1.1 创建 projects 表 migration
- [x] 1.2 创建 project_groups 表 migration
- [x] 1.3 运行 migration 验证表结构

## 2. 后端 Projects API

- [x] 2.1 创建 Project/ProjectGroup SQLAlchemy 模型
- [x] 2.2 实现 GET /api/v1/projects（列表）
- [x] 2.3 实现 POST /api/v1/projects（创建）
- [x] 2.4 实现 GET /api/v1/projects/{id}（详情）
- [x] 2.5 实现 PUT /api/v1/projects/{id}（更新）
- [x] 2.6 实现 DELETE /api/v1/projects/{id}（软删除）

## 3. 后端 Groups API

- [x] 3.1 实现 GET /api/v1/groups（列表）
- [x] 3.2 实现 POST /api/v1/groups（创建）
- [x] 3.3 实现 PUT /api/v1/groups/{id}（更新）
- [x] 3.4 实现 DELETE /api/v1/groups/{id}（删除）

## 4. 后端 Trash API

- [x] 4.1 实现 GET /api/v1/trash（回收站列表）
- [x] 4.2 实现 POST /api/v1/trash/{type}/{id}/restore（恢复）
- [x] 4.3 实现 DELETE /api/v1/trash/{type}/{id}（永久删除）

## 5. 前端 Projects Feature

- [x] 5.1 创建 projects API hooks（useProjects, useProject等）
- [x] 5.1.1 创建 types.ts 类型定义
- [x] 5.1.2 创建 api.test.tsx 测试 (19 tests passed)
- [x] 5.2 实现 ProjectListPage 组件
- [x] 5.3 实现 ProjectCard 组件
- [x] 5.4 实现 CreateProjectDialog 组件
- [x] 5.5 实现项目分组筛选 UI

## 6. 前端 Trash Feature

- [x] 6.1 创建 trash API hooks (在 api.ts 中)
- [x] 6.2 实现 TrashPage 组件
- [x] 6.3 实现恢复/删除操作

## 7. 测试与联调

- [x] 7.1 后端单元测试 - 10 tests passed
- [x] 7.2 前端单元测试 - 43 tests passed
- [x] 7.3 前后端联调 - 12 API routes ✓