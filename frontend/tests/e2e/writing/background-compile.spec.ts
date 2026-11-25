import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { expect, test, type Page } from '@playwright/test'

import { MOCK_PROJECT, TEST_PROJECT_ID, WRITING_URL, mockWritingPageAPIs, seedAuthTokens } from '../helpers'

const samplePdfPath = fileURLToPath(new URL('../fixtures/sample.pdf', import.meta.url))

/** Browser engine stand-in: reports progress, then resolves (or aborts) after a delay. */
function engineModule(pdfBase64: string, delayMs: number): string {
  return `
    const pdf = Uint8Array.from(atob(${JSON.stringify(pdfBase64)}), (char) => char.charCodeAt(0))
    export async function compileLatexInBrowser(_files, _rootFile, _compiler, options = {}) {
      options.onProgress?.({ stage: 'compile', message: 'fixture engine running', pass: 1, totalPasses: 1 })
      await new Promise((resolve, reject) => {
        const timer = setTimeout(resolve, ${delayMs})
        options.signal?.addEventListener('abort', () => {
          clearTimeout(timer)
          reject(new DOMException('Aborted', 'AbortError'))
        }, { once: true })
      })
      return { pdf, log: 'fixture log' }
    }
  `
}

async function installBrowserCompile(page: Page, options: {
  delayMs: number
  jobId: string
  buildId?: string
}): Promise<void> {
  const pdfBase64 = (await readFile(samplePdfPath)).toString('base64')
  await page.route('**/src/lib/latex/swiftlatex-browser-compiler.ts*', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/javascript', body: engineModule(pdfBase64, options.delayMs) })
  })
  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser`, async (route) => {
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: { job_id: options.jobId, status: 'running' } }),
    })
  })
  if (!options.buildId) return
  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/${options.jobId}/browser-result`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: { job_id: options.jobId, build_id: options.buildId, status: 'success' } }),
    })
  })
}

test.describe('Background browser compile', () => {
  test('keeps compiling on the prepare page, toasts on finish, and restores the PDF on return', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await installBrowserCompile(page, { delayMs: 2_500, jobId: 'bg-job-switch', buildId: 'bg-build-switch' })

    await page.goto(WRITING_URL)
    await page.getByTestId('compile-button').click()
    await expect(page.getByText('fixture engine running')).toBeVisible({ timeout: 15_000 })

    await page.getByRole('button', { name: '写作准备' }).click()
    await expect(page).toHaveURL(new RegExp(`/project/${TEST_PROJECT_ID}/prepare$`))
    // The engine keeps running after the writing page unmounted.
    await expect(page.getByText('《Test Project》PDF 编译完成')).toBeVisible({ timeout: 20_000 })

    await page.getByRole('button', { name: '论文撰写' }).click()
    await expect(page).toHaveURL(new RegExp(`/project/${TEST_PROJECT_ID}/writing$`))
    await expect(page.getByRole('button', { name: '下载 PDF' })).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('compile-button')).toBeEnabled()
  })

  test('keeps compiling on the project list and restores the PDF on return', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await page.route('**/api/v1/projects', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: [MOCK_PROJECT], info: '' }),
      })
    })
    await installBrowserCompile(page, { delayMs: 2_500, jobId: 'bg-job-list', buildId: 'bg-build-list' })

    await page.goto(WRITING_URL)
    await page.getByTestId('compile-button').click()
    await expect(page.getByText('fixture engine running')).toBeVisible({ timeout: 15_000 })

    await page.getByRole('button', { name: '返回项目列表' }).click()
    await expect(page).toHaveURL(/\/app\/projects$/)
    await expect(page.getByText('《Test Project》PDF 编译完成')).toBeVisible({ timeout: 20_000 })

    await page.goBack()
    await expect(page).toHaveURL(new RegExp(`/project/${TEST_PROJECT_ID}/writing$`))
    await expect(page.getByRole('button', { name: '下载 PDF' })).toBeVisible({ timeout: 15_000 })
  })

  test('only the explicit cancel button stops the compile', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    let cancelRequests = 0
    await installBrowserCompile(page, { delayMs: 60_000, jobId: 'bg-job-cancel' })
    await page.route((url) => url.pathname.endsWith(`/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/`)
      && url.searchParams.get('job_id') === 'bg-job-cancel', async (route) => {
      if (route.request().method() !== 'DELETE') {
        await route.fallback()
        return
      }
      cancelRequests++
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { cancelled: true, job_id: 'bg-job-cancel' } }),
      })
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('compile-button').click()
    await expect(page.getByText('fixture engine running')).toBeVisible({ timeout: 15_000 })
    await page.getByTestId('cancel-browser-compile').click()

    await expect(page.getByTestId('cancel-browser-compile')).toHaveCount(0)
    await expect.poll(() => cancelRequests).toBe(1)
    await expect(page.getByText('编译错误')).toHaveCount(0)

    await page.getByRole('button', { name: '写作准备' }).click()
    await expect(page.getByText('《Test Project》编译失败')).toHaveCount(0)
  })
})
