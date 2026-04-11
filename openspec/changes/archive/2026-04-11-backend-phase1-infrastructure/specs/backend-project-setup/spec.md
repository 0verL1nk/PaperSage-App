## ADDED Requirements

### Requirement: FastAPI 项目结构
项目 SHALL 使用标准的分层目录结构，包含 api/routes、core、models、schemas、services 目录。

#### Scenario: 项目结构存在
- **WHEN** 开发者查看 backend/ 目录
- **THEN** 存在 app/、api/、core/、models/、schemas/、services/、tests/、alembic/ 目录

### Requirement: 包管理
应用 SHALL 使用 uv 作为 Python 包管理工具。

#### Scenario: uv 安装依赖
- **WHEN** 运行 `uv pip install -e ".[dev]"`
- **THEN** 安装所有项目依赖

#### Scenario: uv 运行脚本
- **WHEN** 运行 `uv run uvicorn app.main:app`
- **THEN** 在虚拟环境中启动应用

### Requirement: 配置管理
应用 SHALL 使用 Pydantic Settings 管理配置，支持从环境变量读取。

#### Scenario: 环境变量加载
- **WHEN** 应用启动时
- **THEN** 从环境变量加载 DATABASE_URL、SECRET_KEY、REDIS_URL 等配置

#### Scenario: .env 文件支持
- **WHEN** 存在 .env 文件
- **THEN** 应用自动加载 .env 中的配置

### Requirement: 入口文件
应用 SHALL 提供 main.py 作为 FastAPI 入口。

#### Scenario: 应用启动
- **WHEN** 运行 `uvicorn app.main:app`
- **THEN** FastAPI 应用成功启动，可以访问 /docs 文档

### Requirement: API 路由注册
应用 SHALL 在 api/routes 目录注册 API 路由。

#### Scenario: 路由可用
- **WHEN** 访问 /api/account/login
- **THEN** 返回 404 (路由未实现) 而不是 404 (路由不存在)