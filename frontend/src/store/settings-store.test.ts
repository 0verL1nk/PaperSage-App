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
    useSettingsStore.setState({ settings: null, loading: false, error: null })
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

    it('captures error message on failure', async () => {
      vi.mocked(getSettings).mockRejectedValue(new Error('network down'))

      await expect(useSettingsStore.getState().fetchSettings()).rejects.toThrow()
      const state = useSettingsStore.getState()
      expect(state.loading).toBe(false)
      expect(state.error).toBe('network down')
    })
  })

  describe('patchCategory', () => {
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

    it('throws a wrapped error on failure', async () => {
      vi.mocked(updateCategory).mockRejectedValue(new Error('server error'))

      await expect(
        useSettingsStore.getState().patchCategory('ui', { theme: 'dark' }),
      ).rejects.toThrow(/Failed to update ui settings/)
    })
  })

  describe('resetCategory', () => {
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
