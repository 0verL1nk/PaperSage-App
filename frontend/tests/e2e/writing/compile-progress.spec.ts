import { expect, test, type Page } from '@playwright/test'

import { TEST_PROJECT_ID, WRITING_URL, mockWritingPageAPIs, seedAuthTokens } from '../helpers'

/**
 * Compile progress — the preview pane must switch to a live stage view the
 * moment a compile starts, instead of leaving the "还没有生成预览" empty state
 * up while a cold WASM compile downloads packages for minutes.
 */

function engineModule(cached: boolean): string {
  return `
    export async function compileLatexInBrowser(_files, _rootFile, _compiler, options = {}) {
      options.onProgress?.({ stage: 'engine', message: '正在启动 XeTeX WebAssembly 引擎' })
      options.onProgress?.({
        stage: 'format',
        message: ${JSON.stringify(cached ? '已从浏览器缓存载入 LaTeX 格式文件' : '正在首次生成 LaTeX 格式文件')},
        cached: ${cached},
      })
      options.onProgress?.({
        stage: 'resource',
        message: '正在加载 ctex.sty',
        resource: { name: 'ctex.sty', kind: 'package', state: 'loading' },
      })
      return await new Promise((_resolve, reject) => {
        options.signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })
      })
    }
  `
}

async function installHangingCompile(page: Page, options: { cached: boolean }) {
  await page.route('**/src/lib/latex/swiftlatex-browser-compiler.ts*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/javascript',
      body: engineModule(options.cached),
    })
  })
  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser`, async (route) => {
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: { job_id: 'progress-job', status: 'running' } }),
    })
  })
  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/progress-job/status/`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: { job_id: 'progress-job', status: 'cancelled' } }),
    })
  })
  await page.route(
    (url) => url.pathname.endsWith(`/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/`)
      && url.searchParams.get('job_id') === 'progress-job',
    async (route) => {
      if (route.request().method() !== 'DELETE') {
        await route.fallback()
        return
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { cancelled: true, job_id: 'progress-job' } }),
      })
    },
  )
}

test.describe('Compile progress view', () => {
  test('switches the preview pane to live stages and cancels on demand (1440×900)', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await installHangingCompile(page, { cached: false })

    await page.goto(WRITING_URL)
    await page.getByTestId('compile-button').click()

    const cancel = page.getByTestId('preview-cancel-compile')
    await expect(cancel).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText('还没有生成预览')).toHaveCount(0)
    await expect(page.getByText(/首次冷编译/)).toBeVisible()
    await expect(page.getByText('下载宏包与字体', { exact: true })).toBeVisible()
    await expect(page.getByText(/正在下载：ctex\.sty/)).toBeVisible()

    const timer = page.getByText(/已用时/)
    await expect(timer).toBeVisible()
    const firstReading = await timer.textContent()
    await expect
      .poll(async () => (await timer.textContent()) !== firstReading, { timeout: 4_000 })
      .toBe(true)

    await cancel.click()
    await expect(cancel).toHaveCount(0)
    await expect(page.getByText('还没有生成预览')).toBeVisible({ timeout: 10_000 })
  })

  test('tells the user when the browser format cache is hit', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await installHangingCompile(page, { cached: true })

    await page.goto(WRITING_URL)
    await page.getByTestId('compile-button').click()

    await expect(page.getByText(/已命中浏览器格式缓存/)).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText(/首次冷编译/)).toHaveCount(0)
    await page.getByTestId('preview-cancel-compile').click()
  })

  for (const viewport of [{ width: 390, height: 844 }, { width: 320, height: 640 }]) {
    test(`opens the progress pane with a reachable cancel button at ${viewport.width}px`, async ({ page }) => {
      await page.setViewportSize(viewport)
      await seedAuthTokens(page)
      await mockWritingPageAPIs(page)
      await installHangingCompile(page, { cached: false })

      await page.goto(WRITING_URL)
      await page.getByTestId('compile-button').click()

      await expect(page.getByTestId('writing-pane-preview')).toHaveAttribute('aria-selected', 'true')
      const cancel = page.getByTestId('preview-cancel-compile')
      await expect(cancel).toBeVisible({ timeout: 15_000 })
      await expect(page.getByText(/已用时/)).toBeVisible()

      const box = await cancel.boundingBox()
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44)

      await cancel.click()
      await expect(cancel).toHaveCount(0)
    })
  }
})
