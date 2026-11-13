import { getJson, postJson, putJson, requestApi } from './client'
import { SETTINGS } from './api-routes'
import { useSessionStore } from '@/store'
import { GuestCapabilityError } from './guest-local-workspace'

function requireAccountSettings() {
  if (useSessionStore.getState().isGuest) throw new GuestCapabilityError('AI 助手')
}

// --- Types ---

export type SettingsCategory = 'ui' | 'editor' | 'agent'

export interface UISettings {
  theme: 'system' | 'light' | 'dark'
  language: string
  sidebar_collapsed: boolean
}

export interface EditorSettings {
  font_size: number
  tab_size: number
  auto_save: boolean
  auto_save_interval: number
  word_wrap: boolean
}

export type ProviderType = 'openai_compatible' | 'anthropic'

export interface ModelMetadata {
  context_window?: number
  max_output_tokens?: number
  supports_vision?: boolean
  supports_tools?: boolean
}

export interface ProviderConfig {
  id: string
  name: string
  type: ProviderType
  base_url: string
  api_key: string
  models: string[]
  model_metadata?: Record<string, ModelMetadata>
  active_model: string
}

export interface AgentSettings {
  providers: ProviderConfig[]
  active_provider_id: string
  temperature: number
  writing_style: string
  memory_enabled: boolean
  server_fallback?: {
    configured: boolean
    model: string
  }
}

export interface AgentTestRequest {
  base_url: string
  api_key: string
  model: string
}

export interface AgentTestResult {
  ok: boolean
  models?: string[]
  model_metadata?: Record<string, ModelMetadata>
}

export interface ModelSelection {
  provider_id: string
  model: string
}

export type ModelSelectionPatch = { model_selection: ModelSelection }
export type CategorySettings = UISettings | EditorSettings | AgentSettings

export interface AllSettings {
  ui: UISettings
  editor: EditorSettings
  agent: AgentSettings
}

export interface CategoryResponse {
  category: string
  settings: Record<string, unknown>
}

// --- API functions ---

export async function getSettings(): Promise<AllSettings> {
  requireAccountSettings()
  const res = await getJson<{ settings: AllSettings }>(SETTINGS.all)
  return res.settings
}

export async function getCategorySettings<T extends CategorySettings>(
  category: SettingsCategory,
): Promise<T> {
  requireAccountSettings()
  const res = await getJson<CategoryResponse>(SETTINGS.category(category))
  return res.settings as T
}

export function updateCategory(category: 'agent', settings: ModelSelectionPatch): Promise<CategoryResponse>
export function updateCategory(category: SettingsCategory, settings: Partial<CategorySettings>): Promise<CategoryResponse>
export async function updateCategory(
  category: SettingsCategory,
  settings: Partial<CategorySettings> | ModelSelectionPatch,
): Promise<CategoryResponse> {
  requireAccountSettings()
  return requestApi<CategoryResponse, { settings: Partial<CategorySettings> | ModelSelectionPatch }>({
    method: 'patch',
    skipGlobalLoading: true,
    url: SETTINGS.category(category),
    data: { settings },
  })
}

export async function replaceCategory(
  category: SettingsCategory,
  settings: CategorySettings,
): Promise<CategoryResponse> {
  requireAccountSettings()
  return putJson<CategoryResponse, { settings: CategorySettings }>(
    SETTINGS.category(category),
    { settings },
  )
}

export async function resetCategory(
  category: SettingsCategory,
): Promise<CategoryResponse> {
  requireAccountSettings()
  return requestApi<CategoryResponse>({
    method: 'delete',
    url: SETTINGS.category(category),
  })
}

export async function testAgentConnection(
  payload: AgentTestRequest,
): Promise<AgentTestResult> {
  requireAccountSettings()
  return postJson<AgentTestResult, AgentTestRequest>(SETTINGS.agentTest, payload, {
    skipGlobalLoading: true,
  })
}
