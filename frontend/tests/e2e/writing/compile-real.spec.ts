import { test, expect } from '@playwright/test'

import { loginAs, e2eEnv } from '../helpers'

/**
 * 真实 LaTeX 编译链路（CLSI + TeX）：编辑 → 提交 → 轮询 success →
 * PDF 预览出现。仅在 CLSI 可用的环境跑（CI 的 clsi job /
 * 本地 make clsi-dev），普通 e2e 套件自动跳过。
 */
const CLSI_ENABLED = process.env.RUN_CLSI_E2E === '1'

test.skip(!CLSI_ENABLED, '需要真实 CLSI（RUN_CLSI_E2E=1 的环境）')

test('edit → compile → PDF preview (real CLSI)', async ({ page }) => {
  const projectId = e2eEnv('E2E_PROJECT_ID')
  await loginAs(page)
  await page.goto(`/project/${projectId}/writing`)

  // 等待树与编辑器就绪，选中 main.tex
  const mainFile = page.getByTestId('file-node').filter({ hasText: 'main.tex' }).first()
  await expect(mainFile).toBeVisible({ timeout: 15_000 })
  await mainFile.click()
  await page.waitForTimeout(800)

  // 触发编译
  const compileButton = page.getByTestId('compile-button')
  await expect(compileButton).toBeVisible({ timeout: 10_000 })
  await compileButton.click()

  // 轮询到 success：PDF 预览渲染为 canvas（react-pdf）
  // 真实编译含 latexmk 多轮，给足时间
  await expect(page.locator('canvas').first()).toBeVisible({ timeout: 120_000 })
})
