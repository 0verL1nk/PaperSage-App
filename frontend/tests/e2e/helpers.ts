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
}
