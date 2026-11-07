import { FrontendApiError } from './errors'

export type AgentErrorAction = 'settings' | 'retry-send' | 'retry-run' | 'network' | null

/** 把后端/网络错误收敛成用户能执行下一步的文案。 */
export function describeAgentError(error: unknown): string {
  if (error instanceof DOMException && error.name === 'AbortError') return '已停止接收回答'
  if (error instanceof FrontendApiError) {
    if (error.status === 401 || error.status === 403) return '登录状态已失效或无权访问，请重新登录'
    if (error.status === 429) return '模型服务限流，请稍后重试'
    if (error.status === 408 || error.status === 504) return '模型响应超时，请稍后重试或缩短问题'
    if (error.status === 413) return '上下文超出模型限制，请缩短问题或分多次提问'
    if (error.info && /timeout|deadline/i.test(error.info)) return '模型响应超时，请稍后重试或缩短问题'
    if (error.info && /context|token/i.test(error.info)) return '上下文超出模型限制，请缩短问题或分多次提问'
    if (error.status === 400 && /尚未配置|模型|provider/i.test(error.message)) return '尚未配置可用的 AI 模型，请先到设置中配置'
    return error.message
  }
  if (error instanceof TypeError) return '网络连接中断，回答可能未完成；后台可能仍在运行，请稍后从会话列表重新打开查看，勿重复发送'
  if (error instanceof Error) return error.message
  return 'AI 服务暂时不可用，请稍后重试'
}

export function agentErrorAction(error: unknown): AgentErrorAction {
  if (error instanceof FrontendApiError) {
    if (error.status === 401 || error.status === 403) return 'settings'
    if (error.status === 400 && /尚未配置|模型|provider/i.test(error.message)) return 'settings'
    if (error.status === 429 || error.status === 408 || error.status === 504 || error.status === 413) return 'network'
  }
  if (error instanceof TypeError) return 'network'
  return null
}

/** 409 响应里带最新会话版本；只有这种情况才允许“基于最新版本重试”。 */
export function threadVersionFromError(error: unknown): number | null {
  if (!(error instanceof FrontendApiError)) return null
  const details = error.details as { data?: { threadVersion?: unknown }; threadVersion?: unknown } | undefined
  const raw = details?.data?.threadVersion ?? details?.threadVersion
  return typeof raw === 'number' && Number.isFinite(raw) ? raw : null
}

export function isThreadConflict(error: unknown): boolean {
  return error instanceof FrontendApiError && error.status === 409 && threadVersionFromError(error) !== null
}
