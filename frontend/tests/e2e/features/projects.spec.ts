import { test, expect } from '@playwright/test'

test.describe('Projects', () => {
  test('should load projects page and redirect to auth if not logged in', async ({ page }) => {
    await page.goto('/app/projects')
    // Should redirect to auth
    await page.waitForURL(/\/auth/, { timeout: 5000 })
    await expect(page.getByPlaceholder('请输入您的账号')).toBeVisible()
  })

  test('should display auth page elements', async ({ page }) => {
    await page.goto('/auth')
    await expect(page.getByRole('tab', { name: '账号登录' })).toBeVisible()
    await expect(page.getByRole('button', { name: '立即登录' })).toBeVisible()
  })
})
