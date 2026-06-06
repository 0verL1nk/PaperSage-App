# ci-e2e-workflow

## Why

PaperSage 已有 5 个 Playwright E2E spec,但全部用 `page.route()` mock 后端 API,实际上没有验证前后端联调是否能工作。最近的几次重构(`get_current_user` 迁位、`get_db` 去重、env config 统一)都在 PR 阶段缺乏端到端保护,只有单元 / 集成测试。CI 报错(`test+docs: E2E coverage for file tree search and keyboard shortcuts` 之后)也没有自动跑 E2E,导致 `compile.spec.ts` 在 `pnpm test:e2e` 时全失败,直到本地手测才发现。

需要一个 GitHub Actions workflow 真正在 CI 里把 backend (FastAPI/SQLite) + frontend (Vite) + MinIO (S3) 拉起来,跑覆盖关键用户路径的 E2E 套件。同时把现有 5 个 spec 重新组织进按域 (auth/projects/uploading/writing/settings/navigation/error-states) 划分的目录,并补足真正依赖后端的测试用例。

## What Changes

- 新增 `.github/workflows/e2e.yml`:在 `ubuntu-latest` 上起 backend + frontend + MinIO services,跑 Playwright,上传 HTML 报告和 trace/video
- 新增 `backend/scripts/seed_e2e_user.py`:启动时向 e2e.db 注入固定凭据的测试用户 + 空项目,供 spec 登录使用
- 调整 `frontend/playwright.config.ts`:CI 模式 `retries=1`、`workers=2`、加 `github` reporter
- 重构 `frontend/tests/e2e/`:
  - 现有 5 个 spec 重命名并迁入新目录(`auth/`、`projects/`、`uploading/`、`writing/`、`settings/`、`navigation/`、`error-states/`)
  - 新增 4 个 spec:`auth-flow`、`project-lifecycle`、`document-upload`、`compile-polling`、`settings-persistence`、`cross-route`、`401-redirect`,共 9 个 spec
  - 扩展 `helpers.ts`:`loginAs` 走真表单提交、`uploadFixture` 用 `setInputFiles` 上传
- 新增 `frontend/tests/e2e/fixtures/`:提交最小合法 `sample.pdf`、`sample.docx`、`main.tex`,供上传 spec 使用
- 新增 `.github/workflows/e2e.yml` 文档:在 `README.md` 增补 "E2E 测试" 章节,说明本地复跑方法

## Capabilities

### New Capabilities

- `e2e-infrastructure`:定义 CI E2E workflow 的拓扑结构(services、env、artifacts、并发、缓存)
- `e2e-test-coverage`:列出必须有 E2E 覆盖的关键用户路径,作为 v1 验收清单
- `e2e-environment-isolation`:CI 环境与本地 `.env` 文件如何隔离(workflow env 显式注入,不读 `.env`)

## Impact

- `.github/workflows/e2e.yml` — 新增
- `backend/scripts/seed_e2e_user.py` — 新增
- `frontend/playwright.config.ts` — 调整 `retries` / `workers` / `reporter`
- `frontend/tests/e2e/` — 重命名 + 新增,共 9 个 spec
- `frontend/tests/e2e/fixtures/` — 新增 3 个样例文件
- `README.md` — 增补 E2E 章节
- 后端代码无改动
- OpenSpec change:`openspec/changes/ci-e2e-workflow/`

## Out of Scope

- 真 CLSI LaTeX 编译服务(`LATEX_CLSI_ENABLED=false`,compile spec 改用 `page.route` 模拟异步 job)
- Firefox / WebKit 浏览器矩阵(仅 chromium,跟现有 config 一致)
- 视觉回归(percy / playwright visual comparisons)— 留 v2
- 并发分片(9 个 spec 单 job 跑足够)— 留 v2
- `branch protection` 强制设为 required check(由 repo owner 后续在 Settings 里勾选)
- 性能 / 负载测试(k6 / artillery)— 完全不同的工作流
- `act` 本地复跑(留 v2)
