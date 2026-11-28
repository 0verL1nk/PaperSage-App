import { getJson, postJson, deleteFetch, type ApiRequestConfig } from './client'
import { getAccessToken, useSessionStore } from '@/store'
import { AGENT } from './api-routes'
import { getRequiredApiBaseUrl } from '@/lib/env'
import { GuestCapabilityError } from './guest-local-workspace'

function requireSignedInForAI() {
  if (useSessionStore.getState().isGuest) throw new GuestCapabilityError('AI 助手')
}

// --- Types（与后端 internal/agent 的 DTO 对应）---

export interface AgentConversation {
  id: string
  title: string
}

/** 协议 part：词汇对齐 AI SDK UI Message Stream（见 server/internal/agent/protocol.go）。 */
export type AgentPartType =
  | 'text'
  | 'tool'
  | 'activity'
  | 'proposal'
  | 'approval'
  | 'citation'
  | 'artifact'
  | 'error'

export interface AgentPart {
  id: string
  ordinal: number
  type: AgentPartType
  status: string
  text?: string
  /** 结构化内容（工具入参出参、建议详情等）；后端直接内联 JSON 对象，无需二次 parse */
  content?: unknown
  createdAt: string
  completedAt?: string
}

export interface AgentMessage {
  id: string
  role: 'user' | 'assistant'
  status: string
  runId?: string
  createdAt: string
  parts: AgentPart[]
}

export interface AgentRunEvidence {
  id: string
  sourceName: string
  quote: string
}

/** 服务端按本次注入集合解析出的规范条款；模型只能给引用号，内容由服务端补齐。 */
export interface ProposalNormRef {
  cite: string
  version?: number
  title: string
  rule: string
  source: string
}

/** 三源依据：evidence（材料原文）/norm（已采用规范）/plan（已采用章节计划缺口）。 */
export interface ProposalBasis {
  type: 'evidence' | 'norm' | 'plan'
  stance: 'supports' | 'uncertain'
  note?: string
  evidenceId?: string
  sourceName?: string
  sourceVersion?: string
  locator?: string
  quote?: string
  cite?: string
  normVersion?: number
  normTitle?: string
  normRule?: string
  normSource?: string
  sectionId?: string
  sectionTitle?: string
  gap?: string
  planArtifactId?: string
  planRevision?: number
}

export type AgentSuggestionKind =
  | 'evidence_gap'
  | 'logic_gap'
  | 'norm'
  | 'citation'
  | 'language'
  | 'structure'

export type AgentProposalStatus = 'pending' | 'applied' | 'rejected' | 'stale' | 'reverted' | 'dropped'

export interface AgentChangeProposal {
  id: string
  filePath: string
  oldText: string
  newText: string
  reason?: string
  normRefs?: ProposalNormRef[]
  basis?: ProposalBasis[]
  suggestionKind?: AgentSuggestionKind
  sectionPath?: string
  origin?: 'chat' | 'auto' | 'manual'
  confidence?: 'high' | 'medium' | 'low' | ''
  confidenceReason?: string
  rejectReason?: string
  rejectNote?: string
  createdAt?: string
  status: AgentProposalStatus
}

export interface AgentRunHandle {
  turnId: string
  runId: string
  userMessageId: string
  streamUrl: string
  threadVersion: number
}

// --- 会话 ---

export function listConversations(projectId: string, config?: ApiRequestConfig) {
  requireSignedInForAI()
  return getJson<AgentConversation[]>(AGENT.conversations(projectId), config)
}

export interface AgentConversationHandle {
  id: string
  title: string
  version: number
}

export interface AgentConversationDetail {
  id: string
  title: string
  threadVersion: number
}

/** 新项目还没有会话时，第一条消息先建会话（Run 路径要求会话已存在）。 */
export function createConversation(projectId: string, title?: string, config?: ApiRequestConfig) {
  requireSignedInForAI()
  return postJson<AgentConversationHandle>(AGENT.conversations(projectId), { title: title ?? '' }, config)
}

/** 会话详情：打开已有会话时拿 threadVersion，保证每次提交都能参与 CAS。 */
export function getConversation(conversationId: string, config?: ApiRequestConfig) {
  requireSignedInForAI()
  return getJson<AgentConversationDetail>(AGENT.conversation(conversationId), config)
}

export function listMessages(conversationId: string, config?: ApiRequestConfig) {
  requireSignedInForAI()
  return getJson<AgentMessage[]>(AGENT.messages(conversationId), config)
}

/** 刷新/重进会话时找回仍在执行的 Run；没有则 runId=null。 */
export function getActiveRun(conversationId: string, config?: ApiRequestConfig) {
  requireSignedInForAI()
  return getJson<{ runId: string | null; status?: string; lastSequence?: number }>(AGENT.activeRun(conversationId), config)
}

export function deleteConversation(conversationId: string, config?: ApiRequestConfig) {
  requireSignedInForAI()
  return deleteFetch(AGENT.conversation(conversationId), config)
}

// --- Run ---

export interface CreateRunInput {
  content: string
  /** 幂等键：双击发送/网络重试不会产生第二个 Run */
  clientRequestId?: string
  /** 乐观锁：与当前会话版本不符时后端返回 409 并附最新版本 */
  expectedThreadVersion?: number
}

export function createRun(conversationId: string, input: CreateRunInput, config?: ApiRequestConfig) {
  requireSignedInForAI()
  return postJson<AgentRunHandle>(AGENT.createRun(conversationId), input, config)
}

export function cancelRun(runId: string, config?: ApiRequestConfig) {
  requireSignedInForAI()
  return postJson<{ cancelled: boolean; status?: string }>(AGENT.cancelRun(runId), {}, config)
}

export function retryRun(runId: string, clientRequestId?: string, config?: ApiRequestConfig) {
  requireSignedInForAI()
  return postJson<{ runId: string; streamUrl: string }>(AGENT.retryRun(runId), { clientRequestId }, config)
}

export function listRunEvidence(runId: string, config?: ApiRequestConfig) {
  requireSignedInForAI()
  return getJson<AgentRunEvidence[]>(AGENT.runEvidence(runId), config)
}

export interface RunStreamEvent {
  type: string
  [key: string]: unknown
}

export interface RunStreamMeta {
  /** SSE 标准 id: 行携带的 agent_events.sequence，用于 afterSeq 断点续传。 */
  eventId?: number
}

/**
 * 订阅 Run 的事件流。
 *
 * 用 fetch + ReadableStream 而非 EventSource：EventSource 不支持 POST 与自定义头
 * （鉴权要靠 Authorization），也拿不到我们需要的中止控制。协议见后端 protocol.go：
 * SSE 的 data: 行承载 JSON part，末尾是终止标记 [DONE]（不是 JSON，必须跳过）。
 * id: 行是持久化事件序号，只用于重连游标，不进入 part 载荷。
 */
export async function streamRun(
  runId: string,
  opts: {
    afterSeq?: number
    signal?: AbortSignal
    onEvent: (event: RunStreamEvent, meta: RunStreamMeta) => void
  },
): Promise<void> {
  requireSignedInForAI()
  const baseUrl = getRequiredApiBaseUrl().replace(/\/$/, '')
  const res = await fetch(`${baseUrl}${AGENT.runEvents(runId, opts.afterSeq)}`, {
    headers: {
      Accept: 'text/event-stream',
      Authorization: `Bearer ${getAccessToken() ?? ''}`,
    },
    signal: opts.signal,
  })
  if (!res.ok || !res.body) {
    throw new Error(`run stream failed: ${res.status}`)
  }
  if (res.headers.get('Content-Type')?.split(';')[0].trim() !== 'text/event-stream') {
    throw new Error('AI 接口未返回事件流，请检查后端环境配置')
  }
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      // 完整的 SSE 事件块才交给消费者，支持跨网络分片的 LF / CRLF。
      let separator: RegExpExecArray | null
      while ((separator = /\r?\n\r?\n/.exec(buffer))) {
        const chunk = buffer.slice(0, separator.index)
        buffer = buffer.slice(separator.index + separator[0].length)
        const lines = chunk.split(/\r?\n/)
        let eventId: number | undefined
        const data = lines
          .filter((line) => line === 'data' || line.startsWith('data:'))
          .map((line) => line === 'data' ? '' : line.slice(5).replace(/^ /, ''))
        for (const line of lines) {
          if (!line.startsWith('id:')) continue
          const parsed = Number(line.slice(3).trim())
          if (Number.isFinite(parsed)) eventId = parsed
        }
        if (!data.length) continue
        const payload = data.join('\n')
        if (payload === '[DONE]') return
        let event: RunStreamEvent
        try {
          event = JSON.parse(payload) as RunStreamEvent
        } catch {
          throw new Error('AI 事件流格式错误，回答可能未完成；后台可能仍在运行，请稍后从对话列表重新打开查看，勿重复发送')
        }
        opts.onEvent(event, { eventId })
        if (event.type === 'finish' || event.type === 'abort') return
      }
    }
    throw new Error('AI 连接中断，回答尚未接收完整；后台可能仍在运行，请稍后从对话列表重新打开查看，勿重复发送')
  } finally {
    // 只关闭本次订阅，不调用后台 Run 的取消接口。
    void reader.cancel().catch(() => {})
    reader.releaseLock()
  }
}

// --- 建议 ---

export function listProposals(projectId: string, status?: string, config?: ApiRequestConfig) {
  requireSignedInForAI()
  return getJson<AgentChangeProposal[]>(AGENT.proposals(projectId, status), config)
}

export function applyProposal(id: string, config?: ApiRequestConfig) {
  requireSignedInForAI()
  return postJson<{ status: string }>(AGENT.applyProposal(id), {}, config)
}

/** 忽略原因可选（用于持续评测）；空串表示未选原因。 */
export function rejectProposal(
  id: string,
  reasonCode?: string,
  note?: string,
  config?: ApiRequestConfig,
) {
  requireSignedInForAI()
  return postJson<{ status: string }>(AGENT.rejectProposal(id), { reasonCode: reasonCode ?? '', note: note ?? '' }, config)
}

export function undoProposal(id: string, config?: ApiRequestConfig) {
  requireSignedInForAI()
  return postJson<{ status: string }>(AGENT.undoProposal(id), {}, config)
}

// --- 动态写作建议 ---

export type SuggestionTrigger = 'auto' | 'manual'
export type SuggestionSkipReason = 'pending' | 'duplicate' | 'running' | 'rate_limited' | 'too_short'

export interface SuggestionRunHandle {
  runId: string
  conversationId: string
  streamUrl: string
  trigger: SuggestionTrigger
}

export interface CreateSuggestionInput {
  filePath: string
  paragraphText: string
  sectionPath: string
  trigger: SuggestionTrigger
  clientRequestId: string
}

export function createSuggestion(
  projectId: string,
  input: CreateSuggestionInput,
  config?: ApiRequestConfig,
) {
  requireSignedInForAI()
  return postJson<SuggestionRunHandle | { skipped: SuggestionSkipReason }>(
    AGENT.suggestions(projectId),
    input,
    config,
  )
}

export function artifactUrl(id: string) {
  return AGENT.artifact(id)
}
