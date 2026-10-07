import { describe, expect, it, vi } from 'vitest'
import { emptyChatState, reduceChatEvent, useChatStore, type ChatState } from '@/store/chat-store'
import * as api from '@/services/agent-api'
import type { RunStreamEvent } from '@/services/agent-api'

vi.mock('@/services/agent-api', () => ({
  createConversation: vi.fn(),
  createRun: vi.fn(),
  streamRun: vi.fn(),
  listMessages: vi.fn(),
  cancelRun: vi.fn(),
  applyProposal: vi.fn(),
  rejectProposal: vi.fn(),
}))

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

  it('错误既记进 error 也留成 part', () => {
    const s = feed([
      { type: 'start', messageId: 'm1' },
      { type: 'error', errorText: '模型调用达到限额，请稍后重试' },
    ])
    expect(s.error).toContain('限额')
    expect(s.messages[0].parts.some((p) => p.type === 'error')).toBe(true)
  })

  it('编译联动：从 compile_project 的结果里取出 job id 交给写作页', () => {
    const s = feed([
      { type: 'start', messageId: 'm1' },
      { type: 'tool-input-start', toolCallId: 'c1', toolName: 'compile_project' },
      {
        type: 'tool-output-available',
        toolCallId: 'c1',
        output: '编译任务已提交（job id: 11111111-2222-3333-4444-555555555555），可在编译面板查看进度。',
      },
    ])
    expect(s.pendingCompileJobId).toBe('11111111-2222-3333-4444-555555555555')
  })

  it('编译联动不误取其他工具结果里的 job id', () => {
    const s = feed([
      { type: 'start', messageId: 'm1' },
      { type: 'tool-input-start', toolCallId: 'c1', toolName: 'list_latex_files' },
      {
        type: 'tool-output-available',
        toolCallId: 'c1',
        output: 'job id: 11111111-2222-3333-4444-555555555555',
      },
    ])
    expect(s.pendingCompileJobId).toBeNull()
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
    useChatStore.setState({ ...emptyChatState, conversationId: 'c-old', conversationVersion: 3 })
    vi.mocked(api.createConversation).mockClear()
    stubRun(4)

    await useChatStore.getState().send('p1', '继续')

    expect(api.createConversation).not.toHaveBeenCalled()
    // 乐观锁带上当前版本，服务端据此判断是否有人并发提交
    expect(api.createRun).toHaveBeenCalledWith('c-old', expect.objectContaining({ expectedThreadVersion: 3 }))
  })
})
