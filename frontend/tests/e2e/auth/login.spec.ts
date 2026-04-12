import { test, expect } from '@playwright/test'

test.describe('Login', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/auth')
  })

  test('should show login form by default', async ({ page }) => {
    await expect(page.getByPlaceholder('请输入您的账号')).toBeVisible()
    await expect(page.getByPlaceholder('••••••••')).toBeVisible()
    await expect(page.getByRole('button', { name: '立即登录' })).toBeVisible()
  })
})
