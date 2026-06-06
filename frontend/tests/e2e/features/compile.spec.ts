import { test, expect } from '@playwright/test'
import {
  seedAuthTokens,
  mockWritingPageAPIs,
  WRITING_URL,
  API_LATEX_COMPILE,
} from '../helpers'

const PROJECT_ID = 'test-project-1'

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

    const mainFile = page.getByTestId('file-node').filter({ hasText: 'main.tex' })
    await expect(mainFile).toBeVisible()
    await expect(mainFile).toHaveAttribute('data-active', 'true')

    const compileButton = page.getByTestId('compile-button')
    await compileButton.click()

    // Verify the compile POST was sent
    await expect.poll(() => compileRequested, { timeout: 5000 }).toBe(true)
  })
})
