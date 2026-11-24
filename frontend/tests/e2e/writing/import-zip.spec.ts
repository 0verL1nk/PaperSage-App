import JSZip from 'jszip'
import { test, expect } from '@playwright/test'

import { API_LATEX_FILES, loginAs, mockWritingPageAPIs, MOCK_LATEX_FILES, TEST_PROJECT_ID } from '../helpers'

/**
 * HELE-30 regression: uploading a multi-file LaTeX project ZIP through the
 * writing page must unpack it and keep the archive's directory structure.
 * Runs the real browser + real JSZip against mocked file APIs.
 */
test.describe('Writing page — multi-file project import', () => {
  test('unpacks an uploaded ZIP and keeps nested paths in the file tree', async ({ page }) => {
    await loginAs(page)
    await mockWritingPageAPIs(page)

    const files = [...MOCK_LATEX_FILES.data.files]
    const created: Array<{ path: string; content?: string }> = []
    // Registered after mockWritingPageAPIs so this list route wins.
    await page.route(API_LATEX_FILES, async (route) => {
      if (route.request().method() === 'POST') {
        const body = route.request().postDataJSON()
        created.push(body)
        const item = {
          id: `f-import-${created.length}`,
          file_path: body.path,
          content_type: 'text/x-tex',
          size_bytes: body.content?.length ?? 0,
          last_saved_at: '2026-01-01T00:00:00Z',
        }
        files.push(item)
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ code: 200, data: item }),
        })
        return
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { files } }),
      })
    })

    const zip = new JSZip()
    zip.file('thesis-e2e.tex', '\\documentclass{book}\n\\begin{document}hi\\end{document}')
    zip.file('body/ch1-e2e.tex', 'chapter body')
    const buffer = await zip.generateAsync({ type: 'nodebuffer' })

    await page.goto(`/project/${TEST_PROJECT_ID}/writing`)
    await expect(page.getByRole('heading', { name: '文件', exact: true })).toBeVisible({ timeout: 15_000 })

    await page.setInputFiles('input[type="file"]', {
      name: 'hithesis-e2e.zip',
      mimeType: 'application/zip',
      buffer,
    })

    await expect.poll(() => created.map((item) => item.path).sort()).toEqual([
      'body/ch1-e2e.tex',
      'thesis-e2e.tex',
    ])
    await expect(page.getByTestId('file-node').filter({ hasText: 'thesis-e2e.tex' })).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('folder-node').filter({ hasText: 'body' })).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('file-node').filter({ hasText: 'ch1-e2e.tex' })).toBeVisible({ timeout: 15_000 })
  })
})
