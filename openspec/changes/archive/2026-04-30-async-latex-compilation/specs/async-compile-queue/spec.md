## ADDED Requirements

### Requirement: Compile job persistence
系统 SHALL 将每次编译请求持久化到 `compile_jobs` 数据库表，包含 `id`, `project_id`, `user_id`, `status`, `root_file`, `compiler`, `build_id`, `error`, `logs`, `created_at`, `updated_at` 字段。

#### Scenario: Submit compile job
- **WHEN** 用户提交编译请求
- **THEN** 系统创建 `compile_jobs` 记录，初始状态为 `pending`，返回 `job_id`

#### Scenario: Query compile history
- **WHEN** 用户查询项目的编译历史
- **THEN** 系统返回该项目最近 20 条编译记录，按 `created_at` 降序排列

### Requirement: Compile job state machine
编译任务 SHALL 遵循状态机：`pending` → `running` → `success` / `failed` / `timeout` / `cancelled`。状态转换 SHALL 不可逆（只能向前）。

#### Scenario: Normal compilation flow
- **WHEN** 编译任务被队列取出执行
- **THEN** 状态从 `pending` 变为 `running`，编译完成后变为 `success` 或 `failed`

#### Scenario: Compilation timeout
- **WHEN** 编译执行超过配置的超时时间（默认 120 秒）
- **THEN** 状态变为 `timeout`，系统终止 CLSI 编译进程并清理资源

#### Scenario: User cancellation
- **WHEN** 用户在编译过程中请求取消
- **THEN** 状态从 `running` 变为 `cancelled`，系统终止 CLSI 编译进程

#### Scenario: Invalid state transition
- **WHEN** 尝试将 `success` 状态的任务变为 `pending`
- **THEN** 系统拒绝该操作，保持原状态不变

### Requirement: Async compile API
编译 API SHALL 改为异步模式：POST 提交返回 `job_id`，GET 轮询获取状态。

#### Scenario: Submit compilation
- **WHEN** 用户 POST `/api/v1/projects/{id}/latex/compile`
- **THEN** 系统创建编译任务，立即返回 `{ job_id, status: "pending" }`，HTTP 202

#### Scenario: Poll job status
- **WHEN** 用户 GET `/api/v1/projects/{id}/latex/compile/{job_id}/status`
- **THEN** 系统返回 `{ job_id, status, build_id?, error?, logs?, elapsed_seconds? }`

#### Scenario: Job not found
- **WHEN** 用户查询不存在的 `job_id`
- **THEN** 系统返回 HTTP 404

#### Scenario: Unauthorized access
- **WHEN** 用户查询不属于自己项目的编译任务
- **THEN** 系统返回 HTTP 403

### Requirement: Concurrency control via Semaphore
系统 SHALL 使用 `asyncio.Semaphore` 限制同时运行的编译进程数量。超出限制的任务在队列中等待。

#### Scenario: Within concurrency limit
- **WHEN** 当前运行 2 个编译任务，限制为 3
- **THEN** 新提交的编译任务立即开始执行

#### Scenario: Exceed concurrency limit
- **WHEN** 当前运行 3 个编译任务，限制为 3
- **THEN** 新提交的编译任务保持 `pending` 状态排队等待

#### Scenario: Slot freed up
- **WHEN** 一个编译任务完成（成功或失败），且有 `pending` 任务等待
- **THEN** 队列自动取出下一个 `pending` 任务开始执行

### Requirement: Startup recovery
服务启动时 SHALL 扫描数据库中的残留任务并恢复状态。

#### Scenario: Recover pending jobs
- **WHEN** 服务启动，数据库中存在 `pending` 状态的任务
- **THEN** 系统将这些任务重新提交到编译队列

#### Scenario: Mark orphaned running jobs
- **WHEN** 服务启动，数据库中存在 `running` 状态的任务
- **THEN** 系统将这些任务标记为 `failed`，error 字段记录 "Process restarted during compilation"

### Requirement: Compile timeout and resource cleanup
系统 SHALL 对每个编译任务设置超时，超时后强制终止并清理资源。

#### Scenario: Timeout triggers cleanup
- **WHEN** 编译任务执行超过 `LATEX_COMPILE_TIMEOUT` 秒（默认 120）
- **THEN** 系统调用 CLSI `stop_compile`，然后调用 `clear_cache`，将任务标记为 `timeout`

#### Scenario: Cleanup on failure
- **WHEN** 编译任务失败（非超时原因）
- **THEN** 系统仍清理 CLSI 缓存，防止资源泄漏
