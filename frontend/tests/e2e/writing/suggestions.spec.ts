import { expect, test, type Page } from '@playwright/test'
import { WRITING_URL, mockWritingPageAPIs, seedAuthTokens, TEST_PROJECT_ID } from '../helpers'

/**
 * 导师批注闭环（HELE-69 重做）：
 * - 批注形态：正文行内锚点 + 右侧导师批注栏，不是 AI 助手里的第二个 Tab；
 * - 默认不自动；底栏「请导师批注本段」手动触发；
 * - 卡片首屏是「类型 + 导师评语 + 查看修改 + 依据摘要 + 采纳/忽略」；
 * - 采纳后可撤销，忽略可带原因；自动批注 3s/8s 不抢焦点、不 toast。
 *
 * 全 route mock：`E2E_SKIP_LOGIN=1 pnpm exec playwright test ...` 可在无后端时跑。
 */

test.use({ navigationTimeout: 90_000, actionTimeout: 20_000 })

const PARAGRAPH = '人工智能正在改变科研写作方式，但通用写作助手常常给出缺少依据的空泛建议。'
const PARAGRAPH_B = '第二段讨论研究方法的边界，当前还缺少对数据来源与采样范围的说明。'
const FILE_CONTENT = `${PARAGRAPH}\n\n${PARAGRAPH_B}\n\n\\section{研究背景}\n正文。\n`

const PROPOSAL = {
  id: 'proposal-1',
  filePath: 'main.tex',
  oldText: PARAGRAPH,
  newText: `${PARAGRAPH}本节将据此界定研究问题。`,
  reason: '这一段需要交代研究问题如何从材料中导出。',
  suggestionKind: 'logic_gap',
  confidence: 'high',
  sectionPath: '绪论 / 研究背景',
  origin: 'manual',
  status: 'pending',
  createdAt: new Date().toISOString(),
  basis: [{
    type: 'plan', stance: 'supports', sectionId: 'ch1.1', sectionTitle: '研究背景',
    gap: '未交代研究问题来源', planRevision: 2,
  }],
}

interface Backend {
  posts: Array<Record<string, unknown>>
  proposals: () => unknown[]
  onApply?: () => void
  onReject?: (body: Record<string, unknown>) => void
  onUndo?: () => void
}

async function mockBackend(page: Page, backend: Backend) {
  await mockWritingPageAPIs(page)
  await seedAuthTokens(page)

  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex-files/main.tex`, async (route) => {
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: { path: 'main.tex', content: FILE_CONTENT } }),
    })
  })
  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/agent/proposals**`, async (route) => {
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: backend.proposals(), info: '' }),
    })
  })
  await page.route(/\/api\/v1\/settings\/?$/, async (route) => {
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        data: {
          settings: {
            ui: {}, editor: {},
            agent: {
              providers: [{ id: 'p1', name: '测试模型', api_key: 'sk-test', base_url: 'https://example.invalid/v1', models: ['demo'], active_model: 'demo' }],
              active_provider_id: 'p1',
            },
          },
        },
        info: '',
      }),
    })
  })
  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/agent/suggestions`, async (route) => {
    backend.posts.push(route.request().postDataJSON() as Record<string, unknown>)
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        data: { runId: 'run-1', conversationId: 'conv-1', streamUrl: '/api/v1/agent/runs/run-1/events', trigger: 'manual' },
        info: '',
      }),
    })
  })
  await page.route('**/api/v1/agent/runs/run-1/events', async (route) => {
    await route.fulfill({
      status: 200, contentType: 'text/event-stream',
      body: 'data: {"type":"data-proposal","proposal":{"id":"proposal-1"}}\n\ndata: {"type":"finish"}\n\ndata: [DONE]\n\n',
    })
  })
  await page.route('**/api/v1/agent/change-proposals/proposal-1/apply', async (route) => {
    backend.onApply?.()
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { status: 'applied' } }) })
  })
  await page.route('**/api/v1/agent/change-proposals/proposal-1/reject', async (route) => {
    backend.onReject?.(route.request().postDataJSON() as Record<string, unknown>)
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { status: 'rejected' } }) })
  })
  await page.route('**/api/v1/agent/change-proposals/proposal-1/undo', async (route) => {
    backend.onUndo?.()
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { status: 'reverted' } }) })
  })
}

async function enableAutoComment(page: Page) {
  await expect(page.locator('.monaco-editor').first()).toBeVisible({ timeout: 30_000 })
  await page.getByRole('switch', { name: '自动批注' }).click()
}

test('导师批注：默认不自动、手动批注、依据展开、采纳/撤销/忽略', async ({ page }) => {
  test.setTimeout(120_000)
  let status = 'pending'
  let hasProposal = false
  const posts: Array<Record<string, unknown>> = []
  const rejectBodies: Array<Record<string, unknown>> = []

  await mockBackend(page, {
    posts,
    proposals: () => (hasProposal ? [{ ...PROPOSAL, status }] : []),
    onApply: () => { status = 'applied' },
    onReject: (body) => { rejectBodies.push(body); status = 'rejected' },
    onUndo: () => { status = 'reverted' },
  })

  await page.goto(WRITING_URL)
  await expect(page.getByRole('button', { name: '请导师批注本段' })).toBeVisible({ timeout: 30_000 })

  // 默认不自动建 Run。
  await page.waitForTimeout(1500)
  expect(posts, '默认进入写作页不得自动发 POST /agent/suggestions').toEqual([])

  // 手动批注：请求带当前段落；卡片出现在导师批注栏。
  hasProposal = true
  await page.getByRole('button', { name: '请导师批注本段' }).click()
  await expect.poll(() => posts.length).toBe(1)
  expect(posts[0].paragraphText).toBe(PARAGRAPH)
  expect(posts[0].trigger).toBe('manual')
  await expect(page.getByText('导师批注', { exact: true })).toBeVisible()
  await expect(page.getByText(PROPOSAL.reason)).toBeVisible()
  await expect(page.getByText('论证')).toBeVisible()
  await expect(page.getByText(/依据：章节计划 v2 · 研究背景/)).toBeVisible()

  // 文案审计：不再出现旧代码 Diff / 开发说明书。
  const pageText = await page.locator('body').innerText()
  expect(pageText).not.toMatch(/修改建议|依据充分|依据说明：|依据支持|应用到稿件|待确认|停顿后自动检查|8 秒|不抢光标|检查本段/)

  // 依据可展开；修订对比折叠。
  await expect(page.getByText('原文')).toHaveCount(0)
  await page.getByRole('button', { name: /查看修改/ }).click()
  await expect(page.getByText('原文')).toBeVisible()
  await expect(page.getByText('改为')).toBeVisible()
  await page.getByRole('button', { name: '参考依据' }).click()
  await expect(page.getByText(/计划缺口：未交代研究问题来源/)).toBeVisible()

  // 采纳 → 撤销。
  await page.getByRole('button', { name: '采纳' }).click()
  await expect(page.getByText('已采纳')).toBeVisible()
  await page.getByRole('button', { name: '撤销' }).click()
  await expect(page.getByText('已撤销')).toBeVisible()

  // 忽略原因。
  status = 'pending'
  await page.reload()
  await page.getByText('导师批注', { exact: true }).click()
  await page.getByRole('button', { name: '忽略' }).click()
  await page.getByRole('button', { name: '太泛' }).click()
  await page.getByRole('button', { name: '确认忽略' }).click()
  await expect.poll(() => rejectBodies.length).toBe(1)
  expect(rejectBodies[0].reasonCode).toBe('too_vague')
})

test('自动批注：段落完成 3 秒触发，不抢焦点、不弹 toast', async ({ page }) => {
  test.setTimeout(120_000)
  const posts: Array<Record<string, unknown>> = []
  await mockBackend(page, { posts, proposals: () => [] })

  await page.goto(WRITING_URL)
  await enableAutoComment(page)
  const editor = page.locator('.monaco-editor').first()
  await editor.click()
  await page.keyboard.press('Control+Home')
  await page.keyboard.press('End')
  await page.keyboard.type('补充一句。')
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('ArrowDown')

  await expect.poll(() => posts.length, { timeout: 5_000 }).toBe(1)
  expect(posts[0].trigger).toBe('auto')
  expect(String(posts[0].paragraphText)).toContain('补充一句。')
  expect(await page.evaluate(() => Boolean(document.activeElement?.closest('.monaco-editor')))).toBe(true)
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0)
  await page.waitForTimeout(5_000)
  expect(posts.length).toBe(1)
})

test('自动批注：仍在段内继续编辑时延迟到 8 秒再触发', async ({ page }) => {
  test.setTimeout(120_000)
  const posts: Array<Record<string, unknown>> = []
  await mockBackend(page, { posts, proposals: () => [] })

  await page.goto(WRITING_URL)
  await enableAutoComment(page)
  const editor = page.locator('.monaco-editor').first()
  await editor.click()
  await page.keyboard.press('Control+Home')
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('End')
  await page.keyboard.type('再补充。')

  await page.waitForTimeout(3_400)
  expect(posts, '段内继续编辑时 3s 不得触发').toEqual([])
  await expect.poll(() => posts.length, { timeout: 7_000 }).toBe(1)
  expect(posts[0].trigger).toBe('auto')
  expect(String(posts[0].paragraphText)).toContain('再补充。')
  expect(await page.evaluate(() => Boolean(document.activeElement?.closest('.monaco-editor')))).toBe(true)
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0)
})

test('窄屏点击行内批注弹出浮层，Esc 关闭且不挤压编辑区', async ({ page }) => {
  test.setTimeout(120_000)
  await page.setViewportSize({ width: 390, height: 844 })
  await mockBackend(page, { posts: [], proposals: () => [PROPOSAL] })

  await page.goto(WRITING_URL)
  await expect(page.locator('.monaco-editor').first()).toBeVisible({ timeout: 30_000 })
  // 窄屏不渲染右侧批注栏。
  await expect(page.getByText('条待处理')).toHaveCount(0)

  const box = await page.locator('.monaco-editor').first().boundingBox()
  expect(box).not.toBeNull()
  await page.mouse.click(box!.x + 80, box!.y + 32)
  const dialog = page.getByRole('dialog', { name: '导师批注' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByText('论证')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
})
