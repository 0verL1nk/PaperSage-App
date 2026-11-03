import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  getSettings,
  updateCategory,
  resetCategory,
} from '@/services/settings-api'
import { useSettingsStore } from './settings-store'
import type { AllSettings } from '@/services/settings-api'

vi.mock('@/services/settings-api', () => ({
  getSettings: vi.fn(),
  updateCategory: vi.fn(),
  replaceCategory: vi.fn(),
  resetCategory: vi.fn(),
}))

const fixtureSettings: AllSettings = {
  ui: { theme: 'system', language: 'zh-CN', sidebar_collapsed: false },
  editor: { font_size: 14, tab_size: 2, auto_save: true, auto_save_interval: 30, word_wrap: true },
  agent: {
    providers: [],
    active_provider_id: '',
    temperature: 0.7,
    writing_style: 'academic',
    memory_enabled: true,
  },
}

describe('settings store', () => {
  beforeEach(() => {
    useSettingsStore.getState().clear()
    vi.mocked(getSettings).mockReset()
    vi.mocked(updateCategory).mockReset()
    vi.mocked(resetCategory).mockReset()
  })

  describe('initial state', () => {
    it('starts with null settings, no loading, no error', () => {
      const state = useSettingsStore.getState()
      expect(state.settings).toBeNull()
      expect(state.loading).toBe(false)
      expect(state.error).toBeNull()
    })
  })

  describe('fetchSettings', () => {
    it('sets loading true, then stores data on success', async () => {
      vi.mocked(getSettings).mockResolvedValue(fixtureSettings)

      const promise = useSettingsStore.getState().fetchSettings()

      // Immediately after invocation, loading should be true
      expect(useSettingsStore.getState().loading).toBe(true)
      expect(useSettingsStore.getState().error).toBeNull()

      await promise
      const state = useSettingsStore.getState()
      expect(state.loading).toBe(false)
      expect(state.settings).toEqual(fixtureSettings)
    })

    it('rejects corrupt provider model entries instead of exposing them to the UI', async () => {
      const corrupt = structuredClone(fixtureSettings)
      corrupt.agent.providers = [{ id: 'p', name: 'P', type: 'openai_compatible', base_url: '', api_key: '', models: [{ broken: true }], active_model: '' }] as never
      vi.mocked(getSettings).mockResolvedValue(corrupt)
      await expect(useSettingsStore.getState().fetchSettings()).rejects.toThrow()
      expect(useSettingsStore.getState().settings).toBeNull()
      expect(useSettingsStore.getState().error).not.toBeNull()
    })

    it('rejects a previous account response without changing the new account or UI preferences', async () => {
      let resolveOld!: (value: AllSettings) => void
      const oldSettings = structuredClone(fixtureSettings)
      oldSettings.ui = { ...oldSettings.ui, theme: 'dark', language: 'en-US' }
      const newSettings = structuredClone(fixtureSettings)
      newSettings.ui = { ...newSettings.ui, theme: 'light', language: 'zh-CN' }
      vi.mocked(getSettings).mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve }))
        .mockResolvedValueOnce(newSettings)

      const oldRequest = useSettingsStore.getState().fetchSettings()
      const oldResult = expect(oldRequest).rejects.toThrow()
      useSettingsStore.getState().clear()
      await useSettingsStore.getState().fetchSettings()
      resolveOld(oldSettings)
      await oldResult

      expect(useSettingsStore.getState()).toMatchObject({ settings: newSettings, loading: false, error: null })
    })

    it('does not let a previous account failure change the new account state', async () => {
      let rejectOld!: (error: Error) => void
      vi.mocked(getSettings).mockImplementationOnce(() => new Promise((_, reject) => { rejectOld = reject }))
        .mockResolvedValueOnce(fixtureSettings)
      const oldRequest = useSettingsStore.getState().fetchSettings()
      const oldResult = expect(oldRequest).rejects.toThrow('old network error')
      useSettingsStore.getState().clear()
      await useSettingsStore.getState().fetchSettings()
      rejectOld(new Error('old network error'))
      await oldResult
      expect(useSettingsStore.getState()).toMatchObject({ settings: fixtureSettings, loading: false, error: null })
    })

    it('runs an earlier fetch before a queued category save', async () => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      let resolveFetch!: (value: AllSettings) => void
      vi.mocked(getSettings).mockImplementationOnce(() => new Promise((resolve) => { resolveFetch = resolve }))
      const savedUi = { ...fixtureSettings.ui, theme: 'dark' as const, language: 'en-US' }
      vi.mocked(updateCategory).mockResolvedValueOnce({ category: 'ui', settings: savedUi })
      const request = useSettingsStore.getState().fetchSettings()
      const save = useSettingsStore.getState().patchCategory('ui', { theme: 'dark', language: 'en-US' })
      const response = structuredClone(fixtureSettings)
      response.editor.font_size = 18
      resolveFetch(response)
      const result = await request
      await save
      expect(result.ui).toEqual(savedUi)
      expect(result.editor.font_size).toBe(18)
      expect(useSettingsStore.getState().settings?.ui).toEqual(savedUi)
      expect(useSettingsStore.getState().settings?.editor.font_size).toBe(18)
      expect(vi.mocked(getSettings).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(updateCategory).mock.invocationCallOrder[0])
    })

    it('waits for a pending save before fetching its updated settings', async () => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      let resolveSave!: (value: { category: string; settings: Record<string, unknown> }) => void
      vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveSave = resolve }))
      const fetched = structuredClone(fixtureSettings)
      fetched.ui.theme = 'dark'
      vi.mocked(getSettings).mockResolvedValueOnce(fetched)
      const save = useSettingsStore.getState().patchCategory('ui', { theme: 'dark' })
      const request = useSettingsStore.getState().fetchSettings()
      const storedTheme = useSettingsStore.getState().settings?.ui.theme
      resolveSave({ category: 'ui', settings: { ...fixtureSettings.ui, theme: 'dark' } })
      await save
      const result = await request
      expect(result.ui.theme).toBe('dark')
      expect(storedTheme).toBe('dark')
      expect(vi.mocked(updateCategory).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(getSettings).mock.invocationCallOrder[0])
    })

    it('fetches fresh fields after a pending save fails', async () => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      let rejectSave!: (reason: Error) => void
      vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((_, reject) => { rejectSave = reject }))
      const fetched = structuredClone(fixtureSettings)
      fetched.ui.language = 'en-US'
      vi.mocked(getSettings).mockResolvedValueOnce(fetched)
      const save = useSettingsStore.getState().patchCategory('ui', { theme: 'dark' })
      const failedSave = expect(save).rejects.toThrow('Failed to update ui settings')
      expect(useSettingsStore.getState().settings?.ui.theme).toBe('dark')
      const request = useSettingsStore.getState().fetchSettings()
      rejectSave(new Error('save failed'))
      await failedSave
      const result = await request
      expect(result.ui).toMatchObject({ theme: 'system', language: 'en-US' })
      expect(useSettingsStore.getState().settings?.ui).toMatchObject({ theme: 'system', language: 'en-US' })
    })

    it('applies concurrent fetches in call order, including UI preferences', async () => {
      let resolveFirst!: (value: AllSettings) => void
      const first = structuredClone(fixtureSettings)
      first.ui = { ...first.ui, theme: 'dark', language: 'en-US' }
      const second = structuredClone(fixtureSettings)
      second.ui = { ...second.ui, theme: 'light', language: 'zh-CN' }
      vi.mocked(getSettings).mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve }))
        .mockResolvedValueOnce(second)
      const firstRequest = useSettingsStore.getState().fetchSettings()
      const secondRequest = useSettingsStore.getState().fetchSettings()
      resolveFirst(first)
      await expect(firstRequest).resolves.toEqual(first)
      await expect(secondRequest).resolves.toEqual(second)
      expect(useSettingsStore.getState().settings).toEqual(second)
    })

    it('captures error message on failure', async () => {
      vi.mocked(getSettings).mockRejectedValue(new Error('network down'))

      await expect(useSettingsStore.getState().fetchSettings()).rejects.toThrow()
      const state = useSettingsStore.getState()
      expect(state.loading).toBe(false)
      expect(state.error).toBe('network down')
    })
  })

  describe('patchCategory', () => {
    it.each([
      [{ active_provider_id: 'p' }, 1],
      [{ providers: [] }, 1],
      [{ temperature: 1 }, 0],
    ] satisfies [Partial<AllSettings['agent']>, number][])('tracks model-affecting agent patch %o', async (patch, pending) => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      let resolveSave!: (value: { category: string; settings: Record<string, unknown> }) => void
      vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveSave = resolve }))
      const save = useSettingsStore.getState().patchCategory('agent', patch)
      expect(useSettingsStore.getState().pendingAgentModelChanges).toBe(pending)
      resolveSave({ category: 'agent', settings: { ...fixtureSettings.agent } })
      await save
      expect(useSettingsStore.getState().pendingAgentModelChanges).toBe(0)
    })

    it('tracks queued model selections until each success or failure', async () => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      let resolveFirst!: (value: { category: string; settings: Record<string, unknown> }) => void
      let rejectSecond!: (reason: Error) => void
      vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve }))
        .mockImplementationOnce(() => new Promise((_, reject) => { rejectSecond = reject }))
      const first = useSettingsStore.getState().patchCategory('agent', { model_selection: { provider_id: 'p', model: 'A' } })
      const second = useSettingsStore.getState().patchCategory('agent', { model_selection: { provider_id: 'p', model: 'B' } })
      const secondResult = expect(second).rejects.toThrow()
      expect(useSettingsStore.getState().pendingAgentModelChanges).toBe(2)
      resolveFirst({ category: 'agent', settings: { ...fixtureSettings.agent } })
      await first
      expect(useSettingsStore.getState().pendingAgentModelChanges).toBe(1)
      await vi.waitFor(() => expect(updateCategory).toHaveBeenCalledTimes(2))
      rejectSecond(new Error('failed'))
      await secondResult
      expect(useSettingsStore.getState().pendingAgentModelChanges).toBe(0)
    })

    it('does not let old model saves change pending count after clear', async () => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      let resolveOld!: (value: { category: string; settings: Record<string, unknown> }) => void
      let resolveNew!: (value: { category: string; settings: Record<string, unknown> }) => void
      vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve }))
        .mockImplementationOnce(() => new Promise((resolve) => { resolveNew = resolve }))
      const old = useSettingsStore.getState().patchCategory('agent', { model_selection: { provider_id: 'p', model: 'A' } })
      const oldResult = expect(old).rejects.toThrow()
      useSettingsStore.getState().clear()
      expect(useSettingsStore.getState().pendingAgentModelChanges).toBe(0)
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      const next = useSettingsStore.getState().patchCategory('agent', { model_selection: { provider_id: 'p', model: 'B' } })
      resolveOld({ category: 'agent', settings: { ...fixtureSettings.agent } })
      await oldResult
      expect(useSettingsStore.getState().pendingAgentModelChanges).toBe(1)
      resolveNew({ category: 'agent', settings: { ...fixtureSettings.agent } })
      await next
      expect(useSettingsStore.getState().pendingAgentModelChanges).toBe(0)
    })

    it('reserves model selection queue order before notifying subscribers', async () => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      vi.mocked(updateCategory).mockResolvedValue({ category: 'agent', settings: { ...fixtureSettings.agent } })
      let second: Promise<void> | undefined
      let triggered = false
      const unsubscribe = useSettingsStore.subscribe((state) => {
        if (!triggered && state.pendingAgentModelChanges === 1) {
          triggered = true
          second = useSettingsStore.getState().patchCategory('agent', { model_selection: { provider_id: 'p', model: 'B' } })
        }
      })
      const first = useSettingsStore.getState().patchCategory('agent', { model_selection: { provider_id: 'p', model: 'A' } })
      await first
      await second
      unsubscribe()
      expect(updateCategory).toHaveBeenCalledTimes(2)
      expect(vi.mocked(updateCategory).mock.calls.map(([, partial]) => partial)).toEqual([
        { model_selection: { provider_id: 'p', model: 'A' } },
        { model_selection: { provider_id: 'p', model: 'B' } },
      ])
      expect(useSettingsStore.getState().pendingAgentModelChanges).toBe(0)
    })

    it('queues atomic model selection after earlier saves and keeps returned provider changes', async () => {
      const initial = structuredClone(fixtureSettings)
      initial.agent.providers = [{ id: 'p', name: 'Before', type: 'openai_compatible', base_url: '', api_key: 'secret', models: ['A', 'B'], active_model: 'A' }]
      initial.agent.active_provider_id = 'p'
      useSettingsStore.setState({ settings: initial })
      let resolveFirst!: (value: { category: string; settings: Record<string, unknown> }) => void
      vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve }))
      const updated = structuredClone(initial.agent)
      updated.providers[0].name = 'Updated remotely'
      const selected = structuredClone(updated)
      selected.providers[0].active_model = 'B'
      vi.mocked(updateCategory).mockResolvedValueOnce({ category: 'agent', settings: { ...selected } })
      const first = useSettingsStore.getState().patchCategory('agent', { temperature: 1 })
      const second = useSettingsStore.getState().patchCategory('agent', { model_selection: { provider_id: 'p', model: 'B' } })
      expect(updateCategory).toHaveBeenCalledTimes(1)
      resolveFirst({ category: 'agent', settings: { ...updated, temperature: 1 } })
      await Promise.all([first, second])
      expect(updateCategory).toHaveBeenLastCalledWith('agent', { model_selection: { provider_id: 'p', model: 'B' } })
      expect(useSettingsStore.getState().settings?.agent.providers[0]).toMatchObject({ name: 'Updated remotely', active_model: 'B' })
    })

    it('preserves call order when a store subscriber starts another save', async () => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      vi.mocked(updateCategory).mockImplementation(async (category, partial) => ({
        category, settings: { ...fixtureSettings.editor, ...partial },
      }))
      let second: Promise<void> | undefined
      let triggered = false
      const unsubscribe = useSettingsStore.subscribe((state) => {
        if (!triggered && state.settings?.editor.font_size === 18) {
          triggered = true
          second = useSettingsStore.getState().patchCategory('editor', { font_size: 22 })
        }
      })
      const first = useSettingsStore.getState().patchCategory('editor', { font_size: 18 })
      await first
      await second
      unsubscribe()
      expect(vi.mocked(updateCategory).mock.calls.map(([, partial]) => partial)).toEqual([
        { font_size: 18 }, { font_size: 22 },
      ])
      expect(useSettingsStore.getState().settings?.editor.font_size).toBe(22)
    })

    it('immediately shows queued ui and editor edits while a GET is pending', async () => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      let resolveGet!: (value: AllSettings) => void
      vi.mocked(getSettings).mockImplementationOnce(() => new Promise((resolve) => { resolveGet = resolve }))
      vi.mocked(updateCategory).mockImplementation(async (category) => category === 'ui'
        ? { category, settings: { ...fixtureSettings.ui, theme: 'dark', language: 'en-US' } }
        : { category, settings: { ...fixtureSettings.editor, font_size: 20 } })
      const fetch = useSettingsStore.getState().fetchSettings()
      const ui = useSettingsStore.getState().patchCategory('ui', { theme: 'dark' })
      const editor = useSettingsStore.getState().patchCategory('editor', { font_size: 20 })
      const optimistic = structuredClone(useSettingsStore.getState().settings)
      const fetched = structuredClone(fixtureSettings)
      fetched.ui.language = 'en-US'
      resolveGet(fetched)
      await Promise.all([fetch, ui, editor])
      expect(optimistic?.ui.theme).toBe('dark')
      expect(optimistic?.editor.font_size).toBe(20)
      expect(useSettingsStore.getState().settings?.ui).toMatchObject({ theme: 'dark', language: 'en-US' })
      expect(useSettingsStore.getState().settings?.editor.font_size).toBe(20)
    })

    it('replays later edits after an earlier same-field save fails', async () => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      let rejectFirst!: (reason: Error) => void
      vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((_, reject) => { rejectFirst = reject }))
        .mockResolvedValueOnce({ category: 'editor', settings: { ...fixtureSettings.editor, font_size: 22 } })
      const first = useSettingsStore.getState().patchCategory('editor', { font_size: 18 })
      const firstResult = expect(first).rejects.toThrow()
      const second = useSettingsStore.getState().patchCategory('editor', { font_size: 22 })
      expect(useSettingsStore.getState().settings?.editor.font_size).toBe(22)
      rejectFirst(new Error('first failed'))
      await firstResult
      expect(useSettingsStore.getState().settings?.editor.font_size).toBe(22)
      await second
      expect(useSettingsStore.getState().settings?.editor.font_size).toBe(22)
    })

    it('rolls back a failed later edit to the first confirmed value', async () => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      let resolveFirst!: (value: { category: string; settings: Record<string, unknown> }) => void
      let rejectSecond!: (reason: Error) => void
      vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve }))
        .mockImplementationOnce(() => new Promise((_, reject) => { rejectSecond = reject }))
      const first = useSettingsStore.getState().patchCategory('editor', { font_size: 18 })
      const second = useSettingsStore.getState().patchCategory('editor', { font_size: 22 })
      const secondResult = expect(second).rejects.toThrow()
      expect(useSettingsStore.getState().settings?.editor.font_size).toBe(22)
      resolveFirst({ category: 'editor', settings: { ...fixtureSettings.editor, font_size: 18 } })
      await first
      await vi.waitFor(() => expect(updateCategory).toHaveBeenCalledTimes(2))
      expect(useSettingsStore.getState().settings?.editor.font_size).toBe(22)
      rejectSecond(new Error('second failed'))
      await secondResult
      expect(useSettingsStore.getState().settings?.editor.font_size).toBe(18)
    })

    it.each(['success', 'failure'] as const)('keeps same-field optimistic order when both saves %s', async (outcome) => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      let settleFirst!: (value: { category: string; settings: Record<string, unknown> }) => void
      let rejectFirst!: (error: Error) => void
      vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve, reject) => { settleFirst = resolve; rejectFirst = reject }))
        .mockImplementationOnce(async () => {
          if (outcome === 'failure') throw new Error('second failed')
          return { category: 'editor', settings: { ...fixtureSettings.editor, font_size: 22 } }
        })
      const first = useSettingsStore.getState().patchCategory('editor', { font_size: 18 })
      const second = useSettingsStore.getState().patchCategory('editor', { font_size: 22 })
      const results = Promise.allSettled([first, second])
      expect(useSettingsStore.getState().settings?.editor.font_size).toBe(22)
      if (outcome === 'failure') rejectFirst(new Error('first failed'))
      else settleFirst({ category: 'editor', settings: { ...fixtureSettings.editor, font_size: 18 } })
      const settled = await results
      expect(settled.map((item) => item.status)).toEqual([outcome === 'success' ? 'fulfilled' : 'rejected', outcome === 'success' ? 'fulfilled' : 'rejected'])
      expect(useSettingsStore.getState().settings?.editor.font_size).toBe(outcome === 'success' ? 22 : 14)
    })

    it('applies optimistic update before server responds', async () => {
      useSettingsStore.setState({ settings: fixtureSettings })
      vi.mocked(updateCategory).mockImplementation(
        () => new Promise((resolve) => setTimeout(() => resolve({ category: 'ui', settings: { theme: 'dark' } }), 50)),
      )

      const promise = useSettingsStore.getState().patchCategory('ui', { theme: 'dark' })

      // Optimistic: theme is already dark before the promise resolves
      expect(useSettingsStore.getState().settings?.ui.theme).toBe('dark')

      await promise
    })

    it('does not sync an old account UI save after clear', async () => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      let resolveSave!: (value: { category: string; settings: Record<string, unknown> }) => void
      vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveSave = resolve }))
      const save = useSettingsStore.getState().patchCategory('ui', { theme: 'dark', language: 'en-US' })
      const rejected = expect(save).rejects.toThrow()
      useSettingsStore.getState().clear()
      const newSettings = structuredClone(fixtureSettings)
      newSettings.ui.theme = 'light'
      vi.mocked(getSettings).mockResolvedValueOnce(newSettings)
      await useSettingsStore.getState().fetchSettings()
      resolveSave({ category: 'ui', settings: { ...fixtureSettings.ui, theme: 'dark', language: 'en-US' } })
      await rejected
      expect(useSettingsStore.getState().settings).toEqual(newSettings)
    })

    it('rolls back to previous value on server error', async () => {
      useSettingsStore.setState({ settings: fixtureSettings })
      vi.mocked(updateCategory).mockRejectedValue(new Error('server error'))

      await expect(
        useSettingsStore.getState().patchCategory('ui', { theme: 'dark' }),
      ).rejects.toThrow()

      const state = useSettingsStore.getState()
      // Rolled back to original
      expect(state.settings?.ui.theme).toBe('system')
    })

    it('serializes same-category saves to avoid stale responses and rollback', async () => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      let resolveFirst!: (value: { category: string; settings: Record<string, unknown> }) => void
      vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve }))
        .mockResolvedValueOnce({ category: 'agent', settings: { ...fixtureSettings.agent, temperature: 1, writing_style: 'formal' } })
      const first = useSettingsStore.getState().patchCategory('agent', { temperature: 1 })
      const second = useSettingsStore.getState().patchCategory('agent', { writing_style: 'formal' })
      expect(updateCategory).toHaveBeenCalledTimes(1)
      resolveFirst({ category: 'agent', settings: { ...fixtureSettings.agent, temperature: 1 } })
      await Promise.all([first, second])
      expect(useSettingsStore.getState().settings?.agent).toMatchObject({ temperature: 1, writing_style: 'formal' })
    })

    it('does not apply or send queued changes after the settings store is cleared', async () => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      let resolveFirst!: (value: { category: string; settings: Record<string, unknown> }) => void
      vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve }))
      const first = useSettingsStore.getState().patchCategory('agent', { temperature: 1 })
      const second = useSettingsStore.getState().patchCategory('agent', { writing_style: 'formal' })
      useSettingsStore.getState().clear()
      resolveFirst({ category: 'agent', settings: { ...fixtureSettings.agent, temperature: 1 } })
      await expect(first).rejects.toThrow()
      await expect(second).rejects.toThrow()
      expect(updateCategory).toHaveBeenCalledTimes(1)
      expect(useSettingsStore.getState().settings).toBeNull()
    })

    it('does not undo a successful category save when another category fails', async () => {
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      let rejectAgent!: (reason: Error) => void
      vi.mocked(updateCategory).mockImplementation((category) => category === 'agent'
        ? new Promise((_, reject) => { rejectAgent = reject })
        : Promise.resolve({ category: 'ui', settings: { ...fixtureSettings.ui, theme: 'dark' } }))
      const failed = useSettingsStore.getState().patchCategory('agent', { temperature: 1 })
      const failedResult = expect(failed).rejects.toThrow()
      const saved = useSettingsStore.getState().patchCategory('ui', { theme: 'dark' })
      rejectAgent(new Error('failure'))
      await failedResult
      await saved
      expect(useSettingsStore.getState().settings?.ui.theme).toBe('dark')
      expect(useSettingsStore.getState().settings?.agent.temperature).toBe(0.7)
    })

    it('throws a wrapped error on failure', async () => {
      vi.mocked(updateCategory).mockRejectedValue(new Error('server error'))

      await expect(
        useSettingsStore.getState().patchCategory('ui', { theme: 'dark' }),
      ).rejects.toThrow(/Failed to update ui settings/)
    })
  })

  describe('resetCategory', () => {
    it('rejects an old reset without fetching the new account settings', async () => {
      let resolveReset!: (value: { category: string; settings: Record<string, unknown> }) => void
      vi.mocked(resetCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveReset = resolve }))
      const request = useSettingsStore.getState().resetCategory('ui')
      const result = expect(request).rejects.toThrow()
      useSettingsStore.getState().clear()
      useSettingsStore.setState({ settings: structuredClone(fixtureSettings) })
      resolveReset({ category: 'ui', settings: {} })
      await result
      expect(getSettings).not.toHaveBeenCalled()
      expect(useSettingsStore.getState().settings).toEqual(fixtureSettings)
    })

    it('finishes reset and its read before a later save', async () => {
      let resolveRead!: (value: AllSettings) => void
      vi.mocked(resetCategory).mockResolvedValueOnce({ category: 'ui', settings: {} })
      vi.mocked(getSettings).mockImplementationOnce(() => new Promise((resolve) => { resolveRead = resolve }))
      vi.mocked(updateCategory).mockResolvedValueOnce({ category: 'editor', settings: { ...fixtureSettings.editor, font_size: 20 } })
      const reset = useSettingsStore.getState().resetCategory('ui')
      const save = useSettingsStore.getState().patchCategory('editor', { font_size: 20 })
      await vi.waitFor(() => expect(getSettings).toHaveBeenCalledTimes(1))
      resolveRead(structuredClone(fixtureSettings))
      await reset
      await save
      expect(useSettingsStore.getState().settings?.editor.font_size).toBe(20)
      expect(vi.mocked(getSettings).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(updateCategory).mock.invocationCallOrder[0])
    })

    it('calls resetCategory API and re-fetches settings', async () => {
      vi.mocked(resetCategory).mockResolvedValue({ category: 'ui', settings: {} })
      vi.mocked(getSettings).mockResolvedValue(fixtureSettings)

      await useSettingsStore.getState().resetCategory('ui')

      expect(resetCategory).toHaveBeenCalledWith('ui')
      expect(getSettings).toHaveBeenCalled()
      expect(useSettingsStore.getState().settings).toEqual(fixtureSettings)
    })
  })

  describe('clear', () => {
    it('rejects queued old-account reads and writes without blocking the new account', async () => {
      let resolveOld!: (value: AllSettings) => void
      vi.mocked(getSettings).mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve }))
        .mockResolvedValueOnce(structuredClone(fixtureSettings))
      const oldFetch = useSettingsStore.getState().fetchSettings()
      const oldFetchResult = expect(oldFetch).rejects.toThrow()
      const oldSaveResult = expect(useSettingsStore.getState().patchCategory('ui', { theme: 'dark' })).rejects.toThrow()
      const oldResetResult = expect(useSettingsStore.getState().resetCategory('editor')).rejects.toThrow()
      useSettingsStore.getState().clear()
      await expect(useSettingsStore.getState().fetchSettings()).resolves.toEqual(fixtureSettings)
      resolveOld(structuredClone(fixtureSettings))
      await Promise.all([oldFetchResult, oldSaveResult, oldResetResult])
      expect(updateCategory).not.toHaveBeenCalled()
      expect(resetCategory).not.toHaveBeenCalled()
      expect(getSettings).toHaveBeenCalledTimes(2)
      expect(useSettingsStore.getState().settings).toEqual(fixtureSettings)
    })

    it('resets state to initial', () => {
      useSettingsStore.setState({
        settings: fixtureSettings,
        loading: false,
        error: 'something',
      })

      useSettingsStore.getState().clear()

      const state = useSettingsStore.getState()
      expect(state.settings).toBeNull()
      expect(state.loading).toBe(false)
      expect(state.error).toBeNull()
    })
  })
})
