import { test, expect } from '@playwright/test'

import { loginAs, logout, e2eEnv } from '../helpers'

/**
 * Auth flow — covers the real round-trip against a live backend.
 *
 * Each test uses a unique username (UUID-suffixed) so they can run
 * in parallel without colliding in the e2euser table.
 */
test.describe('Auth flow (real backend)', () => {
  test('login form renders both tabs', async ({ page }) => {
    await page.goto('/auth')
    await expect(page.getByRole('tab', { name: '账号登录' })).toBeVisible()
    await expect(page.getByRole('tab', { name: '注册新账号' })).toBeVisible()
  })

  test('seeded user can log in and reach projects', async ({ page }) => {
    await loginAs(page)
    await expect(page).toHaveURL(/\/app\/projects/)
    // Sidebar or page header should mention project list
    await expect(page.getByText('所有项目')).toBeVisible()
  })

  test('wrong password shows an error and stays on /auth', async ({ page }) => {
    await page.goto('/auth')
    await page.getByPlaceholder('请输入您的账号').fill(e2eEnv('E2E_USER', 'e2euser'))
    await page.getByPlaceholder('••••••••').fill('wrong-password-1234')
    await page.getByRole('button', { name: '立即登录' }).click()
    // Should NOT navigate away
    await expect(page).toHaveURL(/\/auth/)
    // The destructive alert should appear with some error text
    await expect(page.locator('[role="alert"], .text-destructive').first()).toBeVisible({
      timeout: 5_000,
    })
  })

  test('logout returns to /auth', async ({ page }) => {
    await loginAs(page)
    await expect(page).toHaveURL(/\/app\/projects/)
    await logout(page)
    await expect(page).toHaveURL(/\/auth/)
  })

  test('protected route bounces unauthenticated user to /auth', async ({ page }) => {
    // No login — just visit a protected page
    await page.goto('/app/projects')
    await page.waitForURL(/\/auth/, { timeout: 5000 })
  })

  test('a fresh user can register then log in', async ({ page }) => {
    const username = `e2e_reg_${Date.now()}`
    const password = 'e2epass1'

    await page.goto('/auth')
    await page.getByRole('tab', { name: '注册新账号' }).click()
    await page.getByRole('textbox', { name: '创建用户名' }).fill(username)
    // First password field — placeholder makes it unique
    await page.getByPlaceholder('至少 8 位，包含字母与数字').fill(password)
    await page.getByPlaceholder('请再次输入密码').fill(password)
    await page.getByRole('button', { name: '创建我的账号' }).click()

    // After register, the UI auto-switches to login tab
    await expect(page.getByRole('tab', { name: '账号登录' })).toHaveAttribute(
      'data-state',
      'active',
      { timeout: 5000 },
    )

    // Now actually log in
    await page.getByPlaceholder('请输入您的账号').fill(username)
    await page.getByPlaceholder('••••••••').fill(password)
    await page.getByRole('button', { name: '立即登录' }).click()
    await page.waitForURL(/\/app\/projects/, { timeout: 15_000 })
  })
})
