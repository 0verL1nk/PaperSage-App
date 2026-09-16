# 测试策略

> 2026-09-16：当前验收及 AI 对照实验以 [调研驱动方案第9节](12-ai-writing-design.md#9-验证与验收) 为准，下文为历史测试设计，Python 示例不代表现有 Go 后端测试入口。任务书的核心功能通过率 ≥90% 与代码覆盖率不是同一指标；主要操作 ≤3 秒不得仅用平均值或 P95 替代。当前测试入口为 `frontend` 下 `pnpm test`、`pnpm test:e2e`，以及 `server` 下 `go test ./...`，后者须准备各测试要求的环境。本次文档更新没有执行或宣称这些业务测试通过。

## 1. 测试目标

### 1.1 质量目标

| 指标 | 目标值 | 说明 |
|------|--------|------|
| 代码覆盖率 | ≥ 80% | 核心业务逻辑 |
| 单元测试通过率 | 100% | 所有单元测试必须通过 |
| 集成测试通过率 | 100% | 所有集成测试必须通过 |
| E2E 测试通过率 | ≥ 95% | 关键用户路径 |
| Bug 修复率 | ≥ 90% | 发布前 P0/P1 Bug |
| 性能指标 | 符合要求 | 见非功能需求 |

### 1.2 测试分层

```mermaid
graph TB
    subgraph 测试金字塔
        E2E[E2E 测试<br/>关键用户路径]
        IT[集成测试<br/>API 接口]
        UT[单元测试<br/>函数/组件]
    end

    E2E -.- IT
    IT -.- UT
```

| 测试类型 | 占比 | 执行时机 | 工具 |
|----------|------|----------|------|
| 单元测试 | 60% | 每次提交 | Vitest / Pytest |
| 集成测试 | 30% | 每次 PR | Vitest / Pytest |
| E2E 测试 | 10% | 发布前 | Playwright |

## 2. 前端测试

### 2.1 测试工具

| 工具 | 用途 |
|------|------|
| Vitest | 单元测试运行器 |
| React Testing Library | React 组件测试 |
| Playwright | E2E 测试 |
| ESLint | 代码规范检查 |
| TypeScript | 类型检查 |

### 2.2 单元测试示例

```typescript
// frontend/src/features/auth/components/auth-page.test.tsx

import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { AuthPage } from './auth-page'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: false },
    mutations: { retry: false },
  },
})

const renderWithProviders = (component: React.ReactElement) => {
  return render(
    <QueryClientProvider client={queryClient}>
      {component}
    </QueryClientProvider>
  )
}

describe('AuthPage', () => {
  it('should render login form by default', () => {
    renderWithProviders(<AuthPage />)
    expect(screen.getByText('登录')).toBeInTheDocument()
    expect(screen.getByLabelText('用户名')).toBeInTheDocument()
    expect(screen.getByLabelText('密码')).toBeInTheDocument()
  })

  it('should switch to register mode', () => {
    renderWithProviders(<AuthPage />)
    const switchButton = screen.getByText('没有账号？注册')
    fireEvent.click(switchButton)
    expect(screen.getByText('注册')).toBeInTheDocument()
    expect(screen.getByLabelText('确认密码')).toBeInTheDocument()
  })

  it('should show validation errors for empty fields', async () => {
    renderWithProviders(<AuthPage />)
    const submitButton = screen.getByText('登录')
    fireEvent.click(submitButton)
    await waitFor(() => {
      expect(screen.getByText('用户名至少3个字符')).toBeInTheDocument()
    })
  })
})
```

### 2.3 Hooks 测试

```typescript
// frontend/src/features/projects/hooks.test.ts

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useProjects, useCreateProject } from './hooks'
import * as apiClient from '@/shared/api/client'

vi.mock('@/shared/api/client')

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: false },
    mutations: { retry: false },
  },
})

const wrapper = ({ children }: { children: React.ReactNode }) => (
  <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
)

describe('useProjects', () => {
  beforeEach(() => {
    queryClient.clear()
  })

  it('should fetch projects successfully', async () => {
    const mockProjects = [
      { id: '1', name: 'Project 1' },
      { id: '2', name: 'Project 2' },
    ]

    vi.mocked(apiClient.default.get).mockResolvedValue({
      data: mockProjects,
    })

    const { result } = renderHook(() => useProjects(), { wrapper })

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    expect(result.current.data).toEqual(mockProjects)
  })

  it('should handle fetch error', async () => {
    vi.mocked(apiClient.default.get).mockRejectedValue(new Error('Network error'))

    const { result } = renderHook(() => useProjects(), { wrapper })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })
  })
})
```

### 2.4 组件测试

| 组件 | 测试用例 |
|------|----------|
| AuthPage | 登录表单渲染、切换注册模式、表单验证、登录成功跳转 |
| ProjectList | 列表渲染、空状态、加载状态、错误状态 |
| ProjectCard | 渲染项目信息、点击跳转、删除操作 |
| FolderTree | 树形渲染、展开/折叠、点击选择 |
| FileTable | 文件列表渲染、选择、批量操作 |
| PdfViewer | PDF 加载、翻页、缩放 |
| HighlightsPanel | 高亮列表、添加高亮、删除高亮 |

### 2.5 E2E 测试示例

```typescript
// frontend/tests/auth.spec.ts

import { test, expect } from '@playwright/test'

test.describe('Authentication', () => {
  test('should login successfully', async ({ page }) => {
    await page.goto('/auth')

    await page.fill('[name="username"]', 'testuser')
    await page.fill('[name="password"]', 'password123')

    await page.click('button[type="submit"]')

    await expect(page).toHaveURL('/app/documents')
    await expect(page.locator('text=我的项目')).toBeVisible()
  })

  test('should show error for invalid credentials', async ({ page }) => {
    await page.goto('/auth')

    await page.fill('[name="username"]', 'wronguser')
    await page.fill('[name="password"]', 'wrongpassword')

    await page.click('button[type="submit"]')

    await expect(page.locator('text=用户名或密码错误')).toBeVisible()
  })

  test('should redirect to auth when not logged in', async ({ page }) => {
    await page.goto('/app/documents')

    await expect(page).toHaveURL('/auth')
  })
})
```

### 2.6 前端测试配置

```typescript
// frontend/vitest.config.ts

import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      exclude: ['node_modules/', 'src/test/'],
    },
  },
})
```

## 3. 后端测试

### 3.1 测试工具

| 工具 | 用途 |
|------|------|
| Pytest | 测试运行器 |
| Pytest-asyncio | 异步测试支持 |
| httpx | HTTP 客户端（测试用） |
| Ffaker | 测试数据生成 |
| coverage.py | 覆盖率统计 |

### 3.2 测试结构

```text
backend/tests/
├── __init__.py
├── conftest.py              # 共享 fixture
├── unit/
│   ├── test_auth.py
│   ├── test_projects.py
│   └── test_security.py
├── integration/
│   ├── test_api_auth.py
│   ├── test_api_projects.py
│   └── test_api_documents.py
└── fixtures/
    ├── users.json
    └── projects.json
```

### 3.3 测试 Fixture

```python
# backend/tests/conftest.py

import pytest
import pytest_asyncio
from httpx import AsyncClient, ASGITransport
from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from app.main import app
from app.database import Base, get_db
from app.models import User
from core.security import hash_password

# 测试数据库
TEST_DATABASE_URL = "sqlite+aiosqlite:///:memory:"

engine = create_async_engine(TEST_DATABASE_URL, echo=False)


@pytest_asyncio.fixture
async def db_session():
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)

    async with AsyncSession(engine) as session:
        yield session

    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)


@pytest_asyncio.fixture
async def client(db_session: AsyncSession):
    async def override_get_db():
        yield db_session

    app.dependency_overrides[get_db] = override_get_db
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as ac:
        yield ac
    app.dependency_overrides.clear()


@pytest_asyncio.fixture
async def test_user(db_session: AsyncSession):
    user = User(
        id="test-user-id",
        username="testuser",
        email="test@example.com",
        hashed_password=hash_password("password123"),
    )
    db_session.add(user)
    await db_session.commit()
    return user
```

### 3.4 单元测试示例

```python
# backend/tests/unit/test_security.py

import pytest
from core.security import hash_password, verify_password


class TestSecurity:
    def test_hash_password(self):
        password = "test_password"
        hashed = hash_password(password)

        assert hashed != password
        assert len(hashed) > 0

    def test_verify_password_correct(self):
        password = "test_password"
        hashed = hash_password(password)

        assert verify_password(password, hashed) is True

    def test_verify_password_incorrect(self):
        password = "test_password"
        wrong_password = "wrong_password"
        hashed = hash_password(password)

        assert verify_password(wrong_password, hashed) is False
```

### 3.5 集成测试示例

> 状态：可运行参考。`backend/tests/test_settings_integration.py` 是首个落地的集成测试文件，配套 fixture 在 `backend/tests/conftest.py`。

#### 3.5.1 Fixture 设计（`conftest.py`）

```python
import pytest
import pytest_asyncio
from httpx import ASGITransport, AsyncClient
from sqlalchemy.ext.asyncio import (
    AsyncSession,
    async_sessionmaker,
    create_async_engine,
)

from app.main import app
from core.base import Base
from models import User  # noqa — 注册到 Base.metadata

@pytest_asyncio.fixture
async def db_session() -> AsyncGenerator[AsyncSession, None]:
    """每次测试一个全新的 in-memory SQLite + Base.metadata.create_all/drop_all。"""
    engine = create_async_engine("sqlite+aiosqlite:///:memory:")
    factory = async_sessionmaker(engine, expire_on_commit=False)
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    async with factory() as session:
        yield session
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)
    await engine.dispose()


@pytest.fixture
def authed_user() -> User:
    """不需要持久化的假用户。UUID 固定，方便断言。"""
    return User(
        id=uuid.UUID("11111111-1111-1111-1111-111111111111"),
        username="tester",
        password_hash="not-a-real-hash",
        is_active=True,
    )


@pytest_asyncio.fixture
async def client(db_session, authed_user) -> AsyncGenerator[AsyncClient, None]:
    """httpx 客户端，绕过 lifespan/alembic/缓存，注入 DB+user。"""
    from api.dependencies import get_db
    from api.routes.auth import get_current_user

    async def _override_get_db():
        try:
            yield db_session
            await db_session.commit()
        except Exception:
            await db_session.rollback()
            raise

    app.dependency_overrides[get_db] = _override_get_db
    app.dependency_overrides[get_current_user] = lambda: authed_user

    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as ac:
        yield ac
    app.dependency_overrides.clear()
```

**为什么这样选**：
- **in-memory SQLite**：与 `core/database.py` 的 `NullPool` 分支一致；CI 无需 PostgreSQL；每个测试一份新库，天然隔离
- **`dependency_overrides`**：直接复用 `get_db` / `get_current_user`，不引入新的测试钩子；override 的 key 必须是路由实际 `Depends` 的那个函数对象
- **httpx 0.28 不跑 ASGI lifespan**：无需 `asgi-lifespan` 包装，alembic 迁移和缓存初始化都不会触发
- **不用 TokenFactory**：override 走 `get_current_user` 短路了 JWT 验证，测试 token 字符串随便填

#### 3.5.2 典型用例

```python
@pytest.mark.asyncio
class TestGetCategory:
    async def test_returns_ui_defaults_when_no_row(self, client: AsyncClient) -> None:
        r = await client.get("/api/v1/settings/ui")
        assert r.status_code == 200
        data = r.json()["data"]
        assert data["settings"]["theme"] == "system"

    async def test_unknown_category_returns_404(self, client: AsyncClient) -> None:
        r = await client.get("/api/v1/settings/nonexistent")
        assert r.status_code == 404
```

#### 3.5.3 验证 422 用项目信封

项目的 `validation_exception_handler` 把 Pydantic 错误包成 `{code: 422, data: null, info: "..."}`，**不是** FastAPI 默认的 `{detail: ...}`。断言要看 `body["code"]` 和 `body["info"]`，不是 `body["detail"]`。

```python
async def test_invalid_value_returns_422(self, client: AsyncClient) -> None:
    r = await client.patch("/api/v1/settings/ui", json={"settings": {"theme": "neon"}})
    assert r.status_code == 422
    body = r.json()
    assert body["code"] == 422
    assert "UISettings" in body["info"]
```

#### 3.5.4 测鉴权：暂时清除 override

`client` fixture 默认注入了 `get_current_user`。要测 401 时，单独构造一个不注入 auth 的客户端：

```python
async def test_unauthenticated_request_returns_401(self, db_session) -> None:
    app.dependency_overrides[get_db] = lambda_db(db_session)  # 只覆盖 DB
    # 故意不覆盖 get_current_user
    try:
        transport = ASGITransport(app=app)
        async with AsyncClient(transport=transport, base_url="http://test") as ac:
            r = await ac.get("/api/v1/settings")
            assert r.status_code == 401
    finally:
        app.dependency_overrides.clear()
```

#### 3.5.5 何时用集成 vs 单元

| 场景 | 用 |
|------|-----|
| 校验 Pydantic 字段、默认值合并 | 单元（`test_settings_service.py`）|
| 验证路由签名、依赖注册 | 单元（`test_settings_api.py`）|
| 跨请求状态（PATCH 之后 GET 看是否持久化）| **集成**（`test_settings_integration.py`）|
| 鉴权 / 异常信封 / 未知 category 404 | **集成**（httpx 必经）|
| 真实数据库方言行为（PG vs SQLite JSON）| 集成 + 多 DB fixture，不在本套件范围 |

## 4. 测试覆盖要求

### 4.1 前端覆盖率目标

| 模块 | 最低覆盖率 |
|------|-----------|
| 认证模块 (features/auth) | 85% |
| 项目模块 (features/projects) | 80% |
| 上传模块 (features/uploading) | 80% |
| 编码模块 (features/coding) | 80% |
| 共享模块 (shared) | 85% |
| **总计** | **80%** |

### 4.2 后端覆盖率目标

| 模块 | 最低覆盖率 |
|------|-----------|
| 认证服务 (services/auth) | 90% |
| 项目服务 (services/project) | 85% |
| 文档服务 (services/document) | 85% |
| 核心模块 (core) | 90% |
| **总计** | **85%** |

## 5. 测试执行

### 5.1 开发环境测试

```bash
# 前端测试
cd frontend
pnpm test              # 运行所有测试
pnpm test:watch        # 监听模式
pnpm test:coverage     # 生成覆盖率报告

# 后端测试
cd backend
pytest                 # 运行所有测试
pytest --watch         # 监听模式
pytest --cov           # 生成覆盖率报告
```

### 5.2 CI/CD 测试

```yaml
# .github/workflows/test.yml

name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'pnpm'

      - name: Install frontend dependencies
        run: cd frontend && pnpm install

      - name: Run frontend tests
        run: cd frontend && pnpm test

      - name: Setup Python
        uses: actions/setup-python@v5
        with:
          python-version: '3.12'

      - name: Install backend dependencies
        run: cd backend && pip install -e ".[dev]"

      - name: Run backend tests
        run: cd backend && pytest --cov
```

---

*本章节描述了 PaperSage 系统的测试策略，包括测试目标、测试工具、测试用例和覆盖率要求。*
