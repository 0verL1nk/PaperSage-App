# 后端实现计划

## 概述

本计划定义后端系统的实现阶段，按依赖顺序分为 4 个 Phase。

**前提条件**: 前端已完成基础路由和 API 调用封装。

---

## Phase 1: 基础设施

**目标**: 搭建开发环境、配置管理、数据库抽象层

### 1.1 项目初始化

- [ ] 创建 `backend/` 目录结构
- [ ] 配置 `pyproject.toml` 依赖
- [ ] 配置 Ruff 代码规范
- [ ] 配置 Mypy 类型检查
- [ ] 配置 GitHub Actions CI

### 1.2 配置与基础设施

- [ ] 实现 `app/config.py` - Pydantic Settings 配置
- [ ] 实现 `app/exceptions.py` - 全局异常处理
- [ ] 实现 `app/main.py` - FastAPI 应用入口

### 1.3 数据库抽象层

- [ ] 实现 `core/database.py` - SQLAlchemy 异步引擎
- [ ] 实现 `core/base.py` - 基础模型类
- [ ] 配置 Alembic 迁移工具
- [ ] 创建初始迁移脚本
- [ ] 编写数据库抽象文档 (README)

### 1.4 缓存与消息队列

- [ ] 实现 `core/redis.py` - Redis 连接
- [ ] 配置 ARQ 任务队列 (可选)

### 1.5 验收标准

- [ ] `uvicorn app.main:app` 可正常启动
- [ ] 访问 `/docs` 显示 API 文档
- [ ] 可通过 `DATABASE_URL` 切换数据库
- [ ] Alembic 迁移正常工作

---

## Phase 2: 认证模块

**目标**: 实现用户注册、登录、Token 刷新

### 2.1 数据模型

- [ ] 实现 `models/user.py` - 用户模型
  - id (UUID)
  - username (唯一索引)
  - password_hash
  - created_at
  - updated_at

### 2.2 Schemas

- [ ] 实现 `schemas/user.py`
  - UserCreate (注册)
  - UserLogin (登录)
  - UserResponse (响应)
  - TokenResponse (Token 响应)

### 2.3 安全工具

- [ ] 实现 `core/security.py` - 密码哈希工具
- [ ] 实现 `core/token.py` - JWT Token 生成与验证

### 2.4 业务逻辑

- [ ] 实现 `services/auth.py`
  - 用户注册
  - 用户登录 (验证密码)
  - Token 刷新

### 2.5 API 路由

- [ ] 实现 `api/routes/account.py`
  - `POST /api/account/register` - 注册
  - `POST /api/account/login` - 登录
  - `POST /api/account/refresh` - 刷新 Token
  - `POST /api/account/logout` - 登出 (可选)
  - `GET /api/account/me` - 获取当前用户

### 2.6 验收标准

- [ ] 注册返回 201，密码使用 bcrypt 存储
- [ ] 登录返回 Access Token + Refresh Token
- [ ] Token 刷新正常工作
- [ ] `/api/account/me` 返回当前用户信息

---

## Phase 3: 文档管理

**目标**: 实现文档的 CRUD 操作

### 3.1 数据模型

- [ ] 实现 `models/document.py` - 文档模型
  - id (UUID)
  - title
  - content (TEXT/JSON)
  - owner_id (外键 → User)
  - created_at
  - updated_at
  - deleted_at (软删除)

### 3.2 Schemas

- [ ] 实现 `schemas/document.py`
  - DocumentCreate
  - DocumentUpdate
  - DocumentResponse

### 3.3 业务逻辑

- [ ] 实现 `services/document.py`
  - 创建文档
  - 获取文档列表 (分页)
  - 获取文档详情
  - 更新文档
  - 软删除文档
  - 回收站 (已删除文档)

### 3.4 API 路由

- [ ] 实现 `api/routes/documents.py`
  - `GET /api/documents` - 获取文档列表
  - `POST /api/documents` - 创建文档
  - `GET /api/documents/{id}` - 获取文档详情
  - `PUT /api/documents/{id}` - 更新文档
  - `DELETE /api/documents/{id}` - 删除文档
  - `GET /api/documents/trash` - 回收站

### 3.5 权限控制

- [ ] 添加用户只能访问自己文档的过滤
- [ ] 实现软删除 (不影响其他用户)

### 3.6 验收标准

- [ ] 文档 CRUD 正常工作
- [ ] 分页功能正常
- [ ] 用户只能看到自己的文档
- [ ] 软删除与回收站功能正常

---

## Phase 4: 项目管理

**目标**: 实现项目的 CRUD 及成员管理

### 4.1 数据模型

- [ ] 实现 `models/project.py` - 项目模型
  - id (UUID)
  - name
  - description
  - owner_id (外键 → User)
  - created_at
  - updated_at
- [ ] 实现 `models/project_member.py` - 项目成员模型
  - id (UUID)
  - project_id (外键 → Project)
  - user_id (外键 → User)
  - role (owner/member)
  - created_at

### 4.2 Schemas

- [ ] 实现 `schemas/project.py`
  - ProjectCreate
  - ProjectUpdate
  - ProjectResponse
  - ProjectMemberAdd

### 4.3 业务逻辑

- [ ] 实现 `services/project.py`
  - 创建项目
  - 获取项目列表
  - 获取项目详情
  - 更新项目
  - 删除项目
  - 添加/移除成员
  - 获取成员列表

### 4.4 API 路由

- [ ] 实现 `api/routes/projects.py`
  - `GET /api/projects` - 获取项目列表
  - `POST /api/projects` - 创建项目
  - `GET /api/projects/{id}` - 获取项目详情
  - `PUT /api/projects/{id}` - 更新项目
  - `DELETE /api/projects/{id}` - 删除项目
  - `GET /api/projects/{id}/members` - 获取成员列表
  - `POST /api/projects/{id}/members` - 添加成员
  - `DELETE /api/projects/{id}/members/{user_id}` - 移除成员

### 4.5 验收标准

- [ ] 项目 CRUD 正常工作
- [ ] 成员管理功能正常
- [ ] 所有者可以管理项目
- [ ] 成员可以访问项目

---

## 后续功能 (未包含在当前计划)

以下功能待前端实现后逐步添加：

- `/project/:projectId/uploading` - 文件上传
- `/project/:projectId/coding` - 编码处理
- `/project/:projectId/classifying` - 分类
- `/project/:projectId/logic-chain` - 逻辑链
- `/project/:projectId/generating` - 生成

---

## 依赖关系图

```
Phase 1 (基础设施)
    │
    ├── Phase 2 (认证模块)
    │       │
    │       ├── Phase 3 (文档管理)
    │       │
    │       └── Phase 4 (项目管理)
    │
    └── (其他功能模块)
```

---

## 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 数据库兼容性问题 | 中 | 使用 SQLAlchemy 抽象，测试各数据库 |
| Token 安全问题 | 高 | 使用 PyJWT，密钥安全存储 |
| 并发性能 | 中 | 合理配置连接池 |
| 软删除数据膨胀 | 低 | 定期清理回收站 |