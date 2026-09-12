import { test, expect } from '@playwright/test'

import { e2eEnv, loginAs, uploadFixture } from '../helpers'

/**
 * Uploading 页真实 UI 契约：走真实对话框载荷（folders-api.ts 的
 * snake_case、files-api.ts move 的 folder_id）——回归 2026-09-06 的
 * 契约破坏（Go 端只认 camelCase 导致新建文件夹 422，e2e 此前没有
 * 覆盖这两条真实前端路径）。
 */
test.describe('Uploading folder ops (real frontend payloads)', () => {
  test('creates a folder via the real dialog (snake_case payload)', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    await loginAs(page, 'e2euser', 'e2epass1')
    await page.goto(`/project/${projectId}/uploading`)
    await page.getByText(/筛选|Filter|类型/).waitFor({ timeout: 15_000 })

    const folderName = `e2e夹${Date.now()}`
    await page.getByRole('button', { name: /新建文件夹/ }).click()
    await page.getByLabel('文件夹名称').fill(folderName)
    await page.getByRole('button', { name: '创建', exact: true }).click()

    // 对话框关闭 = 创建成功（422 时对话框会留着并显示错误）
    await expect(page.getByRole('dialog')).toHaveCount(0, { timeout: 5_000 })
    // 文件树侧出现该文件夹
    await expect(page.getByText(folderName).first()).toBeVisible({ timeout: 5_000 })
  })

  test('uploads a file, then moves it into a folder via dialog (folder_id payload)', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    await loginAs(page, 'e2euser', 'e2epass1')
    await page.goto(`/project/${projectId}/uploading`)
    await page.getByText(/筛选|Filter|类型/).waitFor({ timeout: 15_000 })

    // 建目标文件夹
    const folderName = `e2e移入${Date.now()}`
    await page.getByRole('button', { name: /新建文件夹/ }).click()
    await page.getByLabel('文件夹名称').fill(folderName)
    await page.getByRole('button', { name: '创建', exact: true }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0, { timeout: 5_000 })

    // 上传（真实 S3/MinIO）
    await page.getByRole('button', { name: '上传', exact: true }).click()
    await uploadFixture(page, 'sample.pdf')
    // 自动上传完成 → 已就绪 → 添加（成功后弹窗自动关闭）
    await expect(page.getByText('1 个已就绪')).toBeVisible({ timeout: 10_000 })
    await page.getByRole('button', { name: /添加 1 个/ }).click()
    await expect(page.getByText('上传文档')).toHaveCount(0, { timeout: 10_000 })
    const row = page.getByRole('row').filter({ hasText: 'sample.pdf' }).first()
    await expect(row).toBeVisible({ timeout: 15_000 })

    // ⋮ → 移动 → 选择目标文件夹 → 确认
    await row.locator('button').last().click({ force: true })
    await page.getByText('移动').last().click()
    await page.getByRole('combobox').click()
    await page.getByRole('option', { name: folderName }).click()
    await page.getByRole('button', { name: /移动|确定/ }).last().click()
    await expect(page.getByRole('dialog')).toHaveCount(0, { timeout: 5_000 })

    // 选中目标文件夹后表格只剩该文件夹下的文档
    await page.getByText(folderName).first().click()
    await page.waitForTimeout(800)
    await expect(page.getByRole('row').filter({ hasText: 'sample.pdf' }).first()).toBeVisible({ timeout: 10_000 })
  })
})
