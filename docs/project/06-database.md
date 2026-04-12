# 数据库设计

## 1. ER 图

```mermaid
erDiagram
    USER {
        string id PK
        string username UK
        string email UK
        string hashed_password
        boolean is_active
        datetime created_at
        datetime updated_at
    }

    PROJECT_GROUP {
        string id PK
        string name
        string owner_id FK
        int sort_order
        datetime created_at
    }

    PROJECT {
        string id PK
        string name
        string description
        string owner_id FK
        string group_id FK
        string status
        boolean is_deleted
        datetime deleted_at
        datetime created_at
        datetime updated_at
    }

    FOLDER {
        string id PK
        string project_id FK
        string parent_id FK
        string name
        datetime created_at
        datetime updated_at
    }

    DOCUMENT {
        string id PK
        string project_id FK
        string folder_id FK
        string name
        string type
        bigint size
        string storage_key
        string remark
        string status
        datetime created_at
        datetime updated_at
    }

    HIGHLIGHT {
        string id PK
        string document_id FK
        string user_id FK
        int page
        string content
        string color
        string note
        json position
        datetime created_at
    }

    TAG {
        string id PK
        string name
        string user_id FK
        datetime created_at
    }

    DOCUMENT_TAG {
        string document_id FK
        string tag_id FK
        PK
    }

    TEX_DOCUMENT {
        string id PK
        string project_id FK
        string name
        text content
        string template
        string status
        string compiled_pdf_key
        string error_message
        int version
        datetime created_at
        datetime updated_at
    }

    TEX_VERSION {
        string id PK
        string tex_document_id FK
        int version
        text content
        string compiled_pdf_key
        datetime created_at
    }

    USER ||--o{ PROJECT : creates
    USER ||--o{ PROJECT_GROUP : owns
    USER ||--o{ TAG : creates
    USER ||--o{ HIGHLIGHT : creates

    PROJECT_GROUP ||--o{ PROJECT : contains

    PROJECT ||--o{ FOLDER : contains
    PROJECT ||--o{ DOCUMENT : contains
    PROJECT ||--o{ TEX_DOCUMENT : contains

    FOLDER ||--o{ FOLDER : contains
    FOLDER ||--o{ DOCUMENT : contains

    DOCUMENT ||--o{ HIGHLIGHT : has

    TAG }o--o{ DOCUMENT : tags

    TEX_DOCUMENT ||--o{ TEX_VERSION : has
```

## 2. 表结构详情

### 2.1 users - 用户表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | VARCHAR(36) | PK | UUID 主键 |
| username | VARCHAR(20) | UK, NOT NULL | 用户名，3-20字符 |
| email | VARCHAR(255) | UK | 邮箱 |
| hashed_password | VARCHAR(255) | NOT NULL | bcrypt 加密的密码 |
| is_active | BOOLEAN | DEFAULT true | 账户是否激活 |
| created_at | TIMESTAMP | DEFAULT now() | 创建时间 |
| updated_at | TIMESTAMP | DEFAULT now() | 更新时间 |

```sql
CREATE TABLE users (
    id VARCHAR(36) PRIMARY KEY,
    username VARCHAR(20) NOT NULL UNIQUE,
    email VARCHAR(255) UNIQUE,
    hashed_password VARCHAR(255) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_email ON users(email);
```

### 2.2 project_groups - 项目分组表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | VARCHAR(36) | PK | UUID 主键 |
| name | VARCHAR(50) | NOT NULL | 分组名称 |
| owner_id | VARCHAR(36) | FK -> users.id | 所属用户 |
| sort_order | INTEGER | DEFAULT 0 | 排序顺序 |
| created_at | TIMESTAMP | DEFAULT now() | 创建时间 |

```sql
CREATE TABLE project_groups (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(50) NOT NULL,
    owner_id VARCHAR(36) NOT NULL REFERENCES users(id),
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_project_groups_owner ON project_groups(owner_id);
```

### 2.3 projects - 项目表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | VARCHAR(36) | PK | UUID 主键 |
| name | VARCHAR(100) | NOT NULL | 项目名称 |
| description | TEXT | | 项目描述 |
| owner_id | VARCHAR(36) | FK -> users.id | 所属用户 |
| group_id | VARCHAR(36) | FK -> project_groups.id | 所属分组 |
| status | VARCHAR(20) | DEFAULT 'active' | 状态: active, archived |
| is_deleted | BOOLEAN | DEFAULT false | 软删除标志 |
| deleted_at | TIMESTAMP | | 删除时间 |
| created_at | TIMESTAMP | DEFAULT now() | 创建时间 |
| updated_at | TIMESTAMP | DEFAULT now() | 更新时间 |

```sql
CREATE TABLE projects (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    owner_id VARCHAR(36) NOT NULL REFERENCES users(id),
    group_id VARCHAR(36) REFERENCES project_groups(id),
    status VARCHAR(20) DEFAULT 'active',
    is_deleted BOOLEAN DEFAULT FALSE,
    deleted_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_projects_owner ON projects(owner_id);
CREATE INDEX idx_projects_group ON projects(group_id);
CREATE INDEX idx_projects_deleted ON projects(is_deleted, deleted_at);
```

### 2.4 folders - 文件夹表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | VARCHAR(36) | PK | UUID 主键 |
| project_id | VARCHAR(36) | FK -> projects.id | 所属项目 |
| parent_id | VARCHAR(36) | FK -> folders.id | 父文件夹（支持无限层级） |
| name | VARCHAR(100) | NOT NULL | 文件夹名称 |
| created_at | TIMESTAMP | DEFAULT now() | 创建时间 |
| updated_at | TIMESTAMP | DEFAULT now() | 更新时间 |

```sql
CREATE TABLE folders (
    id VARCHAR(36) PRIMARY KEY,
    project_id VARCHAR(36) NOT NULL REFERENCES projects(id),
    parent_id VARCHAR(36) REFERENCES folders(id),
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_folders_project ON folders(project_id);
CREATE INDEX idx_folders_parent ON folders(parent_id);
```

### 2.5 documents - 文档表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | VARCHAR(36) | PK | UUID 主键 |
| project_id | VARCHAR(36) | FK -> projects.id | 所属项目 |
| folder_id | VARCHAR(36) | FK -> folders.id | 所属文件夹 |
| name | VARCHAR(255) | NOT NULL | 文件名 |
| type | VARCHAR(20) | NOT NULL | 文件类型: pdf, word, other |
| size | BIGINT | NOT NULL | 文件大小（字节） |
| storage_key | VARCHAR(500) | NOT NULL | MinIO 存储路径 |
| remark | TEXT | | 文件备注 |
| status | VARCHAR(20) | DEFAULT 'ready' | 处理状态: processing, ready, error |
| is_deleted | BOOLEAN | DEFAULT false | 软删除标志 |
| deleted_at | TIMESTAMP | | 删除时间 |
| created_at | TIMESTAMP | DEFAULT now() | 创建时间 |
| updated_at | TIMESTAMP | DEFAULT now() | 更新时间 |

```sql
CREATE TABLE documents (
    id VARCHAR(36) PRIMARY KEY,
    project_id VARCHAR(36) NOT NULL REFERENCES projects(id),
    folder_id VARCHAR(36) REFERENCES folders(id),
    name VARCHAR(255) NOT NULL,
    type VARCHAR(20) NOT NULL,
    size BIGINT NOT NULL,
    storage_key VARCHAR(500) NOT NULL,
    remark TEXT,
    status VARCHAR(20) DEFAULT 'ready',
    is_deleted BOOLEAN DEFAULT FALSE,
    deleted_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_documents_project ON documents(project_id);
CREATE INDEX idx_documents_folder ON documents(folder_id);
CREATE INDEX idx_documents_status ON documents(status);
CREATE INDEX idx_documents_deleted ON documents(is_deleted, deleted_at);
```

### 2.6 highlights - 高亮表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | VARCHAR(36) | PK | UUID 主键 |
| document_id | VARCHAR(36) | FK -> documents.id | 所属文档 |
| user_id | VARCHAR(36) | FK -> users.id | 创建用户 |
| page | INTEGER | NOT NULL | 所在页码 |
| content | TEXT | NOT NULL | 高亮内容 |
| color | VARCHAR(20) | NOT NULL | 颜色: yellow, green, blue, pink, orange |
| note | TEXT | | 批注内容 |
| position | JSON | NOT NULL | 位置坐标 {x, y, width, height} |
| created_at | TIMESTAMP | DEFAULT now() | 创建时间 |

```sql
CREATE TABLE highlights (
    id VARCHAR(36) PRIMARY KEY,
    document_id VARCHAR(36) NOT NULL REFERENCES documents(id),
    user_id VARCHAR(36) NOT NULL REFERENCES users(id),
    page INTEGER NOT NULL,
    content TEXT NOT NULL,
    color VARCHAR(20) NOT NULL,
    note TEXT,
    position JSONB NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_highlights_document ON highlights(document_id);
CREATE INDEX idx_highlights_user ON highlights(user_id);
CREATE INDEX idx_highlights_page ON highlights(page);
```

### 2.7 tags - 标签表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | VARCHAR(36) | PK | UUID 主键 |
| name | VARCHAR(50) | NOT NULL | 标签名称 |
| user_id | VARCHAR(36) | FK -> users.id | 所属用户 |
| created_at | TIMESTAMP | DEFAULT now() | 创建时间 |

```sql
CREATE TABLE tags (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(50) NOT NULL,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tags_user ON tags(user_id);
```

### 2.8 document_tags - 文档标签关联表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| document_id | VARCHAR(36) | FK -> documents.id | 文档 ID |
| tag_id | VARCHAR(36) | FK -> tags.id | 标签 ID |

```sql
CREATE TABLE document_tags (
    document_id VARCHAR(36) NOT NULL REFERENCES documents(id),
    tag_id VARCHAR(36) NOT NULL REFERENCES tags(id),
    PRIMARY KEY (document_id, tag_id)
);

CREATE INDEX idx_document_tags_tag ON document_tags(tag_id);
```

### 2.9 tex_documents - LaTeX 文档表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | VARCHAR(36) | PK | UUID 主键 |
| project_id | VARCHAR(36) | FK -> projects.id | 所属项目 |
| name | VARCHAR(100) | NOT NULL | 文档名称 |
| content | TEXT | NOT NULL | LaTeX 源码 |
| template | VARCHAR(20) | | 模板类型: thesis, paper, custom |
| status | VARCHAR(20) | DEFAULT 'draft' | 编译状态: draft, compiling, success, error |
| compiled_pdf_key | VARCHAR(500) | | MinIO 存储的 PDF 路径 |
| error_message | TEXT | | 编译错误信息 |
| version | INTEGER | DEFAULT 1 | 版本号 |
| created_at | TIMESTAMP | DEFAULT now() | 创建时间 |
| updated_at | TIMESTAMP | DEFAULT now() | 更新时间 |

```sql
CREATE TABLE tex_documents (
    id VARCHAR(36) PRIMARY KEY,
    project_id VARCHAR(36) NOT NULL REFERENCES projects(id),
    name VARCHAR(100) NOT NULL,
    content TEXT NOT NULL,
    template VARCHAR(20),
    status VARCHAR(20) DEFAULT 'draft',
    compiled_pdf_key VARCHAR(500),
    error_message TEXT,
    version INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tex_documents_project ON tex_documents(project_id);
CREATE INDEX idx_tex_documents_status ON tex_documents(status);
```

### 2.10 tex_versions - LaTeX 文档版本历史表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | VARCHAR(36) | PK | UUID 主键 |
| tex_document_id | VARCHAR(36) | FK -> tex_documents.id | 所属文档 |
| version | INTEGER | NOT NULL | 版本号 |
| content | TEXT | NOT NULL | 源码内容 |
| compiled_pdf_key | VARCHAR(500) | | PDF 存储路径 |
| created_at | TIMESTAMP | DEFAULT now() | 创建时间 |

```sql
CREATE TABLE tex_versions (
    id VARCHAR(36) PRIMARY KEY,
    tex_document_id VARCHAR(36) NOT NULL REFERENCES tex_documents(id),
    version INTEGER NOT NULL,
    content TEXT NOT NULL,
    compiled_pdf_key VARCHAR(500),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tex_versions_document ON tex_versions(tex_document_id);
```

## 3. 数据库配置

### 3.1 支持的数据库

| 数据库 | 连接串格式 | 驱动 |
|--------|-----------|------|
| PostgreSQL | `postgresql+asyncpg://user:pass@host/db` | asyncpg |
| MySQL | `mysql+aiomysql://user:pass@host/db` | aiomysql |
| SQLite | `sqlite+aiosqlite:///path/to/db.sqlite` | aiosqlite |

### 3.2 连接池配置

```python
# core/database.py

from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from sqlalchemy.orm import sessionmaker
from app.config import settings

engine = create_async_engine(
    settings.database_url,
    echo=settings.debug,
    pool_pre_ping=True,      # 连接前检查
    pool_size=10,            # 基础连接数
    max_overflow=20,         # 最大额外连接
    pool_recycle=3600,       # 连接回收时间
)

async_session = sessionmaker(
    engine,
    class_=AsyncSession,
    expire_on_commit=False,
)


async def get_db() -> AsyncSession:
    async with async_session() as session:
        try:
            yield session
        finally:
            await session.close()
```

## 4. 索引优化

### 4.1 常用查询索引

| 查询场景 | 索引 |
|----------|------|
| 按用户查找项目 | `idx_projects_owner` |
| 按分组筛选项目 | `idx_projects_group` |
| 查找项目下的文档 | `idx_documents_project` |
| 查找文件夹下的文档 | `idx_documents_folder` |
| 查找文档的高亮 | `idx_highlights_document` |
| 用户搜索项目名 | `idx_projects_name` (全文索引) |

### 4.2 分区策略（可选）

对于大数据量文档表，可考虑按项目ID分区：

```sql
CREATE TABLE documents (
    -- ...
) PARTITION BY LIST (left(project_id, 1));

-- 按 project_id 首字母分区
CREATE TABLE documents_0 PARTITION OF documents FOR VALUES IN ('0');
CREATE TABLE documents_a PARTITION OF documents FOR VALUES IN ('a', 'b', 'c');
```

## 5. 数据迁移

### 5.1 Alembic 配置

```python
# alembic/env.py

from sqlalchemy import pool
from sqlalchemy.ext.asyncio import async_engine_from_config
from alembic import context
from app.models import Base
from app.config import settings

config = context.config
config.set_main_option("sqlalchemy.url", settings.database_url)

target_metadata = Base.metadata


def run_migrations_offline() -> None:
    url = config.get_main_option("sqlalchemy.url")
    context.configure(
        url=url,
        target_metadata=target_metadata,
        literal_binds=True,
        dialect_opts={"paramstyle": "named"},
    )
    with context.begin_transaction():
        context.run_migrations()


def run_migrations_online() -> None:
    connectable = async_engine_from_config(
        config.get_section(config.config_ini_section),
        prefix="sqlalchemy.",
        poolclass=pool.NullPool,
    )
    with connectable.connect() as connection:
        context.configure(connection=connection, target_metadata=target_metadata)
        with context.begin_transaction():
            context.run_migrations()
```

### 5.2 迁移命令

```bash
# 创建迁移
alembic revision --autogenerate -m "add documents table"

# 执行迁移
alembic upgrade head

# 回滚
alembic downgrade -1
```

---

*本章节详细描述了 PaperSage 系统的数据库设计，包括 ER 图、表结构、索引和迁移策略。*