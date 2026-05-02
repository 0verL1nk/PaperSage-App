import { test, expect } from '@playwright/test'

const PROJECT_ID = 'test-project-1'
const WRITING_URL = `/project/${PROJECT_ID}/writing`

// API route patterns (see src/services/api-routes.ts)
const API_AUTH_ME = '**/api/v1/auth/me'
const API_PROJECT_DETAIL = `**/api/v1/projects/${PROJECT_ID}`
const API_LATEX_FILES = `**/api/v1/projects/${PROJECT_ID}/latex-files/`
const API_LATEX_COMPILE = `**/api/v1/projects/${PROJECT_ID}/latex/compile/`

/**
 * Helper: set localStorage tokens so TanStack Router auth guard passes
 * before the page loads. The keys must match session-store.ts constants.
 */
async function seedAuthTokens(page: import('@playwright/test').Page) {
  await page.goto('/')
  await page.evaluate(() => {
    window.localStorage.setItem('papersage_access_token', 'fake-access-token')
    window.localStorage.setItem('papersage_refresh_token', 'fake-refresh-token')
  })
}

/**
 * Helper: install all API route mocks needed for the writing page to render.
 */
async function mockWritingPageAPIs(page: import('@playwright/test').Page) {
  // Auth identity check
  await page.route(API_AUTH_ME, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        data: { id: 'user-1', username: 'test', email: 'test@test.com' },
      }),
    })
  })

  // Project detail
  await page.route(API_PROJECT_DETAIL, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        data: { id: PROJECT_ID, name: 'Test Project' },
      }),
    })
  })

  // LaTeX file listing
  await page.route(API_LATEX_FILES, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        data: {
          files: [
            { id: 'f1', file_path: 'main.tex', content_type: 'text/x-tex', size_bytes: 42, last_saved_at: '2025-01-01T00:00:00Z' },
          ],
        },
      }),
    })
  })
}

test.describe('Compile feature', () => {
  test('should redirect to auth when not logged in', async ({ page }) => {
    await page.goto(WRITING_URL)
    await page.waitForURL(/\/auth/, { timeout: 5000 })
    await expect(page.getByPlaceholder('请输入您的账号')).toBeVisible()
  })

  test('should load writing page with mocked auth', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    await page.goto(WRITING_URL)

    // The page should render the Explorer sidebar heading
    await expect(page.getByText('Explorer')).toBeVisible({ timeout: 10000 })

    // The empty preview area should show "Ready to Compile"
    await expect(page.getByText('Ready to Compile')).toBeVisible()
  })

  test('should trigger compile API when compile button is clicked', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    // Mock compile submit — returns an async job
    let compileRequested = false
    await page.route(API_LATEX_COMPILE, async (route) => {
      if (route.request().method() === 'POST') {
        compileRequested = true
        await route.fulfill({
          status: 202,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 202,
            data: { job_id: 'job-123', status: 'pending' },
          }),
        })
      }
    })

    // Mock file content fetch for main.tex (needed before compile auto-saves)
    await page.route(`**/api/v1/projects/${PROJECT_ID}/latex-files/main.tex`, async (route) => {
      if (route.request().method() === 'GET') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 200,
            data: { path: 'main.tex', content: '\\documentclass{article}\n\\begin{document}\nHello\n\\end{document}' },
          }),
        })
      } else {
        // PUT (auto-save)
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: null }) })
      }
    })

    await page.goto(WRITING_URL)
    await expect(page.getByText('Explorer')).toBeVisible({ timeout: 10000 })

    // Wait for file to be selected and editor to load
    await expect(page.getByText('main.tex')).toBeVisible()

    // Click the compile button (Play icon in toolbar with green styling)
    const compileButton = page.locator('button').filter({ has: page.locator('svg.lucide-play') }).first()
    await compileButton.click()

    // Verify the compile POST was sent
    await expect.poll(() => compileRequested, { timeout: 5000 }).toBe(true)
  })
})
