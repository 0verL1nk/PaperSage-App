# 后端系统设计

> 历史文档：下文描述早期 Python/FastAPI 方案，不代表当前 Go 后端。2026-09-16 的代码基础与后续写作流程设计分别见 [项目总览](../PROJECT.md) 和 [调研驱动方案](../project/12-ai-writing-design.md)。请勿据此另起一套后端或将示例接口视为已实现。

## 1. 系统架构

### 1.1 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                         Client (Frontend)                        │
└─────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│                        Nginx (Reverse Proxy)                     │
│                   (HTTPS Termination + Static)                   │
└─────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│                    FastAPI + Uvicorn (ASGI)                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │   Routes    │  │   Services  │  │  Middleware │              │
│  │  (API End)  │  │  (Business) │  │  (Auth, CORS│              │
│  └─────────────┘  └─────────────┘  └─────────────┘              │
└─────────────────────────────────────────────────────────────────┘
                                  │
          ┌───────────────────────┼───────────────────────┐
          │                       │                       │
          ▼                       ▼                       ▼
┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐
│  PostgreSQL     │   │      Redis      │   │   Cloudflare R2  │
│  (Primary DB)   │   │  (Cache/Session)│   │   (S3 Object)  │
└─────────────────┘   └─────────────────┘   └─────────────────┘
```

### 1.2 部署架构

- **开发环境**: Uvicorn (单进程)
- **生产环境**: Gunicorn + Uvicorn Worker (多进程)
- **容器化**: Docker + Docker Compose

---

## 2. 技术栈

### 2.1 核心框架

| 组件 | 选型 | 理由 |
|------|------|------|
| Web 框架 | FastAPI | 高性能、自动生成 API 文档、原生支持类型提示 |
| ASGI 服务器 | Uvicorn | 极速、ASGI 标准实现 |
| 进程管理 | Gunicorn | 生产环境稳定、多 worker 支持 |
| 配置管理 | Pydantic Settings | 类型安全、环境变量驱动 |

### 2.2 数据持久化

| 组件 | 选型 | 理由 |
|------|------|------|
| ORM | SQLAlchemy 2.0 (Async) | 生态成熟、异步支持、类型安全 |
| 数据库 | PostgreSQL (默认) | 功能最强大的开源 RDBMS |
| 迁移工具 | Alembic | SQLAlchemy 官方推荐 |
| 驱动 | asyncpg | 高性能异步 PostgreSQL 驱动 |

### 2.3 缓存与队列

| 组件 | 选型 | 理由 |
|------|------|------|
| 缓存 | Redis | Session、热点数据、限流 |
| 任务队列 | ARQ | 轻量级、原生 asyncio 支持 |

### 2.4 安全

| 组件 | 选型 | 理由 |
|------|------|------|
| 认证 | OAuth2 + JWT | 行业标准、无状态 |
| 密码哈希 | Passlib + bcrypt | 安全、社区验证 |
| 令牌库 | PyJWT | 稳定、功能完整 |

### 2.5 开发工具

| 组件 | 选型 | 理由 |
|------|------|------|
| 代码规范 | Ruff | 极速、集成 format/lint |
| 类型检查 | Mypy | 类型安全 |
| 测试 | Pytest + HTTPX | 异步测试支持 |

---

## 3. 项目结构

```
backend/
├── app/
│   ├── __init__.py
│   ├── main.py              # 应用入口
│   ├── config.py            # 配置管理
│   └── exceptions.py        # 全局异常
├── api/
│   ├── __init__.py
│   ├── routes/              # API 路由
│   │   ├── __init__.py
│   │   ├── account.py       # 认证相关
│   │   ├── documents.py     # 文档管理
│   │   └── projects.py      # 项目管理
│   ├── dependencies.py      # 依赖注入
│   └── response.py          # 响应格式化
├── core/
│   ├── __init__.py
│   ├── security.py          # 安全工具
│   ├── token.py             # JWT 工具
│   └── database.py          # 数据库连接
├── models/                  # SQLAlchemy 模型
│   ├── __init__.py
│   ├── user.py
│   ├── document.py
│   └── project.py
├── schemas/                 # Pydantic schemas
│   ├── __init__.py
│   ├── user.py
│   ├── document.py
│   └── project.py
├── services/                # 业务逻辑
│   ├── __init__.py
│   ├── auth.py
│   ├── document.py
│   └── project.py
├── utils/                   # 工具函数
│   └── __init__.py
├── tests/                   # 测试
│   ├── __init__.py
│   ├── conftest.py
│   └── api/
├── alembic/                 # 数据库迁移
│   ├── env.py
│   └── versions/
├── .env.example             # 环境变量示例
├── pyproject.toml           # 项目配置
├── ruff.toml                # Linter 配置
├── Dockerfile               # Docker 镜像
├── docker-compose.yml       # 本地开发
└── README.md                # 项目说明
```

---

## 4. API 设计规范

### 4.1 响应格式

所有 API 响应使用统一的信封格式：

```typescript
interface ApiEnvelope<T> {
  code: number      // 状态码 (200 = 成功)
  data: T           // 响应数据
  info: string      // 附加信息 (可选)
}
```

示例响应：

```json
// 成功
{
  "code": 200,
  "data": {
    "id": "1",
    "name": "Test"
  },
  "info": ""
}

// 错误
{
  "code": 400,
  "data": null,
  "info": "Invalid request parameters"
}
```

### 4.2 HTTP 方法规范

| 方法 | 用途 | 幂等 |
|------|------|------|
| GET | 获取资源 | 是 |
| POST | 创建资源 | 否 |
| PUT | 完整更新资源 | 是 |
| PATCH | 部分更新资源 | 否 |
| DELETE | 删除资源 | 是 |

### 4.3 状态码

| 状态码 | 含义 |
|--------|------|
| 200 | 成功 |
| 201 | 创建成功 |
| 400 | 请求参数错误 |
| 401 | 未认证 |
| 403 | 无权限 |
| 404 | 资源不存在 |
| 422 | 业务逻辑错误 |
| 500 | 服务器内部错误 |

### 4.4 认证方式

使用 Bearer Token 认证：

```
Authorization: Bearer <access_token>
```

### 4.5 分页

列表接口支持分页：

```
GET /api/documents?page=1&limit=20
```

响应包含元数据：

```json
{
  "code": 200,
  "data": [...],
  "info": ""
}
```

---

## 5. 数据库抽象设计

### 5.1 目标

通过统一的配置和抽象层，支持 MySQL、SQLite、PostgreSQL 三种数据库，无需修改业务代码。

### 5.2 架构

```
┌─────────────────────────────────────────────┐
│              Application Code                │
│     (Services, Models, API Endpoints)        │
└─────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────┐
│           SQLAlchemy Async Engine            │
│         (统一的异步 ORM 接口)                  │
└─────────────────────────────────────────────┘
                     │
        ┌────────────┼────────────┐
        ▼            ▼            ▼
┌────────────┐ ┌──────────┐ ┌───────────┐
│ asyncpg    │ │ aiosqlite│ │ aiomysql  │
│ (PostgreSQL)│ │ (SQLite) │ │ (MySQL)   │
└────────────┘ └──────────┘ └───────────┘
```

### 5.3 连接字符串格式

通过 `DATABASE_URL` 环境变量配置：

| 数据库 | URL 格式 | 驱动 |
|--------|----------|------|
| PostgreSQL | `postgresql+asyncpg://user:pass@host/db` | asyncpg |
| MySQL | `mysql+aiomysql://user:pass@host/db` | aiomysql |
| SQLite | `sqlite+aiosqlite:///path/to/db.sqlite` | aiosqlite |

### 5.4 实现

```python
# core/database.py
from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from sqlalchemy.orm import sessionmaker

# 根据 DATABASE_URL 自动选择驱动
engine = create_async_engine(
    settings.DATABASE_URL,
    echo=settings.DEBUG,
    pool_pre_ping=True,
    pool_size=10,
    max_overflow=20,
)

async_session = sessionmaker(
    engine, class_=AsyncSession, expire_on_commit=False
)
```

### 5.5 依赖安装

```toml
# pyproject.toml
dependencies = [
    "sqlalchemy[asyncio]>=2.0",
    "asyncpg>=0.29",
    "aiomysql>=0.2",
    "aiosqlite>=0.19",
    "alembic>=1.13",
]
```

### 5.6 注意事项

1. **事务一致性**: 跨数据库事务不被支持
2. **特性差异**:
   - SQLite: 不支持复杂并发写入
   - MySQL: 注意 charset 和 collation
   - PostgreSQL: 推荐使用 JSONB 类型
3. **连接池**: 生产环境务必配置连接池大小

---

## 6. 配置管理

### 6.1 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `DATABASE_URL` | 数据库连接串 | `postgresql+asyncpg://user:pass@localhost/db` |
| `REDIS_URL` | Redis 连接串 | `redis://localhost:6379/0` |
| `SECRET_KEY` | JWT 密钥 | (必须设置) |
| `ACCESS_TOKEN_EXPIRE_MINUTES` | Access Token 过期时间 | 30 |
| `REFRESH_TOKEN_EXPIRE_DAYS` | Refresh Token 过期时间 | 7 |
| `DEBUG` | 调试模式 | `false` |
| `ALLOWED_HOSTS` | CORS 允许的域名 | `*` |

### 6.2 配置代码

```python
# app/config.py
from pydantic_settings import BaseSettings, SettingsConfigDict

class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        case_sensitive=False,
    )

    database_url: str = "postgresql+asyncpg://user:pass@localhost/db"
    redis_url: str = "redis://localhost:6379/0"
    secret_key: str = ""

    access_token_expire_minutes: int = 30
    refresh_token_expire_days: int = 7

    debug: bool = False
    allowed_hosts: list[str] = ["*"]

settings = Settings()
```

---

## 7. 错误处理

### 7.1 全局异常

```python
# app/exceptions.py
from fastapi import HTTPException, status

class AppException(HTTPException):
    def __init__(self, code: int, detail: str):
        super().__init__(status_code=code, detail=detail)
```

### 7.2 错误响应格式

```json
{
  "code": 400,
  "data": null,
  "info": "User not found"
}
```

---

## 8. 日志规范

### 8.1 日志级别

| 级别 | 用途 |
|------|------|
| DEBUG | 调试信息 |
| INFO | 正常流程 |
| WARNING | 警告（非错误） |
| ERROR | 错误（需要关注） |
| CRITICAL | 严重错误 |

### 8.2 日志格式

```
[%(asctime)s] %(levelname)s [%(name)s:%(lineno)d] %(message)s
```

示例：
```
[2024-01-15 10:30:45] INFO [api.routes.account:23] User login successful
```

---

## 9. 对象存储 (Cloudflare R2)

### 9.1 概述

项目使用 Cloudflare R2 作为对象存储服务，用于存储用户上传的文档、PDF 等文件。

**为什么选择 R2：**
- **S3 兼容**: 可使用任何 S3 SDK/库
- **无 egress 费用**: 流出流量免费（相比 AWS S3 节省成本）
- **免费额度**: 每月 1GB 存储 + 1GB 流出

### 9.2 S3 兼容性

R2 提供 S3-compatible API，支持以下操作：

| 操作 | 支持 |
|------|------|
| PutObject (上传) | ✅ |
| GetObject (下载) | ✅ |
| DeleteObject (删除) | ✅ |
| ListObjects (列表) | ✅ |
| 预签名 URL | ✅ |

### 9.3 配置

环境变量配置：

| 变量 | 说明 | 必填 |
|------|------|------|
| `R2_ENDPOINT` | R2 endpoint URL | 是 |
| `R2_ACCESS_KEY_ID` | Access Key ID | 是 |
| `R2_SECRET_ACCESS_KEY` | Secret Access Key | 是 |
| `R2_BUCKET_NAME` | Bucket 名称 | 是 |
| `R2_PUBLIC_URL` | 公开访问 URL（可选） | 否 |

### 9.4 Python boto3 配置示例

```python
import boto3
from botocore.config import Config

# R2 配置
r2_config = Config(
    region_name='auto',
    signature_version='s3v4'
)

s3_client = boto3.client(
    's3',
    endpoint_url='https://<account-id>.r2.cloudflarestorage.com',
    aws_access_key_id='<your-access-key-id>',
    aws_secret_access_key='<your-secret-key>',
    config=r2_config
)
```

### 9.5 使用示例

```python
# 上传文件
s3_client.upload_file('local.pdf', 'my-bucket', 'papers/doc.pdf')

# 下载文件
s3_client.download_file('my-bucket', 'papers/doc.pdf', 'local.pdf')

# 生成预签名 URL (供前端直接上传/下载)
url = s3_client.generate_presigned_url(
    'put_object',
    Params={'Bucket': 'my-bucket', 'Key': 'papers/doc.pdf'},
    ExpiresIn=3600
)
```

### 9.6 R2 API Token 创建步骤

1. 登录 [Cloudflare Dashboard](https://dash.cloudflare.com)
2. 进入 **R2** → **Manage API Tokens**
3. 点击 **Create API Token**
4. 配置权限：
   - **Permissions**: Admin Read & Write
   - **Specify buckets**: 选择对应 bucket（可选）
5. 复制生成的 `Access Key ID` 和 `Secret Access Key`

### 9.7 CORS 配置

如果前端需要直接从浏览器上传到 R2，需要在 Bucket 设置中配置 CORS：

```json
[
  {
    "AllowedOrigins": ["http://localhost:5173"],
    "AllowedMethods": ["GET", "PUT", "DELETE"],
    "AllowedHeaders": ["*"]
  }
]
```
