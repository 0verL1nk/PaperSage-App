# Overleaf 整体架构分析

## 1. 架构概述

Overleaf 是一个开源的在线 LaTeX 编辑器，采用**微服务架构**，由多个独立的 Node.js 服务组成，通过 HTTP/REST、Socket.IO（WebSocket）和 Redis Pub/Sub 进行通信。

### 1.1 技术栈

| 层级 | 技术 |
|------|------|
| 前端 | Vanilla JavaScript/TypeScript + 少量 React 组件 |
| 后端服务 | Node.js (Express) |
| 实时通信 | Socket.IO (WebSocket) |
| 数据库 | MongoDB (文档存储) + Redis (缓存/会话/消息队列) |
| LaTeX 编译 | TeX Live (通过 Docker 沙箱) |
| 部署 | Docker Compose |

### 1.2 架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                           客户端浏览器                                │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     Nginx / Docker 反向代理                         │
│                    (端口 80/443)                                    │
└─────────────────────────────────────────────────────────────────────┘
                                  │
        ┌──────────────────────────┼──────────────────────────┐
        ▼                          ▼                          ▼
┌───────────────┐        ┌───────────────┐        ┌───────────────┐
│  Web Service  │        │ Real-time     │        │ Document-     │
│  (端口 3000)   │◄──────►│ Service       │◄──────►│ Updater       │
│               │  HTTP  │ (端口 3001)    │ Redis  │ (端口 3003)   │
│  Express.js   │  REST  │  Socket.IO    │  Pub/   │               │
│  + Mongoose   │        │               │  Sub   │  OT 算法       │
└───────────────┘        └───────────────┘        └───────────────┘
        │                          │                        │
        │                          │                        │
        ▼                          ▼                        ▼
┌───────────────┐        ┌───────────────┐        ┌───────────────┐
│   MongoDB     │        │    Redis      │        │    Redis      │
│  (文档存储)    │        │ (会话/队列)    │        │ (OT操作)       │
└───────────────┘        └───────────────┘        └───────────────┘
        │                          │                        │
        └──────────────────────────┼────────────────────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    ▼                             ▼
          ┌───────────────┐             ┌───────────────┐
          │   CLSI        │             │  Filestore    │
          │ (端口 3013)    │             │ (端口 3009)   │
          │               │             │               │
          │ LaTeX 编译    │             │ 文件存储      │
          └───────────────┘             └───────────────┘
                    │                             │
                    ▼                             ▼
          ┌───────────────┐             ┌───────────────┐
          │ TeX Live      │             │  GridFS       │
          │ (Docker)      │             │ (MongoDB)     │
          └───────────────┘             └───────────────┘
```

---

## 2. 服务组件详解

### 2.1 服务列表

Overleaf 包含以下 11 个核心服务（定义于 `server-ce/services.js`）：

| 服务名 | 端口 | 功能 |
|--------|------|------|
| **web** | 3000 | 主 Web 服务，处理 HTTP 请求和页面渲染 |
| **real-time** | 3001 | WebSocket 服务，处理实时协作 |
| **document-updater** | 3003 | OT 算法实现，处理文档更新 |
| **clsi** | 3013 | LaTeX 编译服务 |
| **filestore** | 3009 | 文件存储服务 |
| **docstore** | - | 文档存储服务 |
| **chat** | - | 聊天服务 |
| **contacts** | - | 联系人服务 |
| **notifications** | - | 通知服务 |
| **project-history** | - | 项目历史服务 |
| **history-v1** | - | 历史版本服务 |

### 2.2 Web Service (主服务)

**路径**: `services/web/`

**职责**:
- 处理所有 HTTP 请求
- Express.js 应用入口 (`app.mjs`)
- Mongoose ODM 与 MongoDB 交互
- 会话管理
- 路由分发

**关键目录结构**:
```
services/web/
├── app.mjs                    # 应用入口
├── app/src/
│   ├── Features/              # 功能模块 (按领域组织)
│   │   ├── Authentication/    # 认证
│   │   ├── Project/           # 项目管理
│   │   ├── Editor/            # 编辑器
│   │   ├── Compile/           # 编译控制
│   │   ├── DocumentUpdater/   # 文档更新
│   │   └── ...
│   ├── infrastructure/        # 基础设施
│   │   ├── Server.mjs         # HTTP 服务器
│   │   ├── Mongoose.mjs       # MongoDB 连接
│   │   ├── RedisWrapper.mjs   # Redis 客户端
│   │   └── ...
│   └── models/                # Mongoose 模型
└── frontend/                  # 前端资源
    ├── js/
    │   ├── main/             # 主应用代码
    │   ├── ide/              # IDE 相关
    │   ├── features/         # 前端功能模块
    │   └── react.ts          # React 组件入口
    └── stylesheets/          # 样式表
```

**前端架构**:
- 使用 **Vanilla JavaScript/TypeScript**
- 部分功能使用 **React** 组件 (`frontend/js/react.ts`)
- **Socket.IO Client** 用于实时通信
- 前端代码按功能模块组织 (`features/`)

### 2.3 Real-time Service (实时服务)

**路径**: `services/real-time/`

**职责**:
- WebSocket 服务器 (Socket.IO)
- 处理客户端连接
- 房间/频道管理
- 与 Document Updater 通过 Redis Pub/Sub 通信

**关键组件**:
```javascript
// app.js
Router.js              // 路由分发
WebsocketController.js // WebSocket 连接处理
DocumentUpdaterController.js // 与 Document Updater 通信
RoomManager.js         // 房间管理
ChannelManager.js     // 频道管理
RedisClientManager.js // Redis 客户端
```

**通信流程**:
```
客户端 WebSocket ──► Real-time Service
                           │
                           ▼ Redis Pub/Sub
                           │
                    Document Updater
                           │
                           ▼ Redis Pub/Sub
                           │
                    Real-time Service ──► 其他客户端
```

### 2.4 Document Updater Service

**路径**: `services/document-updater/`

**职责**:
- **OT (Operational Transformation)** 算法实现
- 文档更新处理
- 冲突解决
- 与 Real-time Service 通过 Redis 通信

**核心组件**:
```javascript
DocumentManager.js      // 文档管理
ShareJsUpdateManager.js // ShareJS 更新管理
UpdateManager.js       // OT 更新处理
DiffCodec.js           // 差异编解码
RangesManager.js       // 范围管理
RedisManager.js        // Redis 操作
PersistenceManager.js  // 持久化管理
```

### 2.5 CLSI (LaTeX 编译服务)

**路径**: `services/clsi/`

**职责**:
- LaTeX 文档编译
- PDF/HTML 输出生成
- 沙箱化编译 (Docker)

**API 端点**:
```
POST /project/:projectId/compile
```

**编译选项**:
```json
{
  "compile": {
    "options": {
      "compiler": "pdflatex|xelatex|lualatex",
      "timeout": 40
    },
    "rootResourcePath": "main.tex",
    "resources": [...]
  }
}
```

---

## 3. 共享库 (Libraries)

**路径**: `libraries/`

提供跨服务共享的核心库：

| 库名 | 功能 |
|------|------|
| `@overleaf/logger` | 日志系统 |
| `@overleaf/metrics` | 指标监控 |
| `@overleaf/settings` | 配置管理 |
| `@overleaf/redis-wrapper` | Redis 客户端封装 |
| `@overleaf/mongo-utils` | MongoDB 工具 |
| `@overleaf/validation-tools` | 输入验证 |
| `@overleaf/fetch-utils` | HTTP 请求 |
| `@overleaf/o-error` | 错误处理 |
| `@overleaf/ranges-tracker` | 范围追踪 (用于 OT) |
| `@overleaf/object-persistor` | 对象持久化 |
| `@overleaf/stream-utils` | 流处理工具 |
| `@overleaf/promise-utils` | Promise 工具 |
| `@overleaf/access-token-encryptor` | 访问令牌加密 |

---

## 4. 数据存储

### 4.1 MongoDB

**用途**:
- 用户数据
- 项目/文件夹/文档
- 权限信息
- 聊天消息

**连接方式**:
- Mongoose ODM
- 副本集部署 (replica set)

### 4.2 Redis

**用途**:
- 会话存储 (`connect-redis`)
- 实时消息 Pub/Sub
- 文档更新队列
- 缓存
- 速率限制

**配置示例**:
```javascript
{
  redis: {
    websessions: { /* 会话存储 */ },
    pubsub: { /* 发布订阅 */ },
    documentupdater: { /* 文档更新 */ }
  }
}
```

---

## 5. 前后端通信机制

### 5.1 HTTP REST

传统请求/响应模式，用于：
- 页面加载
- 文件上传/下载
- 项目 CRUD
- 用户认证

### 5.2 WebSocket (Socket.IO)

实时双向通信，用于：
- 文档协作编辑
- 光标位置同步
- 聊天消息
- 在线状态

**主要事件**:
```javascript
// 客户端
socket.emit('joinDoc', { doc_id, fromVersion, options })
socket.emit('applyOtUpdate', { doc_id, update })
socket.emit('getCursor', { doc_id })

// 服务端
socket.on('applyUpdate', callback)
socket.on('cursorUpdate', callback)
```

### 5.3 Redis Pub/Sub

服务间通信，用于：
- Document Updater 与 Real-time Service 同步
- 跨服务事件分发

---

## 6. 核心设计模式

### 6.1 微服务架构

每个服务独立部署和扩展，通过明确定义的接口通信。

### 6.2 OT (Operational Transformation)

文档协作编辑的核心算法，处理并发编辑冲突。

**关键点**:
- 客户端应用本地更新
- 发送 OT 操作到服务器
- 服务器转换并广播操作
- 其他客户端应用转换后的操作

### 6.3 事件驱动架构

- Redis Pub/Sub 解耦服务
- 异步处理长时间操作 (如编译)
- 消息队列处理高并发

### 6.4 插件系统

Overleaf 支持通过环境变量启用/禁用功能：
```javascript
// Features.mjs
const Features = {
  async init() {
    // 动态加载功能模块
  }
}
```

---

## 7. 部署架构

### 7.1 Docker Compose 部署

```yaml
services:
  sharelatex:       # 主应用 (包含所有服务)
    image: sharelatex/sharelatex
    ports:
      - "80:80"
    depends_on:
      mongo:
        condition: service_healthy
      redis:
        condition: service_started

  mongo:            # MongoDB 6.0 副本集
    image: mongo:6.0

  redis:            # Redis 6.2
    image: redis:6.2
```

### 7.2 沙箱编译

LaTeX 编译在 Docker 容器中执行：
- 主机挂载 Docker Socket
- CLSI 服务启动兄弟容器运行 TeX Live
- 编译文件通过绑定挂载共享

---

## 8. 关键设计决策

1. **Node.js 单线程 + 多进程**: 每个服务是一个 Node.js 进程，通过 cluster 模式利用多核
2. **MongoDB 用于文档存储**: 灵活的 schema 适合 LaTeX 文档结构
3. **Redis 多用途**: 会话、缓存、Pub/Sub、消息队列一身
4. **OT 协作**: 成熟的冲突解决算法支持多人同时编辑
5. **沙箱编译**: Docker 隔离确保安全性
6. **传统前端**: 未使用 React/Vue 等框架，保持简单

---

## 10. API 网关与路由设计

### 10.1 Nginx 配置架构

Overleaf 使用 Nginx 作为反向代理和 API 网关，主要配置文件位于 `server-ce/nginx/nginx.conf.template`。

**核心配置结构**:
```nginx
# Nginx 主配置模板 (server-ce/nginx/nginx.conf.template)
daemon off;
user www-data;
worker_processes ${NGINX_WORKER_PROCESSES};

events {
    worker_connections ${NGINX_WORKER_CONNECTIONS};
}

http {
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout ${NGINX_KEEPALIVE_TIMEOUT};

    client_max_body_size 50m;  # 上传文件大小限制

    gzip on;  # 启用压缩
    gzip_proxied any;
}
```

**关键特性**:

| 配置项 | 用途 |
|--------|------|
| `client_max_body_size 50m` | 限制请求体大小，支持大文件上传 |
| `gzip on` | 启用 HTTP 压缩，减少传输带宽 |
| `worker_connections` | 控制并发连接数 |
| `keepalive_timeout` | 保持连接超时设置 |

### 10.2 CLSI 服务 Nginx 配置

CLSI (编译服务) 有独立的 Nginx 配置 (`services/clsi/nginx.conf`)，负责 PDF 输出文件的访问控制。

```nginx
server {
  listen 8080;
  server_name clsi-nginx;
  disable_symlinks on;  # 禁止符号链接，防止目录遍历

  # PDF 范围请求缓存 (一天)
  location ~ ^/project/([0-9a-f]+)/content/([0-9a-f-]+/[0-9a-f]+)$ {
    expires 1d;
    alias /output/$1/content/$2;
  }

  # 编译输出文件路由
  location ~ ^/project/([0-9a-f]+)/build/([0-9a-f-]+)/output/(.+)$ {
    root /output/$1/generated-files/$2/;
  }
}
```

### 10.3 路由规则设计

**请求路由流程**:
```
客户端请求
    │
    ▼
Nginx (端口 80/443)
    │
    ├──► 静态资源 → 直接返回
    │
    ├──► Web Service (端口 3000) → Express.js 应用
    │
    ├──► Real-time Service (端口 3001) → Socket.IO WebSocket
    │
    └──► CLSI Service (端口 3013) → LaTeX 编译
```

**Web Service 内部路由** (`services/web/app/src/router.mjs`):

Overleaf 使用 Express Router 进行模块化路由:

```javascript
// 路由初始化 (router.mjs)
const webRouter = express.Router()
const privateApiRouter = express.Router()
const publicApiRouter = express.Router()

// 路由挂载
app.use(publicApiRouter)  // 公共 API
app.use(privateApiRouter) // 私有 API
app.use(webRouter)        // Web 页面路由
```

**主要路由模块**:

| 路由模块 | 路径 | 功能 |
|---------|------|------|
| `EditorRouter` | `/editor` | 富文本编辑器路由 |
| `ProjectApiController` | `/api/project` | 项目 REST API |
| `CompileController` | `/api/compile` | 编译控制 API |
| `HistoryRouter` | `/history` | 版本历史路由 |
| `ChatController` | `/api/chat` | 聊天消息 API |
| `FileStoreController` | `/api/file` | 文件存储 API |

---

## 11. Docker 部署架构

### 11.1 docker-compose.yml 分析

主部署配置文件 (`docker-compose.yml`) 定义了完整的 Overleaf 服务栈:

```yaml
services:
  sharelatex:           # 主应用服务
    image: sharelatex/sharelatex
    container_name: sharelatex
    ports:
      - "80:80"       # HTTP 端口映射
    depends_on:
      mongo:
        condition: service_healthy  # 等待 MongoDB 就绪
      redis:
        condition: service_started
    volumes:
      - ~/sharelatex_data:/var/lib/overleaf  # 持久化数据卷
    environment:
      OVERLEAF_APP_NAME: Overleaf Community Edition
      OVERLEAF_MONGO_URL: mongodb://mongo/sharelatex
      OVERLEAF_REDIS_HOST: redis
      SANDBOXED_COMPILES: "true"
      DOCKER_RUNNER: "true"

  mongo:               # MongoDB 6.0 副本集
    image: mongo:6.0
    command: "--replSet overleaf"

  redis:               # Redis 6.2 会话和缓存
    image: redis:6.2
```

### 11.2 服务级 docker-compose

每个微服务都有独立的 `docker-compose.yml` (`services/*/docker-compose.yml`)，用于开发和 CI 环境:

```yaml
# services/web/docker-compose.yml (示例)
services:
  test_unit:
    image: node:24.13.0
    volumes:
      - .:/overleaf/services/web
    working_dir: /overleaf/services/web
    env_file: docker-compose.common.env
```

### 11.3 数据卷管理

| 宿主机路径 | 容器内路径 | 用途 |
|-----------|----------|------|
| `~/sharelatex_data` | `/var/lib/overleaf` | 主数据目录 |
| `~/sharelatex_data/data/compiles` | `/var/lib/overleaf/data/compiles` | 编译临时文件 |
| `~/sharelatex_data/data/output` | `/var/lib/overleaf/data/output` | 编译输出 PDF |
| `~/mongo_data` | `/data/db` | MongoDB 数据 |
| `~/redis_data` | `/data` | Redis 持久化 |

### 11.4 环境变量配置

```bash
# 核心环境变量
OVERLEAF_APP_NAME          # 应用名称
OVERLEAF_MONGO_URL         # MongoDB 连接字符串
OVERLEAF_REDIS_HOST        # Redis 主机

# 编译相关
SANDBOXED_COMPILES         # 启用沙箱编译 "true"
DOCKER_RUNNER              # Docker 运行器 "true"

# 安全相关
SESSION_SECRET             # 会话密钥 (必需)
COOKIE_NAME               # Cookie 名称 "overleaf.sid"
```

---

## 12. 服务间通信机制

### 12.1 HTTP REST API

**使用场景**:
- 页面加载和模板渲染
- 项目 CRUD 操作 (创建、读取、更新、删除)
- 文件上传/下载
- 用户认证 (登录、注册)

**Web Service 内部 HTTP 调用**:
```javascript
// 服务间 API 调用配置 (services/web/config/settings.defaults.js)
apis: {
  web: {
    url: `http://${process.env.WEB_API_HOST || '127.0.0.1'}:3000`,
  },
  documentupdater: {
    url: `http://${process.env.DOCUMENT_UPDATER_HOST || '127.0.0.1'}:3003`,
  },
}
```

### 12.2 Socket.IO (WebSocket)

**使用场景**:
- 实时文档协作编辑
- 光标位置同步
- 聊天消息实时推送
- 用户在线状态
- 编译进度实时反馈

**Real-time Service WebSocket 配置** (`services/real-time/config/settings.defaults.cjs`):

```javascript
{
  redis: {
    pubsub: { host, port, password },
    realtime: {
      host, port, password,
      key_schema: {
        clientsInProject: `clients_in_project:{${project_id}}`,
        connectedUser: `connected_user:{${project_id}}:${client_id}`
      }
    },
  },
  gracefulReconnectTimeoutMs: 75000,  // 优雅重连超时
  shutdownDrainTimeWindow: 9,        // 关闭排空时间窗口
}
```

**主要 WebSocket 事件**:

```javascript
// 客户端 → 服务端
socket.emit('joinDoc', { doc_id, fromVersion, options })      // 加入文档
socket.emit('applyOtUpdate', { doc_id, update })             // 提交 OT 更新
socket.emit('getCursor', { doc_id })                         // 获取光标位置

// 服务端 → 客户端
socket.on('otUpdateApplied', callback)   // OT 更新已应用
socket.on('cursorUpdate', callback)      // 光标位置更新
socket.on('clientTracking.refresh', callback)  // 客户端追踪刷新
socket.on('otUpdateError', callback)      // OT 错误
```

### 12.3 Redis Pub/Sub

**使用场景**:
- Real-time Service 与 Document Updater 之间的文档更新同步
- 跨服务事件分发
- 消息队列 (pending updates)

**RealTimeRedisManager 实现** (`services/document-updater/app/js/RealTimeRedisManager.js`):

```javascript
const RealTimeRedisManager = {
  sendData(data) {
    const messageId = `doc:${HOST}:${RND}-${COUNT++}`
    data._id = messageId
    const blob = JSON.stringify(data)

    // 发布到 applied-ops 频道
    if (Settings.publishOnIndividualChannels) {
      return pubsubClient.publish(`applied-ops:${data.doc_id}`, blob)
    } else {
      return pubsubClient.publish('applied-ops', blob)
    }
  },
}
```

### 12.4 通信机制对比

| 机制 | 协议 | 特点 | 适用场景 |
|------|------|------|---------|
| HTTP REST | TCP | 同步请求/响应 | 页面加载、API 调用 |
| Socket.IO | WebSocket | 实时双向、低延迟 | 协作编辑、聊天 |
| Redis Pub/Sub | TCP | 发布/订阅、异步 | 服务间同步、事件分发 |
| Redis 队列 | TCP | 消息持久化 | 异步任务、pending updates |

---

## 13. 数据流架构

### 13.1 用户请求完整数据流

**场景: 用户打开并编辑一个 LaTeX 项目**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           用户请求流程                                        │
└─────────────────────────────────────────────────────────────────────────────┘

  1. 页面加载阶段
  ┌──────────┐     ┌──────────┐     ┌──────────────┐     ┌─────────────┐
  │ 浏览器    │────►│  Nginx    │────►│  Web Service  │────►│  MongoDB    │
  │          │     │ (反向代理) │     │  (Express)   │     │  (文档存储)  │
  └──────────┘     └──────────┘     └──────────────┘     └─────────────┘
       │                                      │
       │                                      │ 渲染页面，初始化 Socket.IO
       │                                      ▼
       │                             ┌──────────────┐
       │                             │ Real-time    │
       │                             │ Service      │
       │                             └──────────────┘

  2. WebSocket 连接建立
  ┌──────────┐     ┌──────────────┐     ┌──────────────┐
  │ 浏览器    │────►│ Real-time    │────►│ Redis        │
  │ Socket.IO│     │ Service      │     │ (订阅频道)    │
  └──────────┘     └──────────────┘     └──────────────┘
                         │
                         │ 加载文档内容
                         ▼
                 ┌──────────────┐
                 │ Document     │
                 │ Updater      │
                 └──────────────┘

  3. 文档编辑阶段 (OT 协作)
  ┌──────────┐     ┌──────────────┐     ┌──────────────┐
  │ 浏览器    │────►│ Real-time    │────►│ Document     │
  │          │     │ Service      │     │ Updater      │
  │ 发送 OT  │     │              │     │ (OT 处理)    │
  │ Update   │     │              │     │              │
  └──────────┘     └──────────────┘     └──────────────┘
                         │                    │
                         │                    │ Redis Pub/Sub
                         │                    │ 发布更新
                         │◄───────────────────┘
                         │
                         │ 广播更新到所有客户端
                         ▼
  ┌──────────┐     ┌──────────────┐
  │ 其他用户  │◄────│ Real-time    │
  │ 浏览器    │     │ Service      │
  └──────────┘     └──────────────┘
```

---

## 14. Session 管理

### 14.1 Redis Session 存储

Overleaf 使用 `connect-redis` 结合自定义 `CustomSessionStore` 实现会话管理。

**CustomSessionStore 特性** (`services/web/app/src/infrastructure/CustomSessionStore.mjs`):

```javascript
class CustomSessionStore extends RedisStore {
  constructor({ client }) {
    super({ client })
    this.#updateInPlaceStore = new RedisStore({
      client: new CustomSetRedisClient(client, 'XX'),
    })
    this.#initialSetStore = new RedisStore({
      client: new CustomSetRedisClient(client, 'NX'),
    })
  }

  set(sid, sess, cb) {
    // 刷新验证令牌
    sess.validationToken = computeValidationToken(sid)

    if (sid === originalId || sid === sess.req.newSessionId) {
      this.#updateInPlaceStore.set(sid, sess, cb)  // XX
    } else {
      this.#initialSetStore.set(sid, sess, cb)     // NX
    }
  }
}
```

**Redis 会话键结构**:
```
overleaf.sid:{sessionId}  →  会话数据 (JSON)
```

**会话数据结构**:
```javascript
{
  "cookie": { "maxAge": 86400000, "secure": true, "httpOnly": true },
  "user": { "id": "user_id", "email": "user@example.com" },
  "validationToken": "v1:xxxx",  // 安全验证令牌
  "loginToken": { "hashedToken": "...", "createdAt": "..." }
}
```

### 14.2 Validation Token 机制

验证令牌防止会话固定攻击:

```javascript
function computeValidationToken(sid) {
  return 'v1:' + sid.slice(-4)  // 基于 sessionID 的确定性函数
}

function checkValidationToken(sid, sess) {
  const sessionToken = sess.validationToken
  if (sessionToken) {
    const clientToken = computeValidationToken(sid)
    if (sessionToken === clientToken) {
      return true
    }
  }
  return false
}
```

---

## 15. 错误处理与监控

### 15.1 错误处理架构

Overleaf 使用 `OError` (Overleaf Error) 类提供结构化错误处理:

**OError 特性** (`libraries/o-error/index.cjs`):

```javascript
class OError extends Error {
  constructor(message, info, cause) {
    super(message)
    this.name = this.constructor.name
    this.info = info    // 额外上下文信息
    this.cause = cause   // 底层错误
  }

  withInfo(info) {
    this.info = info
    return this
  }

  withCause(cause) {
    this.cause = cause
    return this
  }
}
```

### 15.2 日志系统

**Bunyan Logger 配置** (`libraries/logger/logging-manager.js`):

```javascript
const LoggingManager = {
  initialize(name, options = {}) {
    this.isProduction = process.env.NODE_ENV === 'production'
    this.defaultLevel = process.env.LOG_LEVEL ||
      (this.isProduction ? 'info' : 'debug')

    this.logger = bunyan.createLogger({
      name,
      serializers: { err: Serializers.err, req: Serializers.req },
      streams: options.streams ?? [this._getOutputStreamConfig()],
    })
  },
}
```

**日志级别**:
| 级别 | 用途 |
|------|------|
| `fatal` | 进程即将终止的错误 |
| `error` | 操作失败的错误 |
| `warn` | 潜在问题警告 |
| `info` | 一般信息 |
| `debug` | 调试信息 |

### 15.3 Prometheus 指标

**Metrics 模块** (`libraries/metrics/index.js`):

```javascript
// 指标类型
metrics.inc('counter_name', value, labels)      // 计数器
metrics.count('counter_name', value, labels)    // 批量计数
metrics.summary('metric_name', value, labels)   // 摘要
metrics.histogram('metric_name', value, buckets, labels)  // 直方图
metrics.timing('metric_name', timeMs, labels)   // 计时

// 定时器
const timer = new metrics.Timer('operation_name')
timer.done()  // 记录耗时
```

**指标端点**:
```
GET /metrics  # Prometheus 格式指标
```

---

## 16. 关键配置文件索引

| 配置文件 | 路径 |
|---------|------|
| 主 docker-compose | `/docker-compose.yml` |
| Nginx 配置 | `/server-ce/nginx/nginx.conf.template` |
| CLSI Nginx | `/services/clsi/nginx.conf` |
| Web Service 配置 | `/services/web/config/settings.defaults.js` |
| Real-time 配置 | `/services/real-time/config/settings.defaults.cjs` |
| Document Updater 配置 | `/services/document-updater/config/settings.defaults.js` |
| Redis 配置 | `/libraries/redis-wrapper/index.js` |
| Logger 配置 | `/libraries/logger/logging-manager.js` |
| Metrics 配置 | `/libraries/metrics/initialize.js` |

---

## 17. 参考资料

- 官方仓库: https://github.com/overleaf/overleaf
- CLSI 服务文档: `services/clsi/README.md`
- Document Updater: `services/document-updater/README.md`
