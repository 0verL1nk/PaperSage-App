## 1. 项目初始化

- [x] 1.1 创建 backend/ 目录结构 (app/, api/, core/, models/, schemas/, services/, tests/, alembic/)
- [x] 1.2 初始化 pyproject.toml，配置 uv 依赖
- [x] 1.3 配置 ruff.toml 代码规范
- [x] 1.4 配置 mypy.ini 类型检查 (改为 ty)
- [x] 1.5 创建 .env.example 环境变量示例

## 2. 配置管理

- [x] 2.1 创建 app/config.py - Pydantic Settings 配置类
- [x] 2.2 创建 app/__init__.py - 应用元信息
- [x] 2.3 创建 app/exceptions.py - 全局异常处理
- [x] 2.4 创建 app/main.py - FastAPI 应用入口，配置 CORS、中间件

## 3. 数据库抽象层

- [x] 3.1 创建 core/database.py - SQLAlchemy 异步引擎和 Session
- [x] 3.2 创建 core/base.py - 基础模型类 (含 id, created_at, updated_at)
- [x] 3.3 创建 core/__init__.py - 导出数据库工具

## 4. Alembic 迁移

- [x] 4.1 初始化 alembic: `alembic init alembic`
- [x] 4.2 配置 alembic.ini 指向数据库
- [x] 4.3 配置 alembic/env.py 使用 async engine
- [ ] 4.4 创建初始迁移脚本

## 5. Redis 缓存

- [x] 5.1 创建 core/redis.py - Redis 连接工具
- [x] 5.2 实现基础缓存操作 (get, set, delete)
- [x] 5.3 添加 Redis 健康检查

## 6. 安全工具

- [x] 6.1 创建 core/security.py - 密码哈希 (bcrypt)
- [x] 6.2 创建 core/token.py - JWT Token 生成和验证
- [x] 6.3 创建 core/__init__.py - 导出安全工具

## 7. API 路由框架

- [x] 7.1 创建 api/__init__.py - API 路由入口
- [x] 7.2 创建 api/dependencies.py - 依赖注入 (get_db, get_redis)
- [x] 7.3 创建 api/response.py - 统一响应格式
- [x] 7.4 创建 api/routes/__init__.py - 路由模块
- [x] 7.5 创建 api/routes/account.py - 账户路由框架 (预留)

## 8. 验证测试

- [ ] 8.1 运行 `uv run uvicorn app.main:app` 验证启动
- [ ] 8.2 访问 /docs 验证 API 文档可用
- [ ] 8.3 访问 /health 验证健康检查
- [ ] 8.4 运行 `ruff check .` 验证代码规范
- [ ] 8.5 运行 `ty check app/` 验证类型检查