/**
 * Shared E2E helpers for tests under tests/e2e.
 *
 * Centralizes the "navigate to the writing page in a logged-in, mocked
 * state" pattern so each spec only declares the API surface it actually
 * exercises. The 4 existing specs (login, register, projects, compile)
 * have the same logic inlined — see TODO in 2026-06 docs.
 *
 * All mocks use page.route() with a glob pattern so the dev-server proxy
 * (vite.config.ts) and direct CORS calls both match.
 */

import { existsSync } from 'node:fs'
import { resolve } from 'node:path'
import type { Page } from '@playwright/test'

export const TEST_PROJECT_ID = 'test-project-1'
export const WRITING_URL = `/project/${TEST_PROJECT_ID}/writing`

// API route patterns — see frontend/src/services/api-routes.ts
export const API_AUTH_ME = '**/api/v1/auth/me'
export const API_PROJECT_DETAIL = `**/api/v1/projects/${TEST_PROJECT_ID}`
export const API_LATEX_FILES = `**/api/v1/projects/${TEST_PROJECT_ID}/latex-files/`
export const API_LATEX_FILE_CONTENT = (path: string) =>
  `**/api/v1/projects/${TEST_PROJECT_ID}/latex-files/${encodeURIComponent(path)}`
export const API_LATEX_COMPILE = `**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/`
export const API_AUTH_LOGIN = '**/api/v1/auth/login'

/** Session-store localStorage keys (see session-store.ts). */
export const ACCESS_TOKEN_KEY = 'papersage_access_token'
export const REFRESH_TOKEN_KEY = 'papersage_refresh_token'

/**
 * Seed fake JWTs into localStorage so the auth guard lets the page load.
 * Must run BEFORE the first navigation to a protected route.
 */
export async function seedAuthTokens(page: Page) {
  await page.goto('/')
  await page.evaluate(
    ([access, refresh]) => {
      window.localStorage.setItem(access, 'fake-access-token')
      window.localStorage.setItem(refresh, 'fake-refresh-token')
    },
    [ACCESS_TOKEN_KEY, REFRESH_TOKEN_KEY],
  )
}

/** Minimal user object returned from /auth/me. */
export const MOCK_USER = {
  id: 'user-1',
  username: 'test',
  email: 'test@test.com',
}

/** Minimal project returned from /projects/{id}. */
export const MOCK_PROJECT = {
  id: TEST_PROJECT_ID,
  name: 'Test Project',
}

/**
 * Standard LaTeX file fixtures used across specs. Override per-test by
 * calling `page.route(API_LATEX_FILES, ...)` again — Playwright's last
 * matching route wins.
 */
export const MOCK_LATEX_FILES = {
  code: 200,
  data: {
    files: [
      { id: 'f1', file_path: 'main.tex', content_type: 'text/x-tex', size_bytes: 42, last_saved_at: '2025-01-01T00:00:00Z' },
      { id: 'f2', file_path: 'sections/intro.tex', content_type: 'text/x-tex', size_bytes: 100, last_saved_at: '2025-01-02T00:00:00Z' },
      { id: 'f3', file_path: 'refs.bib', content_type: 'text/x-bibtex', size_bytes: 50, last_saved_at: '2025-01-03T00:00:00Z' },
    ],
  },
}

/**
 * Install the standard set of API mocks needed for the writing page
 * to render. Call AFTER seedAuthTokens and BEFORE page.goto(WRITING_URL).
 *
 * Mocks the file listing, per-file content fetches, and auto-save
 * (PUT) so the page never hits the vite proxy for `/api/*` while
 * the backend is down. Without the file-content mock, the dev
 * server returns 502 to the auto-fetched content request and the
 * resulting toast interferes with Playwright's click stability
 * checks.
 */
export async function mockWritingPageAPIs(page: Page) {
  await page.route(API_AUTH_ME, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: MOCK_USER }),
    })
  })

  await page.route(API_PROJECT_DETAIL, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: MOCK_PROJECT }),
    })
  })

  await page.route(API_LATEX_FILES, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(MOCK_LATEX_FILES),
    })
  })

  // Per-file content + auto-save + rename/move. The list endpoint
  // `.../latex-files/` ends with a slash so the glob below never
  // matches it; the trailing-slash branch handles the corner case.
  await page.route('**/api/v1/projects/*/latex-files/**', async (route) => {
    const url = route.request().url()
    const method = route.request().method()

    // List endpoint — handled by the dedicated route above
    if (url.endsWith('/')) {
      await route.fallback()
      return
    }

    // ── Sub-resource actions: rename / move ──
    if (url.endsWith('/rename') && method === 'POST') {
      const body = route.request().postDataJSON()
      const newPath = body?.new_path ?? 'renamed.tex'
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          data: { id: 'f-renamed', file_path: newPath, content_type: 'text/x-tex', size_bytes: 42, last_saved_at: '2025-06-01T00:00:00Z' },
        }),
      })
      return
    }

    if (url.endsWith('/move') && method === 'POST') {
      const body = route.request().postDataJSON()
      const newDir = body?.new_dir ?? 'sections'
      const originalSegment = decodeURIComponent(url.split('/latex-files/').pop()?.replace('/move', '') ?? 'refs.bib')
      const fileName = originalSegment.split('/').pop() ?? 'file.tex'
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          data: { id: 'f-moved', file_path: `${newDir}/${fileName}`, content_type: 'text/x-tex', size_bytes: 42, last_saved_at: '2025-06-01T00:00:00Z' },
        }),
      })
      return
    }

    // ── GET file content ──
    if (method === 'GET') {
      const path = decodeURIComponent(url.split('/latex-files/').pop() ?? '')
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          data: {
            path,
            content: '\\documentclass{article}\n\\begin{document}\nHello\n\\end{document}',
          },
        }),
      })
      return
    }

    // ── PUT (auto-save) / DELETE (single file) ──
    if (method === 'PUT' || method === 'DELETE') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: null, info: '' }),
      })
      return
    }

    await route.fallback()
  })

  // Create file/folder (POST to the list endpoint)
  await page.route(API_LATEX_FILES, async (route) => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON()
      const filePath = body?.file_path ?? body?.path ?? 'new-file.tex'
      const isFolder = filePath.endsWith('/.gitkeep')
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          data: {
            id: `f-new-${Date.now()}`,
            file_path: isFolder ? filePath.replace(/\/\.gitkeep$/, '') : filePath,
            content_type: isFolder ? 'directory' : 'text/x-tex',
            size_bytes: 0,
            last_saved_at: '2025-06-01T00:00:00Z',
          },
        }),
      })
      return
    }
    // GET falls through to the existing MOCK_LATEX_FILES response
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(MOCK_LATEX_FILES),
    })
  })

  // Recursive folder delete
  await page.route('**/api/v1/projects/*/latex-files/folder', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: { deleted_count: 3 }, info: '' }),
    })
  })

  // Compile mock — returns immediate success
  await page.route(API_LATEX_COMPILE, async (route) => {
    const method = route.request().method()

    if (method === 'POST') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          data: { job_id: 'job-1', status: 'success', build_id: 'build-1' },
        }),
      })
      return
    }

    // GET compile status (job polling)
    if (method === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          data: { status: 'success', build_id: 'build-1', output_files: ['output.pdf'] },
        }),
      })
      return
    }

    await route.fallback()
  })

  // Compile output (PDF)
  await page.route('**/api/v1/projects/*/latex/output/**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/pdf',
      body: '<mock-pdf-content>',
    })
  })
}

// ─── Real-backend helpers (used by the new spec suite in CI) ─────────

/**
 * Walk the real login form (visiting /auth, filling the inputs, clicking
 * "立即登录") and wait for the post-login redirect to /app/projects.
 *
 * Use this when the spec exercises the real backend flow — register,
 * token refresh, 401 redirect, etc. For spec that only need a logged-in
 * shell to render the page, prefer `seedAuthTokens` (faster, no API hit).
 *
 * Reads E2E_USER / E2E_PASS from process.env when no args are given,
 * matching the GitHub Actions workflow env.
 */
export async function loginAs(
  page: Page,
  username?: string,
  password?: string,
): Promise<void> {
  const u = username ?? process.env.E2E_USER ?? 'e2euser'
  const p = password ?? process.env.E2E_PASS ?? 'e2epass1'
  await page.goto('/auth')
  await page.getByPlaceholder('请输入您的账号').fill(u)
  await page.getByPlaceholder('••••••••').fill(p)
  await page.getByRole('button', { name: '立即登录' }).click()
  await page.waitForURL(/\/app\/projects/, { timeout: 15_000 })
}

/**
 * Clear the auth session and return to /auth. The router's beforeLoad
 * guard should bounce the user back to /auth if they try to navigate
 * anywhere protected after this.
 */
export async function logout(page: Page): Promise<void> {
  await page.evaluate(
    ([access, refresh]) => {
      window.localStorage.removeItem(access)
      window.localStorage.removeItem(refresh)
    },
    [ACCESS_TOKEN_KEY, REFRESH_TOKEN_KEY],
  )
  await page.goto('/auth')
}

/**
 * Generic API mock factory. `pattern` is a glob (passed straight to
 * Playwright's `page.route()`), `body` is any JSON-serializable value.
 * Use this when a spec only needs to stub a handful of endpoints and
 * doesn't need the full `mockWritingPageAPIs` set.
 */
export async function apiMock(
  page: Page,
  pattern: string,
  status: number,
  body: unknown,
  options: { method?: string | RegExp } = {},
): Promise<void> {
  await page.route(pattern, async (route) => {
    if (options.method && !matchMethod(route.request().method(), options.method)) {
      await route.fallback()
      return
    }
    await route.fulfill({
      status,
      contentType: 'application/json',
      body: JSON.stringify(body),
    })
  })
}

function matchMethod(actual: string, expected: string | RegExp): boolean {
  if (typeof expected === 'string') return actual === expected
  return expected.test(actual)
}

/**
 * Resolve the absolute path of an E2E fixture file. Playwright's
 * `setInputFiles` accepts absolute paths; we anchor on the project
 * root so the helper works from any working directory.
 */
export function fixturePath(name: string): string {
  // tests/e2e/helpers.ts → tests/e2e/fixtures/{name}
  // import.meta.url 在 Playwright 的 transform 缓存下可能不指向源文件，
  // 兜底用 process.cwd()（playwright 始终以 frontend/ 为工作目录运行）
  const viaImportUrl = new URL(`./fixtures/${name}`, import.meta.url)
  if (viaImportUrl.protocol === 'file:' && existsSync(viaImportUrl.pathname)) {
    return viaImportUrl.pathname
  }
  return resolve(process.cwd(), 'tests/e2e/fixtures', name)
}

/**
 * Upload a fixture file through the page's first <input type="file">.
 * Use after the upload dialog has been opened by the page (e.g. after
 * clicking the "上传" button on the uploading page). `name` is the
 * basename under `tests/e2e/fixtures/` (e.g. "sample.pdf").
 */
export async function uploadFixture(
  page: Page,
  name: string,
  options: { inputSelector?: string } = {},
): Promise<void> {
  const selector = options.inputSelector ?? 'input[type="file"]'
  await page.setInputFiles(selector, fixturePath(name))
}

/**
 * 在已登录页面上经真实 API 创建 latex 文件（用例自建夹具，避免
 * 依赖/污染种子状态——并行 worker 下共享 'sections' 会互相踩）。
 */
export async function apiCreateLatexFile(
  page: Page,
  projectId: string,
  path: string,
  content = '',
): Promise<void> {
  const token = await page.evaluate(() => localStorage.getItem('papersage_access_token') ?? '')
  const resp = await page.request.post(`/api/v1/projects/${projectId}/latex-files/`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { path, content },
  })
  if (resp.status() !== 200) {
    throw new Error(`apiCreateLatexFile(${path}) -> ${resp.status()} ${await resp.text()}`)
  }
}

/** 删除文件夹（递归），测试自清理用；失败静默（夹具可能已被被测行为删除）。 */
export async function apiDeleteLatexFolder(
  page: Page,
  projectId: string,
  folderPath: string,
): Promise<void> {
  const token = await page.evaluate(() => localStorage.getItem('papersage_access_token') ?? '')
  await page.request.delete(`/api/v1/projects/${projectId}/latex-files/folder`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { folder_path: folderPath },
  })
}

/**
 * Read an env var with a default. Used to pick up E2E_* values
 * exported by the seed script.
 */
export function e2eEnv(name: string, fallback = ''): string {
  return process.env[name] ?? fallback
}
