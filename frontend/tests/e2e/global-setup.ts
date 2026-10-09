import { mkdir } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'

import { chromium, type FullConfig } from '@playwright/test'

export default async function globalSetup(config: FullConfig) {
  const baseURL = String(config.projects[0]?.use.baseURL ?? 'http://localhost:5173')
  const storageStatePath = resolve('playwright/.auth/user.json')
  const browser = await chromium.launch()

  try {
    const context = await browser.newContext({ baseURL })
    const page = await context.newPage()
    await page.goto('/auth')
    await page.getByPlaceholder('请输入您的账号').fill(process.env.E2E_USER ?? 'e2euser')
    await page.getByPlaceholder('••••••••').fill(process.env.E2E_PASS ?? 'e2epass1')
    await page.getByRole('button', { name: '立即登录' }).click()
    await page.waitForURL(/\/app\/projects/, { timeout: 15_000 })

    await mkdir(dirname(storageStatePath), { recursive: true })
    await context.storageState({ path: storageStatePath })
    await context.close()
  } finally {
    await browser.close()
  }
}
