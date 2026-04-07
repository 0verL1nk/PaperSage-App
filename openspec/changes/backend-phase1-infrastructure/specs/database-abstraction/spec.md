## ADDED Requirements

### Requirement: 数据库连接池
应用 SHALL 使用 SQLAlchemy 异步引擎创建数据库连接池。

#### Scenario: 连接池配置
- **WHEN** 创建数据库引擎
- **THEN** 配置 pool_size=20, max_overflow=30, pool_pre_ping=True

### Requirement: 多数据库支持
应用 SHALL 支持 PostgreSQL、MySQL、SQLite 三种数据库。

#### Scenario: PostgreSQL 连接
- **WHEN** DATABASE_URL 为 postgresql+asyncpg://...
- **THEN** 使用 asyncpg 驱动连接

#### Scenario: MySQL 连接
- **WHEN** DATABASE_URL 为 mysql+aiomysql://...
- **THEN** 使用 aiomysql 驱动连接

#### Scenario: SQLite 连接
- **WHEN** DATABASE_URL 为 sqlite+aiosqlite://...
- **THEN** 使用 aiosqlite 驱动连接

### Requirement: 异步 Session
应用 SHALL 提供异步数据库 Session 用于 CRUD 操作。

#### Scenario: Session 注入
- **WHEN** API 路由需要数据库操作
- **THEN** 通过 Depends 注入获取 async session

### Requirement: 基础模型类
应用 SHALL 提供基础模型类，包含 id、created_at、updated_at 字段。

#### Scenario: 模型继承
- **WHEN** 创建新的数据模型
- **THEN** 模型继承自基础模型类，自动包含通用字段