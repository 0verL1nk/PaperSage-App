# 业务流程图

## 1. 核心业务流程

### 1.1 用户登录流程

```mermaid
sequenceDiagram
    participant User as 用户
    participant Front as 前端
    participant API as 后端 API
    participant DB as 数据库

    User->>Front: 输入用户名密码
    Front->>Front: MD5 加密密码
    Front->>API: POST /api/auth/login
    API->>DB: 验证用户凭据
    DB-->>API: 用户信息
    API->>API: 生成 JWT Token
    API-->>Front: {accessToken, refreshToken, user}
    Front->>Front: 存储 Token 到 localStorage
    Front->>User: 跳转至项目列表
```

### 1.2 项目创建流程

```mermaid
sequenceDiagram
    participant User as 用户
    participant Front as 前端
    participant API as 后端 API
    participant DB as 数据库

    User->>Front: 点击"新建项目"
    Front->>Front: 显示项目创建表单
    User->>Front: 输入项目名称和描述
    Front->>API: POST /api/projects
    API->>DB: 创建项目记录
    DB-->>API: 项目数据
    API-->>Front: 项目数据
    Front->>User: 显示创建成功，跳转项目
```

### 1.3 文档上传流程

```mermaid
sequenceDiagram
    participant User as 用户
    participant Front as 前端
    participant API as 后端 API
    participant Storage as S3 存储
    participant DB as 数据库

    User->>Front: 选择要上传的文件
    Front->>Front: 检查文件类型和大小
    Front->>API: POST /api/documents/upload (分片)
    API->>Storage: 上传到对象存储
    Storage-->>API: 存储成功
    API->>DB: 创建文档记录
    DB-->>API: 文档数据
    API-->>Front: 文档数据
    Front->>User: 显示上传进度，完成后刷新列表
```

### 1.4 PDF 高亮流程

```mermaid
sequenceDiagram
    participant User as 用户
    participant Front as 前端
    participant API as 后端 API
    participant DB as 数据库

    User->>Front: 在 PDF 中选中文本
    Front->>Front: 显示高亮工具栏
    User->>Front: 选择高亮颜色
    Front->>API: POST /api/highlights
    API->>DB: 保存高亮记录
    DB-->>API: 高亮数据
    API-->>Front: 高亮数据
    Front->>Front: 渲染高亮效果
```

## 2. 用户界面流程

### 2.1 主导航流程

```mermaid
flowchart TD
    A[首页] --> B{已登录?}
    B -->|是| C[项目列表 /app/documents]
    B -->|否| D[登录页 /auth]
    D --> E{登录成功?}
    E -->|是| C
    E -->|否| D

    C --> F[项目列表]
    C --> G[回收站 /app/trash]
    C --> H[个人中心 /app/profile]

    F --> I[点击项目]
    I --> J[项目工作区]

    J --> K[上传模块]
    J --> L[编码模块]
    J --> M[分类模块]
    J --> N[逻辑链]
    J --> O[内容生成]
```

### 2.2 项目工作区流程

```mermaid
flowchart LR
    subgraph 项目工作区
        A[项目根] --> B[上传模块]
        B --> C[编码模块]
        C --> D[分类模块]
        D --> E[逻辑链]
        E --> F[内容生成]
    end

    A -.->|返回| G[项目列表]
    B -.->|点击| H[PDF预览]
    C -.->|点击| I[高亮面板]
```

## 3. 数据流图

### 3.1 前端数据流

```mermaid
flowchart TB
    subgraph UI["UI 组件层"]
        Pages[页面组件]
        Components[业务组件]
        UIElements[UI 元素]
    end

    subgraph State["状态管理层"]
        Router[TanStack Router]
        Query[TanStack Query]
        Store[Zustand]
    end

    subgraph API["API 层"]
        Client[Axios 客户端]
        Interceptors[拦截器]
    end

    subgraph Network["网络层"]
        HTTP[HTTP 请求]
    end

    Pages --> Router
    Components --> Query
    UIElements --> Store

    Router --> Client
    Query --> Client
    Store --> Client

    Client --> Interceptors
    Interceptors --> HTTP
```

### 3.2 后端数据流

```mermaid
flowchart TB
    subgraph Request["请求处理"]
        Route[路由]
        Middleware[中间件]
        Depends[依赖注入]
    end

    subgraph Business["业务层"]
        Schema[Pydantic 验证]
        Service[服务层]
        Model[数据模型]
    end

    subgraph Data["数据层"]
        ORM[SQLAlchemy]
        Cache[Redis]
        Storage[S3 兼容存储]
        DB[(SQLite/MySQL)]
    end

    Route --> Middleware
    Middleware --> Depends
    Depends --> Schema
    Schema --> Service
    Service --> Model
    Model --> ORM
    ORM --> DB
    ORM --> Cache
    Service --> Storage
```

## 4. 状态机

### 4.1 项目状态

```mermaid
stateDiagram-v2
    [*] --> Active: 创建项目
    Active --> Archived: 归档项目
    Archived --> Active: 取消归档
    Active --> Trash: 删除项目
    Trash --> Active: 恢复项目
    Trash --> [*]: 永久删除
```

### 4.2 文档状态

```mermaid
stateDiagram-v2
    [*] --> Processing: 开始上传
    Processing --> Ready: 处理完成
    Processing --> Error: 处理失败
    Ready --> Ready: 查看/编辑
    Ready --> Trash: 删除文档
    Trash --> Ready: 恢复文档
    Trash --> [*]: 永久删除
    Error --> Processing: 重试处理
```

### 4.3 上传状态

```mermaid
stateDiagram-v2
    [*] --> Idle: 初始状态
    Idle --> Selecting: 选择文件
    Selecting --> Uploading: 开始上传
    Uploading --> Processing: 上传完成
    Processing --> Success: 处理成功
    Processing --> Failed: 处理失败
    Success --> Idle: 重置
    Failed --> Uploading: 重试
```

## 5. 时序图

### 5.1 认证流程

```mermaid
sequenceDiagram
    participant User as 用户
    participant Browser as 浏览器
    participant API as API 服务器
    participant Redis as Redis

    Note over User,API: 登录流程
    User->>Browser: 输入用户名密码
    Browser->>API: POST /auth/login (MD5密码)
    API->>API: 验证密码
    API->>Redis: 记录会话
    API->>API: 生成 JWT
    API-->>Browser: 返回 tokens
    Browser->>Browser: 存储 tokens

    Note over User,API: 后续请求
    Browser->>API: GET /projects (Bearer Token)
    API->>API: 验证 JWT
    API-->>Browser: 项目列表

    Note over User,API: Token 刷新
    Browser->>API: POST /auth/refresh
    API->>API: 验证 refresh token
    API-->>Browser: 新 tokens
```

### 5.2 文件上传流程

```mermaid
sequenceDiagram
    participant User as 用户
    participant Browser as 浏览器
    participant API as API 服务器
    participant S3 as S3 兼容存储

    User->>Browser: 选择 PDF 文件
    Browser->>Browser: 验证文件类型
    Browser->>Browser: 生成分片（如有大文件）
    Browser->>API: POST /documents/upload (文件分片)

    loop 每个分片
        API->>S3: 上传分片
        S3-->>API: 分片存储成功
    end

    API->>API: 合并分片
    API->>API: 提取 PDF 元数据
    API-->>Browser: 文档创建成功

    Browser->>User: 显示上传成功
```

## 6. 架构图

### 6.1 系统架构总览

```mermaid
graph TB
    subgraph Client["客户端"]
        Web[Web 应用]
    end

    subgraph Gateway["网关层"]
        Nginx[Nginx]
    end

    subgraph Backend["应用层"]
        FastAPI[FastAPI]
    end

    subgraph Service["服务层"]
        Auth[认证服务]
        Project[项目服务]
        Document[文档服务]
        AI[AI 服务]
    end

    subgraph Data["数据层"]
        DB[(SQLite/MySQL)]
        Redis[(Redis)]
        S3[(S3 兼容存储)]
    end

    subgraph External["外部服务"]
        LLM[LLM API]
    end

    Web --> Nginx
    Nginx --> FastAPI
    FastAPI --> Auth
    FastAPI --> Project
    FastAPI --> Document
    FastAPI --> AI

    Auth --> PG
    Auth --> Redis
    Project --> PG
    Document --> PG
    Document --> S3
    AI --> LLM
```

---

*本章节通过多种图表（时序图、流程图、状态机）详细描述了 PaperSage 系统的核心业务流程。*