import { expect, test, type Page } from '@playwright/test'
import { WRITING_URL, mockWritingPageAPIs, seedAuthTokens } from '../helpers'

/**
 * 对话可靠性与体验（agent-chat-reliability-and-ux）。
 *
 * 全部使用 mock，不依赖真实后端/模型；断言用户可见行为：
 * 新对话、409 自愈、历史恢复、取消、断线续传、切会话竞态、失败重试。
 *
 * 契约假设（与 design.md 对齐）：
 * - GET /agent/conversations/:id -> { id, title, threadVersion }
 * - GET .../messages -> 消息数组，part 保留 type/content
 * - GET /projects/:id/agent/proposals?status=pending -> 建议数组
 * - POST /runs 409 -> { code:409, info, data:{ threadVersion } }
 * - SSE 使用标准 id: 行；重连带 ?afterSeq=
 * - 取消/重试接口为 POST /agent/runs/:id/{cancel,retry}
 */

const CONVERSATIONS = '**/api/v1/projects/*/agent/conversations'
const CONVERSATION_DETAIL = '**/api/v1/agent/conversations/*'
const RUNS = '**/api/v1/agent/conversations/*/runs'
const RUN_API = '**/api/v1/agent/runs/**'
const PROPOSALS = '**/api/v1/projects/*/agent/proposals*'

const PROVIDER = {
  id: 'prov-1',
  name: '本地模型',
  models: ['demo-model'],
  active_model: 'demo-model',
  api_key: 'test-key',
  base_url: 'https://example.invalid/v1',
}

function settings(withProvider: boolean) {
  return {
    code: 200,
    info: '',
    data: {
      settings: {
        ui: { theme: 'light', language: 'zh-CN', sidebar_collapsed: false },
        editor: {},
        agent: {
          providers: withProvider ? [PROVIDER] : [],
          active_provider_id: withProvider ? PROVIDER.id : '',
        },
      },
    },
  }
}

type SseItem = { id?: number; data: unknown } | '[DONE]'

function sse(items: SseItem[]): string {
  return items
    .map((item) => {
      if (item === '[DONE]') return 'data: [DONE]\n\n'
      const id = item.id === undefined ? '' : `id: ${item.id}\n`
      return `${id}data: ${JSON.stringify(item.data)}\n\n`
    })
    .join('')
}

function json(body: unknown, status = 200) {
  return {
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  }
}

function streamBody(body: string) {
  return { status: 200, contentType: 'text/event-stream', body }
}

interface MockOptions {
  conversations?: Array<{ id: string; title: string }>
  version?: number
  messages?: Record<string, unknown[]>
  proposals?: unknown[]
  onCreateConversation?: () => void
  onCreateRun?: (body: Record<string, unknown>, attempt: number) => { status: number; body: unknown } | null
  onStream?: (url: string, attempt: number) => string
  onCancel?: (attempt: number) => { status: number; body: unknown }
  onRetry?: (attempt: number) => { status: number; body: unknown }
}

/**
 * 安装 AI 面板所需的全部 mock。后注册的路由优先于 mockWritingPageAPIs 里的默认值。
 */
async function mockAgent(page: Page, opts: MockOptions = {}) {
  const conversations = opts.conversations ?? [{ id: 'conv-1', title: '旧会话' }]
  const version = opts.version ?? 2
  let runAttempt = 0
  let streamAttempt = 0
  let cancelAttempt = 0
  let retryAttempt = 0

  await page.route(/\/api\/v1\/settings\/?$/, (route) => route.fulfill(json(settings(true))))

  await page.route(CONVERSATIONS, async (route) => {
    const method = route.request().method()
    if (method === 'POST') {
      opts.onCreateConversation?.()
      return route.fulfill(json({ code: 200, info: '', data: { id: 'conv-new', title: '新会话', version: 0 } }))
    }
    return route.fulfill(json({ code: 200, info: '', data: conversations }))
  })

  await page.route(CONVERSATION_DETAIL, async (route) => {
    const url = route.request().url()
    if (route.request().method() !== 'GET' || url.includes('/messages')) {
      return route.fallback()
    }
    const id = url.split('/').pop() ?? 'conv-1'
    return route.fulfill(json({ code: 200, info: '', data: { id, title: id, threadVersion: version } }))
  })

  await page.route('**/api/v1/agent/conversations/*/messages', async (route) => {
    const id = route.request().url().split('/').slice(-2)[0]
    return route.fulfill(json({ code: 200, info: '', data: opts.messages?.[id] ?? [] }))
  })

  await page.route(PROPOSALS, (route) =>
    route.fulfill(json({ code: 200, info: '', data: opts.proposals ?? [] })),
  )

  await page.route(RUNS, async (route) => {
    runAttempt++
    const body = (route.request().postDataJSON() ?? {}) as Record<string, unknown>
    const override = opts.onCreateRun?.(body, runAttempt)
    if (override) return route.fulfill(json(override.body, override.status))
    return route.fulfill(json({
      code: 200,
      info: '',
      data: {
        turnId: `t${runAttempt}`,
        runId: `r${runAttempt}`,
        userMessageId: `m${runAttempt}`,
        streamUrl: `/api/v1/agent/runs/r${runAttempt}/events`,
        threadVersion: version + runAttempt,
      },
    }))
  })

  await page.route(RUN_API, async (route) => {
    const url = route.request().url()
    const method = route.request().method()

    if (url.includes('/events')) {
      streamAttempt++
      return route.fulfill(streamBody(opts.onStream?.(url, streamAttempt) ?? sse([
        { id: 1, data: { type: 'start', messageId: `m-stream-${streamAttempt}` } },
        { id: 2, data: { type: 'text-start', id: 't1' } },
        { id: 3, data: { type: 'text-delta', id: 't1', delta: '好的' } },
        { id: 4, data: { type: 'text-end', id: 't1' } },
        { id: 5, data: { type: 'finish' } },
        '[DONE]',
      ])))
    }
    if (url.endsWith('/cancel') && method === 'POST') {
      cancelAttempt++
      const result = opts.onCancel?.(cancelAttempt) ?? { status: 200, body: { code: 200, info: '', data: { cancelled: true } } }
      return route.fulfill(json(result.body, result.status))
    }
    if (url.endsWith('/retry') && method === 'POST') {
      retryAttempt++
      const result = opts.onRetry?.(retryAttempt) ?? {
        status: 200,
        body: { code: 200, info: '', data: { runId: 'r-retry', streamUrl: '/api/v1/agent/runs/r-retry/events' } },
      }
      return route.fulfill(json(result.body, result.status))
    }
    if (url.includes('/evidence')) {
      return route.fulfill(json({ code: 200, info: '', data: [] }))
    }
    return route.fallback()
  })

  return {
    runAttempts: () => runAttempt,
    streamAttempts: () => streamAttempt,
    cancelAttempts: () => cancelAttempt,
    retryAttempts: () => retryAttempt,
  }
}

test.describe('AI 对话可靠性与体验', () => {
  test('新对话保持空白，不被自动重开第一条历史会话', async ({ page }) => {
    await page.goto('/')
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    let created = false
    const mock = await mockAgent(page, {
      conversations: [{ id: 'conv-1', title: '旧会话' }],
      messages: {
        'conv-1': [{
          id: 'm-old', role: 'assistant', status: 'completed', createdAt: new Date().toISOString(),
          parts: [{ id: 't-old', ordinal: 0, type: 'text', status: 'completed', text: '旧回答', createdAt: new Date().toISOString() }],
        }],
      },
      onCreateConversation: () => { created = true },
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('writing-tab-ai').click()
    await expect(page.getByText('旧回答')).toBeVisible()

    await page.getByRole('button', { name: /旧会话/ }).click()
    await page.getByRole('menuitem', { name: '新对话' }).click()

    // 关键：新对话后不能又自动打开 conv-1
    await expect(page.getByText('旧回答')).toHaveCount(0)
    await expect(page.getByPlaceholder('问点什么…')).toBeVisible()

    await page.getByPlaceholder('问点什么…').fill('新问题')
    await page.getByPlaceholder('问点什么…').press('Enter')

    await expect.poll(() => created).toBe(true)
    await expect.poll(() => mock.runAttempts()).toBe(1)
    await expect(page.getByText('好的')).toBeVisible()
  })

  test('409 更新线程版本后可重试，且只产生一个成功 Run', async ({ page }) => {
    await page.goto('/')
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    const versions: unknown[] = []
    const mock = await mockAgent(page, {
      version: 2,
      onCreateRun: (body, attempt) => {
        versions.push(body.expectedThreadVersion)
        if (attempt === 1) {
          return { status: 409, body: { code: 409, info: '会话已更新', data: { threadVersion: 3 } } }
        }
        return null
      },
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('writing-tab-ai').click()
    await page.getByPlaceholder('问点什么…').fill('继续')
    await page.getByPlaceholder('问点什么…').press('Enter')

    await expect(page.getByText('会话已更新')).toBeVisible()
    await page.getByRole('button', { name: /重试/ }).first().click()

    await expect(page.getByText('好的')).toBeVisible()
    await expect.poll(() => mock.runAttempts()).toBe(2)
    expect(versions[1]).toBe(3)
  })

  test('刷新后历史中的工具卡与建议卡可恢复', async ({ page }) => {
    await page.goto('/')
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    await mockAgent(page, {
      messages: {
        'conv-1': [
          {
            id: 'm-user', role: 'user', status: 'completed', createdAt: new Date().toISOString(),
            parts: [{ id: 'u1', ordinal: 0, type: 'text', status: 'completed', text: '看看文件', createdAt: new Date().toISOString() }],
          },
          {
            id: 'm-assistant', role: 'assistant', status: 'completed', runId: 'r-old', createdAt: new Date().toISOString(),
            parts: [
              { id: 't1', ordinal: 0, type: 'text', status: 'completed', text: '历史回答', createdAt: new Date().toISOString() },
              {
                id: 'tool-call-1', ordinal: 1, type: 'tool', status: 'succeeded', createdAt: new Date().toISOString(),
                content: { toolCallId: 'tool-call-1', toolName: 'read_document', input: { name: 'a.pdf' }, output: 'ok' },
              },
              {
                id: 'proposal-p1', ordinal: 2, type: 'proposal', status: 'pending', createdAt: new Date().toISOString(),
                content: { proposalId: 'p1' },
              },
            ],
          },
        ],
      },
      proposals: [{ id: 'p1', filePath: 'main.tex', oldText: '旧句', newText: '新句', status: 'pending', reason: '更清楚' }],
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('writing-tab-ai').click()

    await expect(page.getByText('历史回答')).toBeVisible()
    await expect(page.getByText('read_document')).toBeVisible()
    await expect(page.getByText('成功')).toBeVisible()
    await expect(page.getByText('修改建议')).toBeVisible()
    await expect(page.getByText('main.tex', { exact: true }).first()).toBeVisible()
    // 旧实现会把工具卡显示成「工具 / completed」并丢掉建议卡
    await expect(page.getByText('工具', { exact: true })).toHaveCount(0)
    await expect(page.getByText('completed', { exact: true })).toHaveCount(0)
  })

  test('取消显示正在停止，成功后显示已停止', async ({ page }) => {
    await page.goto('/')
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    let releaseEvents: () => void = () => {}
    const held = new Promise<void>((resolve) => { releaseEvents = resolve })

    await mockAgent(page, {
      onStream: () => sse([{ id: 1, data: { type: 'start', messageId: 'm-slow' } }]),
    })
    await page.route('**/api/v1/agent/runs/*/events*', async (route) => {
      await held
      await route.fulfill(streamBody(sse([
        { id: 1, data: { type: 'start', messageId: 'm-slow' } },
        { id: 2, data: { type: 'abort', reason: 'cancelled' } },
      ])))
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('writing-tab-ai').click()
    await page.getByPlaceholder('问点什么…').fill('慢慢回答')
    await page.getByPlaceholder('问点什么…').press('Enter')

    await expect(page.getByRole('button', { name: '停止' })).toBeVisible()
    await page.getByRole('button', { name: '停止' }).click()

    await expect(page.getByRole('button', { name: '正在停止' })).toBeVisible()
    releaseEvents()
    await expect(page.getByText('已停止')).toBeVisible()
  })

  test('取消失败时显示错误，不假装已取消', async ({ page }) => {
    await page.goto('/')
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    let releaseEvents: () => void = () => {}
    const held = new Promise<void>((resolve) => { releaseEvents = resolve })

    await mockAgent(page, {
      onCancel: () => ({ status: 500, body: { code: 500, info: '取消失败，请稍后重试', data: null } }),
    })
    await page.route('**/api/v1/agent/runs/*/events*', async (route) => {
      await held
      await route.fulfill(streamBody(''))
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('writing-tab-ai').click()
    await page.getByPlaceholder('问点什么…').fill('慢慢回答')
    await page.getByPlaceholder('问点什么…').press('Enter')
    await page.getByRole('button', { name: '停止' }).click()

    await expect(page.getByText(/取消失败/)).toBeVisible()
    await expect(page.getByText('已停止')).toHaveCount(0)
    releaseEvents()
  })

  test('断线后带 afterSeq 重连并完成回答，不重复文本', async ({ page }) => {
    await page.goto('/')
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    const eventUrls: string[] = []
    const mock = await mockAgent(page, {
      onStream: (url, attempt) => {
        eventUrls.push(url)
        if (attempt === 1) {
          return sse([
            { id: 1, data: { type: 'start', messageId: 'm-reconnect' } },
            { id: 2, data: { type: 'text-start', id: 't1' } },
            { id: 3, data: { type: 'text-delta', id: 't1', delta: '第一段' } },
          ])
        }
        return sse([
          { id: 4, data: { type: 'text-delta', id: 't1', delta: '第二段' } },
          { id: 5, data: { type: 'text-end', id: 't1' } },
          { id: 6, data: { type: 'finish' } },
          '[DONE]',
        ])
      },
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('writing-tab-ai').click()
    await page.getByPlaceholder('问点什么…').fill('继续')
    await page.getByPlaceholder('问点什么…').press('Enter')

    await expect(page.getByText('第一段第二段')).toBeVisible()
    await expect.poll(() => mock.streamAttempts()).toBe(2)
    expect(eventUrls[1]).toContain('afterSeq=3')
  })

  test('快速切会话时迟到的历史响应不覆盖当前会话', async ({ page }) => {
    await page.goto('/')
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    let releaseFirst: () => void = () => {}
    const firstHeld = new Promise<void>((resolve) => { releaseFirst = resolve })

    await mockAgent(page, {
      conversations: [
        { id: 'conv-1', title: '会话一' },
        { id: 'conv-2', title: '会话二' },
      ],
      messages: {
        'conv-1': [{
          id: 'm1', role: 'assistant', status: 'completed', createdAt: new Date().toISOString(),
          parts: [{ id: 'p1', ordinal: 0, type: 'text', status: 'completed', text: '第一条会话内容', createdAt: new Date().toISOString() }],
        }],
        'conv-2': [{
          id: 'm2', role: 'assistant', status: 'completed', createdAt: new Date().toISOString(),
          parts: [{ id: 'p2', ordinal: 0, type: 'text', status: 'completed', text: '第二条会话内容', createdAt: new Date().toISOString() }],
        }],
      },
    })
    // 让 conv-1 的历史晚于 conv-2 返回
    await page.route('**/api/v1/agent/conversations/conv-1/messages', async (route) => {
      await firstHeld
      await route.fulfill(json({
        code: 200,
        info: '',
        data: [{
          id: 'm1', role: 'assistant', status: 'completed', createdAt: new Date().toISOString(),
          parts: [{ id: 'p1', ordinal: 0, type: 'text', status: 'completed', text: '第一条会话内容', createdAt: new Date().toISOString() }],
        }],
      }))
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('writing-tab-ai').click()

    // 面板会自动打开 conv-1（挂起）；立即切到 conv-2
    await page.getByRole('button', { name: /会话一/ }).click()
    await page.getByRole('menuitem', { name: '会话二' }).click()
    await expect(page.getByText('第二条会话内容')).toBeVisible()

    releaseFirst()
    await page.waitForTimeout(300)
    await expect(page.getByText('第二条会话内容')).toBeVisible()
    await expect(page.getByText('第一条会话内容')).toHaveCount(0)
  })

  test('失败的回答提供重试入口，重试后展示新回答', async ({ page }) => {
    await page.goto('/')
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    const mock = await mockAgent(page, {
      onStream: (_url, attempt) => attempt === 1
        ? sse([
            { id: 1, data: { type: 'start', messageId: 'm-fail' } },
            { id: 2, data: { type: 'error', errorText: '模型调用失败，请稍后重试' } },
            { id: 3, data: { type: 'finish' } },
            '[DONE]',
          ])
        : sse([
            { id: 1, data: { type: 'start', messageId: 'm-retry' } },
            { id: 2, data: { type: 'text-start', id: 't1' } },
            { id: 3, data: { type: 'text-delta', id: 't1', delta: '重试成功' } },
            { id: 4, data: { type: 'finish' } },
            '[DONE]',
          ]),
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('writing-tab-ai').click()
    await page.getByPlaceholder('问点什么…').fill('回答我')
    await page.getByPlaceholder('问点什么…').press('Enter')

    await expect(page.getByText('模型调用失败，请稍后重试')).toBeVisible()
    await page.getByRole('button', { name: /重试/ }).first().click()

    await expect(page.getByText('重试成功')).toBeVisible()
    await expect.poll(() => mock.retryAttempts()).toBe(1)
  })
})
