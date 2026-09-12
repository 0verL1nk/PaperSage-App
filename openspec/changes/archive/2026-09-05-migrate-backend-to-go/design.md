# Design: 后端 Go 迁移与单二进制交付

## Context

现有后端：FastAPI + SQLAlchemy 2.0 async + Pydantic，约 6,100 行实现 + 2,750 行 pytest（19 个测试文件）。7 组路由 49 个端点，8 张表（users、user_settings、project_groups、projects、folders、documents、latex_files、compile_jobs）。外部依赖：PostgreSQL、Redis（内存兜底）、S3 兼容存储（R2/MinIO）、CLSI（`latex_render/`，Node，端口 3013）、邮件（Resend/SMTP）。

前端为 Vite SPA（TanStack Router 路由，history 模式），Axios baseURL 默认取 `VITE_API_BASE_URL`，未设置时为同源相对路径（`frontend/src/lib/env.ts`）——这正是单二进制模式需要的形态。Playwright E2E（7 个域目录）已锁定后端行为，作为等价性验收手段。

## Goals / Non-Goals

**Goals:**
- 单个 Go 二进制：内嵌前端产物，一个进程、一个端口同时提供 SPA 与 `/api/v1` API
- 行为等价移植：API 路径、请求/响应格式、状态码、错误信封逐字段对齐现有实现
- 既有 JWT（同 `SECRET_KEY` 签发）在新实现下仍然有效，用户无感切换
- 数据库 schema 零变更，Alembic → golang-migrate 平滑交接
- Playwright E2E 套件对 Go 二进制全绿作为切换门槛
- 确定 AI 模块的 Agent 基座选型（本变更只选型不实现）
- CI / Docker / docker-compose 同步切换

**Non-Goals:**
- 不改任何 API 行为、不加新端点（发现 bug 单独提 issue）
- 不重写 CLSI（继续作为外部 Node 服务）
- 不做性能优化（等价优先，性能提升是自然收益）
- 不实现 agent / LLM 功能（roadmap v0.5 的独立变更）
- 不做 PostgreSQL 之外的数据库支持收敛（SQLite 仅用于测试）

## Decisions

### D1: Web 框架 — gin（2026-09-05 修订：原选 chi，按用户决策改为 gin）

**选择**: [`github.com/gin-gonic/gin`](https://github.com/gin-gonic/gin) v1.10.x + `gin-contrib/cors`。

**理由**: gin 是 Go 最主流的 Web 框架，资料与社区支持最广（对毕设答辩友好），生态中间件齐全（CORS/验证/日志等均有现成 gin-contrib 组件，符合「不造轮子」原则）。注意钉住 v1.10.x——2026 年的最新版要求 Go ≥ 1.25，而本机工具链为 1.23。

**实现要点**（与 FastAPI 行为对齐）:
- `HandleMethodNotAllowed = true` + `NoMethod`/`NoRoute` 自定义 handler → 404/405 走信封（对齐 Starlette HTTPException 语义）
- panic 恢复用自定义中间件而非 `gin.Recovery()`（后者输出纯文本 500，不满足信封契约）
- CORS 用 `gin-contrib/cors`（`AllowAllOrigins` + expose `Content-Disposition`）

**替代方案**:
- chi — 更贴近 stdlib、零依赖，但生态规模小于 gin（原 D1 选择，已被用户决策取代）
- 纯 stdlib — 中间件全要自建，违背不造轮子原则
- fiber — fasthttp 而非 net/http，与 `go:embed`/`http.FileServer` 标准设施配合差

### D2: 数据访问 — GORM + pgx

**选择**: `gorm.io/gorm` + `gorm.io/driver/postgres`（底层 pgx），JSONB 字段用 `gorm.io/datatypes.JSON`。保持现有 repository/service 分层：`internal/store` 提供按域拆分的 Store，路由层不直接摸 DB。

**理由**: GORM 的模型定义/关联/查询构造与 SQLAlchemy 心智模型最接近，8 张表的移植成本最低；关系（user→projects、project→folders/documents、compile_jobs）映射直接。

**替代方案**:
- sqlc — 编译期校验 SQL 最安全，但 500+ 查询逐条改写 SQL 工作量大，且失去与现有 Python 代码的形状对应
- ent — schema-as-code 强类型，但学习曲线与代码生成物复杂度高
- 纯 sqlx/pgx — 全手写，工作量大

**已知对齐风险（移植时重点测试）**:
- 时间序列化：Pydantic 输出 ISO 8601（无时区后缀），Go `time.Time` 默认 RFC3339 带时区。统一在 DTO 层格式化为与现网一致的字符串形态，以前端解析为准回归
- UUID 主键：沿用 `google/uuid`，值由应用生成（与现 Python 行为一致），不依赖 DB 生成

### D3: 数据库迁移 — golang-migrate + baseline 对接

**选择**: `golang-migrate/migrate`，SQL 迁移文件 embed 进二进制，启动时自动 `up`（替代现 `app/main.py` lifespan 里的 `alembic upgrade head` + `create_all` 兜底）。

**交接策略**: 以当前 Alembic head 的 schema 生成 baseline SQL（`0001_baseline.up.sql`）。对已有部署：Go 启动时检测 Alembic 的 `alembic_version` 表，若存在且版本匹配 baseline 对应版本，则在 `schema_migrations` 表写入对应版本号后接管；全新部署直接从 baseline 起跑。此后的 schema 变更一律走 golang-migrate，Alembic 随 Python 后端一并退役。

### D4: 认证兼容 — 逐字段复刻现有语义

- JWT: `golang-jwt/jwt/v5`，HS256、同一 `SECRET_KEY`、相同 claims（`type: "access" | "refresh"`、sub、exp）→ **切换后现有 token 不失效**
- 口令: 前端 SHA-256 → 后端 `golang.org/x/crypto/bcrypt` 验证，与现有 bcrypt 哈希完全兼容（存量用户可直接登录）
- 登录/注册保持 `application/x-www-form-urlencoded` 表单体（`r.ParseForm`）
- Refresh token 轮换、缓存黑名单语义照搬 `core/token.py` + `core/cache.py`
- **错误响应形状逐字段复刻** `app/exceptions.py` 的三个 handler（HTTPException、RequestValidationError 422、generic 500），信封 `{code, data, info}`；校验错误 `detail` 数组结构以前端 `services/errors.ts` 的解析路径为准写回归测试

### D5: 缓存、存储与邮件

- 缓存: `redis/go-redis/v9`（URL 解析复用官方 `redis.ParseURL`），未配置 `REDIS_URL` 时回退 `patrickmn/go-cache` 内存实现（不手搓 TTL map），保持现有双实现抽象
- S3: `aws-sdk-go-v2/s3`，自定义 `BaseEndpoint` + path-style（MinIO/R2 兼容），预签名 URL、并行拉取、公共 URL 逻辑照搬 `services/storage/s3_client.py`
- 邮件: Resend 优先用官方/社区 Go SDK（`resend/resend-go`），SMTP 走成熟库（`wneessen/go-mail`，不手搓 net/smtp 协议细节）；`EMAIL_PROVIDER` 环境变量语义不变

### D6: LaTeX 编译管线 — HTTP 客户端移植 + goroutine 工作池

- CLSI 客户端: `net/http` 移植 `clsi_client.py`（编译请求、输出 zip 获取、cache 清理端点），CLSI 本身不动
- 异步编译队列: 用 goroutine + 信号量（容量 `MAX_CONCURRENT_COMPILES=3`）替代 Python asyncio 队列；`compile_jobs` 表语义、启动时孤儿任务恢复（`recover_on_startup`）、增量同步、超时与过期清理全部保留
- 编译状态仍为前端轮询（capability `compile-polling-ui` 不涉及 SSE/WebSocket，无长连接移植负担）

### D7: 前端嵌入与路由 — `go:embed` + SPA fallback

**约束**: `go:embed` 无法引用 Go module 目录之外的文件，因此约定构建产物目录 `server/web/`：

```
构建流水线: pnpm --dir frontend build → 复制 frontend/dist/* → server/web/
Dockerfile: stage1 pnpm build → stage2 COPY dist → go build → distroless
本地开发:  server/web/.keep 占位（embed 空目录），前端仍走 Vite 5173
```

**路由优先级**（单端口，默认沿用 5478）:

1. `/api/v1/*`、`/health` → API 路由
2. 静态文件: 从 embed FS 按路径精确匹配（`/assets/*` 等），`/assets/*` 设 `Cache-Control: public, max-age=31536000, immutable`（文件名带 content hash）
3. SPA fallback: 其余非文件路径（如 `/auth`、`/app/projects`）返回 `index.html` 并设 `no-cache`（保证发版后拿新 hash）
4. 保留 `Content-Disposition` expose 头（PDF/zip 下载）

**同源收益**: 前端未设 `VITE_API_BASE_URL` 时即同源相对请求，无需 CORS；开发模式 CORS 中间件保留（Vite 5173 跨端口）。

### D8: Agent 基座 — 选 Eino，不选 ADK-Go

2026-09 调研结论（详细对比见 tasks.md 引用的调研来源）。Go 侧「完整基座」候选：**Eino**（CloudWeGo/字节）、**ADK-Go**（Google）、LangChainGo、Genkit Go、Microsoft Agent Framework Go（2026-07 preview，排除）。

**对比（对本项目要紧的维度）**:

| 维度 | Eino (cloudwego/eino) | ADK-Go (google/adk-go) |
|---|---|---|
| 背书/成熟度 | 字节跳动，内部数百服务生产验证，~12k stars，持续高频迭代 | Google，1.0 稳定版（2026-03），~8.6k stars |
| 定位 | 完整 LLM 应用框架（组件 + Chain/Graph/Workflow + ADK agent 层） | Agent 专用工具包（Python ADK 的 Go 对应物） |
| 编排 | 有向图 + 工作流，支持分支/循环/并行，类型安全 | Sequential / Parallel / Loop 三种编排，图能力较浅 |
| 多智能体 | 5 种协作模式 + 8 种 HITL 模式 + interrupt/resume | 内置多 agent 委派 + A2A 协议 + Sessions/Memory 模块 |
| 模型接入 | OpenAI / Anthropic / Gemini / **DeepSeek** / 豆包 / Ollama / 火山 Ark（eino-ext 一等公民） | Gemini/Vertex 最顺，其他模型走 OpenAI 兼容层/LiteLLM，非一等公民 |
| 流式 | 框架级自动拼接/装箱/合并 | 基础 |
| 可观测 | 三级切面钩子（全局/组件类型/节点） | 依赖 GCP 生态 |
| MCP | 支持 | 支持 |
| 生态倾向 | CloudWeGo（Kitex/Hertz）家族、中文文档社区强 | GCP/Cloud Run 部署集成、A2A 跨框架互通 |
| 风险 | 与 Python 生态无互操作；多实例下 interrupt/resume 状态外置尚在打磨 | 较新（1.0 刚半年），文档偏 Google 生态，非 Gemini 模型是二等路径 |

**选择 Eino 的决定性理由**:
1. PaperSage 的 agent 场景是「论文写作全流程编排」（大纲→分节写作→引用→润色，多步、有分支、需要人机协同中断），Eino 的图编排 + interrupt/resume 正对这个形状；ADK 的三模式编排偏浅，A2A 跨框架互通对本项目无用
2. 默认模型 deepseek（user-settings 默认值）：Eino 一等公民支持 DeepSeek 及 OpenAI 兼容 API；ADK-Go 的最优路径是 Gemini/GCP
3. 自托管单二进制部署（自有服务器/docker-compose），不在 GCP 生态，ADK 的云集成优势用不上
4. 中文文档与社区对毕业设计迭代效率有实际价值

**何时该反过来选 ADK-Go**: 如果 agent 需求最终收敛为「单模型 + 少量工具的直线调用」（无复杂分支），那其实两家都可以不用——Go 惯例是手写 40 行 agent loop（goroutine + context 已提供并发/取消语义）。届时可降级为裸 SDK。本决策按「写作全流程编排」的复杂度假设成立。

**落地方式**: 本变更不引入 eino 依赖；AI 模块（独立 OpenSpec 变更）在 `server/internal/agent/` 下接 `cloudwego/eino-ext` 组件（DeepSeek/OpenAI 兼容），模型偏好沿用 user-settings 的 `agent` 分类。

### D9: 测试策略

- 单元/集成: Go 标准 `testing` + `net/http/httptest`；DB 用 SQLite（`glebarez/sqlite` 纯 Go 驱动，CI 免 CGO），与现 pytest 的 SQLite 测试策略一致；S3 用 MinIO 容器（对齐现 moto 用法的覆盖面）；CLSI 用 httptest 假服务
- 端到端: Playwright 套件原样保留，`webServer` 改为启动单个二进制（+ MinIO/Postgres/CLSI）；`scripts/seed_e2e_user.py` 移植为 `server/cmd/seed`
- 验收门槛: E2E 对 Go 二进制全绿才允许删除 Python 后端

### D10: 仓库布局与切换计划

布局依据 2026-09 Go 工程实践调研（详见 `docs/backend/GO-BEST-PRACTICES.md`）：Go 官方 server 项目模式——实现全进 `internal/`、命令进 `cmd/`、**不设 `pkg/`**；internal 按域分包，GORM 模型集中在 `store` 防跨域 import 环，接口定义在消费方域包内。

```
server/
├── cmd/
│   ├── papersage/     # 主程序入口（唯一职责：装配依赖并启动）
│   └── seed/          # E2E 数据种子（替代 Python 脚本）
├── internal/
│   ├── config/        # env 配置（变量名与 Python 版 1:1）
│   ├── httpapi/       # chi 装配、中间件、信封/错误处理
│   ├── server/        # HTTP server 生命周期、优雅停机、SPA/embed 静态服务
│   ├── auth/          # token、bcrypt、认证域 handler+service
│   ├── projects/      # 项目/分组/文件夹/回收站域
│   ├── documents/     # 文档上传域
│   ├── latex/         # CLSI 客户端、编译队列、latex_files 域
│   ├── settings/      # 用户设置域
│   ├── store/         # DB 句柄 + GORM 模型（集中存放）+ 迁移执行
│   ├── cache/         # Redis / 内存双实现
│   ├── storage/       # S3 客户端
│   └── email/         # Resend / SMTP
├── migrations/        # golang-migrate SQL（embed）
└── web/               # 前端构建产物（embed，构建时填充）
```

**切换顺序**: 并行开发期间 `server/` 与 `backend/` 并存，CI 同时跑两套 → E2E 对二进制全绿 → 单独 PR 删除 `backend/`、`frontend/Dockerfile`、切换 Makefile/compose/工作流 → 归档本变更。环境变量名全部保持（`DATABASE_URL` 改为 Go 驱动格式 `postgres://...`，迁移文档注明）。

**端口**: 二进制单端口默认 5478（`PORT` 可覆盖）；compose 侧 `${APP_PORT:-5173}:5478` 保持对外端口习惯。
