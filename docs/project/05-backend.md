# 后端设计规格

## 1. 后端架构

### 1.1 技术选型理由

| 组件 | 选型 | 理由 |
|------|------|------|
| Web 框架 | FastAPI | 高性能、自动生成 API 文档、原生支持类型提示 |
| ASGI 服务器 | Uvicorn | 极速、ASGI 标准实现 |
| 进程管理 | Gunicorn | 生产环境稳定、多 worker 支持 |
| 配置管理 | Pydantic Settings | 类型安全、环境变量驱动 |

### 1.2 核心依赖

```toml
# pyproject.toml

[project]
name = "papersage-backend"
version = "0.1.0"
requires-python = ">=3.12"

dependencies = [
    "fastapi>=0.115.0",
    "uvicorn[standard]>=0.32.0",
    "gunicorn>=23.0.0",

    # 数据库
    "sqlalchemy[asyncio]>=2.0.0",
    "asyncpg>=0.30.0",
    "alembic>=1.15.0",

    # 缓存
    "redis>=5.0.0",

    # 任务队列
    "arq>=0.24.0",

    # 认证
    "pyjwt>=2.8.0",
    "passlib[bcrypt]>=1.7.4",
    "python-multipart>=0.0.9",

    # 验证
    "pydantic>=2.0.0",
    "pydantic-settings>=2.0.0",

    # 文件存储
    "aiobotocore>=2.0.0",
    "python-magic>=0.4.27",

    # 工具
    "httpx>=0.27.0",
    "python-dotenv>=1.0.0",

    # 日志
    "structlog>=24.0.0",
]

[project.optional-dependencies]
dev = [
    "pytest>=8.0.0",
    "pytest-asyncio>=0.23.0",
    "httpx>=0.27.0",
    "ruff>=0.6.0",
    "mypy>=1.0.0",
]
```

## 2. 项目结构

### 2.1 目录结构

```text
backend/
├── app/
│   ├── __init__.py
│   ├── main.py              # 应用入口
│   ├── config.py            # 配置管理
│   ├── exceptions.py        # 全局异常
│   └── constants.py         # 常量定义
│
├── api/
│   ├── __init__.py
│   ├── routes/
│   │   ├── __init__.py
│   │   ├── auth.py          # 认证接口
│   │   ├── projects.py      # 项目接口
│   │   ├── documents.py     # 文档接口
│   │   ├── folders.py       # 文件夹接口
│   │   ├── highlights.py    # 高亮接口
│   │   └── ai.py            # AI 接口
│   │
│   ├── dependencies.py      # 依赖注入
│   ├── response.py          # 响应格式化
│   └── middleware.py        # 中间件
│
├── core/
│   ├── __init__.py
│   ├── security.py          # 安全工具
│   ├── token.py             # JWT 工具
│   ├── database.py          # 数据库连接
│   ├── storage.py           # 文件存储
│   └── cache.py             # 缓存
│
├── models/                  # SQLAlchemy 模型
│   ├── __init__.py
│   ├── user.py
│   ├── project.py
│   ├── folder.py
│   ├── document.py
│   ├── highlight.py
│   └── tag.py
│
├── schemas/                 # Pydantic Schema
│   ├── __init__.py
│   ├── user.py
│   ├── project.py
│   ├── document.py
│   ├── folder.py
│   ├── highlight.py
│   └── common.py
│
├── services/                # 业务逻辑
│   ├── __init__.py
│   ├── auth.py
│   ├── project.py
│   ├── document.py
│   ├── folder.py
│   ├── highlight.py
│   └── ai.py
│
├── tasks/                   # 后台任务
│   ├── __init__.py
│   └── file_processor.py
│
├── utils/                   # 工具函数
│   ├── __init__.py
│   ├── hash.py
│   └── validators.py
│
├── tests/                   # 测试
│   ├── __init__.py
│   ├── conftest.py
│   └── api/
│
├── alembic/                 # 数据库迁移
│   ├── env.py
│   └── versions/
│
├── .env.example
├── pyproject.toml
├── ruff.toml
├── Dockerfile
└── docker-compose.yml
```

## 3. 配置管理

### 3.1 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `DATABASE_URL` | 数据库连接串 | `postgresql+asyncpg://user:pass@localhost/papersage` |
| `REDIS_URL` | Redis 连接串 | `redis://localhost:6379/0` |
| `SECRET_KEY` | JWT 密钥 | (必须设置) |
| `ACCESS_TOKEN_EXPIRE_MINUTES` | Access Token 过期时间 | 30 |
| `REFRESH_TOKEN_EXPIRE_DAYS` | Refresh Token 过期时间 | 7 |
| `MINIO_ENDPOINT` | MinIO 端点 | localhost:9000 |
| `MINIO_ACCESS_KEY` | MinIO Access Key | minioadmin |
| `MINIO_SECRET_KEY` | MinIO Secret Key | minioadmin |
| `MINIO_BUCKET` | MinIO 存储桶 | papersage |
| `OPENAI_API_KEY` | OpenAI API Key | - |
| `DEBUG` | 调试模式 | false |

### 3.2 配置代码

```python
# app/config.py

from pydantic_settings import BaseSettings, SettingsConfigDict
from pydantic import Field


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        case_sensitive=False,
    )

    # Database
    database_url: str = Field(
        default="postgresql+asyncpg://user:pass@localhost/papersage",
        description="Database connection URL"
    )

    # Redis
    redis_url: str = Field(
        default="redis://localhost:6379/0",
        description="Redis connection URL"
    )

    # Security
    secret_key: str = Field(..., description="JWT secret key")
    access_token_expire_minutes: int = 30
    refresh_token_expire_days: int = 7

    # Storage
    minio_endpoint: str = "localhost:9000"
    minio_access_key: str = "minioadmin"
    minio_secret_key: str = "minioadmin"
    minio_bucket: str = "papersage"
    minio_secure: bool = False

    # AI
    openai_api_key: str | None = None
    anthropic_api_key: str | None = None

    # App
    debug: bool = False
    allowed_hosts: list[str] = ["*"]


settings = Settings()
```

## 4. 数据库模型

### 4.1 用户模型

```python
# models/user.py

from sqlalchemy import String, DateTime, Boolean
from sqlalchemy.orm import Mapped, mapped_column, relationship
from sqlalchemy.sql import func
from app.database import Base


class User(Base):
    __tablename__ = "users"

    id: Mapped[str] = mapped_column(String(36), primary_key=True)
    username: Mapped[str] = mapped_column(String(20), unique=True, index=True)
    email: Mapped[str | None] = mapped_column(String(255), unique=True, index=True)
    hashed_password: Mapped[str] = mapped_column(String(255))
    is_active: Mapped[bool] = mapped_column(Boolean, default=True)
    created_at: Mapped[DateTime] = mapped_column(DateTime, server_default=func.now())
    updated_at: Mapped[DateTime] = mapped_column(DateTime, server_default=func.now(), onupdate=func.now())

    # Relationships
    projects: Mapped[list["Project"]] = relationship(back_populates="owner")
    project_groups: Mapped[list["ProjectGroup"]] = relationship(back_populates="owner")
    highlights: Mapped[list["Highlight"]] = relationship(back_populates="user")
```

### 4.2 项目模型

```python
# models/project.py

from sqlalchemy import String, DateTime, ForeignKey, Text
from sqlalchemy.orm import Mapped, mapped_column, relationship
from sqlalchemy.sql import func
from app.database import Base


class Project(Base):
    __tablename__ = "projects"

    id: Mapped[str] = mapped_column(String(36), primary_key=True)
    name: Mapped[str] = mapped_column(String(100), index=True)
    description: Mapped[str | None] = mapped_column(Text)
    owner_id: Mapped[str] = mapped_column(ForeignKey("users.id"))
    group_id: Mapped[str | None] = mapped_column(ForeignKey("project_groups.id"))
    status: Mapped[str] = mapped_column(String(20), default="active")  # active, archived
    is_deleted: Mapped[bool] = mapped_column(default=False)
    deleted_at: Mapped[DateTime | None] = mapped_column(DateTime)
    created_at: Mapped[DateTime] = mapped_column(DateTime, server_default=func.now())
    updated_at: Mapped[DateTime] = mapped_column(DateTime, server_default=func.now(), onupdate=func.now())

    # Relationships
    owner: Mapped["User"] = relationship(back_populates="projects")
    group: Mapped["ProjectGroup"] = relationship(back_populates="projects")
    folders: Mapped[list["Folder"]] = relationship(back_populates="project")
    documents: Mapped[list["Document"]] = relationship(back_populates="project")


class ProjectGroup(Base):
    __tablename__ = "project_groups"

    id: Mapped[str] = mapped_column(String(36), primary_key=True)
    name: Mapped[str] = mapped_column(String(50))
    owner_id: Mapped[str] = mapped_column(ForeignKey("users.id"))
    sort_order: Mapped[int] = mapped_column(default=0)
    created_at: Mapped[DateTime] = mapped_column(DateTime, server_default=func.now())

    # Relationships
    owner: Mapped["User"] = relationship(back_populates="project_groups")
    projects: Mapped[list["Project"]] = relationship(back_populates="group")
```

## 5. API 接口

### 5.1 认证接口

```python
# api/routes/auth.py

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.ext.asyncio import AsyncSession
from app.api.dependencies import get_db
from app.schemas.auth import *
from app.services.auth import AuthService

router = APIRouter(prefix="/auth", tags=["认证"])


@router.post("/register", response_model=ApiResponse[UserResponse])
async def register(
    data: RegisterRequest,
    db: AsyncSession = Depends(get_db)
):
    """用户注册"""
    service = AuthService(db)
    user = await service.register(data.username, data.password, data.email)
    return ApiResponse(data=user)


@router.post("/login", response_model=ApiResponse[LoginResponse])
async def login(
    data: LoginRequest,
    db: AsyncSession = Depends(get_db)
):
    """用户登录"""
    service = AuthService(db)
    result = await service.login(data.username, data.password)
    if not result:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="用户名或密码错误")
    return ApiResponse(data=result)


@router.post("/refresh", response_model=ApiResponse[LoginResponse])
async def refresh_token(
    data: RefreshTokenRequest,
    db: AsyncSession = Depends(get_db)
):
    """刷新 Token"""
    service = AuthService(db)
    result = await service.refresh_token(data.refresh_token)
    if not result:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Token 已失效")
    return ApiResponse(data=result)


@router.post("/logout")
async def logout():
    """退出登录"""
    return ApiResponse(data=None)
```

### 5.2 项目接口

```python
# api/routes/projects.py

from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.ext.asyncio import AsyncSession
from app.api.dependencies import get_db, get_current_user
from app.schemas.project import *
from app.services.project import ProjectService

router = APIRouter(prefix="/projects", tags=["项目"])


@router.get("", response_model=ApiResponse[list[ProjectResponse]])
async def list_projects(
    group_id: str | None = None,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """获取项目列表"""
    service = ProjectService(db)
    projects = await service.list_projects(current_user.id, group_id)
    return ApiResponse(data=projects)


@router.post("", response_model=ApiResponse[ProjectResponse])
async def create_project(
    data: CreateProjectRequest,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """创建项目"""
    service = ProjectService(db)
    project = await service.create_project(current_user.id, data.name, data.group_id, data.description)
    return ApiResponse(data=project)


@router.delete("/{project_id}")
async def delete_project(
    project_id: str,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """删除项目（进入回收站）"""
    service = ProjectService(db)
    await service.soft_delete_project(project_id, current_user.id)
    return ApiResponse(data=None)
```

## 6. 响应格式

### 6.1 统一响应格式

```python
# api/response.py

from typing import Generic, TypeVar
from pydantic import BaseModel


T = TypeVar("T")


class ApiResponse(BaseModel, Generic[T]):
    """统一 API 响应格式"""
    code: int = 200
    data: T | None = None
    info: str = ""

    @classmethod
    def success(cls, data: T | None = None, info: str = ""):
        return cls(code=200, data=data, info=info)

    @classmethod
    def error(cls, code: int = 400, info: str = "", data: T | None = None):
        return cls(code=code, data=data, info=info)
```

### 6.2 响应示例

```json
// 成功响应
{
  "code": 200,
  "data": {
    "id": "xxx",
    "name": "项目1"
  },
  "info": ""
}

// 错误响应
{
  "code": 400,
  "data": null,
  "info": "项目不存在"
}
```

## 7. 安全实现

### 7.1 JWT 认证

```python
# core/token.py

from datetime import datetime, timedelta, timezone
from typing import Any
import jwt
from app.config import settings


class TokenManager:
    def __init__(self):
        self.secret_key = settings.secret_key
        self.access_expire = settings.access_token_expire_minutes
        self.refresh_expire = settings.refresh_token_expire_days

    def create_access_token(self, data: dict[str, Any]) -> str:
        """创建 Access Token"""
        expire = datetime.now(timezone.utc) + timedelta(minutes=self.access_expire)
        payload = {**data, "exp": expire, "type": "access"}
        return jwt.encode(payload, self.secret_key, algorithm="HS256")

    def create_refresh_token(self, data: dict[str, Any]) -> str:
        """创建 Refresh Token"""
        expire = datetime.now(timezone.utc) + timedelta(days=self.refresh_expire)
        payload = {**data, "exp": expire, "type": "refresh"}
        return jwt.encode(payload, self.secret_key, algorithm="HS256")

    def decode_token(self, token: str) -> dict[str, Any]:
        """解码 Token"""
        try:
            return jwt.decode(token, self.secret_key, algorithms=["HS256"])
        except jwt.ExpiredSignatureError:
            raise HTTPException(status_code=401, detail="Token 已过期")
        except jwt.InvalidTokenError:
            raise HTTPException(status_code=401, detail="无效 Token")


token_manager = TokenManager()
```

### 7.2 密码哈希

```python
# core/security.py

from passlib.context import CryptContext


pwd_context = CryptContext(schemes=["bcrypt"], deprecated="auto")


def hash_password(password: str) -> str:
    """密码哈希"""
    return pwd_context.hash(password)


def verify_password(plain_password: str, hashed_password: str) -> bool:
    """验证密码"""
    return pwd_context.verify(plain_password, hashed_password)
```

## 8. 中间件

### 8.1 请求日志

```python
# api/middleware.py

from fastapi import Request
from starlette.middleware.base import BaseHTTPMiddleware
import structlog

logger = structlog.get_logger()


class LoggingMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next):
        logger.info("request_start", method=request.method, path=request.url.path)

        response = await call_next(request)

        logger.info(
            "request_end",
            method=request.method,
            path=request.url.path,
            status_code=response.status_code
        )

        return response
```

## 9. 错误处理

### 9.1 全局异常

```python
# app/exceptions.py

from fastapi import HTTPException, status


class AppException(HTTPException):
    def __init__(self, code: int, detail: str):
        super().__init__(status_code=code, detail=detail)


class NotFoundError(AppException):
    def __init__(self, resource: str = "资源"):
        super().__init__(status_code=404, detail=f"{resource}不存在")


class UnauthorizedError(AppException):
    def __init__(self, message: str = "未登录或登录已过期"):
        super().__init__(status_code=401, detail=message)


class ForbiddenError(AppException):
    def __init__(self, message: str = "无权限"):
        super().__init__(status_code=403, detail=message)


class ValidationError(AppException):
    def __init__(self, message: str):
        super().__init__(status_code=422, detail=message)
```

---

*本章节描述了 PaperSage 后端的设计规格，包括技术选型、项目结构、数据库模型、API 接口和安全实现。*