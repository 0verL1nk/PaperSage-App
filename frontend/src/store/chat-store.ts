import { create } from 'zustand'
import * as api from '@/services/agent-api'
import type { AgentChangeProposal, AgentMessage, RunStreamEvent } from '@/services/agent-api'
import {
  agentErrorAction,
  describeAgentError,
  isThreadConflict,
  threadVersionFromError,
  type AgentErrorAction,
} from '@/services/agent-errors'
import { FrontendApiError } from '@/services/errors'
import { notifyWhenBackground } from '@/lib/desktop'

// 前端事件归并：Normalized Store + Event Reducer（参考架构 §5）。
//
// 为什么不直接拼字符串：一条 Assistant 消息里同时有文本、工具卡片、子 agent 活动、
// 建议卡片，只有把它们建模成有 id 的 part，才能做到按 id 幂等归并、重放不出错。
//
// 重放语义：服务端事件带 agent_events.sequence（SSE id:），重连用 afterSeq 只补未收到
// 的事件；`start` 仍然重置该消息的 parts，兼容从头重放。

export type ChatPartType = 'text' | 'tool' | 'activity' | 'proposal' | 'error' | 'approval' | 'citation' | 'artifact'

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
  status: 'streaming' | 'completed' | 'failed' | 'cancelled'
  parts: ChatPart[]
}

export interface ChatState {
  conversationId: string | null
  conversationVersion: number
  /** 打开已有会话但详情还没拿到时保持 false；发送前会补拉，不能把 0 当并发版本。 */
  versionKnown: boolean
  messages: ChatMessage[]
  activeRunId: string | null
  streaming: boolean
  cancelling: boolean
  reconnecting: boolean
  lastEventId: number
  error: string | null
  errorAction: AgentErrorAction
  /** 发送失败时保留原文与幂等键，供“重试发送”复用，避免生成第二个 Run。 */
  lastFailedContent: string | null
  lastClientRequestId: string | null
  /** 用户点了“新对话”：禁止面板自动重开第一条历史会话。 */
  isNewConversation: boolean
  proposals: Record<string, AgentChangeProposal>
  /** 助手触发了浏览器编译：把入口文件交给当前写作页启动 WASM。 */
  pendingBrowserCompileRequest: { rootFile: string } | null
}

export const emptyChatState: ChatState = {
  conversationId: null,
  conversationVersion: 0,
  versionKnown: false,
  messages: [],
  activeRunId: null,
  streaming: false,
  cancelling: false,
  reconnecting: false,
  lastEventId: 0,
  error: null,
  errorAction: null,
  lastFailedContent: null,
  lastClientRequestId: null,
  isNewConversation: false,
  proposals: {},
  pendingBrowserCompileRequest: null,
}

/** Agent tool emits a machine-readable browser request; the page creates its own browser job. */
const BROWSER_COMPILE_REQUEST_RE = /browser_compile_request:(\{[^\r\n]+\})/

// 网络断流的重连预算。超过后交给用户显式重试，避免页面无限挂着“重连中”。
const RECONNECT_DELAYS_MS = [300, 800, 1500]

let sendInFlight = false
let openSeq = 0
let currentStream: AbortController | null = null

function delay(ms: number) {
  return new Promise<void>((resolve) => setTimeout(resolve, ms))
}

function isRetryableStreamError(error: unknown): boolean {
  if (error instanceof DOMException && error.name === 'AbortError') return false
  if (error instanceof TypeError) return true
  const message = error instanceof Error ? error.message : ''
  if (/run stream failed: 5\d\d/.test(message)) return true
  return /中断|未完成/.test(message)
}

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
      // 重置：重连从头重放时，同一消息的 parts 从头再来而不是叠加
      const id = String(event.messageId ?? '')
      const existing = next.messages.find((m) => m.id === id)
      if (existing) {
        existing.parts = []
        existing.status = 'streaming'
        existing.runId = next.activeRunId ?? existing.runId
        // 把它挪到末尾，作为当前流式消息
        next.messages = [...next.messages.filter((m) => m.id !== id), existing]
      } else {
        next.messages = [...next.messages, { id, role: 'assistant', status: 'streaming', runId: next.activeRunId ?? undefined, parts: [] }]
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
    case 'tool-output-error': {
      const part = findOrCreatePart(ensureAssistant(), String(event.toolCallId ?? ''), 'tool')
      part.meta = { ...(part.meta ?? {}), output: event.errorText }
      part.status = 'failed'
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
      part.meta = { proposalId: data.id, proposal: data }
      part.status = data.status
      return next
    }
    case 'error': {
      next.error = String(event.errorText ?? '模型调用失败')
      const msg = ensureAssistant()
      msg.status = 'failed'
      const part = findOrCreatePart(msg, `error-${next.messages.length}`, 'error')
      part.text = next.error
      part.status = 'failed'
      return next
    }
    case 'abort': {
      const msg = next.messages[next.messages.length - 1]
      if (msg?.role === 'assistant') {
        msg.status = 'cancelled'
        if (next.activeRunId) msg.runId = next.activeRunId
      }
      next.streaming = false
      next.activeRunId = null
      return next
    }
    case 'finish': {
      const msg = next.messages[next.messages.length - 1]
      if (msg && msg.status !== 'failed' && msg.status !== 'cancelled') {
        msg.status = 'completed'
      }
      if (msg?.role === 'assistant' && next.activeRunId) msg.runId = next.activeRunId
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
    status: (m.status as ChatMessage['status']) || 'completed',
    parts: m.parts.map((p) => {
      const content = (p.content ?? undefined) as Record<string, unknown> | undefined
      const proposalId = typeof content?.proposalId === 'string'
        ? content.proposalId
        : typeof content?.id === 'string' ? content.id : undefined
      return {
        id: p.id,
        type: p.type as ChatPartType,
        text: p.text ?? '',
        status: p.status,
        meta: proposalId ? { ...content, proposalId } : content,
      }
    }),
  }))
}

type ChatStore = ChatState & {
  open: (conversationId: string, projectId?: string) => Promise<void>
  send: (projectId: string, content: string, clientRequestId?: string) => Promise<void>
  retrySend: (projectId: string) => Promise<void>
  retryRun: (runId: string) => Promise<void>
  cancel: () => Promise<void>
  retire: () => void
  decideProposal: (id: string, approve: boolean) => Promise<void>
  consumeBrowserCompileRequest: () => { rootFile: string } | null
  reset: () => void
}

export const useChatStore = create<ChatStore>((set, get) => {
  async function ensureVersion(conversationId: string): Promise<number> {
    if (get().versionKnown) return get().conversationVersion
    const detail = await api.getConversation(conversationId)
    set({ conversationVersion: detail.threadVersion, versionKnown: true })
    return detail.threadVersion
  }

  async function consumeStream(runId: string, afterSeq: number): Promise<void> {
    const controller = new AbortController()
    currentStream = controller
    let seq = afterSeq
    let terminal = false
    let terminalNotified = false
    let attempt = 0
    try {
      for (;;) {
        try {
          await api.streamRun(runId, {
            afterSeq: seq || undefined,
            signal: controller.signal,
            onEvent: (ev, meta) => {
              if (typeof meta.eventId === 'number') {
                seq = meta.eventId
                set({ lastEventId: seq })
              }
              if (ev.type === 'finish' || ev.type === 'abort') terminal = true
              if (!terminalNotified && get().activeRunId === runId) {
                if (ev.type === 'error') {
                  terminalNotified = true
                  void notifyWhenBackground('AI 回答失败', String(ev.errorText ?? '模型调用失败'))
                } else if (ev.type === 'finish') {
                  terminalNotified = true
                  void notifyWhenBackground('AI 回答完成', '回答已完成，可回到应用查看')
                }
              }
              set((s) => s.activeRunId === runId ? reduceChatEvent(s, ev) : s)
            },
          })
          // streamRun 只在终态帧或 [DONE] 时正常返回；提前关闭会在内部抛错。
          break
        } catch (error) {
          if (controller.signal.aborted || terminal) break
          if (!isRetryableStreamError(error) || attempt >= RECONNECT_DELAYS_MS.length) {
            if (!terminalNotified && !controller.signal.aborted) {
              void notifyWhenBackground('AI 连接中断', '回答可能未完成，可回到应用查看')
            }
            throw error
          }
          set({ reconnecting: true })
          await delay(RECONNECT_DELAYS_MS[attempt])
          attempt++
          if (controller.signal.aborted) break
        }
      }
      set((s) => {
        if (s.activeRunId !== runId) return {}
        // 只收到 [DONE] 没收到 finish 时，仍把最后一条流式消息收口。
        const messages = terminal
          ? s.messages
          : s.messages.map((m, i) => i === s.messages.length - 1 && m.role === 'assistant'
            ? { ...m, status: 'completed' as const, runId }
            : m)
        return { ...s, messages, streaming: false, reconnecting: false, cancelling: false, activeRunId: null }
      })
    } finally {
      if (currentStream === controller) currentStream = null
    }
  }

  return {
    ...emptyChatState,

    open: async (conversationId, projectId) => {
      const seq = ++openSeq
      currentStream?.abort()
      currentStream = null
      set({ ...emptyChatState, conversationId, isNewConversation: false })
      const [detail, rows, allProposals] = await Promise.all([
        api.getConversation(conversationId).catch(() => null),
        api.listMessages(conversationId),
        projectId ? api.listProposals(projectId).catch(() => []) : Promise.resolve([]),
      ])
      // 快速切换会话：迟到的响应不能覆盖当前会话。
      if (seq !== openSeq) return
      const proposalById = new Map(allProposals.map((p) => [p.id, p]))
      // 历史 part 里的建议状态是产出时快照；以建议表当前状态为准，应用/拒绝后刷新不倒退。
      const messages = messagesFromApi(rows).map((m) => ({
        ...m,
        parts: m.parts.map((part) => {
          const id = typeof part.meta?.proposalId === 'string' ? part.meta.proposalId : undefined
          const current = id ? proposalById.get(id) : undefined
          return current ? { ...part, status: current.status, meta: { ...part.meta, proposal: current } } : part
        }),
      }))
      const pending = allProposals.filter((p) => p.status === 'pending')
      // 兼容旧消息：历史 parts 里没有建议卡时，用未决建议补一条，刷新后仍可处理。
      for (const proposal of pending) {
        const hasPart = messages.some((m) => m.parts.some((part) => part.meta?.proposalId === proposal.id))
        if (hasPart) continue
        const target = [...messages].reverse().find((m) => m.role === 'assistant')
        if (!target) continue
        target.parts.push({
          id: `proposal-${proposal.id}`,
          type: 'proposal',
          text: '',
          status: proposal.status,
          meta: { proposalId: proposal.id, proposal },
        })
      }
      const proposals = Object.fromEntries(allProposals.map((p) => [p.id, p]))
      set({
        messages,
        proposals,
        conversationVersion: detail?.threadVersion ?? 0,
        versionKnown: detail !== null,
      })
    },

    send: async (projectId, content, clientRequestId = crypto.randomUUID()) => {
      if (sendInFlight || get().streaming || get().cancelling) return
      sendInFlight = true
      const localUserId = `user-${clientRequestId}`
      try {
        set((s) => ({
          ...s,
          error: null,
          errorAction: null,
          messages: s.messages.some((m) => m.id === localUserId)
            ? s.messages
            : [...s.messages, {
              id: localUserId,
              role: 'user' as const,
              status: 'completed' as const,
              parts: [{ id: 'text', type: 'text' as const, text: content, status: 'completed' }],
            }],
        }))
        // 新项目还没有会话：第一条消息先建会话。Run 路径要求会话已存在，
        // 不建的话这一句会**静默消失**（输入被清空、什么也没发生）。
        let conversationId = get().conversationId
        if (!conversationId) {
          const conv = await api.createConversation(projectId, content)
          set({ conversationId: conv.id, conversationVersion: conv.version, versionKnown: true, isNewConversation: false })
          conversationId = conv.id
        }
        const version = await ensureVersion(conversationId)
        let handle: api.AgentRunHandle
        try {
          handle = await api.createRun(conversationId, {
            content,
            clientRequestId,
            expectedThreadVersion: version,
          })
        } catch (error) {
          if (isThreadConflict(error)) {
            const latest = threadVersionFromError(error)
            set({
              conversationVersion: latest ?? get().conversationVersion,
              versionKnown: latest !== null,
              error: '会话已在其他位置更新，请基于最新版本重试',
              errorAction: 'retry-send',
              lastFailedContent: content,
              lastClientRequestId: clientRequestId,
            })
            return
          }
          throw error
        }
        set({
          activeRunId: handle.runId,
          conversationVersion: handle.threadVersion,
          versionKnown: true,
          streaming: true,
          cancelling: false,
          reconnecting: false,
          lastEventId: 0,
          lastFailedContent: null,
          lastClientRequestId: null,
        })
        await consumeStream(handle.runId, 0)
      } catch (error) {
        const message = describeAgentError(error)
        const runId = get().activeRunId
        set((s) => ({
          ...s,
          messages: runId
            ? s.messages.map((m, i) => i === s.messages.length - 1 && m.role === 'assistant' ? { ...m, runId } : m)
            : s.messages,
          streaming: false,
          cancelling: false,
          reconnecting: false,
          activeRunId: null,
          error: message,
          errorAction: agentErrorAction(error),
          lastFailedContent: content,
          lastClientRequestId: clientRequestId,
        }))
        throw new Error(message)
      } finally {
        sendInFlight = false
      }
    },

    retrySend: async (projectId) => {
      const content = get().lastFailedContent
      const clientRequestId = get().lastClientRequestId
      if (!content || !clientRequestId) return
      await get().send(projectId, content, clientRequestId)
    },

    retryRun: async (runId) => {
      if (get().streaming || get().cancelling) return
      set({ error: null, errorAction: null, cancelling: false })
      try {
        const clientRequestId = crypto.randomUUID()
        const handle = await api.retryRun(runId, clientRequestId)
        set({ activeRunId: handle.runId, streaming: true, reconnecting: false, lastEventId: 0 })
        await consumeStream(handle.runId, 0)
      } catch (error) {
        const message = describeAgentError(error)
        set({ streaming: false, activeRunId: null, error: message, errorAction: agentErrorAction(error) })
        throw new Error(message)
      }
    },

    cancel: async () => {
      const runId = get().activeRunId
      if (!runId || get().cancelling) return
      set({ cancelling: true, error: null, errorAction: null })
      try {
        const result = await api.cancelRun(runId, { skipGlobalLoading: true })
        // 服务端已确认终态时直接收口；仍在收尾则等 abort/finish 事件。
        if (result.status && result.status !== 'running') {
          set((s) => ({
            ...s,
            streaming: false,
            cancelling: false,
            reconnecting: false,
            activeRunId: null,
            messages: s.messages.map((m, i) => i === s.messages.length - 1 && m.role === 'assistant'
              ? { ...m, status: result.status === 'cancelled' ? 'cancelled' as const : result.status === 'failed' ? 'failed' as const : 'completed' as const, runId }
              : m),
          }))
        } else if (!get().streaming) {
          set({ cancelling: false })
        }
      } catch (error) {
        set({ cancelling: false, error: `取消失败：${describeAgentError(error)}`, errorAction: 'retry-run' })
      }
    },

    retire: () => {
      currentStream?.abort()
      currentStream = null
      set({ streaming: false, cancelling: false, reconnecting: false, activeRunId: null })
    },

    decideProposal: async (id, approve) => {
      try {
        const res = approve ? await api.applyProposal(id) : await api.rejectProposal(id)
        set((s) => {
          const existing = s.proposals[id]
          if (!existing) return s
          return { ...s, proposals: { ...s.proposals, [id]: { ...existing, status: res.status as AgentChangeProposal['status'] } } }
        })
      } catch (error) {
        if (error instanceof FrontendApiError && error.status === 409) {
          set((s) => {
            const existing = s.proposals[id]
            if (!existing) return s
            return { ...s, proposals: { ...s.proposals, [id]: { ...existing, status: 'stale' } } }
          })
        }
        throw error
      }
    },

    // 取走并清空：浏览器编译请求是一次性交接，避免重放时重复编译。
    consumeBrowserCompileRequest: () => {
      const request = get().pendingBrowserCompileRequest
      if (request) set({ pendingBrowserCompileRequest: null })
      return request
    },

    reset: () => {
      openSeq++
      currentStream?.abort()
      currentStream = null
      set({ ...emptyChatState, isNewConversation: true })
    },
  }
})
