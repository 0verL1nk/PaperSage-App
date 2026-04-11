## Context

前端已完成基础路由和 API 调用封装，使用 TanStack Router + Axios。前端 API 调用约定：
- 响应格式：`{ code: number, data: T, info: string }`
- 认证方式：Bearer Token (Access Token + Refresh Token)
- 登录接口：`/api/account/login`, `/api/account/register` (multipart form)

当前需要后端基础设施来支撑这些 API 调用。

## Goals / Non-Goals

**Goals:**
- 创建 FastAPI 项目结构，包含配置、异常、路由目录
- 实现数据库抽象层，使用 SQLAlchemy 2.0 异步 ORM
- 支持多数据库切换：PostgreSQL (默认)、MySQL、SQLite
- 配置 Alembic 迁移工具
- 实现 Redis 连接和基础缓存工具
- 配置 CORS、异常处理、日志等中间件

**Non-Goals:**
- 用户认证功能 (Phase 2)
- 文档/项目管理功能 (Phase 3-4)
- 部署配置 (Docker, CI/CD)

## Decisions

### 1. 使用 SQLAlchemy 2.0 异步 ORM
**选择理由**:
- 官方支持异步，生态成熟
- 类型安全，配合 Pydantic
- 兼容多种数据库驱动

**替代方案**: 使用 ORM-less 方案如 Drizzle ORM
- Drizzle 更轻量，但生态较小
- SQLAlchemy 2.0 异步已足够轻量

### 2. 使用 Pydantic Settings 管理配置
**选择理由**:
- 类型安全
- 环境变量优先
- 支持 `.env` 文件

### 3. 使用 asyncpg 作为 PostgreSQL 驱动
**选择理由**:
- 性能最高
- 原生异步支持

**替代方案**: psycopg (同步版本)
- 性能较低，不符合异步架构

### 4. 项目结构按功能分层
**选择理由**:
- api/routes: API 端点
- core: 核心工具 (security, token, database)
- models: SQLAlchemy 模型
- schemas: Pydantic schemas
- services: 业务逻辑

**替代方案**: 按资源类型分层 (users/, documents/)
- 不适合多模块场景，功能分层更清晰

### 5. 不使用 FastAPI-Users 库
**选择理由**:
- 自定义程度高
- 学习内部实现

**替代方案**: FastAPI-Users
- 开箱即用但定制困难

## Risks / Trade-offs

- **[风险] 数据库兼容性问题** → 使用 SQLAlchemy 抽象，测试各数据库
- **[风险] 异步代码复杂度** → 遵循 FastAPI 异步最佳实践
- **[风险] 初始开发速度** → 基础设施代码有一定工作量，但后续迭代快

## Migration Plan

1. 创建 `backend/` 目录结构
2. 配置 `pyproject.toml` 依赖
3. 实现配置和入口文件
4. 实现数据库抽象层
5. 配置 Alembic
6. 实现 Redis 连接
7. 验证启动成功

**回滚**: 删除 `backend/` 目录即可，无数据迁移