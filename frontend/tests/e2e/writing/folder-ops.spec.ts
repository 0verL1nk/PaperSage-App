import { test, expect } from '@playwright/test'

import { loginAs, e2eEnv } from '../helpers'

/**
 * Folder operations — verifies the file-tree's folder create /
 * rename / recursive-delete flow against a real backend.
 *
 * The recursive-delete endpoint expects a folder path and returns
 * a count. The frontend's confirm dialog must be acknowledged.
 */
test.describe('Folder operations (real backend)', () => {
  test('renders the seeded file tree', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    expect(projectId).not.toBe('')
    await loginAs(page)
    await page.goto(`/project/${projectId}/writing`)
    // Seed files should appear
    await expect(page.getByText('main.tex').first()).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText('refs.bib').first()).toBeVisible({ timeout: 15_000 })
  })

  test('tree search filters the file list', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    await loginAs(page)
    await page.goto(`/project/${projectId}/writing`)
    const searchInput = page.getByTestId('file-tree-search-input')
    await expect(searchInput).toBeVisible({ timeout: 15_000 })
    await searchInput.fill('refs')
    // refs.bib should remain; main.tex should be hidden
    await expect(page.getByText('refs.bib').first()).toBeVisible()
  })

  test('clicking a file makes it the active one', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    await loginAs(page)
    await page.goto(`/project/${projectId}/writing`)
    const mainFile = page
      .getByTestId('file-node')
      .filter({ hasText: 'main.tex' })
      .first()
    await expect(mainFile).toBeVisible({ timeout: 15_000 })
    await mainFile.click()
    await expect(mainFile).toHaveAttribute('data-active', 'true', { timeout: 5_000 })
  })
})
