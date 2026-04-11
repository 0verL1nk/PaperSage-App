## Why

前端已经完成了基础路由和 API 调用封装，需要后端支持。前端使用 FastAPI，需要实现基础设施来支撑认证、文档管理、项目管理等核心功能。当前没有后端服务，前端无法真正运行。

## What Changes

- 创建 FastAPI 后端项目结构
- 实现配置管理和环境变量支持
- 实现数据库抽象层，支持 PostgreSQL/MySQL/SQLite 切换
- 配置 Alembic 数据库迁移
- 实现 Redis 缓存连接
- 配置 CORS、异常处理、日志等中间件

## Capabilities

### New Capabilities
- **backend-project-setup**: FastAPI 项目初始化，包含目录结构、配置管理
- **database-abstraction**: SQLAlchemy 2.0 异步 ORM 抽象层，支持多数据库
- **alembic-migration**: Alembic 迁移工具配置和使用
- **redis-cache**: Redis 连接和缓存工具
- **backend-security**: JWT 认证、密码哈希基础

### Modified Capabilities
- (无)

## Impact

- 新增 `backend/` 目录，包含完整的 FastAPI 应用
- 新增 `alembic/` 目录用于数据库迁移
- 依赖 Python 3.12、FastAPI、SQLAlchemy、PostgreSQL、Redis