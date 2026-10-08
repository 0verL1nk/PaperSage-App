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

  test('creates a new file via header button after showing a name field', async ({ page }) => {
    let createdPath = ''
    await page.route(API_LATEX_FILES, async (route) => {
      if (route.request().method() === 'POST') {
        const body = route.request().postDataJSON()
        createdPath = body?.path ?? ''
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 200,
            data: {
              id: 'f-new-test',
              file_path: body?.path ?? 'new_file.tex',
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

    // Clicking the Explorer action must immediately show a name field.
    await page.getByRole('button', { name: '新建文件', exact: true }).click()
    const nameInput = page.getByLabel('文件名称')
    await expect(nameInput).toBeVisible()
    await expect(nameInput).toHaveValue('new_file.tex')
    await nameInput.fill('chapter.tex')
    await page.getByRole('button', { name: '确认创建' }).click()
    await expect.poll(() => createdPath, { timeout: 5_000 }).toBe('chapter.tex')
    await expect(nameInput).not.toBeVisible()
  })

  test('creates a new folder via header button after showing a name field', async ({ page }) => {
    let createdPath = ''
    await page.route(API_LATEX_FILES, async (route) => {
      if (route.request().method() === 'POST') {
        const body = route.request().postDataJSON()
        createdPath = body?.path ?? ''
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 200,
            data: {
              id: 'f-new-folder',
              file_path: body?.path ?? 'new_folder/.gitkeep',
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

    // Clicking the Explorer action must immediately show a name field.
    await page.getByRole('button', { name: '新建文件夹', exact: true }).click()
    const nameInput = page.getByLabel('文件夹名称')
    await expect(nameInput).toBeVisible()
    await expect(nameInput).toHaveValue('new_folder')
    await nameInput.fill('chapters')
    await page.getByRole('button', { name: '确认创建' }).click()
    await expect.poll(() => createdPath, { timeout: 5_000 }).toBe('chapters/.gitkeep')
    await expect(nameInput).not.toBeVisible()
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
    await expect(renameInput).toBeVisible({ timeout: 5_000 })

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
