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

## 9. 参考资料

- 官方仓库: https://github.com/overleaf/overleaf
- CLSI 服务文档: `services/clsi/README.md`
- Document Updater: `services/document-updater/README.md`
