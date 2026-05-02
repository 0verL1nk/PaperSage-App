## Why

当前 LaTeX 编译是同步的 HTTP 请求链：前端 POST → 后端 await CLSI → 返回结果。一次编译可能耗时 10-120 秒，期间 HTTP 连接一直被占用。多用户同时编译时，CLSI 无并发控制，可能因 latexmk 进程争抢 CPU/内存导致 OOM。需要将编译改为异步任务模型：提交即返回任务 ID，前端轮询状态，后端控制并发。

## What Changes

- 后端新增编译任务表（`compile_jobs`），记录每次编译的状态、build_id、日志、错误信息
- 后端新增异步编译队列，使用 asyncio Semaphore 限制全局并发数
- 后端 CLSI 客户端增加超时保护和资源回收
- **BREAKING**: 编译 API 从同步返回结果改为返回 `job_id`，客户端需轮询 `/compile/{job_id}/status` 获取结果
- CLSI 层面增加全局并发限制（Semaphore），防止同时运行过多 latexmk 进程
- 前端编译流程改为：提交 → 显示进度 → 轮询结果 → 展示 PDF/错误
- 前端增加编译状态 UI（排队中 / 编译中 / 成功 / 失败）

## Capabilities

### New Capabilities

- `async-compile-queue`: 后端异步编译队列，包括任务持久化、状态管理、并发控制、超时回收
- `compile-polling-ui`: 前端编译轮询 UI，包括提交任务、状态展示、结果获取、错误展示

### Modified Capabilities

- `writing-page-integration`: 写作页面的编译交互从同步等待改为异步轮询模式

## Impact

- **后端 API**: `POST /latex/compile` 响应格式变更（breaking），新增 `GET /latex/compile/{job_id}/status` 端点
- **后端服务层**: `LaTeXService` 新增异步任务管理逻辑，新增 `CompileJobService`
- **后端模型**: 新增 `CompileJob` ORM 模型 + Alembic 迁移
- **后端基础设施**: 引入 asyncio 并发控制（Semaphore），无需引入外部消息队列
- **CLSI 客户端**: `ClsiClient` 增加超时强制终止和资源清理
- **前端 hooks**: `useLatexCompile` 改为异步提交 + 轮询模式
- **前端 UI**: WritingPage 增加编译状态指示器
- **数据库**: 新增 `compile_jobs` 表
