import { create } from 'zustand'
import {
  type AllSettings,
  type SettingsCategory,
  type CategorySettings,
  getSettings,
  updateCategory,
  resetCategory,
} from '@/services/settings-api'

type SettingsState = {
  settings: AllSettings | null
  loading: boolean
  error: string | null

  fetchSettings: () => Promise<AllSettings>
  patchCategory: (
    category: SettingsCategory,
    partial: Partial<CategorySettings>,
  ) => Promise<void>
  resetCategory: (category: SettingsCategory) => Promise<void>
  clear: () => void
}

const DEFAULT_ALL_SETTINGS: AllSettings = {
  ui: { theme: 'system', language: 'zh-CN', sidebar_collapsed: false },
  editor: { font_size: 14, tab_size: 2, auto_save: true, auto_save_interval: 30, word_wrap: true },
  agent: { model_name: 'deepseek', temperature: 0.7, writing_style: 'academic', memory_enabled: true },
}

export const useSettingsStore = create<SettingsState>((set, get) => ({
  settings: null,
  loading: false,
  error: null,

  fetchSettings: async () => {
    set({ loading: true, error: null })
    try {
      const data = await getSettings()
      set({ settings: data, loading: false })
      return data
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to load settings'
      set({ error: msg, loading: false })
      throw err
    }
  },

  patchCategory: async (category, partial) => {
    const current = get().settings
    // Optimistic update
    if (current) {
      set({
        settings: {
          ...current,
          [category]: { ...current[category as keyof AllSettings], ...partial },
        },
      })
    }
    try {
      const res = await updateCategory(category, partial)
      // Reconcile with server response
      const prev = get().settings ?? DEFAULT_ALL_SETTINGS
      set({
        settings: {
          ...prev,
          [category]: res.settings as unknown as AllSettings[typeof category],
        },
      })
    } catch {
      // Rollback on error
      if (current) set({ settings: current })
      throw new Error(`Failed to update ${category} settings`)
    }
  },

  resetCategory: async (category) => {
    try {
      await resetCategory(category)
      // Re-fetch to get merged defaults
      await get().fetchSettings()
    } catch {
      throw new Error(`Failed to reset ${category} settings`)
    }
  },

  clear: () => set({ settings: null, loading: false, error: null }),
}))
