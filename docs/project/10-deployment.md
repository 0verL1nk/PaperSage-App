# 部署运维

## 1. 部署架构

### 1.1 环境说明

| 环境 | 用途 | 域名 |
|------|------|------|
| 开发 | 本地开发调试 | localhost |
| 测试 | QA 测试 | <test-domain> |
| 预发布 | 预发布验收 | <staging-domain> |
| 生产 | 正式环境 | <production-domain> |

### 1.2 基础设施

```mermaid
graph TB
    subgraph Cloud["云服务器 (AWS/阿里云)"]
        subgraph VPC["VPC 私有网络"]
            subgraph Web["Web 服务组"]
                Nginx[Nginx<br/>负载均衡]
            end

            subgraph App["应用服务组"]
                FE1[前端 1]
                FE2[前端 2]
                BE1[后端 1]
                BE2[后端 2]
            end

            subgraph Data["数据服务组"]
                DB[(MySQL<br/>主库)]
                DB[(MySQL<br/>从库)]
                Redis[(Redis<br/>缓存)]
                S3[(S3 兼容存储<br/>MinIO/AWS S3)]
            end

            subgraph Worker["后台任务"]
                Worker[ARQ Worker]
            end
        end

        DNS[DNS]
        SSL[SSL 证书]
    end

    User((用户)) --> DNS
    DNS --> SSL
    SSL --> Nginx
    Nginx --> FE1
    Nginx --> FE2
    Nginx --> BE1
    Nginx --> BE2
    BE1 --> PG1
    BE1 --> PG2
    BE1 --> Redis
    BE1 --> S3
    BE2 --> PG1
    BE2 --> PG2
    BE2 --> Redis
    BE2 --> S3
    Worker --> PG1
    Worker --> S3
```

## 2. Docker 部署

### 2.1 Dockerfile (后端)

```dockerfile
# backend/Dockerfile

# 阶段 1: 构建
FROM python:3.12-slim AS builder

WORKDIR /app

# 安装构建依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    libpq-dev \
    && rm -rf /var/lib/apt/lists/*

# 安装 uv
RUN pip install uv

# 复制依赖文件
COPY pyproject.toml ./

# 安装依赖
RUN uv sync --frozen --no-dev

# 阶段 2: 运行
FROM python:3.12-slim AS runner

WORKDIR /app

# 安装运行时依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    libpq5 \
    && rm -rf /var/lib/apt/lists/*

# 复制依赖
COPY --from=builder /app /app
COPY --from=builder /app /usr/local

# 设置环境变量
ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV PYTHONPATH=/app

# 切换到非 root 用户
RUN useradd --create-home appuser && chown -R appuser:appuser /app
USER appuser

# 暴露端口
EXPOSE 8000

# 启动命令
CMD ["gunicorn", "app.main:app", "-c", "gunicorn.conf.py", "-k", "uvicorn.workers.UvicornWorker"]
```

### 2.2 Dockerfile (前端)

```dockerfile
# frontend/Dockerfile

# 构建阶段
FROM node:20-alpine AS builder

WORKDIR /app

# 安装 pnpm
RUN corepack enable && corepack prepare pnpm@latest --activate

# 复制依赖文件
COPY package.json pnpm-lock.yaml ./

# 安装依赖
RUN pnpm install --frozen-lockfile

# 复制源码
COPY . .

# 构建
RUN pnpm build

# 运行阶段
FROM nginx:alpine AS runner

# 复制构建产物
COPY --from=builder /app/dist /usr/share/nginx/html

# 复制 nginx 配置
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
```

### 2.3 Docker Compose (开发环境)

```yaml
# docker-compose.yml

services:
  # PostgreSQL 数据库
  postgres:
    image: postgres:17-alpine
    container_name: papersage-postgres
    environment:
      POSTGRES_USER: papersage
      POSTGRES_PASSWORD: papersage_dev
      POSTGRES_DB: papersage
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U papersage"]
      interval: 10s
      timeout: 5s
      retries: 5

  # Redis 缓存
  redis:
    image: redis:7-alpine
    container_name: papersage-redis
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5

  # MinIO 对象存储
  minio:
    image: minio/minio:latest
    container_name: papersage-minio
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
    volumes:
      - minio_data:/data
    ports:
      - "9000:9000"
      - "9001:9001"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
      interval: 30s
      timeout: 20s
      retries: 3

  # 后端 API
  backend:
    build:
      context: ./backend
      dockerfile: Dockerfile
    container_name: papersage-backend
    environment:
      DATABASE_URL: postgresql+asyncpg://papersage:papersage_dev@postgres:5432/papersage
      REDIS_URL: redis://redis:6379/0
      MINIO_ENDPOINT: minio:9000
      MINIO_ACCESS_KEY: minioadmin
      MINIO_SECRET_KEY: minioadmin
      SECRET_KEY: dev-secret-key-change-in-production
      DEBUG: "true"
    ports:
      - "8000:8000"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      minio:
        condition: service_healthy
    volumes:
      - ./backend:/app
      - /app/__pycache__

  # 前端
  frontend:
    build:
      context: ./frontend
      dockerfile: Dockerfile
    container_name: papersage-frontend
    ports:
      - "80:80"
    depends_on:
      - backend

volumes:
  postgres_data:
  redis_data:
  minio_data:
```

### 2.4 Nginx 配置 (前端)

```nginx
# frontend/nginx.conf

server {
    listen 80;
    server_name localhost;
    root /usr/share/nginx/html;
    index index.html;

    # Gzip 压缩
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript;
    gzip_min_length 1000;

    # 静态资源缓存
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2)$ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    # API 代理
    location /api {
        proxy_pass http://backend:8000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
    }

    # SPA 路由支持
    location / {
        try_files $uri $uri/ /index.html;
    }

    # 错误页面
    error_page 500 502 503 504 /50x.html;
    location = /50x.html {
        root /usr/share/nginx/html;
    }
}
```

## 3. 环境配置

### 3.1 数据库配置

> **重要**: 系统通过 SQLAlchemy 抽象数据库，支持多数据库无缝切换

| 环境 | 推荐数据库 | 连接串格式 |
|------|------------|------------|
| 开发 | SQLite | `sqlite+aiosqlite:///./papersage.db` |
| 测试 | SQLite | `sqlite+aiosqlite:///./test.db` |
| 生产 | MySQL | `mysql+aiomysql://user:password@host:3306/papersage` |
| 生产(可选) | PostgreSQL | `postgresql+asyncpg://user:password@host:5432/papersage` |

### 3.2 环境变量

```bash
# backend/.env

# ===============================================
# 开发环境配置 (使用 SQLite)
# ===============================================
DATABASE_URL=sqlite+aiosqlite:///./papersage.db

# ===============================================
# 生产环境配置 (使用 MySQL)
# ===============================================
# DATABASE_URL=mysql+aiomysql://user:password@host:3306/papersage

# Redis
REDIS_URL=redis://host:6379/0

# 安全
SECRET_KEY=your-secret-key-here
ACCESS_TOKEN_EXPIRE_MINUTES=30
REFRESH_TOKEN_EXPIRE_DAYS=7

# S3 兼容存储 (MinIO/AWS S3/阿里云 OSS)
S3_ENDPOINT=localhost:9000
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
S3_BUCKET=papersage

# AI 服务
OPENAI_API_KEY=sk-xxx
ANTHROPIC_API_KEY=sk-ant-xxx

# 应用
DEBUG=false
ALLOWED_HOSTS=localhost,127.0.0.1
```

### 3.2 Gunicorn 配置

```python
# backend/gunicorn.conf.py

import multiprocessing

# 绑定地址
bind = "0.0.0.0:8000"

# 工作进程数
workers = multiprocessing.cpu_count() * 2 + 1

# 工作模式
worker_class = "uvicorn.workers.UvicornWorker"

# 超时设置
timeout = 120
graceful_timeout = 30

# 日志
accesslog = "-"
errorlog = "-"
loglevel = "info"

# 预加载应用
preload_app = True

# 最大请求数后重启 worker
max_requests = 1000
max_requests_jitter = 50
```

## 4. CI/CD 流程

### 4.1 GitHub Actions

```yaml
# .github/workflows/deploy.yml

name: Deploy

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Run frontend tests
        run: |
          cd frontend
          pnpm install
          pnpm test
          pnpm build

      - name: Run backend tests
        run: |
          cd backend
          pip install -e ".[dev]"
          pytest --cov

  deploy:
    needs: test
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v4

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: us-east-1

      - name: Login to Amazon ECR
        uses: aws-actions/amazon-ecr-login@v2

      - name: Build and push Docker images
        run: |
          # 构建并推送后端镜像
          docker build -t papersage-backend:${{ github.sha }} ./backend
          docker tag papersage-backend:${{ github.sha }} ${{ secrets.ECR_REGISTRY }}/papersage-backend:${{ github.sha }}
          docker push ${{ secrets.ECR_REGISTRY }}/papersage-backend:${{ github.sha }}

          # 构建并推送前端镜像
          docker build -t papersage-frontend:${{ github.sha }} ./frontend
          docker tag papersage-frontend:${{ github.sha }} ${{ secrets.ECR_REGISTRY }}/papersage-frontend:${{ github.sha }}
          docker push ${{ secrets.ECR_REGISTRY }}/papersage-frontend:${{ github.sha }}

      - name: Deploy to ECS
        run: |
          aws ecs update-service --cluster papersage-prod --service backend --force-new-deployment
          aws ecs update-service --cluster papersage-prod --service frontend --force-new-deployment
```

## 5. 监控与告警

### 5.1 监控指标

| 指标类型 | 监控项 | 告警阈值 |
|----------|--------|----------|
| 可用性 | 服务健康检查 | 失败 |
| 性能 | API 响应时间 P99 | > 2s |
| 性能 | 前端首屏加载 | > 3s |
| 资源 | CPU 使用率 | > 80% |
| 资源 | 内存使用率 | > 85% |
| 资源 | 磁盘使用率 | > 90% |
| 资源 | 数据库连接数 | > 80% |
| 错误 | 5xx 错误率 | > 1% |
| 错误 | 4xx 错误率 | > 5% |

### 5.2 日志收集

```yaml
# docker-compose.logging.yml

services:
  # 日志收集器
  fluentd:
    image: fluent/fluentd:v1.16-debian-1
    volumes:
      - ./fluentd/etc:/fluentd/etc
    ports:
      - "24224:24224"
    networks:
      - backend

  # 日志存储
  elasticsearch:
    image: docker.elastic.co/elasticsearch/elasticsearch:8.11.0
    environment:
      - discovery.type=single-node
      - xpack.security.enabled=false
    volumes:
      - es_data:/usr/share/elasticsearch/data

  # 日志可视化
  kibana:
    image: docker.elastic.co/kibana/kibana:8.11.0
    ports:
      - "5601:5601"
    depends_on:
      - elasticsearch
```

### 5.3 告警配置

| 告警级别 | 条件 | 通知方式 |
|----------|------|----------|
| P0 (紧急) | 服务不可用 | 电话 + 短信 |
| P1 (高) | 错误率 > 5% | 短信 |
| P2 (中) | 响应时间 > 5s | 邮件 |
| P3 (低) | 资源使用率 > 70% | 邮件 |

## 6. 备份与恢复

### 6.1 数据库备份

```bash
# 每日备份脚本

#!/bin/bash
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/backup/postgres"

# 创建备份
pg_dump -h $DB_HOST -U $DB_USER -d $DB_NAME > $BACKUP_DIR/papersage_$DATE.sql

# 压缩
gzip $BACKUP_DIR/papersage_$DATE.sql

# 删除 7 天前的备份
find $BACKUP_DIR -name "papersage_*.sql.gz" -mtime +7 -delete
```

### 6.2 文件存储备份

```bash
# 使用 mc 命令备份 MinIO 数据

# 每天凌晨 2 点执行
0 2 * * * mc mirror minio/papersage s3/backups/papersage-$(date +\%Y\%m\%d)
```

---

*本章节描述了 PaperSage 系统的部署方案，包括 Docker 部署、环境配置、CI/CD 流程和监控告警。*