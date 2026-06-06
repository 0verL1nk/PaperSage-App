import { test, expect } from '@playwright/test'
import {
  seedAuthTokens,
  mockWritingPageAPIs,
  WRITING_URL,
  API_LATEX_FILES,
} from '../helpers'

/**
 * E2E tests for file tree CRUD operations on the writing page.
 *
 * Covers the polish-file-tree OpenSpec change task 10.2: create file,
 * create folder, rename, and delete operations.
 */

test.describe('File tree — CRUD operations (OpenSpec 10.2)', () => {
  test.beforeEach(async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await page.goto(WRITING_URL)
    await expect(page.getByTestId('file-node').first()).toBeVisible({ timeout: 10_000 })
  })

  test('creates a new file via header button', async ({ page }) => {
    let createCalled = false
    await page.route(API_LATEX_FILES, async (route) => {
      if (route.request().method() === 'POST') {
        createCalled = true
        const body = route.request().postDataJSON()
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 200,
            data: {
              id: 'f-new-test',
              file_path: body?.file_path ?? 'new-file.tex',
              content_type: 'text/x-tex',
              size_bytes: 0,
              last_saved_at: '2025-06-01T00:00:00Z',
            },
          }),
        })
        return
      }
      await route.fallback()
    })

    // Click the "New File" (+) button in the Explorer header
    await page.getByTitle('新建文件').click()

    // Inline input appears for naming the new file
    const renameInput = page.getByTestId('inline-rename-input')
    await expect(renameInput).toBeVisible({ timeout: 3_000 })

    // Type a name and confirm
    await renameInput.fill('appendix.tex')
    await renameInput.press('Enter')

    // The POST to create the file was sent
    await expect.poll(() => createCalled, { timeout: 3_000 }).toBe(true)
  })

  test('creates a new folder via header button', async ({ page }) => {
    let createCalled = false
    await page.route(API_LATEX_FILES, async (route) => {
      if (route.request().method() === 'POST') {
        createCalled = true
        const body = route.request().postDataJSON()
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 200,
            data: {
              id: 'f-new-folder',
              file_path: body?.file_path?.replace('/.gitkeep', '') ?? 'new-folder',
              content_type: 'directory',
              size_bytes: 0,
              last_saved_at: '2025-06-01T00:00:00Z',
            },
          }),
        })
        return
      }
      await route.fallback()
    })

    // Click the "New Folder" button in the Explorer header
    await page.getByTitle('新建文件夹').click()

    // Inline input appears for naming the new folder
    const renameInput = page.getByTestId('inline-rename-input')
    await expect(renameInput).toBeVisible({ timeout: 3_000 })

    // Type a name and confirm
    await renameInput.fill('chapters')
    await renameInput.press('Enter')

    // The POST to create the folder (.gitkeep) was sent
    await expect.poll(() => createCalled, { timeout: 3_000 }).toBe(true)
  })

  test('renames a file via F2 + Enter', async ({ page }) => {
    let renameCalled = false
    await page.route('**/api/v1/projects/*/latex-files/**/rename', async (route) => {
      if (route.request().method() === 'POST') {
        renameCalled = true
        const body = route.request().postDataJSON()
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 200,
            data: {
              id: 'f1',
              file_path: body?.new_path ?? 'renamed.tex',
              content_type: 'text/x-tex',
              size_bytes: 42,
              last_saved_at: '2025-06-01T00:00:00Z',
            },
          }),
        })
        return
      }
      await route.fallback()
    })

    const mainFile = page.getByTestId('file-node').filter({ hasText: 'main.tex' })
    await expect(mainFile).toBeVisible()
    await mainFile.click()
    await expect(mainFile).toHaveAttribute('data-active', 'true')

    // F2 triggers inline rename
    await page.keyboard.press('F2')
    const renameInput = page.getByTestId('inline-rename-input')
    await expect(renameInput).toBeVisible({ timeout: 2_000 })

    // Clear and type new name
    await renameInput.fill('index.tex')
    await renameInput.press('Enter')

    // The rename POST was called
    await expect.poll(() => renameCalled, { timeout: 3_000 }).toBe(true)
  })

  test('deletes a file via Delete key + confirm dialog', async ({ page }) => {
    let deleteCalled = false
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

    const mainFile = page.getByTestId('file-node').filter({ hasText: 'main.tex' })
    await expect(mainFile).toBeVisible()
    await mainFile.click()
    await expect(mainFile).toHaveAttribute('data-active', 'true')

    // Press Delete — opens confirm dialog
    await page.keyboard.press('Delete')
    const confirmButton = page.getByRole('button', { name: '删除' })
    await expect(confirmButton).toBeVisible()

    // Confirm — fires DELETE request
    await confirmButton.click()
    await expect.poll(() => deleteCalled, { timeout: 3_000 }).toBe(true)
  })
})
