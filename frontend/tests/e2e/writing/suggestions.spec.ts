import { expect, test } from '@playwright/test'
import { WRITING_URL, mockWritingPageAPIs, seedAuthTokens, TEST_PROJECT_ID } from '../helpers'

/**
 * 动态写作建议链路（HELE-69）：
 * - 默认进入写作页不发 POST /agent/suggestions（专注写作、不打扰）；
 * - 手动「检查本段」才发请求，事件流只进侧栏；
 * - 建议卡展示三源依据，可应用、可带原因忽略、可一键撤销。
 */

const PARAGRAPH = '人工智能正在改变科研写作方式，但通用写作助手常常给出缺少依据的空泛建议。'
const FILE_CONTENT = `${PARAGRAPH}\n\n\\section{研究背景}\n正文。\n`

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

test('默认不自动检查；手动检查与建议卡应用、忽略、撤销闭环', async ({ page }) => {
  test.setTimeout(90_000)
  let status = 'pending'
  const suggestionPosts: Array<Record<string, unknown>> = []
  const rejectBodies: Array<Record<string, unknown>> = []

  await mockWritingPageAPIs(page)
  await seedAuthTokens(page)

  // 覆盖：正文首行即段落，光标默认落在段内。
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
      body: JSON.stringify({ code: 200, data: [{ ...PROPOSAL, status }], info: '' }),
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
    suggestionPosts.push(route.request().postDataJSON() as Record<string, unknown>)
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: { runId: 'run-1', conversationId: 'conv-1', streamUrl: '/api/v1/agent/runs/run-1/events', trigger: 'manual' }, info: '' }),
    })
  })
  await page.route('**/api/v1/agent/runs/run-1/events', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      body: 'data: {"type":"tool-input-available","toolCallId":"t1","toolName":"skip_suggestion","input":{"reason":"本段表达已清晰"}}\n\ndata: {"type":"finish"}\n\ndata: [DONE]\n\n',
    })
  })
  await page.route('**/api/v1/agent/change-proposals/proposal-1/apply', async (route) => {
    status = 'applied'
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { status } }) })
  })
  await page.route('**/api/v1/agent/change-proposals/proposal-1/reject', async (route) => {
    rejectBodies.push(route.request().postDataJSON() as Record<string, unknown>)
    status = 'rejected'
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { status } }) })
  })
  await page.route('**/api/v1/agent/change-proposals/proposal-1/undo', async (route) => {
    status = 'reverted'
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { status } }) })
  })

  await page.goto(WRITING_URL)
  await expect(page.getByRole('button', { name: 'AI 助手侧栏，1 条待处理建议' })).toBeVisible()

  // 验收：默认进入写作页不自动建 Run。
  await page.waitForTimeout(1500)
  expect(suggestionPosts, '默认进入写作页不得自动发 POST /agent/suggestions').toEqual([])

  // 手动检查：请求体必须带当前段落原文，结果只进侧栏。
  await page.getByRole('button', { name: '检查本段' }).first().click()
  await expect.poll(() => suggestionPosts.length).toBe(1)
  expect(suggestionPosts[0].paragraphText).toBe(PARAGRAPH)
  expect(suggestionPosts[0].trigger).toBe('manual')
  await expect(page.getByText('本段暂无需要修改的问题。')).toBeVisible()

  // 建议卡：依据可展开、可应用、可撤销。
  await page.getByText('建议 (1)').click()
  await expect(page.getByText('修改建议')).toBeVisible()
  await expect(page.getByText('章节计划 · 研究背景')).toBeVisible()
  await page.getByRole('button', { name: /章节计划 · 研究背景/ }).click()
  await expect(page.getByText('计划缺口：未交代研究问题的来源')).toBeVisible()

  await page.getByRole('button', { name: '应用到稿件' }).click()
  await expect(page.getByText('已应用')).toBeVisible()
  await page.getByRole('button', { name: '撤销这次修改' }).click()
  await expect(page.getByText('已撤销')).toBeVisible()

  // 带原因忽略：请求体必须携带 reasonCode，供质量评测。
  status = 'pending'
  await page.reload()
  await page.getByText('建议 (1)').click()
  await page.getByRole('button', { name: '忽略' }).click()
  await page.getByRole('button', { name: '太泛' }).click()
  await page.getByRole('button', { name: '确认忽略' }).click()
  await expect.poll(() => rejectBodies.length).toBe(1)
  expect(rejectBodies[0].reasonCode).toBe('too_vague')
})
