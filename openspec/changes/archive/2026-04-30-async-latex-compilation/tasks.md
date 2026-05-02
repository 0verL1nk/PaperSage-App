## 1. Database & Models

- [x] 1.1 Create `CompileJob` ORM model in `backend/models/compile_job.py` with fields: id, project_id, user_id, status, root_file, compiler, build_id, error, logs, created_at, updated_at
- [x] 1.2 Add `CompileJobStatus` enum: pending, running, success, failed, timeout, cancelled
- [x] 1.3 Register model in `backend/models/__init__.py`
- [x] 1.4 Create Pydantic schemas in `backend/schemas/compile_job.py`: CompileJobCreate, CompileJobStatusResponse, CompileJobListItem
- [x] 1.5 Register schemas in `backend/schemas/__init__.py`
- [x] 1.6 Generate Alembic migration: `compile_jobs` table
- [x] 1.7 Run migration and verify table creation

## 2. Backend Compile Queue Service

- [x] 2.1 Create `backend/services/latex/compile_queue.py` with `CompileQueueService` class
- [x] 2.2 Implement `asyncio.Semaphore` with configurable `MAX_CONCURRENT_COMPILES` from env
- [x] 2.3 Implement `submit()` method: create CompileJob record, schedule background task
- [x] 2.4 Implement `_execute()` method: acquire semaphore, update status to running, call CLSI, update result
- [x] 2.5 Implement `get_status()` method: query CompileJob by job_id with ownership check
- [x] 2.6 Implement `cancel()` method: update status to cancelled, call CLSI stop_compile
- [x] 2.7 Implement timeout handling: `asyncio.wait_for` with `LATEX_COMPILE_TIMEOUT`, cleanup on timeout
- [x] 2.8 Implement `recover_on_startup()`: scan pending/running jobs, re-queue pending, fail running

## 3. Backend API Routes

- [x] 3.1 Modify `POST /api/v1/projects/{id}/latex/compile` to return `{ job_id, status }` with HTTP 202
- [x] 3.2 Add `GET /api/v1/projects/{id}/latex/compile/{job_id}/status` endpoint
- [x] 3.3 Modify `DELETE /api/v1/projects/{id}/latex/compile` to accept optional `job_id` param
- [x] 3.4 Add `GET /api/v1/projects/{id}/latex/compile/history` endpoint (last 20 jobs)
- [x] 3.5 Update `backend/api/routes/latex.py` to wire new service

## 4. Backend Config & Startup

- [x] 4.1 Add config fields to `backend/app/config.py`: `max_concurrent_compiles` (int, default 3), `latex_compile_timeout` (int, default 120), `compile_job_max_age_hours` (int, default 24), `compile_history_limit` (int, default 20)
- [x] 4.2 Wire `recover_on_startup()` into FastAPI lifespan in `backend/app/main.py`
- [x] 4.3 Update `backend/.env.example` with new env vars: `MAX_CONCURRENT_COMPILES`, `LATEX_COMPILE_TIMEOUT`, `COMPILE_JOB_MAX_AGE_HOURS`, `COMPILE_HISTORY_LIMIT`
- [x] 4.4 Update `deploy/.env.example` with same vars for Docker Compose scenario

## 5. Frontend API Layer

- [x] 5.1 Update `frontend/src/services/latex-api.ts`: compileLatex returns `{ job_id, status }`
- [x] 5.2 Add `getCompileStatus(projectId, jobId)` API function
- [x] 5.3 Add `cancelCompile(projectId, jobId)` API function
- [x] 5.4 Add `getCompileHistory(projectId)` API function
- [x] 5.5 Update TypeScript types for new API contracts

## 6. Frontend Constants & Hooks

- [x] 6.1 Create `frontend/src/views/projects/constants.ts` with: POLL_INTERVAL_PENDING_MS (1000), POLL_INTERVAL_RUNNING_MS (2000), COMPILE_TIMEOUT_DISPLAY_S (120), ERROR_SUMMARY_LINES (5), HISTORY_PAGE_SIZE (20)
- [x] 6.2 Rewrite `useLatexCompile` hook: submit → return jobId, trigger polling
- [x] 6.3 Create `useCompileStatusPolling` hook: TanStack Query with `refetchInterval`, auto-stop on terminal state, interval from constants
- [x] 6.4 Implement variable polling interval from constants: pending → running → stop (terminal)

## 7. Frontend UI

- [x] 7.1 Create compile status indicator component (idle/queued/compiling/success/failed states)
- [x] 7.2 Update WritingPage toolbar: integrate status indicator, disable compile button during active job
- [x] 7.3 Add compile error log panel with collapse/expand in PDF preview area
- [x] 7.4 Add elapsed time display during compilation
- [x] 7.5 Handle timeout state with user-friendly message

## 8. Testing

- [x] 8.1 Backend unit tests: CompileJob model, state transitions, semaphore behavior
- [x] 8.2 Backend API tests: submit, poll status, cancel, history endpoints
- [x] 8.3 Backend integration test: full async compile flow with mocked CLSI
- [x] 8.4 Frontend tests: useLatexCompile hook, polling hook behavior
