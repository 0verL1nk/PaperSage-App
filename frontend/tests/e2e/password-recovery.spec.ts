import { expect, test } from '@playwright/test'

test.use({ storageState: { cookies: [], origins: [] } })

const viewports = [
  { width: 320, height: 640 },
  { width: 390, height: 844 },
  { width: 768, height: 1024 },
  { width: 1440, height: 900 },
]

test('password recovery request is clear and responsive from phone to desktop', async ({ page }) => {
  const requests: string[] = []
  await page.route('**/api/v1/auth/password-reset/request', async (route) => {
    requests.push(route.request().url())
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: { message: 'generic' }, info: '' }),
    })
  })

  for (const viewport of viewports) {
    await page.setViewportSize(viewport)
    await page.goto('/auth')
    await page.getByRole('link', { name: '忘记密码？' }).click()
    await expect(page).toHaveURL(/\/auth\/forgot-password$/)
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true)
    const submit = page.getByRole('button', { name: '发送重置链接' })
    await page.getByLabel('邮箱地址').fill('writer@example.com')
    await expect(submit).toBeInViewport()
    expect(await submit.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
    await submit.click()
    await expect(page.getByRole('status')).toContainText('如果该邮箱已验证并关联 PaperSage 账号')
    await page.getByRole('link', { name: '返回登录' }).click()
  }

  expect(requests).toHaveLength(viewports.length)
})

test('reset link consumes the fragment token and completes the new password form', async ({ page }) => {
  let body: { token?: string; password?: string } = {}
  await page.route('**/api/v1/auth/password-reset/confirm', async (route) => {
    body = route.request().postDataJSON() as typeof body
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { message: 'done' }, info: '' }) })
  })

  await page.setViewportSize({ width: 320, height: 640 })
  await page.goto('/auth/reset-password#token=one-time-secret')
  await expect(page).toHaveURL(/\/auth\/reset-password$/)
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true)
  await page.getByLabel('新密码', { exact: true }).fill('NewPassword123')
  await page.getByLabel('确认新密码', { exact: true }).fill('NewPassword123')
  const submit = page.getByRole('button', { name: '更新密码' })
  await expect(submit).toBeInViewport()
  expect(await submit.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
  await submit.click()
  await expect(page.getByRole('heading', { name: '密码已更新' })).toBeVisible()
  expect(body.token).toBe('one-time-secret')
  expect(body.password).toMatch(/^[0-9a-f]{64}$/)
})
