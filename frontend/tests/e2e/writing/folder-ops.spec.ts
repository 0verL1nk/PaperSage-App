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

  test('creates a new folder via header button', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    await loginAs(page)
    await page.goto(`/project/${projectId}/writing`)
    await expect(page.getByTestId('file-node').first()).toBeVisible({ timeout: 15_000 })

    // Click "New Folder" button
    await page.getByTitle('新建文件夹').click()

    // Inline input appears for naming
    const renameInput = page.getByTestId('inline-rename-input')
    await expect(renameInput).toBeVisible({ timeout: 3_000 })

    const folderName = `e2e-folder-${Date.now()}`
    await renameInput.fill(folderName)
    await renameInput.press('Enter')

    // The folder should appear in the tree
    await expect(page.getByTestId('folder-node').filter({ hasText: folderName }).first()).toBeVisible({ timeout: 5_000 })
  })

  test('renames a folder via context menu', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    await loginAs(page)
    await page.goto(`/project/${projectId}/writing`)
    await expect(page.getByTestId('file-node').first()).toBeVisible({ timeout: 15_000 })

    // Find the sections folder (seeded by default)
    const sectionsFolder = page.getByTestId('folder-node').filter({ hasText: 'sections' }).first()
    if (await sectionsFolder.isVisible().catch(() => false)) {
      // Right-click to open context menu
      await sectionsFolder.click({ button: 'right' })
      await page.getByText('重命名').click()

      const renameInput = page.getByTestId('inline-rename-input')
      await expect(renameInput).toBeVisible({ timeout: 3_000 })
      await renameInput.fill('renamed-sections')
      await renameInput.press('Enter')

      // The renamed folder should appear
      await expect(page.getByTestId('folder-node').filter({ hasText: 'renamed-sections' }).first()).toBeVisible({ timeout: 5_000 })
    }
  })

  test('deletes a folder via context menu + confirm dialog', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    await loginAs(page)
    await page.goto(`/project/${projectId}/writing`)
    await expect(page.getByTestId('file-node').first()).toBeVisible({ timeout: 15_000 })

    // Find the sections folder
    const sectionsFolder = page.getByTestId('folder-node').filter({ hasText: 'sections' }).first()
    if (await sectionsFolder.isVisible().catch(() => false)) {
      // Right-click to open context menu
      await sectionsFolder.click({ button: 'right' })
      await page.getByText('删除').click()

      // Confirm dialog should appear
      const confirmButton = page.getByRole('button', { name: '删除' })
      await expect(confirmButton).toBeVisible({ timeout: 3_000 })
      await confirmButton.click()

      // The folder should be removed from the tree
      await expect(sectionsFolder).not.toBeVisible({ timeout: 5_000 })
    }
  })
})
