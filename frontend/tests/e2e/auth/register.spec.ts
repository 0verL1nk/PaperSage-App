import { test, expect } from '@playwright/test'

test.describe('Authentication', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/auth')
  })

  test('should load auth page with login tab active', async ({ page }) => {
    await expect(page.getByRole('tab', { name: '账号登录' })).toBeVisible()
    await expect(page.getByRole('tab', { name: '注册新账号' })).toBeVisible()
  })

  test('should switch to register tab', async ({ page }) => {
    await page.getByRole('tab', { name: '注册新账号' }).click()
    await page.waitForTimeout(300)
    // Register form should appear
    await expect(page.getByRole('textbox', { name: '创建用户名' })).toBeVisible()
  })
})
