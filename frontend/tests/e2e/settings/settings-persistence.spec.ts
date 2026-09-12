import { test, expect } from '@playwright/test'

import { loginAs } from '../helpers'

/**
 * Settings persistence — verify the three settings tabs render
 * and that changes survive a page reload (the store is backed
 * by the API, so reload re-fetches from the DB).
 */
test.describe('Settings (real backend)', () => {
  test('all three settings tabs render', async ({ page }) => {
    await loginAs(page)
    await page.goto('/app/settings')
    // Tabs are General / Editor / AI Assistant
    await expect(page.getByRole('tab', { name: 'General' })).toBeVisible()
    await expect(page.getByRole('tab', { name: 'Editor' })).toBeVisible()
    await expect(page.getByRole('tab', { name: 'AI Assistant' })).toBeVisible()
  })

  test('changing font size persists across reload', async ({ page }) => {
    await loginAs(page)
    await page.goto('/app/settings')

    // Switch to Editor tab
    await page.getByRole('tab', { name: 'Editor' }).click()
    const fontSize = page.locator('#font-size')
    await expect(fontSize).toBeVisible({ timeout: 10_000 })
    await fontSize.fill('18')
    await fontSize.blur()

    // Reload — the API should return the new value
    await page.reload()
    await page.getByRole('tab', { name: 'Editor' }).click()
    await expect(page.locator('#font-size')).toHaveValue('18', { timeout: 10_000 })
  })

  test('changing theme persists across reload', async ({ page }) => {
    await loginAs(page)
    await page.goto('/app/settings')

    // Theme is on the General tab
    const themeTrigger = page.locator('#theme')
    await expect(themeTrigger).toBeVisible({ timeout: 10_000 })
    await themeTrigger.click()
    // Appearance 文案已 i18n（zh: 深色 / en: Dark），双语匹配
    await page.getByRole('option', { name: /^深色$|^Dark$/ }).click()

    // Reload
    await page.reload()
    await expect(page.locator('#theme')).toContainText(/深色|Dark/, { timeout: 10_000 })
  })
})
