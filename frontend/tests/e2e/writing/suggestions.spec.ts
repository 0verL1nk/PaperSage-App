import { expect, test, type Page } from '@playwright/test'
import { WRITING_URL, mockWritingPageAPIs, seedAuthTokens, TEST_PROJECT_ID } from '../helpers'

// 本机 Vite 冷启动 + 首次编译较慢，放宽导航等待；CI 不受影响。
test.use({ navigationTimeout: 90_000, actionTimeout: 20_000 })

/**
 * 动态写作建议链路（HELE-69）：
 * - 默认进入写作页不发 POST /agent/suggestions（专注写作、不打扰）；
 * - 手动「检查本段」才发请求，事件流只进侧栏；
 * - 开启自动检查后：段落完成 3s 触发，段内继续写延迟到 8s，且不抢焦点/不 toast；
 * - 建议卡展示三源依据，可应用、可带原因忽略、可一键撤销。
 *
 * 全 route mock：本地无后端时用 `E2E_SKIP_LOGIN=1 pnpm exec playwright test ...` 跑。
 */

const PARAGRAPH = '人工智能正在改变科研写作方式，但通用写作助手常常给出缺少依据的空泛建议。'
const PARAGRAPH_B = '第二段讨论研究方法的边界，当前还缺少对数据来源与采样范围的说明。'
const FILE_CONTENT = `${PARAGRAPH}\n\n${PARAGRAPH_B}\n\n\\section{研究背景}\n正文。\n`

const PROPOSAL = {
  id: 'proposal-1',
  filePath: 'main.tex',
  oldText: PARAGRAPH,
  newText: `${PARAGRAPH}本节将据此界定研究问题。`,
  reason: '章节计划要求本段交代研究问题的来源，当前缺少衔接。',
  suggestionKind: 'logic_gap',
  confidence: 'high',
  confidenceReason: '已采用章节计划明确要求本节界定研究问题。',
  sectionPath: '绪论 / 研究背景',
  origin: 'manual',
  status: 'pending',
  createdAt: new Date().toISOString(),
  basis: [
    {
      type: 'plan',
      stance: 'supports',
      note: '第一章计划要求本节界定研究问题',
      sectionId: 'ch1.1',
      sectionTitle: '研究背景',
      gap: '未交代研究问题的来源',
      planArtifactId: 'artifact-1',
      planRevision: 1,
    },
  ],
}

interface SuggestionBackend {
  posts: Array<Record<string, unknown>>
  proposals: () => unknown[]
  onApply?: () => void
  onReject?: (body: Record<string, unknown>) => void
  onUndo?: () => void
  stream?: string
}

async function mockSuggestionBackend(page: Page, backend: SuggestionBackend) {
  await mockWritingPageAPIs(page)
  await seedAuthTokens(page)

  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex-files/main.tex`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: { path: 'main.tex', content: FILE_CONTENT } }),
    })
  })
  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/agent/proposals**`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: backend.proposals(), info: '' }),
    })
  })
  await page.route(/\/api\/v1\/settings\/?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        data: {
          settings: {
            ui: {},
            editor: {},
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
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        data: { runId: 'run-1', conversationId: 'conv-1', streamUrl: '/api/v1/agent/runs/run-1/events', trigger: 'manual' },
        info: '',
      }),
    })
  })
  await page.route('**/api/v1/agent/runs/run-1/events', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      body: backend.stream ?? 'data: {"type":"tool-input-available","toolCallId":"t1","toolName":"skip_suggestion","input":{"reason":"本段表达已清晰"}}\n\ndata: {"type":"finish"}\n\ndata: [DONE]\n\n',
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

async function enableAutoCheck(page: Page) {
  // 先等写作页渲染，再切 AI 侧栏（面板只在切到该 tab 时挂载）。
  await expect(page.locator('.monaco-editor').first()).toBeVisible({ timeout: 30_000 })
  await page.getByRole('button', { name: /^AI 助手侧栏/ }).click({ timeout: 20_000 })
  await page.getByText(/^建议 \(/).click({ timeout: 20_000 })
  await page.getByRole('checkbox', { name: '停顿后自动检查' }).check()
}

test('默认不自动检查；手动检查与建议卡应用、忽略、撤销闭环', async ({ page }) => {
  test.setTimeout(120_000)
  let status = 'pending'
  const posts: Array<Record<string, unknown>> = []
  const rejectBodies: Array<Record<string, unknown>> = []

  await mockSuggestionBackend(page, {
    posts,
    proposals: () => [{ ...PROPOSAL, status }],
    onApply: () => { status = 'applied' },
    onReject: (body) => { rejectBodies.push(body); status = 'rejected' },
    onUndo: () => { status = 'reverted' },
  })

  await page.goto(WRITING_URL)
  await expect(page.getByRole('button', { name: 'AI 助手侧栏，1 条待处理建议' })).toBeVisible({ timeout: 20_000 })

  // 验收：默认进入写作页不自动建 Run。
  await page.waitForTimeout(1500)
  expect(posts, '默认进入写作页不得自动发 POST /agent/suggestions').toEqual([])

  // 手动检查：请求体必须带当前段落原文，结果只进侧栏。
  await page.getByRole('button', { name: '检查本段' }).first().click()
  await expect.poll(() => posts.length).toBe(1)
  expect(posts[0].paragraphText).toBe(PARAGRAPH)
  expect(posts[0].trigger).toBe('manual')
  await expect(page.getByText('本段暂无需要修改的问题。')).toBeVisible()

  // 建议卡：依据可展开、可应用、可撤销。
  await expect(page.getByText('修改建议')).toBeVisible()
  await expect(page.getByText('章节计划 · 研究背景')).toBeVisible()
  await page.getByRole('button', { name: /章节计划 · 研究背景/ }).click()
  await expect(page.getByText('计划缺口：未交代研究问题的来源')).toBeVisible()

  await page.getByRole('button', { name: '应用到稿件' }).click()
  await expect(page.getByText('已应用')).toBeVisible()
  await page.getByRole('button', { name: '撤销这次修改' }).click()
  await expect(page.getByText('已撤销', { exact: true })).toBeVisible()

  // 带原因忽略：请求体必须携带 reasonCode，供质量评测。
  status = 'pending'
  await page.reload()
  await page.getByText(/^建议 \(/).click()
  await page.getByRole('button', { name: '忽略' }).click()
  await page.getByRole('button', { name: '太泛' }).click()
  await page.getByRole('button', { name: '确认忽略' }).click()
  await expect.poll(() => rejectBodies.length).toBe(1)
  expect(rejectBodies[0].reasonCode).toBe('too_vague')
})

test('自动检查：段落完成后 3 秒触发，且不抢焦点、不弹 toast', async ({ page }) => {
  test.setTimeout(120_000)
  const posts: Array<Record<string, unknown>> = []
  await mockSuggestionBackend(page, { posts, proposals: () => [] })

  await page.goto(WRITING_URL)
  await enableAutoCheck(page)
  const editor = page.locator('.monaco-editor').first()
  await editor.click()
  await page.keyboard.press('Control+Home')
  await page.keyboard.press('End')
  await page.keyboard.type('补充一句。')
  // 光标离开刚编辑的段落（下移到空行再进入下一段）：视为「段落完成」。
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('ArrowDown')

  await expect.poll(() => posts.length, { timeout: 5_000 }).toBe(1)
  expect(posts[0].trigger).toBe('auto')
  expect(String(posts[0].paragraphText)).toContain('补充一句。')

  // 不打断：焦点仍在编辑器、没有 toast。
  expect(await page.evaluate(() => Boolean(document.activeElement?.closest('.monaco-editor')))).toBe(true)
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0)
  // 3s 路径已触发后，8s 计时不应再发第二次。
  await page.waitForTimeout(5_000)
  expect(posts.length).toBe(1)
})

test('自动检查：仍在段内继续编辑时延迟到 8 秒才触发', async ({ page }) => {
  test.setTimeout(120_000)
  const posts: Array<Record<string, unknown>> = []
  await mockSuggestionBackend(page, { posts, proposals: () => [] })

  await page.goto(WRITING_URL)
  await enableAutoCheck(page)
  const editor = page.locator('.monaco-editor').first()
  await editor.click()
  await page.keyboard.press('Control+Home')
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('End')
  await page.keyboard.type('再补充。')

  // 3s 时不得触发（光标仍在同一段）。
  await page.waitForTimeout(3_400)
  expect(posts, '段内继续编辑时 3s 不得触发').toEqual([])

  await expect.poll(() => posts.length, { timeout: 7_000 }).toBe(1)
  expect(posts[0].trigger).toBe('auto')
  expect(String(posts[0].paragraphText)).toContain('再补充。')
  expect(await page.evaluate(() => Boolean(document.activeElement?.closest('.monaco-editor')))).toBe(true)
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0)
})
