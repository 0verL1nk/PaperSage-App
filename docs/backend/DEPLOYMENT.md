# 后端部署配置

## 1. Docker 配置

### 1.1 Dockerfile

```dockerfile
# 构建阶段
FROM python:3.12-slim as builder

WORKDIR /app

# 安装 uv (快速包管理)
RUN pip install uv

# 复制依赖文件
COPY pyproject.toml ./

# 安装依赖 (优化层缓存)
RUN uv pip install --system --no-cache --frozen-file-deps

# 运行阶段
FROM python:3.12-slim

WORKDIR /app

# 创建非 root 用户
RUN useradd --create-home --shell /bin/bash appuser

# 复制依赖
COPY --from=builder /usr/local/lib/python3.12/site-packages /usr/local/lib/python3.12/site-packages

# 复制应用代码
COPY . .

# 设置权限
RUN chown -R appuser:appuser /app

# 切换到非 root 用户
USER appuser

# 暴露端口
EXPOSE 8000

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

# 启动命令
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
```

### 1.2 .dockerignore

```gitignore
__pycache__/
*.pyc
*.pyo
*.pyd
.Python
*.so
*.egg
*.egg-info/
dist/
build/
.pytest_cache/
.coverage
htmlcov/
.env
.venv/
venv/
.git/
.gitignore
*.md
docs/
```

---

## 2. Docker Compose (本地开发)

### 2.1 docker-compose.yml

```yaml
version: '3.9'

services:
  # PostgreSQL 数据库
  postgres:
    image: postgres:16-alpine
    container_name: papersage-postgres
    environment:
      POSTGRES_USER: papersage
      POSTGRES_PASSWORD: papersage_dev_password
      POSTGRES_DB: papersage
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U papersage"]
      interval: 5s
      timeout: 5s
      retries: 5

  # Redis 缓存
  redis:
    image: redis:7-alpine
    container_name: papersage-redis
    command: redis-server --requirepass papersage_dev_password
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5

  # 后端 API
  backend:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: papersage-backend
    ports:
      - "8000:8000"
    environment:
      - DATABASE_URL=postgresql+asyncpg://papersage:papersage_dev_password@postgres:5432/papersage
      - REDIS_URL=redis://:papersage_dev_password@redis:6379/0
      - SECRET_KEY=dev-secret-key-change-in-production
      - DEBUG=true
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    volumes:
      - ./app:/app/app:ro
    command: uvicorn app.main:app --host 0.0.0.0 --port 8000 --reload

volumes:
  postgres_data:
  redis_data:
```

### 2.2 启动命令

```bash
# 启动所有服务
docker-compose up -d

# 查看日志
docker-compose logs -f backend

# 停止所有服务
docker-compose down

# 重新构建
docker-compose up -d --build
```

---

## 3. 生产环境部署

### 3.1 生产 docker-compose.yml

```yaml
version: '3.9'

services:
  nginx:
    image: nginx:alpine
    container_name: papersage-nginx
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./ssl:/etc/nginx/ssl:ro
    depends_on:
      - backend
    restart: unless-stopped

  backend:
    build:
      context: .
      dockerfile: Dockerfile
      args:
        - BUILD_MODE=production
    container_name: papersage-backend
    environment:
      - DATABASE_URL=${DATABASE_URL}
      - REDIS_URL=${REDIS_URL}
      - SECRET_KEY=${SECRET_KEY}
      - DEBUG=false
    restart: unless-stopped
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '0.5'
          memory: 512M

  postgres:
    image: postgres:16-alpine
    container_name: papersage-postgres
    environment:
      POSTGRES_USER: ${DB_USER}
      POSTGRES_PASSWORD: ${DB_PASSWORD}
      POSTGRES_DB: ${DB_NAME}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    restart: unless-stopped
    deploy:
      resources:
        limits:
          cpus: '1'
          memory: 1G

  redis:
    image: redis:7-alpine
    container_name: papersage-redis
    command: redis-server --requirepass ${REDIS_PASSWORD} --maxmemory 512mb --maxmemory-policy allkeys-lru
    volumes:
      - redis_data:/data
    restart: unless-stopped

volumes:
  postgres_data:
  redis_data:
```

### 3.2 Nginx 配置

```nginx
events {
    worker_connections 1024;
}

http {
    upstream backend {
        server backend:8000;
    }

    # HTTP -> HTTPS 重定向
    server {
        listen 80;
        server_name api.example.com;
        return 301 https://$server_name$request_uri;
    }

    # HTTPS
    server {
        listen 443 ssl http2;
        server_name api.example.com;

        ssl_certificate /etc/nginx/ssl/server.crt;
        ssl_certificate_key /etc/nginx/ssl/server.key;
        ssl_protocols TLSv1.2 TLSv1.3;

        location / {
            proxy_pass http://backend;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }

        location /docs {
            proxy_pass http://backend;
            proxy_set_header Host $host;
        }

        location /health {
            proxy_pass http://backend;
            proxy_set_header Host $host;
        }
    }
}
```

---

## 4. GitHub Actions CI/CD

### 4.1 CI 工作流

```yaml
# .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest

    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_USER: test
          POSTGRES_PASSWORD: test
          POSTGRES_DB: test
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
      - uses: actions/checkout@v4

      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: '3.12'

      - name: Install uv
        run: pip install uv

      - name: Install dependencies
        run: uv pip install --system -e ".[dev]"

      - name: Run linter
        run: ruff check .

      - name: Type check
        run: mypy app/

      - name: Run tests
        env:
          DATABASE_URL: postgresql+asyncpg://test:test@localhost:5432/test
          REDIS_URL: redis://localhost:6379/0
          SECRET_KEY: test-secret-key
        run: pytest -v

      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          files: ./coverage.xml
```

### 4.2 CD 工作流 (部署到服务器)

```yaml
# .github/workflows/deploy.yml
name: Deploy

on:
  push:
    branches: [main]
  workflow_dispatch:

jobs:
  deploy:
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/main'

    steps:
      - uses: actions/checkout@v4

      - name: Login to Docker Hub
        uses: docker/login-action@v3
        with:
          username: ${{ secrets.DOCKER_USERNAME }}
          password: ${{ secrets.DOCKER_PASSWORD }}

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          tags: ${{ secrets.DOCKER_USERNAME }}/papersage-backend:latest

      - name: Deploy to server
        uses: appleboy/ssh-action@v1
        with:
          host: ${{ secrets.SERVER_HOST }}
          username: ${{ secrets.SERVER_USER }}
          key: ${{ secrets.SSH_KEY }}
          script: |
            cd /opt/papersage
            docker-compose pull
            docker-compose up -d
            docker system prune -f
```

---

## 5. 环境变量配置

### 5.1 生产环境 .env

```bash
# 必需
SECRET_KEY=<生成 32+ 字符随机字符串>
DATABASE_URL=postgresql+asyncpg://user:password@host:5432/db
REDIS_URL=redis://:password@host:6379/0

# 可选
DEBUG=false
ACCESS_TOKEN_EXPIRE_MINUTES=30
REFRESH_TOKEN_EXPIRE_DAYS=7
ALLOWED_HOSTS=api.example.com
```

### 5.2 生成密钥

```bash
# 生成随机密钥
python -c "from secrets import token_hex; print(token_hex(32))"
```

---

## 6. 健康检查

### 6.1 健康检查端点

```python
# app/main.py
from fastapi import APIRouter

router = APIRouter()

@router.get("/health")
async def health_check():
    return {
        "status": "healthy",
        "version": "1.0.0"
    }

@router.get("/health/ready")
async def readiness_check():
    # 检查数据库
    try:
        await db.execute("SELECT 1")
    except Exception as e:
        return {"status": "not_ready", "database": "down"}

    # 检查 Redis
    try:
        await redis.ping()
    except Exception:
        return {"status": "not_ready", "redis": "down"}

    return {"status": "ready"}
```

---

## 7. 部署检查清单

### 部署前

- [ ] 所有测试通过
- [ ] 代码已 Code Review
- [ ] 敏感配置已设置环境变量
- [ ] SSL 证书已配置

### 部署后

- [ ] 服务启动成功
- [ ] 健康检查通过
- [ ] API 文档可访问 (`/docs`)
- [ ] 登录功能正常
- [ ] 日志正常输出

### 监控

- [ ] 错误率正常
- [ ] 响应时间正常
- [ ] 资源使用正常