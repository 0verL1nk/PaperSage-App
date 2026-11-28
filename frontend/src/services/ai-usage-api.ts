import { AI_USAGE } from './api-routes'
import { downloadBlob, getJson, type ApiRequestConfig } from './client'

/** 一个分组的用量聚合；Token 与分位数为 null 表示没有可统计样本。 */
export interface AiUsageBucket {
  key?: string
  calls: number
  success: number
  error: number
  timeout: number
  cancelled: number
  success_rate: number
  prompt_tokens: number
  completion_tokens: number
  usage_missing_calls: number
  latency_p50_ms: number | null
  latency_p95_ms: number | null
  first_token_p50_ms: number | null
}

export interface AiUsageReport {
  from: string
  to: string
  timezone: string
  totals: AiUsageBucket
  by_day: AiUsageBucket[]
  by_model: AiUsageBucket[]
  by_feature: AiUsageBucket[]
  by_error: AiUsageBucket[]
}

export interface AiUsageQuery {
  from?: string
  to?: string
  model?: string
  feature_kind?: string
}

function toQueryString(query: AiUsageQuery): string {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value) params.set(key, value)
  }
  const encoded = params.toString()
  return encoded ? `?${encoded}` : ''
}

/** 个人用量：服务端强制按当前登录用户过滤，前端不传任何用户参数。 */
export function getMyAiUsage(query: AiUsageQuery = {}, config?: ApiRequestConfig) {
  return getJson<AiUsageReport>(`${AI_USAGE.personal}${toQueryString(query)}`, config)
}

/** 管理端汇总：仅 superuser，接口 403 兜底。 */
export function getAdminAiUsage(query: AiUsageQuery = {}, config?: ApiRequestConfig) {
  return getJson<AiUsageReport>(`${AI_USAGE.admin}${toQueryString(query)}`, config)
}

/** 导出聚合报表（日 × 服务商 × 模型 × 功能），不含个人维度。 */
export function downloadAdminAiUsage(format: 'csv' | 'json', query: AiUsageQuery = {}) {
  const params = new URLSearchParams({ format })
  for (const [key, value] of Object.entries(query)) {
    if (value) params.set(key, value)
  }
  return downloadBlob(`${AI_USAGE.export}?${params.toString()}`)
}
