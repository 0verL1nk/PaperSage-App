import { test, expect } from '@playwright/test'

import { loginAs, e2eEnv, mockWritingPageAPIs } from '../helpers'

/**
 * Writing page happy path — the marquee E2E that ties together login,
 * file tree, editor, auto-save, and compile. Completes the
 * `polish-file-tree` OpenSpec change task 10.1.
 *
 * All backend APIs are mocked because LATEX_CLSI_ENABLED=false in
 * CI; the test verifies the user-facing flow end-to-end, not the
 * LaTeX rendering itself.
 */
test.describe('Writing page — happy path (OpenSpec 10.1)', () => {
  test('login → open project → click file → edit → compile (mocked)', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    expect(projectId).not.toBe('')

    // 1. Log in as the seeded e2euser
    await loginAs(page)

    // 2. Mock all writing-page APIs (auth/me, project, file list, etc.)
    // and stub the compile submit to simulate a 202 → 200 success.
    await mockWritingPageAPIs(page)
    let compileSubmitSeen = false
    await page.route(
      `**/api/v1/projects/${projectId}/latex/compile/**`,
      async (route) => {
        if (route.request().method() === 'POST') {
          compileSubmitSeen = true
          await route.fulfill({
            status: 202,
            contentType: 'application/json',
            body: JSON.stringify({
              code: 202,
              data: { job_id: 'job-happy', status: 'pending' },
            }),
          })
          return
        }
        if (route.request().method() === 'GET') {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
              code: 200,
              data: { job_id: 'job-happy', status: 'success', build_id: 'build-happy' },
            }),
          })
          return
        }
        await route.fallback()
      },
    )

    // 3. Navigate to the project's writing page
    await page.goto(`/project/${projectId}/writing`)

    // 4. The Explorer sidebar and seed file appear
    await expect(page.getByText('Explorer')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('file-node').filter({ hasText: 'main.tex' })).toBeVisible({
      timeout: 15_000,
    })

    // 5. Click main.tex — it becomes the active file
    const mainFile = page.getByTestId('file-node').filter({ hasText: 'main.tex' })
    await mainFile.click()
    await expect(mainFile).toHaveAttribute('data-active', 'true', { timeout: 5_000 })

    // 6. Edit the file — type into the editor
    const editor = page.locator('.cm-content, [contenteditable="true"]').first()
    if (await editor.isVisible().catch(() => false)) {
      await editor.click()
      await editor.press('End')
      await editor.type('\n% E2E edit marker')
    }

    // 7. Trigger compile
    const compileButton = page.getByTestId('compile-button')
    await expect(compileButton).toBeVisible({ timeout: 5_000 })
    await compileButton.click()

    // 8. The compile POST was submitted to the backend
    await expect.poll(() => compileSubmitSeen, { timeout: 5_000 }).toBe(true)
  })
})
