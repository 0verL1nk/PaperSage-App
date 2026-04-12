# Overleaf 实时协作功能架构分析

## 概述

Overleaf 的实时协作功能是其核心特性之一，基于 WebSocket 通信和操作转换(OT)算法实现多用户同步编辑。本文深入分析其架构设计与实现细节。

---

## 1. WebSocket 连接管理

### 1.1 服务架构

实时协作功能由 `services/real-time` 微服务处理，它是一个独立的 Socket.IO 服务器。

**核心文件**: `services/real-time/app.js`

```javascript
// 创建 HTTP 服务器和 Socket.IO 实例
const server = http.createServer(app)
const io = socketIO.listen(server, {
  logger: socketIoLogger,
})

// 配置传输协议
io.set('transports', ['websocket', 'xhr-polling'])
```

### 1.2 连接建立流程

**文件**: `services/real-time/app/js/WebsocketController.js`

```javascript
joinProject(client, user, projectId, callback) {
  // 1. 通过 WebApiManager 验证用户权限
  WebApiManager.joinProject(projectId, user, function (error, project, privilegeLevel, userMetadata) {
    // 2. 存储用户上下文信息
    client.ol_context = {}
    client.ol_context.privilege_level = privilegeLevel
    client.ol_context.user_id = userId
    client.ol_context.project_id = projectId
    // ...
  })
  
  // 3. 加入项目房间
  RoomManager.joinProject(client, projectId, callback)
}
```

### 1.3 房间管理

**文件**: `services/real-time/app/js/RoomManager.js`

Overleaf 使用 Socket.IO 的 room 概念来管理用户分组：

- **Project Room**: 每个项目一个房间 (`project-{projectId}`)
- **Doc Room**: 每个文档一个房间 (`doc-{docId}`)

```javascript
joinProject(client, projectId, callback) {
  this.joinEntity(client, 'project', projectId, callback)
}

joinDoc(client, docId, callback) {
  this.joinEntity(client, 'doc', docId, callback)
}

// 房间激活时发出事件
RoomEvents.emit(`${entity}-active`, id)  // project-active, doc-active
// 房间空时发出事件
RoomEvents.emit(`${entity}-empty`, id)    // project-empty, doc-empty
```

### 1.4 Session 与认证

**文件**: `services/real-time/app/js/SessionSockets.js`

使用 Redis 存储 session，实现跨节点会话共享：

```javascript
const sessionStore = new RedisStore({ client: sessionRedisClient })
const sessionSockets = new SessionSockets(io, sessionStore, cookieParser, Settings.cookieName)
```

### 1.5 用户状态存储

**文件**: `services/real-time/app/js/ConnectedUsersManager.js`

用户在线状态和光标位置存储在 Redis 中：

```javascript
// Redis key schema
Keys.clientsInProject({ project_id: projectId })  // Set: 项目中的所有客户端
Keys.connectedUser({ project_id, client_id })    // Hash: 用户详细信息

// 存储用户信息
multi.hset(Keys.connectedUser(...), 'user_id', user._id)
multi.hset(Keys.connectedUser(...), 'cursorData', JSON.stringify(cursorData))
```

---

## 2. 操作转换(OT)算法实现

### 2.1 核心技术栈

Overleaf 使用 **ShareJS** 作为 OT 算法的基础库，集成在 `services/document-updater` 服务中。

**关键文件**:
- `services/document-updater/app/js/sharejs/types/text-tp2.js` - TextTP2 类型实现
- `services/document-updater/app/js/sharejs/types/text-tp2-api.js` - TextTP2 API

### 2.2 文档更新流程

```
Client A (编辑) 
    │
    ▼
WebsocketController.applyOtUpdate()
    │
    ▼
DocumentUpdaterManager.queueChange() ──► Redis Queue
    │
    ▼
DispatchManager (Worker) ◄──────────── Redis BLPOP
    │
    ▼
UpdateManager.processOutstandingUpdatesWithLock()
    │
    ├──► ShareJS OT 转换
    │
    └──► 广播给其他客户端
```

### 2.3 文档版本控制

**文件**: `services/document-updater/app/js/RedisManager.js`

```javascript
// 每个文档维护一个版本号
Keys.projectHistoryVersion({ project_id, doc_id })  // 文档版本
Keys.projectHistoryOps({ project_id, doc_id })       // 操作日志
```

### 2.4 更新队列处理

**文件**: `services/document-updater/app/js/DispatchManager.js`

```javascript
// 使用 Redis BLPOP 实现阻塞队列
worker._waitForUpdateThenDispatchWorker(callback) {
  return worker.client.blpop(pendingListKey, 0, function (error, result) {
    const [projectId, docId] = Keys.splitProjectIdAndDocId(docKey)
    // 分发到 UpdateManager 处理
    UpdateManager.processOutstandingUpdatesWithLock(projectId, docId, callback)
  })
}
```

### 2.5 OT 操作类型

**文件**: `libraries/overleaf-editor-core/lib/operation/`

Overleaf 定义了多种操作类型：

```javascript
// 文本操作
edit_operation.js        // 文本插入/删除
edit_operation_transformer.js  // 操作转换

// 注释操作
add_comment_operation.js
delete_comment_operation.js

// 文件操作
add_file_operation.js
edit_file_operation.js
move_file_operation.js
delete_file_operation.js
```

### 2.6 操作数据结构

```javascript
// 插入操作
{ i: "text to insert", p: position }

// 删除操作
{ d: length, p: position }

// 保留操作（光标移动）
{ r: length, p: position }
```

---

## 3. 用户状态同步

### 3.1 光标位置同步

**文件**: `services/real-time/app/js/WebsocketController.js`

```javascript
updateClientPosition(client, cursorData, callback) {
  // cursorData 包含: row, column, doc_id
  ConnectedUsersManager.updateUserPosition(
    projectId,
    client.publicId,
    user,
    { row: cursorData.row, column: cursorData.column, doc_id: cursorData.doc_id },
    callback
  )
  
  // 广播给房间内其他用户
  WebsocketLoadBalancer.emitToRoom(
    projectId,
    'clientTracking.clientUpdated',
    cursorData
  )
}
```

### 3.2 在线用户列表

```javascript
getConnectedUsers(client, callback) {
  // 刷新请求
  WebsocketLoadBalancer.emitToRoom(projectId, 'clientTracking.refresh')
  
  // 延迟获取确保收到所有刷新响应
  setTimeout(() => {
    ConnectedUsersManager.getConnectedUsers(projectId, callback)
  }, WebsocketController.CLIENT_REFRESH_DELAY)  // 1000ms
}
```

### 3.3 用户断开处理

```javascript
leaveProject(io, client, callback) {
  // 通知其他用户
  WebsocketLoadBalancer.emitToRoom(
    projectId,
    'clientTracking.clientDisconnected',
    client.publicId
  )
  
  // 标记用户离线
  ConnectedUsersManager.markUserAsDisconnected(projectId, client.publicId)
  
  // 如果没有用户了，flush 文档到 MongoDB
  if (remainingClients.length === 0) {
    DocumentUpdaterManager.flushProjectToMongoAndDelete(projectId)
  }
}
```

---

## 4. 聊天/评论系统

### 4.1 服务架构

聊天功能由独立的 `services/chat` 微服务处理。

**核心文件**: `services/chat/app/js/Features/Messages/MessageManager.js`

### 4.2 消息存储

```javascript
// MongoDB collections
db.messages  // 聊天消息

// 消息结构
{
  content: String,      // 消息内容
  room_id: ObjectId,    // 项目 ID
  user_id: ObjectId,   // 发送者 ID
  timestamp: Date,     // 时间戳
  edited_at: Date      // 编辑时间（可选）
}
```

### 4.3 消息操作

```javascript
// 创建消息
async function createMessage(roomId, userId, content, timestamp) {
  return await db.messages.insertOne({
    content, room_id: roomId, user_id: userId, timestamp
  })
}

// 获取消息（分页）
async function getMessages(roomId, limit, before) {
  return await db.messages
    .find({ room_id: roomId, timestamp: { $lt: before } })
    .sort({ timestamp: -1 })
    .limit(limit)
    .toArray()
}

// 更新消息
async function updateMessage(roomId, messageId, userId, content, timestamp) {
  await db.messages.updateOne(
    { _id: messageId, room_id: roomId, user_id: userId },
    { $set: { content, edited_at: timestamp } }
  )
}
```

### 4.4 Thread 系统

**文件**: `services/chat/app/js/Features/Threads/ThreadManager.js`

支持对话线程功能，用于长篇讨论。

### 4.5 权限控制

```javascript
// 消息权限检查
async function deleteUserMessage(userId, roomId, messageId) {
  await db.messages.deleteOne({
    _id: new ObjectId(messageId),
    user_id: new ObjectId(userId),  // 只能删除自己的消息
    room_id: new ObjectId(roomId)
  })
}
```

---

## 5. 权限管理

### 5.1 权限级别

**文件**: `services/real-time/app/js/AuthorizationManager.js`

Overleaf 定义了多个权限级别：

```javascript
// 权限级别
OWNER_ACCESS      // 所有者
READ_AND_WRITE    // 读写
READ_ONLY         // 只读
NO_ACCESS         // 无权限
```

### 5.2 权限检查流程

```javascript
// 项目级权限检查
assertClientCanViewProject(client, callback)

// 文档级权限检查
assertClientCanViewProjectAndDoc(client, docId, callback)

// 编辑权限检查
assertClientCanEditProjectAndDoc(client, docId, callback)

// 审核权限检查（Track Changes）
assertClientCanReviewProjectAndDoc(client, docId, callback)
```

### 5.3 权限检查时机

```javascript
// joinDoc 时检查
_assertClientAuthorization(client, docId, callback) {
  // 1. 先检查项目级权限
  AuthorizationManager.assertClientCanViewProject(client)
  // 2. 再检查文档级权限
  AuthorizationManager.assertClientCanViewProjectAndDoc(client, docId)
  // 3. 如果没有缓存，检查 doc-updater
  DocumentUpdaterManager.checkDocument(projectId, docId)
}

// applyOtUpdate 时检查
_assertClientCanApplyUpdate(client, docId, update, callback) {
  if (this._isCommentUpdate(update)) {
    return assertClientCanViewProjectAndDoc(client, docId)
  } else if (update.meta?.tc) {  // Track Changes
    return assertClientCanReviewProjectAndDoc(client, docId)
  } else {
    return assertClientCanEditProjectAndDoc(client, docId)
  }
}
```

### 5.4 受限用户

```javascript
// 受限用户无法看到评论
if (isRestrictedUser && ranges && ranges.comments) {
  ranges.comments = []  // 过滤掉评论
}

// 受限用户无法看到在线用户列表
if (isRestrictedUser) {
  return callback(null, [])  // 返回空列表
}
```

---

## 6. 关键数据结构

### 6.1 客户端上下文

```javascript
client.ol_context = {
  privilege_level: String,     // 权限级别
  user_id: String,            // 用户 ID
  project_id: String,         // 项目 ID
  owner_id: String,           // 项目所有者 ID
  first_name: String,
  last_name: String,
  email: String,
  connected_time: Date,
  is_restricted_user: Boolean,
  is_token_member: Boolean,
  is_invited_member: Boolean
}
```

### 6.2 文档更新消息

```javascript
update = {
  v: Number,        // 文档版本号
  op: Array,        // 操作数组
  meta: {
    source: String,     // 客户端 publicId
    user_id: String,
    tsRT: Number        // 服务端接收时间戳
  }
}
```

### 6.3 光标数据

```javascript
cursorData = {
  id: String,       // 客户端 publicId
  user_id: String,
  name: String,
  row: Number,      // 行号
  column: Number,   // 列号
  doc_id: String    // 文档 ID
}
```

---

## 7. 服务间通信

### 7.1 Redis Pub/Sub

**文件**: `services/real-time/app/js/WebsocketLoadBalancer.js`

不同 real-time 节点之间通过 Redis Pub/Sub 同步事件：

```javascript
// 广播事件到所有节点
WebsocketLoadBalancer.emitToRoom(projectId, 'clientTracking.clientUpdated', cursorData)

// 监听来自其他节点的事件
WebsocketLoadBalancer.listenForEditorEvents(io)
```

### 7.2 频道

```javascript
'applied-ops'     // 文档操作更新
'editor-events'   // 编辑器事件（用户加入/离开等）
```

### 7.3 Document Updater 集成

**文件**: `services/real-time/app/js/DocumentUpdaterController.js`

```javascript
// 监听 document-updater 的更新
DocumentUpdaterController.listenForUpdatesFromDocumentUpdater(io)
```

---

## 8. 错误处理

### 8.1 错误类型

**文件**: `services/real-time/app/js/Errors.js`

```javascript
CodedError
JoinLeaveEpochMismatchError   // joinDoc/leaveDoc 顺序错乱
NotAuthorizedError
NotJoinedError                 // 未加入项目
ClientRequestedMissingOpsError // 请求了不存在的操作版本
```

### 8.2 断线重连

```javascript
// 更新太大时触发
if ((error.message) === 'update is too large') {
  client.emit('otUpdateError', message.error, message)
  client.disconnect()  // 断开连接让客户端重试
}

// joinLeaveEpoch 不匹配时
if (joinLeaveEpoch !== client.joinLeaveEpoch) {
  callback(new JoinLeaveEpochMismatchError())
}
```

---

## 9. 健康检查与优雅关闭

### 9.1 健康检查

```javascript
// Redis 健康检查
rclient.healthCheck(function (error) {
  if (error) return res.sendStatus(500)
})

// PubSub 健康检查
HealthCheckManager.isFailing()
```

### 9.2 优雅关闭

```javascript
// 停止接收新连接
DrainManager.startDrainTimeWindow(io, shutdownDrainTimeWindow, () => {
  // 等待所有客户端断开
  shutdownAfterAllClientsHaveDisconnected()
})

// 强制断开超时客户端
setTimeout(() => {
  staleClients.forEach(client => client.disconnect())
}, Settings.gracefulReconnectTimeoutMs)
```

---

## 10. 性能优化

### 10.1 指标监控

```javascript
Metrics.event_loop.monitor(logger)
Metrics.open_sockets.monitor()
Metrics.injectMetricsRoute(app)

// Histogram buckets for join latency
JOIN_DOC_CATCH_UP_LENGTH_BUCKETS = [0, 5, 10, 25, 50, 100, 150, 200, 250, 500, 1000]
```

### 10.2 用户超时

```javascript
const USER_TIMEOUT_IN_S = ONE_HOUR_IN_S / 4  // 15分钟
const REFRESH_TIMEOUT_IN_S = 10  // 10秒内未响应的客户端被认为离线
```

---

## 11. OT 客户端状态机

### 11.1 状态定义

**文件**: `libraries/overleaf-editor-core/lib/ot_client.js`

```javascript
const STATE_DISCONNECTED = 0  // 断开连接
const STATE_LOADING = 1       // 加载文档中
const STATE_READY = 2         // 就绪，可接受本地和远程操作
const STATE_WAITING = 3       // 等待服务器确认（已发送本地操作）
```

### 11.2 状态转换

| 状态 | 转换条件 | 目标状态 |
|-----|---------|---------|
| DISCONNECTED | connect() 调用 | LOADING |
| LOADING | 收到 `load` 事件 | READY |
| READY | 发送本地操作 | WAITING |
| WAITING | 收到 `ack` 确认 | READY（同时处理 buffer 中的远程操作）|

### 11.3 Change 结构

```javascript
class Change {
  operations: Operation[]      // 操作列表
  timestamp: Date              // 时间戳
  authors: Author[]           // 作者 ID 列表
  origin: Origin               // 变更来源 (e.g., 'restore', 'external')
  v2Authors: string[]         // V2 格式作者
  projectVersion: string      // 项目版本 "major.minor"
  v2DocVersions: V2DocVersions // 各文档版本
}
```

### 11.4 Operation 类型层次

```
Operation (基类)
├── NoOperation              // 空操作
├── AddFileOperation        // 添加文件
├── MoveFileOperation       // 移动/重命名/删除文件
├── EditFileOperation       // 编辑文件内容
│   └── TextOperation      // 文本操作 (retain/insert/remove)
└── SetFileMetadataOperation // 设置文件元数据
```

### 11.5 OT Transform 核心不变量

```
apply(apply(S, A), B') = apply(apply(S, B), A')
```

即：无论操作 A 和 B 以何种顺序并发应用，最终文档状态应一致。

---

## 12. Room 与 Channel 分离架构

### 12.1 双层架构

Overleaf 使用**两层订阅架构**来实现高效的跨节点消息分发：

| 层 | 组件 | 职责 |
|---|------|------|
| Socket.IO 层 | RoomManager | 管理客户端所在的 room |
| Redis Pub/Sub 层 | ChannelManager | 管理跨节点的消息分发 |

### 12.2 ChannelManager 防重复订阅

**文件**: `services/real-time/app/js/ChannelManager.js`

```javascript
// 防止同一节点重复订阅同一 channel
ClientMap: Map<rclient, Map<channel, subscribePromise>>

subscribe(channel) {
  if (this.ClientMap.get(rclient)?.has(channel)) {
    return this.ClientMap.get(rclient).get(channel)  // 返回已有的 promise
  }
  // ... 执行订阅
}
```

### 12.3 协作流程

```
RoomManager.joinEntity() 触发 room 激活
        │
        ▼
RoomEvents.emit('project-active', projectId)
        │
        ▼
WebsocketLoadBalancer.handleRoomUpdates() 监听事件
        │
        ▼
ChannelManager.subscribe() 订阅 Redis 频道
```

---

## 13. 整体架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Client (Browser)                             │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────────────────┐ │
│  │  OT Client  │  │  Chat UI     │  │  Connected Users Display   │ │
│  │  (状态机)   │  │              │  │                           │ │
│  └──────┬──────┘  └──────┬───────┘  └──────────────┬──────────────┘ │
└─────────┼────────────────┼──────────────────────────┼───────────────┘
          │                │                          │
          ▼                ▼                          ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    Services (Node.js)                                │
│  ┌────────────────────────────────────┐  ┌─────────────────────────┐ │
│  │        real-time Service           │  │      chat Service       │ │
│  │                                    │  │                         │ │
│  │  Router.js                         │  │  MessageManager.js      │ │
│  │  (Socket.IO events)                │  │  ThreadManager.js       │ │
│  │                                    │  │                         │ │
│  │  WebsocketController.js            │  │         MongoDB         │ │
│  │  - joinProject/joinDoc             │  │  ┌─────┐ ┌──────────┐   │ │
│  │  - applyOtUpdate                   │  │  │rooms│ │ messages │   │ │
│  │  - updateClientPosition             │  │  └─────┘ └──────────┘   │ │
│  │                                    │  │                         │ │
│  │  WebsocketLoadBalancer.js ─────────┼──┼──► Redis pub/sub         │ │
│  │  (跨实例消息分发)                   │  │                         │ │
│  │                                    │  │                         │ │
│  │  RoomManager.js                    │  │                         │ │
│  │  ChannelManager.js                 │  │                         │ │
│  └────────────────────────────────────┘  └─────────────────────────┘ │
│                              │                                        │
│                              ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────┐ │
│  │                      Document Updater Service                    │ │
│  │  - 文档内容管理 (lines, version, ranges)                        │ │
│  │  - OT 操作应用 & 转换                                            │ │
│  │  - Redis 存储 pending updates                                    │ │
│  └─────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 14. 关键消息类型

| 消息名 | 方向 | 内容 |
|-------|------|------|
| `joinProject` | C→S | 加入项目 |
| `joinProjectResponse` | S→C | 返回项目和权限信息 |
| `joinDoc` | C→S | 加入文档 |
| `applyOtUpdate` | C→S | 提交 OT 操作 |
| `otUpdateApplied` | S→C | 操作已应用确认 |
| `change` | S→C | 远程 OT 操作 |
| `ack` | S→C | 操作确认 |
| `clientTracking.updatePosition` | C→S | 更新光标位置 |
| `clientTracking.clientUpdated` | S→C | 其他用户光标更新 |
| `clientTracking.getConnectedUsers` | C→S | 获取在线用户 |
| `clientTracking.refresh` | S→S | 刷新用户活跃状态 |

---

## 总结

Overleaf 的实时协作系统是一个复杂的分布式架构，核心组件包括：

1. **WebSocket 服务** (`services/real-time`): 处理连接管理和房间路由
2. **Document Updater 服务** (`services/document-updater`): 处理 OT 算法的实际执行
3. **Chat 服务** (`services/chat`): 处理实时聊天和评论
4. **Redis**: 用于状态存储、Pub/Sub 和消息队列

**核心技术特点**：

- **Socket.IO + Redis Pub/Sub**: 实现跨节点实时消息分发
- **OT 状态机**: 4 状态（DISCONNECTED/LOADING/READY/WAITING）管理并发编辑
- **Room-Channel 分离**: Socket.IO room 管理本地客户端，Redis channel 管理跨节点通信
- **版本向量**: 每个文档维护版本号，确保操作顺序一致
- **权限分层**: 项目级 + 文档级双重权限检查

这套架构通过版本向量、房间隔离和权限分层，实现了高效且安全的实时协作编辑体验。

---

## 15. OT 算法的数学证明

### 15.1 收敛性不变量（Convergence Invariant）

OT 算法的核心数学不变量为**交换律**：

```
apply(apply(S, A), B') = apply(apply(S, B), A')
```

其中：
- `S` 为共享文档的初始状态
- `A`, `B` 为两个并发操作
- `A' = transform(B, A)` — 将 `A` 相对于 `B` 转换后的结果
- `B' = transform(A, B)` — 将 `B` 相对于 `A` 转换后的结果

### 15.2 操作转换算法

| 组合 | 处理策略 |
|------|---------|
| Insert vs Insert | 优先处理 `op1`（本地优先），对 `op2` 执行 `retain(ins.length)` |
| Insert vs Delete | 插入位置不受删除影响 |
| Delete vs Insert | 删除操作需调整位置以反映并发插入 |
| Delete vs Delete | 相同位置删除需去重 |

---

## 16. 冲突解决策略

### 16.1 三类冲突场景

| 冲突类型 | 示例 | 解决策略 |
|---------|------|---------|
| **文本操作冲突** | 两用户同时在位置 P 插入不同文本 | 位置优先 + 游标调整 |
| **文本-注释冲突** | 用户 A 编辑文本，用户 B 在同一位置添加评论 | 注释随文本移动 |
| **评论状态冲突** | 两用户同时 resolve 同一评论 | 后到者的 `SetCommentState` 被转为 `NoOp` |

### 16.2 版本过旧（Stale Version）处理

当客户端请求一个已经过期的文档版本时，服务器返回 `ClientRequestedMissingOpsError`：

```javascript
if (error instanceof ClientRequestedMissingOpsError) {
  emitJoinDocCatchUpMetrics('missing', error.info)
  return callback(error)
}
```

客户端收到错误后需要重新 `joinDoc` 获取最新快照。

---

## 17. Cursor 渲染细节

### 17.1 光标颜色分配算法

Overleaf 使用基于用户 ID 的 **HSL 色相（Hue）分配**：

```typescript
const ANONYMOUS_HUE = 100
const OWN_HUE = 200

export function getHueForUserId(userId?: string): number {
  if (userId == null || userId === 'anonymous-user') {
    return ANONYMOUS_HUE
  }
  if (getMeta('ol-user_id') === userId) {
    return OWN_HUE
  }
  let hue = getHueForId(userId)
  if (hue > OWN_HUE - OWN_HUE_BLOCKED_SIZE &&
      hue < OWN_HUE + OWN_HUE_BLOCKED_SIZE) {
    hue = hue - OWN_HUE + TOTAL_HUES - OWN_HUE_BLOCKED_SIZE
  }
  return hue
}
```

颜色通过 MD5 哈希从用户 ID 派生，保证同一用户每次访问获得相同颜色。

### 17.2 光标节流

```typescript
const cursorUpdateInterval =
  onlineUsersArray.length > 0 ? 500 : 60 * 1000 * 5
// 多用户协作时每 500ms 发送一次
// 独自编辑时每 5 分钟才检查一次
```

---

## 18. 离线支持

### 18.1 客户端本地操作队列

Overleaf 的 OT 客户端在断网时将操作缓存：

```javascript
this.handleLocalOperation = function(operation) {
  switch (_state) {
    case STATE_READY:
      sendLocalOperation(operation)  // 在线：直接发送
      break
    case STATE_WAITING:
      queueLocalOperation(operation)  // 离线：加入 pending
      break
  }
}
```

### 18.2 离线恢复流程

```
1. 客户端断线  → OtClient._state = STATE_DISCONNECTED
2. 网络恢复    → ConnectionManager.onOnline()
3. 重连成功    → socket.emit('authenticate')
                → socket.on('load')  重新加载文档快照
                → socket.on('ack')   按顺序确认 _outstanding 和 _pending 中的操作
```

---

## 19. 性能优化

### 19.1 批量更新处理

Document Updater 使用 `MAX_OPS_PER_ITERATION = 8` 限制每次循环处理的操作数：

```javascript
const MAX_OPS_PER_ITERATION = 8

getPendingUpdatesForDoc(docId, callback) {
  multi.lrange(
    Keys.pendingUpdates({ doc_id: docId }),
    0,
    MAX_OPS_PER_ITERATION - 1  // 每次最多取 8 条
  )
}
```

### 19.2 光标更新节流

```typescript
const cursorUpdateInterval =
  onlineUsersArray.length > 0 ? 500 : 60 * 1000 * 5
```

---

## 20. 断线重连

### 20.1 重连策略

| 策略 | 触发条件 | 行为 |
|------|---------|------|
| **自动倒计时重连** | 任意断线 | 3-10 秒随机延迟后重连 |
| **用户活动触发的重连** | 用户在编辑器中活动 | 5 秒后尝试重连 |
| **立即重连** | 用户点击重连按钮 | `tryReconnectNow()` |
| **优雅重连** | 服务端发送 `reconnectGracefully` | 分段延迟重连 |

### 20.2 WebSocket 传输升级

重连时优先使用 WebSocket，失败 3 次后降级到 XHR 轮询：

```typescript
this.socket.socket.transports = ['xhr-polling']
if (this.websocketFailureCount < RETRY_WEBSOCKET) {
  this.socket.socket.transports.unshift('websocket')  // 优先 websocket
}
```

---

## 21. 聊天系统

### 21.1 Thread 模型

```
Project
  │
  ├── GLOBAL_THREAD (room_id: null)  — 项目级全局聊天
  │
  └── Comment Thread (room_id: ObjectId)  — 每个评论一个 thread
       │
       └── Messages[]  — 该 thread 下的消息列表
```

### 21.2 Thread 生命周期

```javascript
// 解决（Resolve）Thread
export async function resolveThread(projectId, threadId, userId) {
  await db.rooms.updateOne(
    { project_id, thread_id: threadId },
    { $set: { resolved: { user_id: userId, ts: new Date() } } }
  )
}

// 重新打开（Reopen）Thread
export async function reopenThread(projectId, threadId) {
  await db.rooms.updateOne(
    { project_id, thread_id: threadId },
    { $unset: { resolved: true } }
  )
}
```

### 21.3 消息存储结构

```javascript
// MongoDB: db.messages
{
  _id: ObjectId,
  content: String,       // 消息内容（最大 10KB）
  room_id: ObjectId,    // 关联的 Thread room
  user_id: ObjectId,    // 发送者
  timestamp: Date,
  edited_at: Date
}

// MongoDB: db.rooms
{
  _id: ObjectId,
  project_id: ObjectId,
  thread_id: ObjectId,  // GLOBAL_THREAD 为 null
  resolved: { user_id: ObjectId, ts: Date }
}
```

---

## 22. 参考资料

- ShareJS OT: https://github.com/share/ShareJS
- Socket.IO: https://socket.io/
- Overleaf Editor Core: `libraries/overleaf-editor-core/`
