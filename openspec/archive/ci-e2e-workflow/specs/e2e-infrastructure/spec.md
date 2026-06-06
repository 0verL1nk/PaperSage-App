## ADDED Requirements

### Requirement: Single-job E2E workflow with services
GitHub Actions SHALL 提供 `.github/workflows/e2e.yml`,在 `ubuntu-latest` 单 job 里拉起 backend、frontend、MinIO 三个服务,跑 Playwright E2E 套件,上传 HTML 报告和 trace/video。

#### Scenario: Push to main triggers E2E
- **WHEN** 代码 push 到 `main` 且路径匹配 `frontend/**`、`backend/**`、`.github/workflows/e2e.yml` 或 `frontend/playwright.config.ts`
- **THEN** workflow 在 1 分钟内排队并开始执行

#### Scenario: Pull request triggers E2E
- **WHEN** 任何 PR 打开 / 同步到 `main` 且路径匹配上述
- **THEN** workflow 跑同 E2E 套件

#### Scenario: Manual dispatch
- **WHEN** 用户在 GitHub UI 手动触发 workflow
- **THEN** workflow 跑 E2E 套件,不读 path 过滤

#### Scenario: Concurrent run cancellation
- **WHEN** 同一分支有新的 push 触发
- **THEN** 旧的 in-progress run 自动取消,新 run 继续

#### Scenario: MinIO service starts before main steps
- **WHEN** job 开始
- **THEN** `minio/minio:latest` 容器在 9000 端口 ready,`minio/mc` init 容器跑一次创建 `papersage` bucket

#### Scenario: Backend starts within 30s
- **WHEN** 主 steps 跑到 `Start backend`
- **THEN** `nohup uvicorn main:app --port 5478` 启动,`/health` 端点在 30s 内返回 200

#### Scenario: Frontend starts within 60s
- **WHEN** 主 steps 跑到 `Start frontend`
- **THEN** `nohup pnpm dev --port 5173` 启动,根路径在 60s 内返回 200

#### Scenario: Playwright browsers installed and cached
- **WHEN** 第一次跑 workflow
- **THEN** 安装 chromium + 依赖(~300MB),后续 run 通过 `actions/cache` 复用

#### Scenario: E2E tests run with retries in CI
- **WHEN** `pnpm test:e2e` 执行
- **THEN** Playwright 用 `retries=1`、`workers=2`、GitHub reporter,失败时重试一次

#### Scenario: Artifacts uploaded on always
- **WHEN** step 结束(成功或失败)
- **THEN** `playwright-report/`、`playwright-results.json`、`test-results/` 全部上传,保留 7 天

#### Scenario: Backend/frontend logs dumped on failure
- **WHEN** job 失败
- **THEN** `backend.log` 和 `frontend.log` 最后 200 行被 tail 到 GitHub UI

### Requirement: Workflow env explicitly set, .env ignored
CI 环境 SHALL 通过 workflow `env:` 段显式注入所有配置,不读仓库的 `.env` 文件。

#### Scenario: .env file not loaded
- **WHEN** backend 启动
- **THEN** `app.config.Settings` 不读 `backend/.env`,所有字段来自 `os.environ` 显式值

#### Scenario: SQLite file is per-job
- **WHEN** job 开始
- **THEN** `e2e.db` 不在仓库内,`DATABASE_URL=sqlite+aiosqlite:///./e2e.db` 指向 job 工作目录

#### Scenario: CLSI disabled in CI
- **WHEN** workflow env 注入 `LATEX_CLSI_ENABLED=false`
- **THEN** 后端 `services.latex.service.compile_project` 立即 raise 503,避免真编译

#### Scenario: S3 points to MinIO
- **WHEN** workflow env 注入 `S3_ENDPOINT=http://minio:9000`
- **THEN** `boto3.client(endpoint_url=...)` 指向同 job 的 MinIO service

#### Scenario: Email provider no-op
- **WHEN** workflow env 注入 `EMAIL_PROVIDER=smtp` + 空 `SMTP_HOST`
- **THEN** 后端 email 调用不会真发邮件(但会失败/不调用,以免阻塞测试)

### Requirement: Cache strategies for fast feedback
workflow SHALL 缓存 pnpm store、uv cache、Playwright browsers,跨 run 复用。

#### Scenario: pnpm store cached
- **WHEN** 第二次及之后跑 workflow
- **THEN** `pnpm install --frozen-lockfile` 命中 `actions/setup-node` 的 `cache: 'pnpm'`,< 30s 完成

#### Scenario: uv cache mounted
- **WHEN** 跑 `uv sync --extra dev`
- **THEN** `UV_CACHE_DIR=/tmp/setup-uv-cache` 被 setup-uv action 挂载,跨 step 复用

#### Scenario: Playwright browsers cached
- **WHEN** 跑 `pnpm exec playwright install`
- **THEN** 浏览器二进制从 `actions/cache` 命中,key 含 `@playwright/test` 版本
