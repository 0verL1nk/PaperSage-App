import { expect, test } from '@playwright/test'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { WRITING_URL, mockWritingPageAPIs, seedAuthTokens } from '../helpers'

/**
 * AI 助手面板（模式 B：先建 Run 再订阅事件）。
 *
 * 这里把后端事件流用 route.fulfill 直接喂出去——协议是 SSE 的 data: 行，
 * 前端用 fetch + ReadableStream 解析（EventSource 不支持 POST/Authorization）。
 */

const AGENT_BASE = `**/api/v1/projects/*/agent/**`
const RUN_BASE = '**/api/v1/agent/**'

function settings(providers: unknown[]) {
  return {
    code: 200,
    info: '',
    // 后端 GET /settings 的信封是 {data: {settings: {ui, editor, agent}}}，
    // 少一层 settings 包装的话前端拿到 undefined（e2e 真踩过）。
    data: {
      settings: {
        ui: { theme: 'light', language: 'zh-CN', sidebar_collapsed: false },
        editor: {},
        agent: { providers, active_provider_id: providers.length ? (providers[0] as { id: string }).id : '' },
      },
    },
  }
}

const PROVIDER = {
  id: 'prov-1',
  name: '本地模型',
  models: ['demo-model'],
  active_model: 'demo-model',
}
const samplePdfPath = fileURLToPath(new URL('../fixtures/sample.pdf', import.meta.url))

async function mockAgentApis(
  page: import('@playwright/test').Page,
  opts: { withProvider: boolean; stream?: string; emptyConversations?: boolean; onCreateConversation?: () => void; messages?: unknown[] },
) {
  // 用正则而不是 glob：`**/api/v1/settings` 匹配不到带尾斜杠的 `/api/v1/settings/`，
  // 那样面板会一直显示「尚未配置 AI 模型」（真实踩到过）。
  await page.route(/\/api\/v1\/settings\/?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(settings(opts.withProvider ? [PROVIDER] : [])),
    })
  })

  await page.route(AGENT_BASE, async (route) => {
    const url = route.request().url()
    if (url.includes('/agent/conversations') && !url.includes('/runs')) {
      // POST = 建会话（新项目第一条消息的路径）；GET = 列会话
      if (route.request().method() === 'POST') {
        opts.onCreateConversation?.()
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            code: 200,
            info: '',
            data: { id: 'conv-new', title: '看看我的文件', version: 0 },
          }),
        })
        return
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          data: opts.emptyConversations ? [] : [{ id: 'conv-1', title: '看看我的文件' }],
          info: '',
        }),
      })
      return
    }
    if (url.includes('/agent/proposals')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: [], info: '' }),
      })
      return
    }
    await route.fallback()
  })

  await page.route(RUN_BASE, async (route) => {
    const url = route.request().url()
    const method = route.request().method()

    if (url.includes('/evidence') && method === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          info: '',
          data: [{ id: 'e1', sourceName: '研究计划.pdf', quote: '这是一段可回查的原文依据。' }],
        }),
      })
      return
    }
    if (url.endsWith('/runs') && method === 'POST') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          info: '',
          data: {
            turnId: 't1',
            runId: 'r1',
            userMessageId: 'm1',
            streamUrl: '/api/v1/agent/runs/r1/events',
            threadVersion: 2,
          },
        }),
      })
      return
    }
    if (url.includes('/events')) {
      await route.fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body: opts.stream ?? '',
      })
      return
    }
    if (url.includes('/messages')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: opts.messages ?? [], info: '' }),
      })
      return
    }
    await route.fallback()
  })
}

test.describe('AI 助手面板', () => {
  test('未配置模型时给出设置入口，且不给空输入框', async ({ page }) => {
    await page.goto('/')
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await mockAgentApis(page, { withProvider: false })

    await page.goto(WRITING_URL)
    await page.getByTestId('writing-tab-ai').click()

    await expect(page.getByText('尚未配置 AI 模型')).toBeVisible()
    await expect(page.getByRole('button', { name: '去设置' })).toBeVisible()
    // 关键：不给一个发不出去的空输入框
    await expect(page.getByPlaceholder('问点什么…')).toHaveCount(0)
  })

  test('已配置模型时可发送并看到流式回复', async ({ page }) => {
    const stream =
      'data: {"type":"start","messageId":"m2"}\n\n' +
      'data: {"type":"data-conversation","data":{"conversationId":"conv-1","runId":"r1"}}\n\n' +
      'data: {"type":"text-start","id":"t1"}\n\n' +
      'data: {"type":"text-delta","id":"t1","delta":"你的项目里有 "}\n\n' +
      'data: {"type":"text-delta","id":"t1","delta":"main.tex"}\n\n' +
      'data: {"type":"text-end","id":"t1"}\n\n' +
      'data: {"type":"finish"}\n\n' +
      'data: [DONE]\n\n'

    await page.goto('/')
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await mockAgentApis(page, { withProvider: true, stream })

    await page.goto(WRITING_URL)
    await page.getByTestId('writing-tab-ai').click()

    await page.getByPlaceholder('问点什么…').fill('看看我的文件')
    await page.getByPlaceholder('问点什么…').press('Enter')

    await expect(page.getByText('你的项目里有 main.tex')).toBeVisible()
    await expect(page.getByText('参考材料 · 1')).toBeVisible()
    await page.getByText('参考材料 · 1').click()
    await expect(page.getByText('研究计划.pdf')).toBeVisible()
    await expect(page.getByText('这是一段可回查的原文依据。')).toBeVisible()
  })

  test('Agent compile_project routes the requested root file through the browser engine', async ({ page }) => {
    const stream = [
      { type: 'start', messageId: 'm-compile' },
      { type: 'tool-input-start', toolCallId: 'compile-call', toolName: 'compile_project' },
      { type: 'tool-output-available', toolCallId: 'compile-call', output: `browser_compile_request:${JSON.stringify({ root_file: 'sections/intro.tex' })}` },
      { type: 'finish' },
    ].map((event) => `data: ${JSON.stringify(event)}\n\n`).join('') + 'data: [DONE]\n\n'
    const pdfBase64 = (await readFile(samplePdfPath)).toString('base64')
    const compilerModule = `
      export async function compileLatexInBrowser(_files, rootFile, compiler, options = {}) {
        options.onProgress?.({ stage: 'compile', message: 'agent browser compile', pass: 1, totalPasses: 1 });
        if (rootFile !== 'sections/intro.tex' || !['pdflatex', 'xelatex'].includes(compiler)) throw new Error('unexpected browser compile input');
        return { pdf: Uint8Array.from(atob(${JSON.stringify(pdfBase64)}), char => char.charCodeAt(0)), log: 'agent fixture log' };
      }
    `
    let browserJobBody: Record<string, unknown> | null = null
    let artifactUploaded = false

    await page.goto('/')
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await mockAgentApis(page, { withProvider: true, stream })
    await page.route('**/src/lib/latex/swiftlatex-browser-compiler.ts*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/javascript', body: compilerModule })
    })
    await page.route(`**/api/v1/projects/*/latex/compile/browser`, async (route) => {
      browserJobBody = route.request().postDataJSON() as Record<string, unknown>
      await route.fulfill({ status: 202, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { job_id: 'agent-browser-job', status: 'running' } }) })
    })
    await page.route('**/api/v1/projects/*/latex/compile/agent-browser-job/browser-result', async (route) => {
      artifactUploaded = true
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { job_id: 'agent-browser-job', build_id: 'agent-browser-build', status: 'success' } }) })
    })
    await page.route('**/api/v1/projects/*/latex/compile/agent-browser-job/status/', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { job_id: 'agent-browser-job', build_id: 'agent-browser-build', status: 'success' } }) })
    })
    await page.route('**/api/v1/projects/*/latex/output/**', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/pdf', body: await readFile(samplePdfPath) })
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('writing-tab-ai').click()
    await page.getByPlaceholder('问点什么…').fill('编译章节')
    await page.getByPlaceholder('问点什么…').press('Enter')

    await expect.poll(() => browserJobBody, { timeout: 15_000 }).not.toBeNull()
    await expect.poll(() => artifactUploaded).toBe(true)
    expect(browserJobBody).toMatchObject({ root_file: 'sections/intro.tex' })
  })

  test('刷新后仍能从历史助手消息回查 Run 依据', async ({ page }) => {
    await page.goto('/')
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await mockAgentApis(page, {
      withProvider: true,
      messages: [{
        id: 'm-history',
        role: 'assistant',
        status: 'completed',
        runId: 'r-history',
        createdAt: new Date().toISOString(),
        parts: [{ id: 'text', ordinal: 0, type: 'text', status: 'completed', text: '历史回答', createdAt: new Date().toISOString() }],
      }],
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('writing-tab-ai').click()

    await expect(page.getByText('历史回答')).toBeVisible()
    await expect(page.getByText('参考材料 · 1')).toBeVisible()
    await page.getByText('参考材料 · 1').click()
    await expect(page.getByText('研究计划.pdf')).toBeVisible()
    await expect(page.getByText('这是一段可回查的原文依据。')).toBeVisible()
  })

  test('建议卡片可应用，应用后刷新文件树', async ({ page }) => {
    const stream =
      'data: {"type":"start","messageId":"m2"}\n\n' +
      'data: {"type":"tool-input-start","toolCallId":"c1","toolName":"propose_edit"}\n\n' +
      'data: {"type":"data-proposal","data":{"id":"p1","filePath":"main.tex","oldText":"ABC","newText":"XYZ","status":"pending"}}\n\n' +
      'data: {"type":"finish"}\n\n' +
      'data: [DONE]\n\n'

    await page.goto('/')
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
    await mockAgentApis(page, { withProvider: true, stream })

    let applied = false
    await page.route('**/api/v1/agent/change-proposals/p1/apply', async (route) => {
      applied = true
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { status: 'applied' }, info: '' }),
      })
    })
    // 应用后前端会重新拉文件列表——记录它确实发生了
    let refetchedFiles = false
    await page.route('**/api/v1/projects/*/latex-files/', async (route) => {
      refetchedFiles = true
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          info: '',
          data: { files: [{ id: 'f1', file_path: 'main.tex', content_type: 'text/plain', size_bytes: 6, last_saved_at: null }] },
        }),
      })
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('writing-tab-ai').click()

    await page.getByPlaceholder('问点什么…').fill('改一下开头')
    await page.getByPlaceholder('问点什么…').press('Enter')

    await expect(page.getByText('修改建议')).toBeVisible()
    await page.getByRole('button', { name: '应用' }).click()

    await expect.poll(() => applied).toBe(true)
    // 联动：稿件被改写后必须刷新文件树，否则用户会在旧内容上继续编辑
    await expect.poll(() => refetchedFiles).toBe(true)
  })

  test('新项目还没有会话时，第一条消息先建会话再发送', async ({ page }) => {
    const stream =
      'data: {"type":"start","messageId":"m2"}\n\n' +
      'data: {"type":"data-conversation","data":{"conversationId":"conv-new","runId":"r1"}}\n\n' +
      'data: {"type":"text-start","id":"t1"}\n\n' +
      'data: {"type":"text-delta","id":"t1","delta":"好的"}\n\n' +
      'data: {"type":"finish"}\n\n' +
      'data: [DONE]\n\n'

    await page.goto('/')
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    let createdConversation = false
    await mockAgentApis(page, {
      withProvider: true,
      stream,
      emptyConversations: true,
      onCreateConversation: () => {
        createdConversation = true
      },
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('writing-tab-ai').click()

    // 修复前：会话列表为空 → 没有 conversationId → 发送被静默丢弃（输入被清空、什么也没发生）
    await page.getByPlaceholder('问点什么…').fill('看看我的文件')
    await page.getByPlaceholder('问点什么…').press('Enter')

    await expect.poll(() => createdConversation).toBe(true)
    await expect(page.getByText('好的')).toBeVisible()
  })
})
