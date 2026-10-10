import { create } from 'zustand'
import * as api from '@/services/agent-api'
import type { AgentChangeProposal, AgentMessage, RunStreamEvent } from '@/services/agent-api'

// 前端事件归并：Normalized Store + Event Reducer（参考架构 §5）。
//
// 为什么不直接拼字符串：一条 Assistant 消息里同时有文本、工具卡片、子 agent 活动、
// 建议卡片，只有把它们建模成有 id 的 part，才能做到按 id 幂等归并、重放不出错。
//
// 重放语义：协议（AI SDK UI Message Stream）没有 sequence，重连会从头重放该 Run
// 的全部事件。因此 `start` 被定义为「重置该消息的 parts」——客户端始终以 afterSeq=0
// 全量重放，重放落在已重置的消息上，不会把文本接两遍。

export type ChatPartType = 'text' | 'tool' | 'activity' | 'proposal' | 'error'

export interface ChatPart {
  id: string
  type: ChatPartType
  text: string
  status: string
  meta?: Record<string, unknown>
}

export interface ChatMessage {
  id: string
  runId?: string
  role: 'user' | 'assistant'
  status: 'streaming' | 'completed'
  parts: ChatPart[]
}

export interface ChatState {
  conversationId: string | null
  conversationVersion: number
  messages: ChatMessage[]
  activeRunId: string | null
  streaming: boolean
  error: string | null
  proposals: Record<string, AgentChangeProposal>
  /** 助手触发了浏览器编译：把入口文件交给当前写作页启动 WASM。 */
  pendingBrowserCompileRequest: { rootFile: string } | null
}

export const emptyChatState: ChatState = {
  conversationId: null,
  conversationVersion: 0,
  messages: [],
  activeRunId: null,
  streaming: false,
  error: null,
  proposals: {},
  pendingBrowserCompileRequest: null,
}

/** Agent tool emits a machine-readable browser request; the page creates its own browser job. */
const BROWSER_COMPILE_REQUEST_RE = /browser_compile_request:(\{[^\r\n]+\})/

function findOrCreatePart(msg: ChatMessage, id: string, type: ChatPartType): ChatPart {
  let part = msg.parts.find((p) => p.id === id)
  if (!part) {
    part = { id, type, text: '', status: 'streaming' }
    msg.parts.push(part)
  }
  return part
}

/** 纯函数：把一个协议 part 归并进状态。同一 part id 重复到达不会产生重复内容。 */
export function reduceChatEvent(state: ChatState, event: RunStreamEvent): ChatState {
  const type = String(event.type ?? '')
  const next: ChatState = { ...state, messages: state.messages.map((m) => ({ ...m, parts: [...m.parts] })) }

  const last = next.messages[next.messages.length - 1]
  const ensureAssistant = (): ChatMessage => {
    if (last && last.role === 'assistant' && last.status === 'streaming') return last
    const created: ChatMessage = { id: `local-${next.messages.length}`, role: 'assistant', status: 'streaming', parts: [] }
    next.messages = [...next.messages, created]
    return created
  }

  switch (type) {
    case 'start': {
      // 重置：重连全量重放时，同一消息的 parts 从头再来而不是叠加
      const id = String(event.messageId ?? '')
      const existing = next.messages.find((m) => m.id === id)
      if (existing) {
        existing.parts = []
        existing.status = 'streaming'
        // 把它挪到末尾，作为当前流式消息
        next.messages = [...next.messages.filter((m) => m.id !== id), existing]
      } else {
        next.messages = [...next.messages, { id, role: 'assistant', status: 'streaming', parts: [] }]
      }
      next.streaming = true
      return next
    }
    case 'data-conversation': {
      const data = (event.data ?? {}) as Record<string, unknown>
      if (typeof data.conversationId === 'string') next.conversationId = data.conversationId
      if (typeof data.runId === 'string') next.activeRunId = data.runId
      return next
    }
    case 'text-delta': {
      const part = findOrCreatePart(ensureAssistant(), String(event.id ?? 'text'), 'text')
      part.text += String(event.delta ?? '')
      return next
    }
    case 'tool-input-start': {
      const part = findOrCreatePart(ensureAssistant(), String(event.toolCallId ?? ''), 'tool')
      part.meta = { toolName: event.toolName }
      part.status = 'input-available'
      return next
    }
    case 'tool-input-available': {
      const part = findOrCreatePart(ensureAssistant(), String(event.toolCallId ?? ''), 'tool')
      part.meta = { ...(part.meta ?? {}), toolName: event.toolName, input: event.input }
      part.status = 'executing'
      return next
    }
    case 'tool-output-available': {
      const part = findOrCreatePart(ensureAssistant(), String(event.toolCallId ?? ''), 'tool')
      part.meta = { ...(part.meta ?? {}), output: event.output }
      part.status = 'succeeded'
      // 编译联动：交接入口文件，由写作页按用户当前所选引擎创建浏览器任务。
      const toolName = (part.meta as { toolName?: string }).toolName
      const match = typeof event.output === 'string' ? event.output.match(BROWSER_COMPILE_REQUEST_RE) : null
      if (toolName === 'compile_project' && match) {
        try {
          const request: unknown = JSON.parse(match[1])
          if (
            typeof request === 'object'
            && request !== null
            && 'root_file' in request
            && typeof request.root_file === 'string'
            && request.root_file.length > 0
          ) {
            next.pendingBrowserCompileRequest = { rootFile: request.root_file }
          }
        } catch {
          // Keep malformed tool output visible without starting an unintended compile.
        }
      }
      return next
    }
    case 'tool-output-denied': {
      const part = findOrCreatePart(ensureAssistant(), String(event.toolCallId ?? ''), 'tool')
      part.status = 'denied'
      return next
    }
    case 'data-activity': {
      const data = (event.data ?? {}) as Record<string, unknown>
      const part = findOrCreatePart(ensureAssistant(), `activity-${String(data.runId ?? '')}`, 'activity')
      part.meta = data
      part.status = String(data.status ?? 'running')
      return next
    }
    case 'data-proposal': {
      const data = (event.data ?? {}) as unknown as AgentChangeProposal
      if (!data?.id) return next
      next.proposals = { ...next.proposals, [data.id]: data }
      const part = findOrCreatePart(ensureAssistant(), `proposal-${data.id}`, 'proposal')
      part.meta = { proposalId: data.id }
      part.status = data.status
      return next
    }
    case 'error': {
      next.error = String(event.errorText ?? '模型调用失败')
      const part = findOrCreatePart(ensureAssistant(), `error-${next.messages.length}`, 'error')
      part.text = next.error
      part.status = 'failed'
      return next
    }
    case 'abort': {
      const msg = next.messages[next.messages.length - 1]
      if (msg?.role === 'assistant' && next.activeRunId) msg.runId = next.activeRunId
      next.streaming = false
      next.activeRunId = null
      return next
    }
    case 'finish': {
      const msg = next.messages[next.messages.length - 1]
      if (msg) {
        msg.status = 'completed'
        if (msg.role === 'assistant' && next.activeRunId) msg.runId = next.activeRunId
      }
      next.streaming = false
      next.activeRunId = null
      return next
    }
    default:
      return next
  }
}

/** 把后端历史消息（已是 parts 形状）转成展示模型。 */
export function messagesFromApi(rows: AgentMessage[]): ChatMessage[] {
  return rows.map((m) => ({
    id: m.id,
    runId: m.runId,
    role: m.role,
    status: 'completed',
    parts: m.parts.map((p) => ({
      id: p.id,
      type: (p.type === 'tool' || p.type === 'activity' || p.type === 'error' ? p.type : 'text') as ChatPartType,
      text: p.text ?? '',
      status: p.status,
      meta: (p.content ?? undefined) as Record<string, unknown> | undefined,
    })),
  }))
}

type ChatStore = ChatState & {
  open: (conversationId: string) => Promise<void>
  send: (projectId: string, content: string) => Promise<void>
  cancel: () => Promise<void>
  retire: () => void
  decideProposal: (id: string, approve: boolean) => Promise<void>
  consumeBrowserCompileRequest: () => { rootFile: string } | null
  reset: () => void
}

export const useChatStore = create<ChatStore>((set, get) => ({
  ...emptyChatState,

  open: async (conversationId) => {
    set({ ...emptyChatState, conversationId })
    const rows = await api.listMessages(conversationId)
    set({ messages: messagesFromApi(rows) })
  },

  send: async (projectId, content) => {
    // 先本地回显用户消息，避免等待网络
    set((s) => ({
      ...s,
      error: null,
      messages: [...s.messages, { id: `user-${Date.now()}`, role: 'user', status: 'completed', parts: [{ id: 'text', type: 'text', text: content, status: 'completed' }] }],
    }))
    // 新项目还没有会话：第一条消息先建会话。Run 路径要求会话已存在，
    // 不建的话这一句会**静默消失**（输入被清空、什么也没发生）。
    let conversationId = get().conversationId
    if (!conversationId) {
      const conv = await api.createConversation(projectId, content)
      set({ conversationId: conv.id, conversationVersion: conv.version })
      conversationId = conv.id
    }
    const handle = await api.createRun(conversationId, {
      content,
      clientRequestId: crypto.randomUUID(),
      expectedThreadVersion: get().conversationVersion,
    })
    set({ activeRunId: handle.runId, conversationVersion: handle.threadVersion })
    await api.streamRun(handle.runId, {
      onEvent: (ev) => set((s) => reduceChatEvent(s, ev)),
    })
  },

  cancel: async () => {
    const runId = get().activeRunId
    if (!runId) return
    await api.cancelRun(runId)
  },

  retire: () => set({ streaming: false, activeRunId: null }),

  decideProposal: async (id, approve) => {
    const res = approve ? await api.applyProposal(id) : await api.rejectProposal(id)
    set((s) => {
      const existing = s.proposals[id]
      if (!existing) return s
      return { ...s, proposals: { ...s.proposals, [id]: { ...existing, status: res.status as AgentChangeProposal['status'] } } }
    })
  },

  // 取走并清空：浏览器编译请求是一次性交接，避免重放时重复编译。
  consumeBrowserCompileRequest: () => {
    const request = get().pendingBrowserCompileRequest
    if (request) set({ pendingBrowserCompileRequest: null })
    return request
  },

  reset: () => set({ ...emptyChatState }),
}))
