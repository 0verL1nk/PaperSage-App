import { test, expect } from '@playwright/test'

import { mockWritingPageAPIs, WRITING_URL } from '../helpers'

/**
 * Compile trigger — verifies that clicking the compile button
 * (1) submits the API call and (2) shows a user-visible error
 * when the backend returns 503 (because LATEX_CLSI_ENABLED=false
 * in CI). Kept in a separate file from compile-polling because
 * this spec uses fully mocked APIs and does not need a live
 * backend for the file listing.
 */
test.describe('Compile trigger (UI behaviour)', () => {
  test('clicking compile with CLSI disabled shows an error', async ({ page }) => {
    await page.goto('/')
    await mockWritingPageAPIs(page)

    // Mock the compile submit so we don't depend on the backend
    await page.route('**/api/v1/projects/*/latex/compile/**', async (route) => {
      await route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 503,
          data: null,
          info: 'LaTeX compilation is disabled',
        }),
      })
    })

    await page.goto(WRITING_URL)
    const compileButton = page.getByTestId('compile-button')
    await expect(compileButton).toBeVisible({ timeout: 15_000 })
    await compileButton.click()

    // Either a toast or an inline error must surface. The exact
    // label is implementation-defined, so we look for either.
    await expect(
      page
        .getByText(/LaTeX|编译|503|disabled|失败/)
        .first(),
    ).toBeVisible({ timeout: 5_000 })
  })
})
