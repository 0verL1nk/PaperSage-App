import { create } from 'zustand'
import { z } from 'zod'
import {
  type AllSettings,
  type SettingsCategory,
  type CategorySettings,
  type ModelSelectionPatch,
  getSettings,
  updateCategory,
  resetCategory,
} from '@/services/settings-api'

type SettingsState = {
  settings: AllSettings | null
  loading: boolean
  error: string | null
  pendingAgentModelChanges: number

  fetchSettings: () => Promise<AllSettings>
  patchCategory: {
    (category: 'agent', partial: Partial<CategorySettings> | ModelSelectionPatch): Promise<void>
    (category: SettingsCategory, partial: Partial<CategorySettings>): Promise<void>
  }
  resetCategory: (category: SettingsCategory) => Promise<void>
  clear: () => void
}

const agentModelsSchema = z.object({
  providers: z.array(z.object({ models: z.array(z.string().refine((model) => model.trim().length > 0)) })),
})

const DEFAULT_ALL_SETTINGS: AllSettings = {
  ui: { theme: 'system', language: 'zh-CN', sidebar_collapsed: false },
  editor: { font_size: 14, tab_size: 2, auto_save: true, auto_save_interval: 30, word_wrap: true },
  agent: { providers: [], active_provider_id: '', temperature: 0.7, writing_style: 'academic', memory_enabled: true },
}

export const useSettingsStore = create<SettingsState>((set, get) => {
  let tail: Promise<void> | undefined
  let generation = 0
  let confirmed: AllSettings | null = null
  let edits: { category: 'ui' | 'editor'; partial: Partial<CategorySettings> }[] = []
  const effectiveSettings = () => edits.reduce<AllSettings | null>((settings, edit) => settings && ({
    ...settings,
    [edit.category]: { ...settings[edit.category], ...edit.partial },
  }), confirmed)
  const publish = () => set({ settings: effectiveSettings() })
  const enqueue = <T,>(operationGeneration: number, operation: () => Promise<T>, onQueued?: () => void): Promise<T> => {
    const previous = tail
    let finish!: () => void
    const completion = new Promise<void>((resolve) => { finish = resolve })
    tail = completion
    onQueued?.()
    const run = async () => {
      try {
        if (operationGeneration !== generation) throw new DOMException('Settings cleared', 'AbortError')
        const result = await operation()
        if (operationGeneration !== generation) throw new DOMException('Settings cleared', 'AbortError')
        return result
      } finally {
        finish()
        if (tail === completion) tail = undefined
      }
    }
    return previous ? previous.then(run) : run()
  }

  const loadSettings = async (fetchGeneration: number) => {
    set({ loading: true, error: null })
    try {
      if (fetchGeneration !== generation) throw new DOMException('Settings cleared', 'AbortError')
      const data = await getSettings()
      if (fetchGeneration !== generation) throw new DOMException('Settings cleared', 'AbortError')
      if (!agentModelsSchema.safeParse(data.agent).success) throw new Error('Invalid settings response')
      confirmed = data
      const effective = effectiveSettings()!
      set({ settings: effective, loading: false })
      if (fetchGeneration !== generation) throw new DOMException('Settings cleared', 'AbortError')
      return effective
    } catch (err) {
      if (fetchGeneration === generation) {
        const msg = err instanceof Error ? err.message : 'Failed to load settings'
        set({ error: msg, loading: false })
      }
      throw err
    }
  }

  return {
  settings: null,
  loading: false,
  error: null,
  pendingAgentModelChanges: 0,

  fetchSettings: () => {
    const fetchGeneration = generation
    return enqueue(fetchGeneration, () => loadSettings(fetchGeneration))
  },

  patchCategory: (category, partial) => {
    const saveGeneration = generation
    confirmed ??= get().settings
    const selection = 'model_selection' in partial ? partial.model_selection : null
    const changesModel = category === 'agent' && ('model_selection' in partial || 'active_provider_id' in partial || 'providers' in partial)
    const edit = (category === 'ui' || category === 'editor') && !('model_selection' in partial) ? { category, partial: { ...partial } } : null
    if (edit) edits.push(edit)
    const request = enqueue(saveGeneration, async () => {
      const current = get().settings
      if (!edit && current) {
        set({ settings: selection
          ? { ...current, agent: { ...current.agent, active_provider_id: selection.provider_id, providers: current.agent.providers.map((p) => p.id === selection.provider_id ? { ...p, active_model: selection.model } : p) } }
          : { ...current, [category]: { ...current[category], ...partial } } })
      }
      try {
        if (saveGeneration !== generation) throw new DOMException('Settings cleared', 'AbortError')
        const res = 'model_selection' in partial
          ? await updateCategory('agent', partial)
          : await updateCategory(category, partial)
        if (saveGeneration !== generation) throw new DOMException('Settings cleared', 'AbortError')
        if (category === 'agent' && !agentModelsSchema.safeParse(res.settings).success) throw new Error('Invalid settings response')
        confirmed = { ...(confirmed ?? DEFAULT_ALL_SETTINGS), [category]: res.settings as unknown as AllSettings[typeof category] }
        if (edit) edits = edits.filter((item) => item !== edit)
        publish()
      } catch {
        if (saveGeneration !== generation) throw new DOMException('Settings cleared', 'AbortError')
        if (edit) {
          edits = edits.filter((item) => item !== edit)
          publish()
        } else if (current) {
          set((state) => ({ settings: state.settings ? { ...state.settings, [category]: current[category] } : null }))
        }
        throw new Error(`Failed to update ${category} settings`)
      }
    }, changesModel ? () => set((state) => ({ pendingAgentModelChanges: state.pendingAgentModelChanges + 1 })) : undefined)
    if (edit && saveGeneration === generation) publish()
    return changesModel ? request.finally(() => {
      if (saveGeneration === generation) set((state) => ({ pendingAgentModelChanges: state.pendingAgentModelChanges - 1 }))
    }) : request
  },

  resetCategory: (category) => {
    const resetGeneration = generation
    return enqueue(resetGeneration, async () => {
      try {
        await resetCategory(category)
        if (resetGeneration !== generation) throw new DOMException('Settings cleared', 'AbortError')
        await loadSettings(resetGeneration)
      } catch {
        throw new Error(`Failed to reset ${category} settings`)
      }
    })
  },

  clear: () => {
    generation++
    tail = undefined
    confirmed = null
    edits = []
    set({ settings: null, loading: false, error: null, pendingAgentModelChanges: 0 })
  },
  }
})
