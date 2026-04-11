# 后端安全设计

## 1. 认证授权

### 1.1 JWT 认证

使用 Access Token + Refresh Token 双 Token 机制：

```python
# core/token.py
from datetime import datetime, timedelta
from jose import JWTError, jwt

ALGORITHM = "HS256"

def create_access_token(data: dict, expires_delta: timedelta = None):
    to_encode = data.copy()
    expire = datetime.utcnow() + (expires_delta or timedelta(minutes=30))
    to_encode.update({"exp": expire, "type": "access"})
    return jwt.encode(to_encode, settings.SECRET_KEY, algorithm=ALGORITHM)

def create_refresh_token(data: dict):
    to_encode = data.copy()
    expire = datetime.utcnow() + timedelta(days=7)
    to_encode.update({"exp": expire, "type": "refresh"})
    return jwt.encode(to_encode, settings.SECRET_KEY, algorithm=ALGORITHM)

def verify_token(token: str, token_type: str = "access"):
    try:
        payload = jwt.decode(token, settings.SECRET_KEY, algorithms=[ALGORITHM])
        if payload.get("type") != token_type:
            raise JWTError()
        return payload
    except JWTError:
        raise UnauthorizedException("Invalid token")
```

### 1.2 Token 配置

| Token 类型 | 过期时间 | 用途 |
|------------|----------|------|
| Access Token | 30 分钟 | API 鉴权 |
| Refresh Token | 7 天 | 刷新 Access Token |

### 1.3 依赖注入

```python
# api/dependencies.py
from fastapi import Depends
from fastapi.security import HTTPBearer, HTTPAuthorizationCredentials

security = HTTPBearer()

async def get_current_user(
    credentials: HTTPAuthorizationCredentials = Depends(security)
) -> User:
    token = credentials.credentials
    payload = verify_token(token, "access")
    user_id = payload.get("sub")

    user = await get_user_by_id(user_id)
    if not user:
        raise UnauthorizedException("User not found")

    return user
```

### 1.4 路由保护

```python
# 所有需要认证的路由
@router.get("/api/documents")
async def get_documents(current_user: User = Depends(get_current_user)):
    return await document_service.get_user_documents(current_user.id)
```

---

## 2. 密码安全

### 2.1 密码哈希

使用 bcrypt 算法存储密码：

```python
# core/security.py
from passlib.context import CryptContext

pwd_context = CryptContext(schemes=["bcrypt"], deprecated="auto")

def hash_password(password: str) -> str:
    return pwd_context.hash(password)

def verify_password(plain_password: str, hashed_password: str) -> bool:
    return pwd_context.verify(plain_password, hashed_password)
```

### 2.2 前端密码处理

前端使用 MD5 对密码预处理：

```javascript
// 前端 (已在 auth/api.ts 中实现)
import md5 from 'md5'
const hashedPassword = md5(rawPassword)
// 传输到后端
```

**注意**: 这是为了兼容前端现有实现，不应降低后端安全性。后端使用 bcrypt 二次哈希。

### 2.3 安全要求

- 密码最小长度: 8 字符
- 不允许明文传输 (HTTPS)
- 不记录明文密码

---

## 3. CORS 配置

### 3.1 开发环境

```python
# app/main.py
from fastapi.middleware.cors import CORSMiddleware

app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:5173"],  # Vite 默认端口
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)
```

### 3.2 生产环境

```python
app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_hosts,  # 从环境变量读取
    allow_credentials=True,
    allow_methods=["GET", "POST", "PUT", "DELETE"],
    allow_headers=["Authorization", "Content-Type"],
)
```

### 3.3 安全建议

- 不使用 `allow_origins=["*"]` 在生产环境
- 只允许必要的域名
- 限制允许的 HTTP 方法

---

## 4. 请求限流

### 4.1 登录限流

防止暴力破解：

```python
# api/routes/account.py
from fastapi import APIRouter, Request
from slowapi import Limiter
from slowapi.util import get_remote_address

limiter = Limiter(key_func=get_remote_address)
router = APIRouter()

@router.post("/api/account/login")
@limiter.limit("10/minute")  # 每 IP 每分钟 10 次
async def login(request: Request, form_data: LoginForm):
    # 登录逻辑
```

### 4.2 全局限流

```python
# app/middleware.py
from fastapi import Request
from slowapi import Limiter

limiter = Limiter(key_func=get_remote_address)

@app.middleware("http")
@limiter.limit("100/minute")
async def global_rate_limit(request: Request):
    pass
```

| 端点 | 限制 | 说明 |
|------|------|------|
| /api/account/login | 10/分钟 | 防止暴力破解 |
| /api/account/register | 5/分钟 | 防止批量注册 |
| 其他 API | 100/分钟 | 通用限流 |

---

## 5. SQL 注入防护

### 5.1 使用参数化查询

SQLAlchemy 自动防止 SQL 注入：

```python
# 正确 - 使用参数化查询
result = await session.execute(
    select(User).where(User.username == username)
)

# 错误 - 字符串拼接 (禁止)
query = f"SELECT * FROM users WHERE username = '{username}'"
```

### 5.2 ORM 最佳实践

- 始终使用 SQLAlchemy 表达式
- 不使用原始 SQL 拼接
- 复杂查询使用 SQLAlchemy Core

---

## 6. 敏感数据保护

### 6.1 环境变量

敏感配置通过环境变量管理：

```bash
# .env (不提交到版本控制)
SECRET_KEY=your-secret-key-here
DATABASE_URL=postgresql://user:password@host/db
REDIS_URL=redis://:password@host:6379/0
```

### 6.2 密钥管理

| 密钥 | 要求 | 说明 |
|------|------|------|
| SECRET_KEY | 32+ 字符随机字符串 | JWT 签名密钥 |
| DATABASE_URL | 包含用户名密码 | 数据库连接 |
| REDIS_URL | 包含密码 | Redis 连接 |

### 6.3 日志脱敏

```python
# 禁止记录以下内容
logger.info(f"Login with password: {password}")  # 禁止
logger.info("User login", extra={"user_id": user.id})  # 正确
```

---

## 7. HTTPS 配置

### 7.1 生产环境

Nginx 配置 HTTPS：

```nginx
server {
    listen 443 ssl http2;
    server_name api.example.com;

    ssl_certificate /etc/ssl/certs/server.crt;
    ssl_certificate_key /etc/ssl/private/server.key;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;
    ssl_prefer_server_ciphers off;

    location / {
        proxy_pass http://localhost:8000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 7.2 重定向 HTTP 到 HTTPS

```nginx
server {
    listen 80;
    server_name api.example.com;
    return 301 https://$server_name$request_uri;
}
```

---

## 8. 安全检查清单

### 开发阶段

- [ ] 所有 API 端点需要认证 (除 /api/account/login, /api/account/register)
- [ ] 密码使用 bcrypt 哈希存储
- [ ] 不记录敏感信息 (密码、Token)
- [ ] 使用环境变量管理敏感配置
- [ ] 登录接口限流
- [ ] 正确配置 CORS

### 部署阶段

- [ ] 使用 HTTPS
- [ ] 配置防火墙规则
- [ ] 关闭调试模式 (`DEBUG=false`)
- [ ] 设置安全的 HTTP 头 (使用 Helmet)
- [ ] 配置日志告警

### 运维阶段

- [ ] 定期更新依赖包
- [ ] 监控异常登录行为
- [ ] 定期审计日志
- [ ] 密钥定期轮换