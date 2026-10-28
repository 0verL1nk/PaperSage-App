import { expect, test } from '@playwright/test'

test.use({ storageState: { cookies: [], origins: [] } })

test('account import preview and confirmation remain usable from phone to desktop', async ({ page }) => {
  await page.goto('/auth')
  await page.evaluate(async () => {
    const local = await import('/src/services/guest-local-workspace.ts')
    await local.createGuestProject({ name: '待导入的本机论文', scenario: 'undergraduate_thesis' })
    window.localStorage.setItem('papersage_access_token', 'fake-access-token')
    window.localStorage.setItem('papersage_refresh_token', 'fake-refresh-token')
    window.localStorage.setItem('papersage_guest_import_pending', 'true')
  })

  await page.route('**/api/**', async (route) => {
    if (route.request().url().includes('/api/v1/auth/me')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { username: 'researcher', email: 'researcher@example.edu' } }) })
      return
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: [] }) })
  })

  const viewports = [
    { name: 'small phone', width: 320, height: 640 },
    { name: 'phone', width: 390, height: 844 },
    { name: 'tablet portrait', width: 768, height: 1024 },
    { name: 'tablet landscape', width: 1024, height: 768 },
    { name: 'desktop', width: 1440, height: 900 },
  ]
  await page.goto('/app/projects')
  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  await expect(dialog.getByText('researcher', { exact: true })).toBeVisible()

  for (const viewport of viewports) {
    await page.setViewportSize({ width: viewport.width, height: viewport.height })
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), {
      message: `account import must not overflow horizontally at ${viewport.name}`,
    }).toBe(true)
    const geometry = await dialog.evaluate((element) => {
      const rect = element.getBoundingClientRect()
      return { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom, width: rect.width, scrollWidth: element.scrollWidth, clientWidth: element.clientWidth }
    })
    expect(geometry.left, `dialog left edge at ${viewport.name}`).toBeGreaterThanOrEqual(-1)
    expect(geometry.right, `dialog right edge at ${viewport.name}`).toBeLessThanOrEqual(viewport.width + 1)
    expect(geometry.top, `dialog top edge at ${viewport.name}`).toBeGreaterThanOrEqual(-1)
    expect(geometry.bottom, `dialog bottom edge at ${viewport.name}`).toBeLessThanOrEqual(viewport.height + 1)
    expect(geometry.scrollWidth, `dialog content width at ${viewport.name}`).toBeLessThanOrEqual(geometry.clientWidth + 1)
    const confirm = page.getByRole('button', { name: '确认导入到账号' })
    await confirm.scrollIntoViewIfNeeded()
    await expect(confirm).toBeVisible()
    expect(await confirm.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
    await expect(page.getByText('默认保留本机备份')).toBeVisible()
  }
})
