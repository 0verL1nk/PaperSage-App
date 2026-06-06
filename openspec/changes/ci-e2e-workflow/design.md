## Context

项目当前在 GitHub Actions 跑 3 个 workflow:`backend.yml`(lint + pytest)、`frontend.yml`(typecheck + lint + vitest)、`docker.yml`(构建并推 3 个镜像到 ghcr.io)。三个 workflow 都不跑 E2E。`playwright.config.ts` 已配置 `testDir: ./tests/e2e`、`projects: chromium only`,但 `pnpm test:e2e` 在 CI 里从未被触发。

后端用 FastAPI + 异步 SQLAlchemy 2.0,默认 SQLite 文件;Redis 可选(留空走 in-memory cache);S3 用 boto3 直连 R2/MinIO/S3;CLSI 是独立 Docker 服务。`app/main.py` 的 lifespan 会跑 alembic 升级 + `Base.metadata.create_all` + cache connect + 编译队列恢复。

前端用 Vite 5 + React 19 + TanStack Router,dev server 默认 5173 端口,`vite.config.ts` 把 `/api` 代理到 `http://localhost:5478`。登录用 `Form(...)` 上传,前端先把密码 SHA-256 再发,后端再 bcrypt 一次。

Playwright 配置的 5 个 spec 全在 `tests/e2e/`,以 `page.route()` mock 所有后端 API,实际上不依赖后端,只验证前端组件 + 路由守卫 + 交互。这种"伪 E2E"在重构时给了基本信心,但无法发现前后端协议不匹配、真实 multipart 上传、token 刷新等问题。

## Goals / Non-Goals

**Goals:**
- 在 CI 同一 job 里拉起 backend + frontend + MinIO,跑 9 个 spec
- 覆盖关键用户路径:auth、project 全生命周期、文档上传、编译轮询、settings 持久化、跨页导航、401 拦截
- 失败时自动上传 HTML 报告、trace、video、backend/frontend 日志
- 用 `services:` 段(Docker 边车)而非 docker compose,降低复杂度
- 缓存 pnpm / uv / Playwright browsers,首次 < 8min,后续 < 5min
- 显式 workflow env,不读 `.env`,CI 与本地完全隔离
- DB / MinIO bucket 在 job 结束自动清理

**Non-Goals:**
- 不引入真 CLSI
- 不加多浏览器
- 不做视觉回归
- 不强制 branch protection
- 不在 v1 引入 `act` 本地复跑
- 不并发分片

## Decisions

### 1. Topology: 单 job + 3 个 services

**选择:** 单 job,backend 和 frontend 用 nohup 后台进程 + curl healthcheck 等待;MinIO 用 `services:` 边车容器。

**理由:** 项目 9 个 spec 总耗时预估 < 3min,单 job 足够。`services:` 边车在 GitHub Actions 里自动 DNS 解析,比 docker compose 简单。`nohup ... & disown` 起 backend/frontend 是行业标配,日志重定向到文件便于失败时 tail。

**权衡:** 失去跨 job 缓存(deps-backend / deps-frontend 复用的模式);换 9 个 spec 起来更直接。

### 2. CLSI 处理:`LATEX_CLSI_ENABLED=false`

**选择:** workflow env 显式关 CLSI,compile spec 改为 `page.route` mock 异步 job 状态。

**理由:** CLSI 是独立 Docker 服务,要 4GB+ 镜像,启动慢 2-3min。现有 `compile.spec.ts` 已经把 compile API mock 掉,只测 UI 触发;`compile-polling.spec.ts` 改为 `page.route` 模拟 202 → 200 的轮询,无需真 CLSI。

**权衡:** 不验证真 LaTeX 编译,但 v1 范围;v2 可加手动触发的 `e2e-full` job。

### 3. S3 处理: MinIO 边车容器

**选择:** 用 `minio/minio:latest` 容器 + `minio/mc` 一次性 init 容器创建 bucket。

**理由:** `boto3.client(endpoint_url=...)` 不区分 R2/MinIO/S3,只要 endpoint 正确就能调通。`MC_HOST_minio=http://ci-access:ci-secret@minio:9000 mc mb papersage` 在 services 段跑一次,主 job 起来时 bucket 已存在。

**权衡:** 容器 +200MB 镜像,job 时长 +1min;但 document-upload spec 才能测真 multipart。

### 4. DB 隔离: job 内 SQLite 文件 + seed 脚本

**选择:** `DATABASE_URL=sqlite+aiosqlite:///./e2e.db`,job 开始时删旧文件,启动后跑 `seed_e2e_user.py` 注入 `e2euser` / `e2epass1`。

**理由:** 0 外部依赖,启动快。Seed 脚本直连 DB,绕开 SHA-256 + bcrypt 流程,用 `core.security.hash_password()` 直接生成符合 schema 的 password_hash(注意 schema 要求 `^(?=.*[A-Za-z])(?=.*\d).{8,}$`,seed 用 `e2epass1` 满足)。

**权衡:** test 间不 reset DB(部分 spec 依赖前面 spec 创建的项目);job 级别 reset 即可,9 个 spec 顺序或并行都可。

### 5. Playwright 配置 CI 分支

**选择:** `retries: process.env.CI ? 1 : 0`、`workers: process.env.CI ? 2 : 1`、reporter 数组加 `['github']` 通道。

**理由:** 1 次重试降低 flaky 误报,2 个 worker 提升 ~40% 速度。`['github']` reporter 自动把失败 summary 写到 GitHub Actions UI。

**权衡:** 2 worker 在单 job 里偶尔争抢 Playwright 内部资源,但 9 个 spec 规模下不构成问题。

### 6. 缓存策略: pnpm + uv + ms-playwright

**选择:** pnpm 用 `actions/setup-node@v4` 的 `cache: 'pnpm'`;uv 用 `UV_CACHE_DIR` env;Playwright 用 `actions/cache@v4` 缓存 `~/.cache/ms-playwright`,key 含 `@playwright/test` 版本。

**理由:** uv 和 pnpm 都是 immutable lockfile 安装,缓存命中率高。Playwright 浏览器 ~300MB 一次下载,缓存可省 90% 时间。

**权衡:** ms-playwright cache key 用版本而不是 hash,跨 lockfile 升级会失效,但能避开兼容性陷阱。

### 7. Spec 组织: 按域而非按文件

**选择:** 从 `tests/e2e/{auth,features}/*.spec.ts` 改为 `tests/e2e/{auth,projects,uploading,writing,settings,navigation,error-states}/*.spec.ts`。

**理由:** 当前混合(spec 文件名是组件名,目录分 auth/features)不利于按用户旅程查找。9 个 spec 各归一个域目录,后续加 spec 一眼能找到位置。

**权衡:** 5 个现有 spec 全部要重命名/移动;旧的 `compile.spec.ts` 已经被用户改写过,合并保留到 `writing/compile-trigger.spec.ts`。

### 8. Spec 写法的统一约定

- 用 `loginAs(page, 'e2euser', 'e2epass1')` 替代手写 `page.fill` + 提交
- 用 `uploadFixture(page, 'sample.pdf')` 替代直接 `setInputFiles(absolutePath)`
- `apiMock(page, pattern, status, body)` 工厂统一 mock 风格
- 每个 spec 顶部 `test.describe('...')` 写明用户旅程
- 失败 case 显式 `await expect(toast).toContainText('...')` 而不是只 `expect(status).toBe(4xx)`

## Rollout

按这个顺序落地,每步独立可验证:

1. OpenSpec 框架(proposal + tasks + design + 3 specs)— `openspec validate ci-e2e-workflow` 应通过
2. `.github/workflows/e2e.yml` 骨架 + 缓存策略 — 空 `pnpm test:e2e` 应 PASS
3. `backend/scripts/seed_e2e_user.py` — 本地 `uv run python backend/scripts/seed_e2e_user.py` 应能创建用户
4. `playwright.config.ts` CI 分支 — 本地 `CI=true pnpm test:e2e` 验证 reporter
5. `helpers.ts` 扩展 + `fixtures/` — 单 spec 跑通
6. 9 个 spec 串行添加,每个 spec 跑通再下一个
7. README 增补
8. 推到分支,GitHub Actions 上跑一次,看真实 CI 时长
9. 走 `openspec archive ci-e2e-workflow` 归档

## Open Questions

- 是否在 workflow 跑完后自动评论 PR?(`peter-evans/create-or-update-comment`)— v2
- 是否在 SPEC 路径上加 E2E 必跑限制(比如改 `frontend/src/views/auth/**` 必跑 E2E)— v2
- 是否把 e2e artifacts 也存到 ghcr 镜像作为 regression baseline — 否,过大
