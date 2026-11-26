import { expect, test, type Page } from '@playwright/test'

/**
 * 管理后台「编译错误日志」：列表 / 筛选 / 详情 / 空 / 错误 / 403 / 320px。
 * 接口全部 mock，不需要真实诊断数据。
 */

const ADMIN_URL = '/app/admin/compile-diagnostics'
const LIST_URL = '**/api/v1/admin/compile-diagnostics*'
const DETAIL_URL = '**/api/v1/admin/compile-diagnostics/*'

const ITEM = {
  id: '01a21861-38fc-72c8-bbc1-15f044c684c2',
  created_at: '2026-11-27T01:02:03+08:00',
  user_id: null,
  username: null,
  project_id: null,
  project_name: null,
  is_guest: true,
  client_kind: 'web',
  engine: 'xelatex',
  engine_version: 'papersage-swiftlatex-2026-10-2',
  resource_profile: 'texlive-2026-r80534',
  failure_kind: 'latex',
  failure_stage: 'compile',
  duration_ms: 8421,
  http_status: null,
  error_message: '! Undefined control sequence.',
  failed_resources: [{ kind: 'package', name: 'ctex.sty', state: 'missing', status: 404, error: 'NoSuchKey' }],
}

const DETAIL = {
  ...ITEM,
  compile_job_id: null,
  log_excerpt: '! Undefined control sequence.\nl.42',
  client_env: { platform: 'Linux', language: 'zh-CN', online: true },
}

const EMPTY_PAGE = { items: [], total: 0, page: 1, page_size: 20 }

async function mockMe(page: Page, isSuperuser: boolean) {
  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        data: { id: 'user-1', username: 'admin', email: 'admin@test.com', isSuperuser },
      }),
    })
  })
}

test.describe('Admin compile diagnostics', () => {
  test('lists records, filters by kind and opens the detail sheet', async ({ page }) => {
    await mockMe(page, true)
    const listQueries: string[] = []
    await page.route(LIST_URL, async (route) => {
      const url = new URL(route.request().url())
      listQueries.push(url.search)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { items: [ITEM], total: 1, page: 1, page_size: 20 } }),
      })
    })
    await page.route(DETAIL_URL, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: DETAIL }),
      })
    })

    await page.goto(ADMIN_URL)
    await expect(page.getByText('! Undefined control sequence.')).toBeVisible()
    await expect(page.getByText('游客')).toBeVisible()

    // 筛选：选择「LaTeX 编译」后请求应带上 failure_kind。
    await page.getByRole('combobox', { name: '错误类型' }).click()
    await page.getByRole('option', { name: 'LaTeX 编译' }).click()
    await expect.poll(() => listQueries.some((query) => query.includes('failure_kind=latex'))).toBe(true)

    await page.getByRole('button', { name: '查看详情' }).first().click()
    await expect(page.getByText('编译错误详情')).toBeVisible()
    await expect(page.getByText('! Undefined control sequence.')).toHaveCount(2)
    await expect(page.getByRole('button', { name: /复制 JSON/ })).toBeVisible()
    await expect(page.getByRole('button', { name: /导出 JSON/ })).toBeVisible()
  })

  test('shows the empty state', async ({ page }) => {
    await mockMe(page, true)
    await page.route(LIST_URL, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: EMPTY_PAGE }),
      })
    })

    await page.goto(ADMIN_URL)
    await expect(page.getByText('暂无编译错误记录')).toBeVisible()
  })

  test('shows a retryable error state', async ({ page }) => {
    await mockMe(page, true)
    await page.route(LIST_URL, async (route) => {
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ code: 500, data: null, info: 'Internal server error' }),
      })
    })

    await page.goto(ADMIN_URL)
    await expect(page.getByText('无法加载编译错误日志')).toBeVisible()
    await expect(page.getByRole('button', { name: /重试/ })).toBeVisible()
  })

  test('shows a permission notice instead of logging the user out on 403', async ({ page }) => {
    await mockMe(page, false)
    await page.route(LIST_URL, async (route) => {
      await route.fulfill({
        status: 403,
        contentType: 'application/json',
        body: JSON.stringify({ code: 403, data: null, info: 'Admin access required' }),
      })
    })

    await page.goto(ADMIN_URL)
    await expect(page.getByText('需要管理员权限')).toBeVisible()
    await expect(page).toHaveURL(/\/app\/admin\/compile-diagnostics/)
  })

  test('does not overflow horizontally at 320px', async ({ page }) => {
    await page.setViewportSize({ width: 320, height: 720 })
    await mockMe(page, true)
    await page.route(LIST_URL, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { items: [ITEM], total: 1, page: 1, page_size: 20 } }),
      })
    })

    await page.goto(ADMIN_URL)
    await expect(page.getByText('查看详情')).toBeVisible()
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
    expect(overflow).toBeLessThanOrEqual(0)
  })
})
