import { test, expect } from '@playwright/test'

import { loginAs, logout, e2eEnv } from '../helpers'

/**
 * Cross-route navigation — verify the protected layout, the
 * /auth redirect when not logged in, and the basic flow between
 * the major pages.
 */
test.describe('Cross-route navigation', () => {
  test('protected layout renders the sidebar', async ({ page }) => {
    await loginAs(page)
    await expect(page).toHaveURL(/\/app\/projects/)
    // Sidebar / header should be present
    await expect(page.getByText('所有项目')).toBeVisible()
  })

  test('user can navigate to settings and back', async ({ page }) => {
    await loginAs(page)
    await page.goto('/app/settings')
    await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible({
      timeout: 10_000,
    })
    await page.goto('/app/projects')
    await expect(page.getByText('所有项目')).toBeVisible()
  })

  test('user can navigate to trash and back', async ({ page }) => {
    await loginAs(page)
    await page.goto('/app/trash')
    // Empty trash should show some placeholder
    await expect(page.getByText(/回收站|Trash|暂无/)).toBeVisible({ timeout: 10_000 })
  })

  test('direct link to a writing project works when logged in', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    expect(projectId).not.toBe('')
    await loginAs(page)
    await page.goto(`/project/${projectId}/writing`)
    await expect(page.getByText('Explorer')).toBeVisible({ timeout: 15_000 })
  })

  test('logout clears session and protected route bounces back', async ({ page }) => {
    await loginAs(page)
    await expect(page).toHaveURL(/\/app\/projects/)
    await logout(page)
    await expect(page).toHaveURL(/\/auth/)
    // Now try to visit a protected page — should be bounced
    await page.goto('/app/projects')
    await page.waitForURL(/\/auth/, { timeout: 5_000 })
  })
})
