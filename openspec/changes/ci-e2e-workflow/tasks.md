## 1. OpenSpec 框架

- [x] 1.1 写 `openspec/changes/ci-e2e-workflow/proposal.md`(Why / What / Capabilities / Impact / Out of Scope)
- [x] 1.2 写 `openspec/changes/ci-e2e-workflow/design.md`(Context / Goals / Decisions / Rollout / Open Questions)
- [x] 1.3 写 `openspec/changes/ci-e2e-workflow/tasks.md`(本文件)
- [x] 1.4 写 `specs/e2e-infrastructure/spec.md`(CI workflow 拓扑、env、artifacts)
- [x] 1.5 写 `specs/e2e-test-coverage/spec.md`(9 个 spec 的覆盖矩阵)
- [x] 1.6 写 `specs/e2e-environment-isolation/spec.md`(env 不读 .env 的隔离规则)

## 2. CI Workflow

- [x] 2.1 写 `.github/workflows/e2e.yml`:触发器 + concurrency + 单 job 骨架
- [x] 2.2 加 `services:` 段(MinIO + mc init 容器)
- [x] 2.3 加 env 段(DATABASE_URL / S3_* / LATEX_CLSI_ENABLED=false / 等)
- [x] 2.4 加 backend 启动 + healthcheck(`nohup uvicorn` + `curl /health` 轮询)
- [x] 2.5 加 frontend 启动 + healthcheck(`nohup pnpm dev` + `curl :5173` 轮询)
- [x] 2.6 加 seed 步骤(`uv run python backend/scripts/seed_e2e_user.py`)
- [x] 2.7 加 Playwright 浏览器安装 + 缓存
- [x] 2.8 加 `pnpm test:e2e --reporter=list` 步骤
- [x] 2.9 加 artifact 上传(`playwright-report` + `test-results` + 日志)
- [x] 2.10 加 failure 时的 log dump step

## 3. Seed 脚本

- [x] 3.1 写 `backend/scripts/seed_e2e_user.py`:在 `DATABASE_URL` 指定的 DB 里创建 `e2euser` / `e2epass1` + 一个空 `default-project` + 3 个空 latex files
- [x] 3.2 处理 `core/security.hash_password()` 接受明文密码(SHA-256 在 seed 里手工算)
- [x] 3.3 输出 `E2E_USER` / `E2E_PASS` / `E2E_PROJECT_ID` 到 `GITHUB_ENV`
- [x] 3.4 幂等:重复跑不报错(已存在则跳过)

## 4. playwright.config.ts 调整

- [x] 4.1 `retries: process.env.CI ? 1 : 0`
- [x] 4.2 `workers: process.env.CI ? 2 : 1`
- [x] 4.3 `timeout: 30_000`
- [x] 4.4 `expect.timeout: 5_000`
- [x] 4.5 `reporter` 数组:加 `['github']` 通道(CI 模式)
- [x] 4.6 `actionTimeout: 10_000` 保持

## 5. helpers.ts 扩展

- [x] 5.1 加 `loginAs(page, username, password)`:走真表单提交,等待 `/app/projects` 出现
- [x] 5.2 加 `logout(page)`:清 localStorage + 跳 /auth
- [x] 5.3 加 `seedAuthTokens(page, access, refresh)`:保留现有 API(塞假 token)
- [x] 5.4 加 `uploadFixture(page, name, mime?)`:用 `setInputFiles` 上传 `tests/e2e/fixtures/{name}`
- [x] 5.5 加 `apiMock(page, pattern, status, body)`:统一 mock 工厂
- [x] 5.6 ~~加 `seedDatabase()`~~ — CI 在 spec 外部运行 seed 脚本,本地开发直接用 `seedAuthTokens`,不需要此 helper

## 6. fixtures

- [x] 6.1 生成 `frontend/tests/e2e/fixtures/sample.pdf`(~1KB 最小合法 PDF)
- [x] 6.2 生成 `frontend/tests/e2e/fixtures/sample.docx`(~3KB 最小合法 DOCX)
- [x] 6.3 写 `frontend/tests/e2e/fixtures/main.tex`(1 页 LaTeX 文档)
- [ ] 6.4 写 `docs/agents/recipes/2026-06-e2e-fixtures.md` 生成 recipe

## 7. 9 个 spec 文件

- [x] 7.1 `auth/auth-flow.spec.ts`:register → login → logout → token refresh → 401 拦截
- [x] 7.2 `projects/project-lifecycle.spec.ts`:create project → upload doc → write file → compile → delete → restore
- [x] 7.3 `projects/group-filter.spec.ts`:建 2 group → 在各建项目 → filter 切换
- [x] 7.7 `uploading/document-upload.spec.ts`:真 multipart 上传、folder 移动、preview
- [x] 7.4 `writing/compile-polling.spec.ts`:202 → 200 轮询、pending → success、failure 状态显示
- [x] 7.5 `writing/folder-ops.spec.ts`:create folder → rename → recursive delete
- [x] 7.6 `writing/compile-trigger.spec.ts`:保留现有 `compile.spec.ts` 内容,改名迁入
- [x] 7.8 `settings/settings-persistence.spec.ts`:改 theme → reload → 仍改
- [x] 7.9 `navigation/cross-route.spec.ts`:跨页导航 + back 按钮 + 未登录直链保护
- [x] 7.10 `error-states/401-redirect.spec.ts`:注入坏 token → 401 → 跳 /auth
- [x] 7.11 删除旧的 `tests/e2e/auth/login.spec.ts` + `auth/register.spec.ts` + `features/projects.spec.ts` + `features/compile.spec.ts`(内容已迁到新位置)
- [x] 7.12 保留 `features/file-tree.spec.ts` 迁到 `writing/file-tree.spec.ts`

## 8. 文档

- [ ] 8.1 `README.md` 增补 "E2E 测试" 章节:本地复跑 (`pnpm test:e2e`)、CI 链接、fixtures 说明

## 9. 本地验证

- [ ] 9.1 `cd frontend && pnpm typecheck`
- [ ] 9.2 `cd backend && uvx ruff check app api core` + `uvx ruff format --check .`
- [ ] 9.3 `cd backend && uv run pytest tests/`
- [ ] 9.4 `cd frontend && pnpm test`
- [ ] 9.5 手工 `pnpm test:e2e` 跑 1-2 个 spec 确认能起

## 10. 上线 & 归档

- [ ] 10.1 推分支,在 GitHub Actions 上跑一次,记录实际时长
- [ ] 10.2 修任何 CI 实际跑出来的 issue
- [ ] 10.3 走 `openspec archive ci-e2e-workflow` 归档
- [ ] 10.4 (可选) 通知 repo owner 在 branch protection 里把 `e2e` 设为 required check
