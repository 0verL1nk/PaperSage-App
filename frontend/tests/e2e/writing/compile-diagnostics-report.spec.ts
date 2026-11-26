import { expect, test } from '@playwright/test'

import { mockWritingPageAPIs, seedAuthTokens, TEST_PROJECT_ID, WRITING_URL } from '../helpers'

/**
 * 浏览器编译失败 → 自动上报脱敏诊断（引擎用桩，不需要真实 WASM）。
 * 隐私断言：payload 里绝不能出现源码回显或项目文件路径。
 */

const DIAGNOSTICS_URL = '**/api/v1/compile-diagnostics'
const BROWSER_JOB_URL = `**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser`

async function stubCompiler(page: import('@playwright/test').Page, body: string) {
  await page.route('**/src/lib/latex/swiftlatex-browser-compiler.ts*', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/javascript', body })
  })
}

test.describe('Compile diagnostics reporting', () => {
  // 失败路径要等引擎桩抛错 + 异步上报，慢机器上 30s 默认值偏紧。
  test.describe.configure({ timeout: 60_000 })
  test('reports a sanitized diagnostic when the browser engine fails', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    await stubCompiler(page, `
      export async function compileLatexInBrowser() {
        throw new Error('! Undefined control sequence.\\nl.42 \\\\secretSourceText\\n(./sections/intro.tex');
      }
    `)
    // 诊断里的 engine 必须与真实创建编译任务时用的引擎一致；页面默认引擎由
    // 文件内容探测（这里 main.tex 是 article，默认 pdfLaTeX），由请求体给出真值。
    let jobCompiler = ''
    await page.route(BROWSER_JOB_URL, async (route) => {
      const body = route.request().postDataJSON() as { compiler?: string }
      jobCompiler = body.compiler ?? ''
      await route.fulfill({
        status: 202,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { job_id: 'diag-job-1', status: 'running' } }),
      })
    })

    const payloads: Array<Record<string, unknown>> = []
    await page.route(DIAGNOSTICS_URL, async (route) => {
      payloads.push(route.request().postDataJSON() as Record<string, unknown>)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { id: 'diag-e2e-1' }, info: '' }),
      })
    })

    await page.goto(WRITING_URL, { timeout: 60_000 })
    // 首屏 Monaco 编译会短时阻塞主线程；慢机器/冷启动下 10s 默认值不够。
    await page.getByTestId('compile-button').click({ timeout: 30_000 })

    // 以实际上报请求为准（UI 文案随后渲染），避免慢渲染掩盖上报结果。
    await expect.poll(() => payloads.length, { timeout: 30_000 }).toBe(1)

    const payload = payloads[0]
    expect(payload.failure_kind).toBe('latex')
    expect(payload.failure_stage).toBe('compile')
    expect(jobCompiler).not.toBe('')
    expect(payload.engine).toBe(jobCompiler)
    expect(payload.compile_job_id).toBe('diag-job-1')
    expect(String(payload.log_excerpt)).toContain('l.42')
    expect(String(payload.log_excerpt)).not.toContain('secretSourceText')
    expect(String(payload.log_excerpt)).not.toContain('intro.tex')
    expect(String(payload.error_message)).toBe('! Undefined control sequence.')
    await expect(page.getByText(/已自动上报诊断信息/)).toBeVisible({ timeout: 15_000 })
  })

  test('does not report anything when the user cancels compilation', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    await stubCompiler(page, `
      export async function compileLatexInBrowser(_files, _rootFile, _compiler, options = {}) {
        return await new Promise((_resolve, reject) => {
          options.signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true });
        });
      }
    `)
    await page.route(BROWSER_JOB_URL, async (route) => {
      await route.fulfill({
        status: 202,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { job_id: 'diag-job-cancel', status: 'running' } }),
      })
    })
    await page.route((url) => url.pathname.endsWith(`/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/`)
      && url.searchParams.get('job_id') === 'diag-job-cancel', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { cancelled: true, job_id: 'diag-job-cancel' } }),
      })
    })

    let diagnosticRequests = 0
    await page.route(DIAGNOSTICS_URL, async (route) => {
      diagnosticRequests++
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { id: 'should-not-happen' }, info: '' }),
      })
    })

    await page.goto(WRITING_URL, { timeout: 60_000 })
    // 首屏 Monaco 编译会短时阻塞主线程；慢机器/冷启动下 10s 默认值不够。
    await page.getByTestId('compile-button').click({ timeout: 30_000 })
    await expect(page.getByTestId('cancel-browser-compile')).toBeVisible({ timeout: 20_000 })
    await page.getByTestId('cancel-browser-compile').click()

    await expect(page.getByTestId('cancel-browser-compile')).toHaveCount(0)
    await expect.poll(() => diagnosticRequests).toBe(0)
  })
})
