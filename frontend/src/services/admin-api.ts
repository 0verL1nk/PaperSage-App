import { getJson, PAGE_QUERY_TIMEOUT_MS, putJson } from './client'

export interface SystemSettings {
  allowed_email_domains: string[]
}

export function getSystemSettings() {
  return getJson<SystemSettings>('/api/v1/admin/settings')
}

export function putSystemSettings(kv: Partial<SystemSettings>) {
  return putJson<SystemSettings, Partial<SystemSettings>>('/api/v1/admin/settings', kv)
}

export type DiagnosticFailureKind = 'resource' | 'timeout' | 'service' | 'latex' | 'unknown'

export interface CompileDiagnosticResource {
  kind: string
  name: string
  state: string
  status?: number
  error?: string
}

export interface CompileDiagnosticItem {
  id: string
  created_at: string
  user_id: string | null
  username: string | null
  project_id: string | null
  project_name: string | null
  is_guest: boolean
  client_kind: string
  engine: string
  engine_version: string
  resource_profile: string
  failure_kind: DiagnosticFailureKind
  failure_stage: string
  duration_ms: number
  http_status: number | null
  error_message: string
  failed_resources: CompileDiagnosticResource[] | null
}

export interface CompileDiagnosticDetail extends CompileDiagnosticItem {
  compile_job_id: string | null
  log_excerpt: string | null
  client_env: {
    user_agent?: string
    platform?: string
    language?: string
    hardware_concurrency?: number
    device_memory_gb?: number
    online?: boolean
  } | null
}

export interface CompileDiagnosticPage {
  items: CompileDiagnosticItem[]
  total: number
  page: number
  page_size: number
}

export interface CompileDiagnosticQuery {
  page?: number
  page_size?: number
  failure_kind?: string
  engine?: string
  is_guest?: boolean
  user_id?: string
  project_id?: string
  from?: string
  to?: string
}

export function listCompileDiagnostics(query: CompileDiagnosticQuery) {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === '') continue
    params.set(key, String(value))
  }
  const suffix = params.size > 0 ? `?${params.toString()}` : ''
  // 页面自己渲染错误态（含 403），不再叠加全局 toast。
  return getJson<CompileDiagnosticPage>(`/api/v1/admin/compile-diagnostics${suffix}`, {
    skipGlobalLoading: true,
    timeout: PAGE_QUERY_TIMEOUT_MS,
  })
}

export function getCompileDiagnostic(id: string) {
  return getJson<CompileDiagnosticDetail>(`/api/v1/admin/compile-diagnostics/${id}`, {
    skipGlobalLoading: true,
    timeout: PAGE_QUERY_TIMEOUT_MS,
  })
}
