import { test, expect } from '@playwright/test'
import { loginAs, e2eEnv } from '../helpers'

test('theme + language + current project workflow navigation', async ({ page }) => {
  await loginAs(page, e2eEnv('E2E_USER', 'e2euser'), e2eEnv('E2E_PASS', 'e2epass1'))

  // 1) 主题：默认无 dark → 设置切深色 → html.dark 出现 + localStorage
  expect(await page.evaluate(() => document.documentElement.classList.contains('dark'))).toBe(false)
  await page.goto('/app/settings')
  await page.getByRole('tab', { name: /通用|General/ }).waitFor({ timeout: 10_000 })
  await page.locator('#theme').click()
  await page.getByRole('option', { name: '深色' }).click()
  await page.waitForTimeout(600)
  expect(await page.evaluate(() => document.documentElement.classList.contains('dark'))).toBe(true, '切深色应立即生效')
  expect(await page.evaluate(() => localStorage.getItem('papersage_theme'))).toBe('dark')

  // 刷新后保持
  await page.reload()
  await page.waitForTimeout(800)
  expect(await page.evaluate(() => document.documentElement.classList.contains('dark'))).toBe(true, '刷新后应保持深色')
  // 后端也存了
  const ui = await page.evaluate(async () => {
    const h = { Authorization: `Bearer ${localStorage.getItem('papersage_access_token')}` }
    return await fetch('/api/v1/settings/ui', { headers: h }).then((r) => r.json())
  })
  expect(ui.data?.settings?.theme).toBe('dark')

  // 2) 语言：切 English → tab 文案变英文
  await page.locator('#language').click()
  await page.getByRole('option', { name: 'English' }).click()
  await page.waitForTimeout(600)
  const pid = e2eEnv('E2E_PROJECT_ID')
  await page.goto(`/project/${pid}/writing`)
  await expect(page.getByRole('button', { name: 'Writing', exact: true })).toBeVisible({ timeout: 10_000 })
  await expect(page.getByRole('button', { name: 'Preparation', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Review', exact: true })).toBeVisible()

  // 3) Legacy planned URLs redirect into the current three-stage workflow.
  await page.goto(`/project/${pid}/logic-chain`)
  await expect(page).toHaveURL(new RegExp(`/project/${pid}/prepare$`), { timeout: 5000 })
  await expect(page.getByRole('heading', { name: /先把研究说清楚/ })).toBeVisible()

  // 还原：zh-CN + system（避免污染后续测试）
  await page.goto('/app/settings')
  await page.getByRole('tab', { name: /通用|General/ }).waitFor({ timeout: 10_000 })
  await page.waitForTimeout(600)
  await page.locator('#language').click()
  await page.getByRole('option', { name: '简体中文' }).click().catch(() => {})
  await page.waitForTimeout(400)
  await page.locator('#theme').click()
  await page.getByRole('option', { name: /跟随系统|System/ }).click().catch(() => {})
})
