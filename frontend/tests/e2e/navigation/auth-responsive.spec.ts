import { test, expect } from '@playwright/test'

test('auth and browser-local guest entry remain usable on narrow phones through desktop', async ({ page }) => {
  const unmockedApiRequests: string[] = []
  await page.route('**/api/**', async (route) => {
    if (route.request().url().includes('/api/v1/auth/email-domains')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { domains: [] } }),
      })
      return
    }
    unmockedApiRequests.push(route.request().url())
    await route.abort()
  })

  const viewports = [
    { name: 'small phone', width: 320, height: 640 },
    { name: 'phone', width: 390, height: 844 },
    { name: 'tablet', width: 768, height: 1024 },
    { name: 'tablet landscape', width: 1024, height: 768 },
    { name: 'desktop', width: 1440, height: 900 },
  ]

  for (const viewport of viewports) {
    await page.goto('/auth')
    await page.evaluate(() => window.localStorage.clear())
    await page.reload()
    await page.setViewportSize({ width: viewport.width, height: viewport.height })
    await expect(page.getByRole('tab', { name: '账号登录' })).toBeVisible()
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), {
      message: `auth must not overflow horizontally at ${viewport.name}`,
    }).toBe(true)

    const loginButton = page.getByRole('button', { name: '立即登录' })
    await loginButton.scrollIntoViewIfNeeded()
    await expect(loginButton).toBeInViewport()
    expect(await loginButton.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)

    await page.getByRole('tab', { name: '注册新账号' }).click()
    const registerButton = page.getByRole('button', { name: '创建我的账号' })
    await registerButton.scrollIntoViewIfNeeded()
    await expect(page.getByLabel('学校邮箱')).toBeVisible()
    await expect(registerButton).toBeInViewport()
    expect(await registerButton.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)

    await page.getByRole('link', { name: '以游客身份体验 PaperSage' }).click()
    await expect(page).toHaveURL(/\/guest$/)
    await expect(page.getByRole('heading', { name: /先开始写作/ })).toBeVisible()
    await expect(page.getByText(/不会上传到云端/)).toBeVisible()
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), {
      message: `guest entry must not overflow horizontally at ${viewport.name}`,
    }).toBe(true)
    const guestButton = page.getByRole('button', { name: '开始游客体验' })
    await guestButton.scrollIntoViewIfNeeded()
    await expect(guestButton).toBeInViewport()
    expect(await guestButton.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
    await guestButton.click()
    await expect(page).toHaveURL(/\/app\/projects$/)
    await expect(page.getByRole('heading', { name: '所有项目' })).toBeVisible()
    await expect(page.getByText('本机游客 · 未同步')).toBeVisible()
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), {
      message: `local guest project dashboard must not overflow horizontally at ${viewport.name}`,
    }).toBe(true)
  }

  expect(unmockedApiRequests, 'auth and guest landing pages must not make unmocked API calls').toEqual([])
})
