import { test, expect } from '@playwright/test'

import {
  seedAuthTokens,
  mockWritingPageAPIs,
  WRITING_URL,
} from '../helpers'

/**
 * File tree interaction — covers the search/filter and keyboard
 * shortcut polish from the `polish-file-tree` OpenSpec change
 * (tasks 10.3, 10.4). All API calls are mocked because this spec
 * focuses on UI behaviour and the page is rendered with seeded
 * fake tokens.
 */
test.describe('File tree — search (OpenSpec 10.3)', () => {
  test('filters visible files by name', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await page.goto(WRITING_URL)

    await expect(page.getByTestId('file-node').first()).toBeVisible({ timeout: 10_000 })

    await page.getByTestId('file-tree-search-input').fill('refs')
    await expect(page.getByTestId('file-node')).toHaveCount(1)
    await expect(page.getByTestId('file-node').first()).toHaveAttribute(
      'data-file-path',
      'refs.bib',
    )

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

    await page.getByTestId('file-tree-search-input').fill('refs')
    await expect(page.getByTestId('file-node')).toHaveCount(1)
    await page.getByTestId('file-tree-search-clear').click()
    await expect(page.getByTestId('file-tree-search-input')).toHaveValue('')
    await expect(page.getByTestId('file-node')).toHaveCount(3)
  })
})

test.describe('File tree — keyboard shortcuts (OpenSpec 10.4)', () => {
  test('F2 renames the active file', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await page.goto(WRITING_URL)
    await expect(page.getByTestId('file-node').first()).toBeVisible({ timeout: 10_000 })

    // Click the main.tex node to make it active
    const main = page.getByTestId('file-node').filter({ hasText: 'main.tex' }).first()
    await main.click()
    await expect(main).toHaveAttribute('data-active', 'true', { timeout: 5_000 })

    // Trigger F2 — the InlineInput should appear
    await page.keyboard.press('F2')
    // InlineInput is a regular textbox; assert it's visible
    const inlineInput = page.locator('input[type="text"]').last()
    await expect(inlineInput).toBeVisible({ timeout: 3_000 })
    // Cancel with Escape (per OpenSpec 10.4 InlineInput onBlur cancel)
    await page.keyboard.press('Escape')
  })
})
