import { test, expect } from '@playwright/test'

test.use({ storageState: { cookies: [], origins: [] } })

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
    await page.goto('/auth', { waitUntil: 'domcontentloaded' })
    await page.evaluate(() => window.localStorage.clear())
    await page.reload({ waitUntil: 'domcontentloaded' })
    await page.setViewportSize({ width: viewport.width, height: viewport.height })
    await expect(page.getByRole('tab', { name: '账号登录' })).toBeVisible()
    const backendToggle = page.getByRole('button', { name: /切换后端，当前：在线/ })
    await expect(backendToggle).toBeVisible()
    expect(await backendToggle.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
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

    const guestButton = page.getByRole('button', { name: '以游客身份体验 PaperSage' })
    await expect(guestButton).toBeVisible()
    expect(await guestButton.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
    await guestButton.click()
    await expect(page).toHaveURL(/\/app\/projects$/)
    await expect(page.getByRole('heading', { name: '所有项目' })).toBeVisible()
    await expect(page.getByText('本机游客 · 未同步')).toBeVisible()
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), {
      message: `guest project dashboard must not overflow horizontally at ${viewport.name}`,
    }).toBe(true)
  }

  expect(unmockedApiRequests, 'auth and local guest entry must not make unmocked API calls').toEqual([])
})

test('auth aurora is animated by default and quiet for reduced-motion preference', async ({ page }) => {
  await page.goto('/auth', { waitUntil: 'domcontentloaded' })
  const orb = page.getByTestId('auth-background-orb')
  await expect(orb).toBeVisible()
  expect(await orb.evaluate((element) => getComputedStyle(element).animationName)).toContain('auth-mesh-drift')

  await page.emulateMedia({ reducedMotion: 'reduce' })
  expect(await orb.evaluate((element) => getComputedStyle(element).animationName)).toBe('none')
})
