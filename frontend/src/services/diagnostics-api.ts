import { postJson } from './client'
import { DIAGNOSTICS } from './api-routes'

export type DiagnosticFailureKind = 'resource' | 'timeout' | 'service' | 'latex' | 'unknown'
export type DiagnosticFailureStage = 'prepare' | 'job_create' | 'compile' | 'upload'

export interface DiagnosticResource {
  kind: string
  name: string
  state: string
  status?: number
  error?: string
}

export interface DiagnosticClientEnv {
  user_agent: string
  platform: string
  language: string
  hardware_concurrency: number
  device_memory_gb: number
  online: boolean
}

export interface CompileDiagnosticPayload {
  compile_job_id?: string
  project_id?: string
  client_kind: 'web' | 'desktop'
  engine: string
  engine_version?: string
  resource_profile?: string
  failure_kind: DiagnosticFailureKind
  failure_stage: DiagnosticFailureStage
  duration_ms: number
  http_status?: number | null
  error_message: string
  log_excerpt: string
  failed_resources: DiagnosticResource[]
  client_env: DiagnosticClientEnv
}

/** 异步上报编译失败诊断；调用方不 await，失败静默降级。 */
export function reportCompileDiagnostic(payload: CompileDiagnosticPayload) {
  return postJson<{ id: string }, CompileDiagnosticPayload>(DIAGNOSTICS.report, payload, {
    skipGlobalLoading: true,
    skipAuthRefresh: true,
    timeout: 10_000,
  })
}
