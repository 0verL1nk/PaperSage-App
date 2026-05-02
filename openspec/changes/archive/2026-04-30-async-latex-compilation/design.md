## Context

当前 LaTeX 编译是同步阻塞链路：Frontend POST → Backend await CLSI → 等待 latexmk 执行完毕 → 返回结果。编译耗时 10-120 秒不等，期间整个 HTTP 连接被占用。

**当前架构问题：**
1. HTTP 连接长时间占用，容易因超时断开
2. CLSI 无全局并发限制，多用户同时编译可能导致 latexmk 进程争抢资源，引发 OOM
3. 编译失败无重试机制，用户只能手动重新提交
4. 前端无编译排队状态展示，用户体验差

**约束：**
- 不引入外部消息队列（Redis/Celery），保持部署简单
- CLSI 是 vendored 的第三方服务，不大幅重写其内部
- 需向后兼容现有 API 结构（可 breaking 但需明确迁移路径）

## Goals / Non-Goals

**Goals:**
- 编译请求立即返回 task_id，不阻塞 HTTP 连接
- 后端通过 asyncio 并发控制，限制同时运行的编译进程数量
- CLSI 层增加全局并发 Semaphore，超出限制的请求排队等待
- 前端通过轮询获取编译状态，展示排队/编译中/成功/失败
- 编译超时自动终止并回收资源，防止僵尸进程和 OOM

**Non-Goals:**
- 不做 WebSocket/SSE 实时日志推送（后续可扩展）
- 不做分布式编译（多 CLSI 实例负载均衡）
- 不做编译结果缓存/增量编译优化
- 不修改 CLSI 的 Docker sandboxing（保持 LocalCommandRunner）

## Decisions

### Decision 1: 使用 asyncio.Semaphore 而非外部消息队列

**选择**: 在 FastAPI 进程内使用 `asyncio.Semaphore` 控制编译并发数。

**替代方案:**
- Celery + Redis: 功能强大但引入重依赖，部署复杂度高
- asyncio.Queue: 能排队但无法持久化，进程重启丢失任务
- Database polling: 简单但增加 DB 负载和延迟

**理由**: 项目规模为单机部署，Semaphore 足够。编译任务状态持久化到 PostgreSQL 保证可靠性。进程重启时，启动脚本扫描 pending 任务重新入队。

**配置**: `MAX_CONCURRENT_COMPILES=3`（环境变量），默认 3 个并发编译。

### Decision 2: 编译任务持久化到 PostgreSQL

**选择**: 新增 `compile_jobs` 表，字段包括 `id`, `project_id`, `user_id`, `status`, `root_file`, `compiler`, `build_id`, `error`, `logs`, `created_at`, `updated_at`。

**状态机**: `pending` → `running` → `success` / `failed` / `timeout` / `cancelled`

**理由**: 任务状态持久化后，进程重启不丢失，且可查询历史编译记录。比 Redis 队列更适合此场景（数据量小，查询需求多）。

### Decision 3: API 改为异步提交 + 轮询模式

**选择**:
- `POST /api/v1/projects/{id}/latex/compile` → 返回 `{ job_id: "xxx", status: "pending" }`
- `GET /api/v1/projects/{id}/latex/compile/{job_id}/status` → 返回 `{ job_id, status, build_id?, error?, logs?, progress? }`
- `GET /api/v1/projects/{id}/latex/output/{build_id}/pdf` → 保持不变

**替代方案:**
- SSE 推送: 实时性更好但复杂度高，需管理连接
- WebSocket: 双向通信但过重，编译状态只需单向拉取

**理由**: 轮询模式最简单可靠。前端每 2 秒轮询一次，编译通常 10-120 秒，总请求数 5-60 次，完全可接受。

### Decision 4: CLSI 客户端增加超时强制终止

**选择**: `ClsiClient.compile()` 增加可配置超时（默认 120s），超时后调用 `stop_compile()` + `clear_cache()`。

**理由**: 防止 latexmk 僵尸进程占用资源。超时值通过 `LATEX_COMPILE_TIMEOUT=120` 环境变量配置。

### Decision 5: 前端轮询策略

**选择**: 使用 TanStack Query 的 `refetchInterval` 实现轮询，编译完成后自动停止。

**行为:**
- 提交编译 → 立即显示"排队中"状态
- 状态变为 `running` → 显示"编译中"动画
- 状态变为 `success` → 自动加载 PDF，停止轮询
- 状态变为 `failed`/`timeout` → 显示错误日志，停止轮询

**轮询间隔**: 状态为 `pending` 时 1 秒，`running` 时 2 秒。

### Decision 6: 配置管理策略

**选择**: 遵循项目现有模式——后端 Pydantic Settings + 环境变量，前端常量文件。

**后端新增配置** (加入 `backend/app/config.py` 的 `Settings` 类):

| 字段 | 环境变量 | 类型 | 默认值 | 说明 |
|------|---------|------|--------|------|
| `max_concurrent_compiles` | `MAX_CONCURRENT_COMPILES` | int | 3 | 全局最大并发编译数 |
| `latex_compile_timeout` | `LATEX_COMPILE_TIMEOUT` | int | 120 | 单次编译超时秒数 |
| `compile_job_max_age_hours` | `COMPILE_JOB_MAX_AGE_HOURS` | int | 24 | 历史任务保留时长 |
| `compile_history_limit` | `COMPILE_HISTORY_LIMIT` | int | 20 | 历史查询返回条数 |

**理由**: `LATEX_CLSI_TIMEOUT_SECONDS` 已存在于配置中（HTTP 客户端超时），新增的 `latex_compile_timeout` 是业务层超时（包含排队等待时间），两者语义不同应分开配置。

**前端常量** (新建 `frontend/src/views/projects/constants.ts`):

| 常量 | 值 | 说明 |
|------|-----|------|
| `POLL_INTERVAL_PENDING_MS` | 1000 | 排队状态轮询间隔 |
| `POLL_INTERVAL_RUNNING_MS` | 2000 | 编译中轮询间隔 |
| `COMPILE_TIMEOUT_DISPLAY_S` | 120 | 前端超时提示阈值 |
| `ERROR_SUMMARY_LINES` | 5 | 错误摘要显示行数 |
| `HISTORY_PAGE_SIZE` | 20 | 历史记录分页大小 |

**理由**: 前端这些值不需要运行时切换，但集中管理避免魔法数字散落各处。项目已有 `views/uploading/constants.ts` 先例。

## Risks / Trade-offs

- **[Semaphore 不跨进程]** → 单实例部署可接受。如需多实例，后续可替换为 Redis 分布式锁。当前设计预留了 `CompileQueueService` 抽象层。
- **[进程重启丢失排队任务]** → 启动时扫描 `pending` 状态任务重新提交。running 状态任务标记为 `failed`（进程已死，编译结果丢失）。
- **[Breaking API 变更]** → 前后端同步发布，无向后兼容需求（内部项目）。
- **[轮询增加请求量]** → 编译期间约 50-60 次请求，对 FastAPI 无压力。后续可升级为 SSE。
- **[CLSI 单实例瓶颈]** → Semaphore 限制并发为 3，超出排队。单机 3 并发编译对 CPU/内存已接近极限（每个 latexmk 约 200-500MB 内存）。
