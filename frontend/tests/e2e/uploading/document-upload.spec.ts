import { existsSync, statSync } from 'node:fs'

import { test, expect } from '@playwright/test'

import { loginAs, e2eEnv, fixturePath, uploadFixture } from '../helpers'

/**
 * Document upload — exercises the real multipart → backend → MinIO path.
 *
 * CI workflow sets LATEX_CLSI_ENABLED=false and points S3_ENDPOINT to
 * a service-local MinIO. This spec verifies:
 *   - The upload dialog can be opened
 *   - setInputFiles sends the file to the backend
 *   - The file appears in the data table after upload
 */
test.describe('Document upload (real S3)', () => {
  test('upload dialog opens and accepts a PDF fixture', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    expect(projectId).not.toBe('')
    await loginAs(page)

    await page.goto(`/project/${projectId}/uploading`)
    // The page should render — wait for the filter bar to appear
    await expect(page.getByText(/筛选|Filter|类型/)).toBeVisible({ timeout: 15_000 })

    // Open the upload dialog — there should be a button labeled "上传"
    // 注意精确匹配：侧边栏导航“上传资料”同样命中 /上传|Upload/ 正则，
    // 且在 DOM 序上靠前
    const uploadButton = page.getByRole('button', { name: '上传', exact: true })
    await expect(uploadButton).toBeVisible({ timeout: 10_000 })
    await uploadButton.click()

    // After the dialog opens, find the file input and upload
    await uploadFixture(page, 'sample.pdf')

    // 自动上传 → 已就绪 → 添加 → 弹窗自动关闭 → 表格出现
    await expect(page.getByText('1 个已就绪')).toBeVisible({ timeout: 10_000 })
    await page.getByRole('button', { name: /添加 1 个/ }).click()
    await expect(page.getByText('上传文档')).toHaveCount(0, { timeout: 10_000 })
    await expect(page.getByRole('row').filter({ hasText: 'sample.pdf' }).first()).toBeVisible({ timeout: 15_000 })
  })

  test('helper uploadFixture resolves to a real file', () => {
    // 冒烟：fixturePath 必须解析到 Playwright 可读的真实文件。
    // 纯 Node 侧校验即可——浏览器里找 input[type=file] 与本用例无关
    // （auth 页根本没有文件输入框）。
    for (const name of ['sample.pdf', 'sample.docx', 'main.tex']) {
      const p = fixturePath(name)
      expect(existsSync(p), `${name} -> ${p}`).toBe(true)
      expect(statSync(p).size, name).toBeGreaterThan(0)
    }
    expect(e2eEnv('E2E_USER')).toBeTruthy()
  })
})
