## Purpose

Persist project compile jobs and expose their status/history. The old CLSI queue
implementation remains available for explicit future reactivation, while
current LaTeX compilation runs in browser workers.
## Requirements

This capability documents persisted compile jobs and the retained legacy CLSI
queue. Normal compilation now runs in browser workers; the legacy queue remains
in source but is dormant and is not started or used as a browser-compilation
fallback. Queue-specific requirements below apply only if that legacy path is
explicitly reactivated in a future change.

### Requirement: Compile job persistence
The system SHALL persist each browser compile job in `compile_jobs` with its
project/user identity, status, root file, compiler, build ID, error, logs, and
timestamps. Browser jobs start in `running`; `pending` is used only by the
dormant legacy queue if it is explicitly reactivated.

#### Scenario: Submit compile job
- **WHEN** the writing page submits a browser compile job
- **THEN** the system persists it with `running` status and returns its `job_id`

#### Scenario: Query compile history
- **WHEN** 用户查询项目的编译历史
- **THEN** 系统返回该项目最近 20 条编译记录，按 `created_at` 降序排列

### Requirement: Compile job state machine
Browser compile jobs SHALL transition from `running` to a terminal state
(`success`, `failed`, `timeout`, or `cancelled`) exactly once. If the legacy
queue is explicitly reactivated, its queued jobs may transition from `pending`
to `running` before reaching a terminal state. Terminal states SHALL be
immutable.

#### Scenario: Normal compilation flow
- **WHEN** a browser worker completes a compile job
- **THEN** the backend records its terminal `success` or `failed` state against the existing job

#### Scenario: Compilation timeout
- **WHEN** browser compilation exceeds its configured worker command limit, or an explicitly reactivated CLSI process exceeds its configured server limit
- **THEN** the job becomes `timeout` and the corresponding worker or CLSI process is terminated

#### Scenario: User cancellation
- **WHEN** the user requests cancellation while a browser job is running
- **THEN** the system records `cancelled` and terminates the browser worker

#### Scenario: Invalid state transition
- **WHEN** 尝试将 `success` 状态的任务变为 `pending`
- **THEN** 系统拒绝该操作，保持原状态不变

### Requirement: Async compile API
Compile jobs SHALL support browser execution while preserving the existing asynchronous job identity and status polling. Browser-originated jobs SHALL NOT be routed to a server-side TeX compiler.

#### Scenario: Submit compilation
- **WHEN** the project editor POSTs `/api/v1/projects/{id}/latex/compile/browser` for a supported browser engine
- **THEN** the backend creates a persisted `running` job, returns its `job_id` with HTTP 202, and does not enqueue CLSI work

#### Scenario: Poll job status
- **WHEN** the client GETs `/api/v1/projects/{id}/latex/compile/{job_id}/status`
- **THEN** the backend returns that authorized job's current or terminal status and available build ID, diagnostics, and elapsed time

#### Scenario: Job not found
- **WHEN** a client polls a job ID that does not exist or does not belong to the requested project
- **THEN** the API returns HTTP 404

#### Scenario: Unauthorized access
- **WHEN** a client polls a compile job for a project the user cannot access
- **THEN** the API returns HTTP 403 without disclosing the job data

#### Scenario: Complete a browser-executed job
- **WHEN** a browser worker successfully compiles a project and submits its artifacts
- **THEN** the backend associates the result with the authorized compile job, stores the artifacts, and exposes the terminal state through the existing job polling API

#### Scenario: Browser cannot run a required capability
- **WHEN** the selected browser engine cannot run a required engine or project capability
- **THEN** the job reports an actionable unsupported-capability error through the existing job polling API and is not sent to CLSI

#### Scenario: Legacy server compile submission is disabled
- **WHEN** a client submits a compile through the legacy server-side endpoint
- **THEN** the API returns a clear deprecation response and does not enqueue CLSI work

#### Scenario: Agent requests compilation
- **WHEN** the agent invokes `compile_project` for a valid project root file
- **THEN** the current writing page starts a browser job using the user's selected browser engine and the backend does not compile the source

### Requirement: Bounded concurrency for the legacy queue
If the retained legacy CLSI queue is explicitly reactivated, the server SHALL
bound the number of concurrent CLSI processes and keep excess jobs queued.

#### Scenario: Within concurrency limit
- **WHEN** 当前运行 2 个编译任务，限制为 3
- **THEN** 新提交的编译任务立即开始执行

#### Scenario: Exceed concurrency limit
- **WHEN** 当前运行 3 个编译任务，限制为 3
- **THEN** 新提交的编译任务保持 `pending` 状态排队等待

#### Scenario: Slot freed up
- **WHEN** 一个编译任务完成（成功或失败），且有 `pending` 任务等待
- **THEN** 队列自动取出下一个 `pending` 任务开始执行

### Requirement: Legacy queue startup recovery
While the legacy CLSI queue is dormant, server startup SHALL NOT resume queued
CLSI work or mark browser-originated jobs as orphaned CLSI processes. If a
future change explicitly reactivates the queue, it SHALL define recovery for
that queue before enabling it.

#### Scenario: Recover pending jobs
- **WHEN** legacy CLSI 队列已由后续变更显式重新启用，服务启动且数据库中存在 `pending` 状态的队列任务
- **THEN** 系统将这些任务重新提交到编译队列

#### Scenario: Mark orphaned running jobs
- **WHEN** legacy CLSI 队列已由后续变更显式重新启用，服务启动且数据库中存在 `running` 状态的 CLSI 任务
- **THEN** 系统将这些任务标记为 `failed`，error 字段记录 "Process restarted during compilation"

### Requirement: Timeout and resource cleanup for the legacy queue
If the retained legacy CLSI queue is explicitly reactivated, each CLSI process
SHALL have a configured timeout and SHALL release server-side resources on
timeout or failure. Browser jobs use the browser worker execution limits and
do not start a CLSI process.

#### Scenario: Timeout triggers cleanup
- **WHEN** 编译任务执行超过 `LATEX_COMPILE_TIMEOUT` 秒（默认 120）
- **THEN** 系统调用 CLSI `stop_compile`，然后调用 `clear_cache`，将任务标记为 `timeout`

#### Scenario: Cleanup on failure
- **WHEN** 编译任务失败（非超时原因）
- **THEN** 系统仍清理 CLSI 缓存，防止资源泄漏
