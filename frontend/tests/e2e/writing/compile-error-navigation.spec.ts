import { expect, test } from '@playwright/test'

import { API_LATEX_FILE_CONTENT, mockWritingPageAPIs, seedAuthTokens, TEST_PROJECT_ID, WRITING_URL } from '../helpers'

/**
 * 浏览器编译失败 → 报错条目定位与一键跳行（引擎用桩，不需要真实 WASM）。
 * 断言：多文件日志归属到正确文件、点击后切到该文件、BibTeX 条目显式不可跳。
 */

const BROWSER_JOB_URL = `**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser`
const BROWSER_FAILURE_URL = `**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/*/browser-failure`

async function stubCompiler(page: import('@playwright/test').Page, body: string) {
  await page.route('**/src/lib/latex/swiftlatex-browser-compiler.ts*', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/javascript', body })
  })
}

test.describe('Compile error navigation', () => {
  test.describe.configure({ timeout: 90_000 })

  test('attributes multi-file errors, jumps to the file, and marks BibTeX issues non-navigable', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    const log = [
      '--- pdflatex pass 1 ---',
      '(./main.tex',
      '(./sections/intro.tex',
      '! Undefined control sequence.',
      'l.42 \\badcommand',
      ')',
      ')',
      '--- BibTeX main ---',
      "I couldn't open database file missing.bib",
    ].join('\n')
    await stubCompiler(page, `export async function compileLatexInBrowser() { throw new Error(${JSON.stringify(log)}) }`)

    await page.route(BROWSER_JOB_URL, async (route) => {
      await route.fulfill({
        status: 202,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { job_id: 'nav-job-1', status: 'running' } }),
      })
    })
    await page.route(BROWSER_FAILURE_URL, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: null, info: '' }),
      })
    })

    // 报错行 42 必须真实存在于文件内容中，否则 Monaco 不会渲染该行的装饰；
    // 同时记录跨文件内容请求：跳行必须真的切换到 sections/intro.tex 并加载它。
    const introContent = Array.from({ length: 48 }, (_, index) =>
      index === 41 ? '\\badcommand' : `% filler line ${index + 1}`,
    ).join('\n')
    let introFetched = 0
    await page.route(API_LATEX_FILE_CONTENT('sections/intro.tex'), async (route) => {
      if (route.request().method() !== 'GET') {
        await route.fallback()
        return
      }
      introFetched += 1
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { path: 'sections/intro.tex', content: introContent } }),
      })
    })

    await page.goto(WRITING_URL, { timeout: 60_000 })
    await page.getByTestId('compile-button').click({ timeout: 30_000 })

    // 原始日志 <pre> 也包含同一句错误：按「可点击的报错条目」定位，避免 strict mode。
    // （CI 曾因裸 getByText 同时命中条目与日志而 strict mode violation。）
    const panel = page.getByTestId('compile-failure-panel')
    const issueRow = panel.getByRole('button', { name: /sections\/intro\.tex:42/ })
    await expect(issueRow).toBeVisible({ timeout: 30_000 })
    await expect(issueRow).toContainText('Undefined control sequence.')
    await expect(panel.getByText(/BibTeX 日志只给出辅助文件位置/).first()).toBeVisible()

    const navigate = panel.getByTitle('跳转到报错位置')
    await expect(navigate).toHaveCount(1)
    await navigate.click()

    await expect.poll(() => introFetched, { timeout: 15_000 }).toBeGreaterThan(0)
    // 行内标红：Monaco 装饰在内容同步后重建（有 250ms 去抖）。
    await expect(page.locator('.monaco-editor .latex-issue-line-error').first()).toBeVisible({ timeout: 15_000 })
  })
})
