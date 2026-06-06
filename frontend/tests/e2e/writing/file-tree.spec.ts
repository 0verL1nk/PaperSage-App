import { test, expect } from '@playwright/test'
import {
  seedAuthTokens,
  mockWritingPageAPIs,
  WRITING_URL,
} from '../helpers'

/**
 * E2E tests for the file tree component on the writing page.
 *
 * Covers the polish-file-tree OpenSpec change tasks 10.3 (search) and
 * 10.4 (keyboard shortcuts). Task 10.1 (compile flow) is in
 * writing-happy-path.spec.ts. Task 10.2 (create/rename/delete ops)
 * is in file-ops.spec.ts.
 */

test.describe('File tree — search (OpenSpec 10.3)', () => {
  test('filters visible files by name', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await page.goto(WRITING_URL)

    await expect(page.getByTestId('file-node').first()).toBeVisible({ timeout: 10_000 })

    // Type 'refs' — only refs.bib should remain visible
    await page.getByTestId('file-tree-search-input').fill('refs')
    await expect(page.getByTestId('file-node')).toHaveCount(1)
    await expect(page.getByTestId('file-node').first()).toHaveAttribute('data-file-path', 'refs.bib')

    // Type 'main' — only main.tex should remain
    await page.getByTestId('file-tree-search-input').fill('main')
    await expect(page.getByTestId('file-node')).toHaveCount(1)
  })

  test('shows "无匹配文件" when search has no hits', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await page.goto(WRITING_URL)

    await expect(page.getByTestId('file-node').first()).toBeVisible({ timeout: 10_000 })
    await page.getByTestId('file-tree-search-input').fill('nonexistent-pattern-xyz')
    await expect(page.getByText('无匹配文件')).toBeVisible()
    await expect(page.getByTestId('file-node')).toHaveCount(0)
  })

  test('clear button restores all files', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await page.goto(WRITING_URL)

    await expect(page.getByTestId('file-node').first()).toBeVisible({ timeout: 10_000 })

    // Filter down to one file
    await page.getByTestId('file-tree-search-input').fill('refs')
    await expect(page.getByTestId('file-node')).toHaveCount(1)

    // Click clear
    await page.getByTestId('file-tree-search-clear').click()
    await expect(page.getByTestId('file-tree-search-input')).toHaveValue('')
    // All 3 mock files should be visible again
    await expect(page.getByTestId('file-node')).toHaveCount(3)
  })
})

test.describe('File tree — keyboard shortcuts (OpenSpec 10.4)', () => {
  test('F2 starts inline rename, Escape cancels', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await page.goto(WRITING_URL)

    const mainFile = page.getByTestId('file-node').filter({ hasText: 'main.tex' })
    await expect(mainFile).toBeVisible({ timeout: 10_000 })

    // Select the file by clicking
    await mainFile.click()

    // F2 should put the node in edit mode → InlineInput appears
    await page.keyboard.press('F2')
    const renameInput = page.getByTestId('inline-rename-input')
    await expect(renameInput).toBeVisible({ timeout: 2_000 })
    await expect(renameInput).toBeFocused()

    // Escape cancels the edit
    await page.keyboard.press('Escape')
    await expect(renameInput).not.toBeVisible()
  })

  test('Delete shows confirmation dialog and API is called after confirm', async ({ page }) => {
    let deleteCalled = false
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    // Capture DELETE calls on individual file paths
    await page.route('**/api/v1/projects/*/latex-files/*', async (route) => {
      const url = route.request().url()
      if (url.endsWith('/')) {
        await route.fallback()
        return
      }
      if (route.request().method() === 'DELETE') {
        deleteCalled = true
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ code: 200, data: null, info: '' }),
        })
        return
      }
      await route.fallback()
    })

    await page.goto(WRITING_URL)

    const mainFile = page.getByTestId('file-node').filter({ hasText: 'main.tex' })
    await expect(mainFile).toBeVisible({ timeout: 10_000 })
    await mainFile.click()
    await expect(mainFile).toHaveAttribute('data-active', 'true')

    // Pressing Delete opens a confirm dialog
    await page.keyboard.press('Delete')
    const confirmButton = page.getByRole('button', { name: '删除' })
    await expect(confirmButton).toBeVisible()
    expect(deleteCalled).toBe(false)

    // Confirming the dialog fires the actual DELETE request
    await confirmButton.click()
    await expect.poll(() => deleteCalled, { timeout: 3_000 }).toBe(true)
  })
})
