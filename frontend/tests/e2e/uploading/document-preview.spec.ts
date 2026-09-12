import { test, expect } from '@playwright/test'

import { loginAs, e2eEnv, uploadFixture } from '../helpers'

/**
 * DOCX 预览保真：docx-preview 标准产物（section.docx 纸张、内联字号、
 * 表格、列表圆点）。不覆盖文档自身排版（曾用 !important 行高/段距把
 * 格式顶乱，见 2026-09 重构）。
 */
test('rich docx renders with document formatting intact', async ({ page }) => {
  const projectId = e2eEnv('E2E_PROJECT_ID')
  await loginAs(page)
  await page.goto(`/project/${projectId}/uploading`)
  await page.getByText(/筛选|Filter|类型/).waitFor({ timeout: 15_000 })

  await page.getByRole('button', { name: '上传', exact: true }).click()
  await uploadFixture(page, 'rich.docx')
  await expect(page.getByText('1 个已就绪')).toBeVisible({ timeout: 10_000 })
  await page.getByRole('button', { name: /添加 1 个/ }).click()
  const row = page.getByRole('row').filter({ hasText: 'rich.docx' }).first()
  await expect(row).toBeVisible({ timeout: 15_000 })

  await row.getByRole('button', { name: 'rich.docx' }).click()
  await page.waitForTimeout(2500)

  const info = await page.evaluate(() => {
    const sec = document.querySelector('section.docx')
    if (!sec) return { err: true as const }
    const scroll = sec.closest('.overflow-y-auto')
    const paras = [...sec.querySelectorAll('p')] as HTMLParagraphElement[]
    // 字号在内联 span 上（docx-preview 的 run 级样式），段落本身是默认字号
    const spans = [...sec.querySelectorAll('span')] as HTMLSpanElement[]
    const fonts = spans
      .map((sp) => parseFloat(getComputedStyle(sp).fontSize))
      .filter((n) => Number.isFinite(n))
    const maxFont = Math.max(...fonts, 0)
    const minFont = Math.min(...fonts, Infinity)
    return {
      width: Math.round(sec.getBoundingClientRect().width),
      panelW: scroll ? Math.round(scroll.clientWidth) : 0,
      tableRows: sec.querySelectorAll('table tr').length,
      // docx-preview 用 ::before 伪元素 + 符号字体画项目符号（content
      // 为 "\9 "），不在 textContent 里——按伪元素存在性计数
      bullets: paras.filter((p) => getComputedStyle(p, '::before').content !== 'none').length,
      maxFont,
      minFont,
    }
  })
  expect((info as { err?: true }).err).not.toBe(true)
  const i = info as { width: number; tableRows: number; bullets: number; maxFont: number; minFont: number }
  // fit-to-width：纸张（Letter 816px）按面板宽缩放，不溢出不出横向滚动条
  expect(i.width).toBeGreaterThan(200, '缩放后仍有可读宽度')
  expect(i.width).toBeLessThanOrEqual(i.panelW + 1, '不得超出面板宽度')
  // 表格 4 行（含表头）
  expect(i.tableRows).toBe(4)
  // 项目符号列表 3 项
  expect(i.bullets).toBe(3)
  // 标题字号分层存在（实测 H 系 ≈ 1.27× 正文，阈值取保守的 1.15）
  expect(i.maxFont).toBeGreaterThan(i.minFont * 1.15)
})
