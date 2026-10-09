import { test, expect } from '@playwright/test'

import { loginAs } from '../helpers'

/**
 * Settings — 真后端契约：分类默认值合并、PATCH 持久化、跨会话保留。
 */
test.describe('Settings (real backend)', () => {
  test('defaults render for all three categories', async ({ page }) => {
    await loginAs(page)
    await page.goto('/app/settings')

    // 三个标签页可见
    await expect(page.getByRole('tab', { name: /常规|General/ })).toBeVisible({ timeout: 10_000 })
    await expect(page.getByRole('tab', { name: /编辑器|Editor/ })).toBeVisible()
    await expect(page.getByRole('tab', { name: /AI|助手/ })).toBeVisible()
  })

  test('theme change persists across reload', async ({ page }) => {
    await loginAs(page)
    await page.goto('/app/settings')

    // 进入通用标签，把主题切到 dark
    const themeSelect = page.getByRole('combobox').first()
    await expect(themeSelect).toBeVisible({ timeout: 10_000 })
    await themeSelect.click()
    await page.getByRole('option', { name: /深色|Dark/ }).click()

    // 显式等 PATCH 落库再刷新（固定 sleep 在慢 CI 上会抢跑丢掉变更）
    await expect
      .poll(
        async () =>
          page.evaluate(async () => {
            const token = localStorage.getItem('papersage_access_token')
            const res = await fetch('/api/v1/settings/ui', {
              headers: { Authorization: `Bearer ${token}` },
            })
            const body = await res.json()
            return body?.data?.settings?.theme ?? ''
          }),
        { timeout: 10_000 },
      )
      .toBe('dark')
    await page.reload()

    const selectAgain = page.getByRole('combobox').first()
    await expect(selectAgain).toBeVisible({ timeout: 10_000 })
    // combobox 的当前值应包含 dark（或对应中文标签）
    const value = await selectAgain.textContent()
    expect(value ?? '').toMatch(/深色|Dark|dark/)
  })
})
