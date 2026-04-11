# 后端可靠性设计

## 1. 错误处理

### 1.1 全局异常处理

所有未捕获的异常通过 FastAPI 的全局异常处理器统一处理：

```python
# app/exceptions.py
from fastapi import Request, status
from fastapi.responses import JSONResponse
from fastapi.exceptions import RequestValidationError

async def validation_exception_handler(request: Request, exc: RequestValidationError):
    return JSONResponse(
        status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
        content={"code": 422, "data": None, "info": str(exc)}
    )

async def generic_exception_handler(request: Request, exc: Exception):
    # 生产环境不暴露详细错误
    logger.error(f"Unhandled exception: {exc}", exc_info=True)
    return JSONResponse(
        status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
        content={"code": 500, "data": None, "info": "Internal server error"}
    )
```

### 1.2 自定义业务异常

```python
class AppException(Exception):
    def __init__(self, code: int, message: str):
        self.code = code
        self.message = message
        super().__init__(message)

class NotFoundException(AppException):
    def __init__(self, resource: str):
        super().__init__(404, f"{resource} not found")

class UnauthorizedException(AppException):
    def __init__(self, message: str = "Unauthorized"):
        super().__init__(401, message)

class ForbiddenException(AppException):
    def __init__(self, message: str = "Forbidden"):
        super().__init__(403, message)
```

### 1.3 错误响应格式

所有错误响应统一格式：

```json
{
  "code": 400,
  "data": null,
  "info": "错误描述信息"
}
```

| 状态码 | 说明 | 常见场景 |
|--------|------|----------|
| 400 | 请求参数错误 | 缺少必填字段、格式错误 |
| 401 | 未认证 | 未提供 Token、Token 无效 |
| 403 | 无权限 | Token 无权访问资源 |
| 404 | 资源不存在 | ID 不存在、已删除 |
| 422 | 业务逻辑错误 | 重复注册、名冲突 |
| 500 | 服务器内部错误 | 未捕获异常 |

---

## 2. 日志规范

### 2.1 日志级别使用

| 级别 | 使用场景 |
|------|----------|
| DEBUG | 详细调试信息（参数、变量） |
| INFO | 正常业务流程（登录、创建、查询） |
| WARNING | 异常但不影响功能（缓存未命中、重试） |
| ERROR | 需要关注的错误（业务异常、外部服务失败） |
| CRITICAL | 严重错误（配置缺失、数据库连接失败） |

### 2.2 日志格式

```
[%(asctime)s] %(levelname)-8s [%(name)s:%(lineno)d] %(message)s
```

示例：
```
[2024-01-15 10:30:45] INFO     [api.routes.account:23] User login successful user_id=1
[2024-01-15 10:30:46] ERROR    [services.auth:45] Login failed invalid_credentials user_id=null
```

### 2.3 日志内容规范

- **敏感信息脱敏**: 禁止记录密码、Token、身份证号等
- **结构化日志**: 使用 `key=value` 格式便于搜索
- **请求追踪**: 记录 `request_id` 便于链路追踪

```python
logger.info(
    "User login",
    extra={
        "user_id": user.id,
        "username": user.username,
        "request_id": request.state.request_id
    }
)
```

### 2.4 日志工具

使用标准库 `logging`，生产环境可集成：

- **结构化日志**: `python-json-logger`
- **日志聚合**: ELK Stack (Elasticsearch + Logstash + Kibana)
- **云服务**: AWS CloudWatch, GCP Logging

---

## 3. 监控指标

### 3.1 关键指标

| 指标 | 描述 | 告警阈值 |
|------|------|----------|
| 请求延迟 (p50) | API 响应时间中位数 | > 200ms |
| 请求延迟 (p99) | API 响应时间 99 分位 | > 1s |
| 错误率 | 5xx 响应占比 | > 1% |
| QPS | 每秒请求数 | 峰值 > 1000 |
| CPU 使用率 | 进程 CPU 占用 | > 80% |
| 内存使用 | 进程内存占用 | > 80% |
| 数据库连接池 | 活跃连接数 | > 80% 池大小 |

### 3.2 Prometheus 指标集成

```python
from prometheus_client import Counter, Histogram, generate_latest

# 请求计数器
request_counter = Counter(
    'http_requests_total',
    'Total HTTP requests',
    ['method', 'endpoint', 'status']
)

# 延迟直方图
request_latency = Histogram(
    'http_request_duration_seconds',
    'HTTP request latency',
    ['method', 'endpoint']
)

# 注册 /metrics 端点
@app.get("/metrics")
async def metrics():
    return Response(content=generate_latest(), media_type="text/plain")
```

### 3.3 健康检查

```python
from fastapi import APIRouter

router = APIRouter()

@router.get("/health")
async def health_check():
    # 检查数据库连接
    try:
        await db.execute("SELECT 1")
    except Exception as e:
        return {"status": "unhealthy", "database": "down"}

    return {"status": "healthy", "database": "up"}
```

---

## 4. 备份与恢复

### 4.1 数据库备份

| 类型 | 频率 | 保留时间 |
|------|------|----------|
| 全量备份 | 每天 02:00 | 30 天 |
| 增量备份 | 每小时 | 7 天 |
| 实时复制 | 实时 | 主从延迟 |

### 4.2 备份脚本示例

```bash
#!/bin/bash
# backup.sh

# PostgreSQL 备份
pg_dump -h $DB_HOST -U $DB_USER $DB_NAME > backup_$(date +%Y%m%d_%H%M%S).sql

# 删除 30 天前的备份
find /backups -name "*.sql" -mtime +30 -delete
```

### 4.3 恢复流程

1. 停止服务
2. 恢复数据库: `psql < backup.sql`
3. 验证数据完整性
4. 重启服务

---

## 5. 限流与熔断

### 5.1 请求限流

使用 Redis 实现分布式限流：

```python
# core/rate_limit.py
from redis import Redis
from fastapi import HTTPException

async def rate_limit(key: str, limit: int, window: int):
    redis = Redis.from_url(settings.REDIS_URL)
    current = redis.incr(key)

    if current == 1:
        redis.expire(key, window)

    if current > limit:
        raise HTTPException(429, "Too many requests")

# 使用示例
@router.post("/api/account/login")
async def login(request: LoginRequest):
    await rate_limit(f"ratelimit:login:{request.account}", 10, 60)
    # ...
```

| 端点 | 限制 |
|------|------|
| /api/account/login | 10 次/分钟 |
| /api/account/register | 5 次/分钟 |
| 其他 API | 100 次/分钟 |

### 5.2 熔断器

当外部服务（数据库、Redis）持续失败时，快速失败防止雪崩：

```python
from fastapi import FastAPI
from slowapi import Limiter
from slowapi.util import get_remote_address

limiter = Limiter(key_func=get_remote_address)
app = FastAPI()

@app.post("/api/account/login")
@limiter.limit("10/minute")
async def login(request: Request):
    # ...
```

---

## 6. 性能优化

### 6.1 数据库连接池

```python
engine = create_async_engine(
    settings.DATABASE_URL,
    pool_size=20,          # 最小连接数
    max_overflow=30,       # 最大额外连接
    pool_pre_ping=True,    # 使用前检测连接
    pool_recycle=3600,     # 连接回收时间
)
```

### 6.2 缓存策略

| 缓存内容 | TTL | 失效策略 |
|----------|-----|----------|
| 用户 Session | 30 分钟 | 主动失效 |
| 用户信息 | 5 分钟 | LRU 淘汰 |
| 热点数据 | 1 分钟 | 主动失效 |

### 6.3 异步处理

- 所有 I/O 操作使用异步 (async/await)
- 避免在请求处理中执行同步阻塞操作
- 大文件上传使用流式处理