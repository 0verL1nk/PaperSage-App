import { getJson, putJson, requestApi } from './client'
import { SETTINGS } from './api-routes'

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

export interface AgentSettings {
  model_name: string
  temperature: number
  writing_style: string
  memory_enabled: boolean
}

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
  const res = await getJson<{ settings: AllSettings }>(SETTINGS.all)
  return res.settings
}

export async function getCategorySettings<T extends CategorySettings>(
  category: SettingsCategory,
): Promise<T> {
  const res = await getJson<CategoryResponse>(SETTINGS.category(category))
  return res.settings as T
}

export async function updateCategory(
  category: SettingsCategory,
  settings: Partial<CategorySettings>,
): Promise<CategoryResponse> {
  return requestApi<CategoryResponse, { settings: Partial<CategorySettings> }>({
    method: 'patch',
    url: SETTINGS.category(category),
    data: { settings },
  })
}

export async function replaceCategory(
  category: SettingsCategory,
  settings: CategorySettings,
): Promise<CategoryResponse> {
  return putJson<CategoryResponse, { settings: CategorySettings }>(
    SETTINGS.category(category),
    { settings },
  )
}

export async function resetCategory(
  category: SettingsCategory,
): Promise<CategoryResponse> {
  return requestApi<CategoryResponse>({
    method: 'delete',
    url: SETTINGS.category(category),
  })
}
