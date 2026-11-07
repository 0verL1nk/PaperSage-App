import { beforeEach, describe, expect, it, vi } from 'vitest'
import { emptyChatState, messagesFromApi, reduceChatEvent, useChatStore, type ChatState } from '@/store/chat-store'
import * as api from '@/services/agent-api'
import type { RunStreamEvent } from '@/services/agent-api'
import { FrontendApiError } from '@/services/errors'

vi.mock('@/services/agent-api', () => ({
  createConversation: vi.fn(),
  createRun: vi.fn(),
  streamRun: vi.fn(),
  listMessages: vi.fn(),
  getConversation: vi.fn(),
  listProposals: vi.fn(),
  retryRun: vi.fn(),
  cancelRun: vi.fn(),
  applyProposal: vi.fn(),
  rejectProposal: vi.fn(),
}))

beforeEach(() => {
  vi.clearAllMocks()
  useChatStore.setState({ ...emptyChatState })
  vi.mocked(api.listMessages).mockResolvedValue([])
  vi.mocked(api.getConversation).mockResolvedValue({ id: 'c-old', title: '旧会话', threadVersion: 1 })
  vi.mocked(api.listProposals).mockResolvedValue([])
  vi.mocked(api.streamRun).mockResolvedValue(undefined)
})

function feed(events: RunStreamEvent[], from: ChatState = emptyChatState): ChatState {
  return events.reduce((s, e) => reduceChatEvent(s, e), from)
}

function textOf(state: ChatState): string {
  const msg = state.messages[state.messages.length - 1]
  return msg.parts.filter((p) => p.type === 'text').map((p) => p.text).join('')
}

describe('reduceChatEvent', () => {
  it('把同 id 的 text-delta 累加进一个 part', () => {
    const s = feed([
      { type: 'start', messageId: 'm1' },
      { type: 'text-start', id: 't1' },
      { type: 'text-delta', id: 't1', delta: '你好' },
      { type: 'text-delta', id: 't1', delta: '，世界' },
      { type: 'text-end', id: 't1' },
      { type: 'finish' },
    ])
    expect(textOf(s)).toBe('你好，世界')
    expect(s.streaming).toBe(false)
  })

  it('重放不会把文本接两遍：start 重置该消息的 parts', () => {
    const first = feed([
      { type: 'start', messageId: 'm1' },
      { type: 'text-delta', id: 't1', delta: 'AB' },
      { type: 'text-delta', id: 't1', delta: 'CD' },
    ])
    expect(textOf(first)).toBe('ABCD')

    // 断线重连：服务端从头重放同一 Run 的事件
    const replayed = feed(
      [
        { type: 'start', messageId: 'm1' },
        { type: 'text-delta', id: 't1', delta: 'AB' },
        { type: 'text-delta', id: 't1', delta: 'CD' },
        { type: 'text-delta', id: 't1', delta: 'EF' },
      ],
      first,
    )
    expect(textOf(replayed)).toBe('ABCDEF')
  })

  it('工具 part 按 toolCallId 归并：同名工具两次调用互不串味', () => {
    const s = feed([
      { type: 'start', messageId: 'm1' },
      { type: 'tool-input-start', toolCallId: 'call_1', toolName: 'read_latex_file' },
      { type: 'tool-input-start', toolCallId: 'call_2', toolName: 'read_latex_file' },
      { type: 'tool-output-available', toolCallId: 'call_2', output: '第二次的结果' },
      { type: 'tool-output-available', toolCallId: 'call_1', output: '第一次的结果' },
    ])
    const tools = s.messages[0].parts.filter((p) => p.type === 'tool')
    expect(tools).toHaveLength(2)
    const byId = Object.fromEntries(tools.map((p) => [p.id, (p.meta as { output?: string })?.output]))
    expect(byId['call_1']).toBe('第一次的结果')
    expect(byId['call_2']).toBe('第二次的结果')
  })

  it('建议 part 登记进 proposals，供卡片渲染', () => {
    const s = feed([
      { type: 'start', messageId: 'm1' },
      {
        type: 'data-proposal',
        data: { id: 'p1', filePath: 'main.tex', oldText: 'A', newText: 'B', status: 'pending' },
      },
    ])
    expect(s.proposals['p1'].filePath).toBe('main.tex')
    expect(s.messages[0].parts.some((p) => p.type === 'proposal')).toBe(true)
  })

  it('会话信息来自 data-conversation（协议无 meta 事件）', () => {
    const s = feed([
      { type: 'start', messageId: 'm1' },
      { type: 'data-conversation', data: { conversationId: 'c1', runId: 'r1' } },
    ])
    expect(s.conversationId).toBe('c1')
    expect(s.activeRunId).toBe('r1')
  })

  it('完成和中止的助手消息保留 Run ID 以便回查依据', () => {
    const completed = feed([
      { type: 'start', messageId: 'm1' },
      { type: 'data-conversation', data: { runId: 'r1' } },
      { type: 'text-delta', id: 't1', delta: '已完成' },
      { type: 'finish' },
    ])
    expect(completed.messages[0].runId).toBe('r1')
    expect(completed.activeRunId).toBeNull()

    const aborted = feed([
      { type: 'start', messageId: 'm2' },
      { type: 'data-conversation', data: { runId: 'r2' } },
      { type: 'abort' },
    ])
    expect(aborted.messages[0].runId).toBe('r2')
    expect(aborted.activeRunId).toBeNull()
  })

  it('从历史 API 消息恢复 Run ID', () => {
    const rows = messagesFromApi([{
      id: 'm-history', role: 'assistant', status: 'completed', runId: 'r-history', createdAt: '',
      parts: [{ id: 'p1', ordinal: 0, type: 'text', status: 'completed', text: '历史回答', createdAt: '' }],
    }])
    expect(rows[0].runId).toBe('r-history')
  })

  it('错误既记进 error 也留成 part', () => {
    const s = feed([
      { type: 'start', messageId: 'm1' },
      { type: 'error', errorText: '模型调用达到限额，请稍后重试' },
    ])
    expect(s.error).toContain('限额')
    expect(s.messages[0].parts.some((p) => p.type === 'error')).toBe(true)
  })

  it('编译联动：从 compile_project 的结果里取出入口文件交给浏览器编译器', () => {
    const s = feed([
      { type: 'start', messageId: 'm1' },
      { type: 'tool-input-start', toolCallId: 'c1', toolName: 'compile_project' },
      {
        type: 'tool-output-available',
        toolCallId: 'c1',
        output: 'browser_compile_request:{"root_file":"chapters/intro.tex"}',
      },
    ])
    expect(s.pendingBrowserCompileRequest).toEqual({ rootFile: 'chapters/intro.tex' })
  })

  it('编译联动不误取其他工具结果中的浏览器请求标记', () => {
    const s = feed([
      { type: 'start', messageId: 'm1' },
      { type: 'tool-input-start', toolCallId: 'c1', toolName: 'list_latex_files' },
      {
        type: 'tool-output-available',
        toolCallId: 'c1',
        output: 'browser_compile_request:{"root_file":"main.tex"}',
      },
    ])
    expect(s.pendingBrowserCompileRequest).toBeNull()
  })

  it('忽略格式错误的 Agent 编译请求，不触发默认编译', () => {
    const s = feed([
      { type: 'start', messageId: 'm1' },
      { type: 'tool-input-start', toolCallId: 'c1', toolName: 'compile_project' },
      { type: 'tool-output-available', toolCallId: 'c1', output: 'browser_compile_request:{bad-json}' },
    ])
    expect(s.pendingBrowserCompileRequest).toBeNull()
  })
})

describe('send 动作', () => {
  function stubRun(threadVersion: number) {
    vi.mocked(api.createRun).mockResolvedValue({
      turnId: 't1',
      runId: 'r1',
      userMessageId: 'm1',
      streamUrl: '/api/v1/agent/runs/r1/events',
      threadVersion,
    })
    vi.mocked(api.streamRun).mockResolvedValue(undefined)
  }

  it('真实 SSE 中途断流保留部分回答，解除 streaming 并提示恢复而不重跑', async () => {
    useChatStore.setState({ ...emptyChatState, conversationId: 'c-old' })
    vi.mocked(api.createRun).mockClear()
    vi.mocked(api.cancelRun).mockClear()
    stubRun(1)
    const realApi = await vi.importActual<typeof import('@/services/agent-api')>('@/services/agent-api')
    vi.mocked(api.streamRun).mockImplementation(realApi.streamRun)
    const bytes = new TextEncoder().encode(
      'data: {"type":"start","messageId":"m1"}\n\n' +
      'data: {"type":"text-delta","id":"t1","delta":"部分回答"}\n\n' +
      'data: {"type":"text-delta","delta":"截断',
    )
    vi.stubGlobal('fetch', vi.fn(async () => new Response(new ReadableStream({
      start(controller) { controller.enqueue(bytes); controller.close() },
    }), { headers: { 'Content-Type': 'text/event-stream' } })))
    try {
      await expect(useChatStore.getState().send('p1', '问题')).rejects.toThrow(/中断|未完成/)
      const state = useChatStore.getState()
      expect(textOf(state)).toBe('部分回答')
      expect(state.messages.at(-1)?.status).not.toBe('completed')
      expect(state.streaming).toBe(false)
      expect(state.error).toMatch(/中断|未完成/)
      expect(state.error).toMatch(/会话/)
      expect(api.createRun).toHaveBeenCalledTimes(1)
      expect(api.cancelRun).not.toHaveBeenCalled()
      // 初次订阅 + 3 次退避重连后仍失败，才交给用户显式重试。
      expect(fetch).toHaveBeenCalledTimes(4)
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('读取连接失败也保留已到达的内容并提示后台可能仍在运行', async () => {
    useChatStore.setState({ ...emptyChatState, conversationId: 'c-old' })
    stubRun(1)
    vi.mocked(api.streamRun).mockImplementation(async (_runId, { onEvent }) => {
      onEvent({ type: 'start', messageId: 'm1' }, {})
      onEvent({ type: 'text-delta', id: 't1', delta: '部分内容' }, {})
      throw new TypeError('network error')
    })
    await expect(useChatStore.getState().send('p1', '问题')).rejects.toThrow(/后台可能仍在运行/)
    expect(textOf(useChatStore.getState())).toBe('部分内容')
    expect(useChatStore.getState().streaming).toBe(false)
    expect(useChatStore.getState().messages.at(-1)?.runId).toBe('r1')
    expect(useChatStore.getState().error).toMatch(/会话列表/)
  })

  it('还没有会话时先建会话再发 Run——否则第一条消息会静默消失', async () => {
    useChatStore.setState({ ...emptyChatState })
    vi.mocked(api.createConversation).mockResolvedValue({ id: 'c-new', title: '你好', version: 0 })
    stubRun(1)

    await useChatStore.getState().send('p1', '你好')

    expect(api.createConversation).toHaveBeenCalledWith('p1', '你好')
    expect(api.createRun).toHaveBeenCalledWith('c-new', expect.objectContaining({ content: '你好' }))
    expect(useChatStore.getState().conversationId).toBe('c-new')
  })

  it('已有会话时沿用会话，不重复建', async () => {
    useChatStore.setState({ ...emptyChatState, conversationId: 'c-old', conversationVersion: 3, versionKnown: true })
    vi.mocked(api.createConversation).mockClear()
    stubRun(4)

    await useChatStore.getState().send('p1', '继续')

    expect(api.createConversation).not.toHaveBeenCalled()
    // 乐观锁带上当前版本，服务端据此判断是否有人并发提交
    expect(api.createRun).toHaveBeenCalledWith('c-old', expect.objectContaining({ expectedThreadVersion: 3 }))
  })

  it('重新打开已有会话后带上服务端版本，不再省略 CAS', async () => {
    vi.mocked(api.getConversation).mockResolvedValue({ id: 'c-old', title: '旧会话', threadVersion: 7 })
    await useChatStore.getState().open('c-old')
    stubRun(8)

    await useChatStore.getState().send('p1', '继续')

    expect(api.createRun).toHaveBeenCalledWith('c-old', expect.objectContaining({ expectedThreadVersion: 7 }))
  })

  it('409 更新会话版本并提供重试，不静默重发', async () => {
    useChatStore.setState({ ...emptyChatState, conversationId: 'c-old', conversationVersion: 1, versionKnown: true })
    vi.mocked(api.createRun).mockRejectedValueOnce(new FrontendApiError('会话已在其他位置被修改', {
      kind: 'http',
      status: 409,
      details: { code: 409, data: { threadVersion: 5 } },
    }))

    await useChatStore.getState().send('p1', '问题')

    const state = useChatStore.getState()
    expect(state.conversationVersion).toBe(5)
    expect(state.errorAction).toBe('retry-send')
    expect(state.lastFailedContent).toBe('问题')
    expect(state.streaming).toBe(false)
  })

  it('新对话 reset 后保持空白，不被自动重开为历史会话', () => {
    useChatStore.setState({
      ...emptyChatState,
      conversationId: 'c-old',
      messages: [{ id: 'm1', role: 'assistant', status: 'completed', parts: [] }],
    })

    useChatStore.getState().reset()

    expect(useChatStore.getState().conversationId).toBeNull()
    expect(useChatStore.getState().messages).toEqual([])
    expect(useChatStore.getState().isNewConversation).toBe(true)
  })

  it('快速切换会话时迟到的历史响应不覆盖当前会话', async () => {
    let resolveFirst!: (rows: Awaited<ReturnType<typeof api.listMessages>>) => void
    vi.mocked(api.listMessages)
      .mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve }))
      .mockResolvedValueOnce([{
        id: 'm2', role: 'assistant', status: 'completed', createdAt: '',
        parts: [{ id: 't2', ordinal: 0, type: 'text', status: 'completed', text: '第二个会话', createdAt: '' }],
      }])

    const first = useChatStore.getState().open('c1')
    const second = useChatStore.getState().open('c2')
    resolveFirst([{
      id: 'm1', role: 'assistant', status: 'completed', createdAt: '',
      parts: [{ id: 't1', ordinal: 0, type: 'text', status: 'completed', text: '第一个会话', createdAt: '' }],
    }])
    await Promise.all([first, second])

    expect(textOf(useChatStore.getState())).toBe('第二个会话')
  })

  it('历史恢复保留工具与建议类型，不降级成 text', () => {
    const rows = messagesFromApi([{
      id: 'm-history', role: 'assistant', status: 'completed', runId: 'r1', createdAt: '',
      parts: [
        { id: 'call_1', ordinal: 0, type: 'tool', status: 'succeeded', content: { toolName: 'read_latex_file', output: 'x' }, createdAt: '' },
        { id: 'proposal-p1', ordinal: 1, type: 'proposal', status: 'pending', content: { proposalId: 'p1' }, createdAt: '' },
      ],
    }])

    expect(rows[0].parts.map((p) => p.type)).toEqual(['tool', 'proposal'])
    expect(rows[0].parts[1].meta?.proposalId).toBe('p1')
  })

  it('快速重复发送只创建一个 Run', async () => {
    useChatStore.setState({ ...emptyChatState, conversationId: 'c-old', conversationVersion: 1, versionKnown: true })
    vi.mocked(api.createRun).mockClear()
    stubRun(2)

    await Promise.all([
      useChatStore.getState().send('p1', '同一条消息'),
      useChatStore.getState().send('p1', '同一条消息'),
    ])

    expect(api.createRun).toHaveBeenCalledTimes(1)
  })

  it('切换会话后旧流事件不再污染新会话', async () => {
    useChatStore.setState({ ...emptyChatState, conversationId: 'c-old', conversationVersion: 1, versionKnown: true })
    vi.mocked(api.listMessages).mockResolvedValue([])
    stubRun(2)
    let release!: () => void
    let started!: () => void
    const startedPromise = new Promise<void>((resolve) => { started = resolve })
    vi.mocked(api.streamRun).mockImplementation(async (_runId, { onEvent }) => {
      onEvent({ type: 'start', messageId: 'm-old' }, {})
      started()
      await new Promise<void>((resolve) => { release = resolve })
      onEvent({ type: 'text-delta', delta: '旧会话内容' }, {})
    })

    const sending = useChatStore.getState().send('p1', '问题')
    await startedPromise
    await useChatStore.getState().open('c-other')
    release()
    await sending

    expect(useChatStore.getState().messages).toEqual([])
  })
})
