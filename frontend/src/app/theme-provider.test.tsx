import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ThemeProvider } from './theme-provider'
import { useTheme } from './theme-context'
import { useSettingsStore } from '@/store/settings-store'
import { useSessionStore } from '@/store/session-store'
import { getSettings, updateCategory, type AllSettings } from '@/services/settings-api'
import i18n, { LANG_STORAGE_KEY } from '@/i18n'
import { EditorSettings } from '@/views/settings/EditorSettings'
import { toast } from 'sonner'

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

vi.mock('@/services/settings-api', () => ({
  getSettings: vi.fn(),
  updateCategory: vi.fn(),
  resetCategory: vi.fn(),
}))

const settings: AllSettings = {
  ui: { theme: 'light', language: 'zh-CN', sidebar_collapsed: false },
  editor: { font_size: 14, tab_size: 2, auto_save: true, auto_save_interval: 30, word_wrap: true },
  agent: { providers: [], active_provider_id: '', temperature: 0.7, writing_style: 'academic', memory_enabled: true },
}

function ThemeProbe() {
  const { theme } = useTheme()
  return <div data-testid="theme">{theme}</div>
}

describe('ThemeProvider settings synchronization', () => {
  beforeEach(() => {
    useSettingsStore.getState().clear()
    vi.mocked(updateCategory).mockReset()
    vi.mocked(getSettings).mockReset()
    vi.mocked(toast.error).mockClear()
    localStorage.setItem('papersage_theme', 'light')
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  })

  it('continues following system theme changes after settings synchronization', () => {
    let listener!: () => void
    const media = { matches: false, addEventListener: vi.fn((_event, fn) => { listener = fn }), removeEventListener: vi.fn() }
    vi.stubGlobal('matchMedia', vi.fn(() => media))
    const systemSettings = structuredClone(settings)
    systemSettings.ui.theme = 'system'
    useSettingsStore.setState({ settings: systemSettings })
    const { unmount } = render(<ThemeProvider><ThemeProbe /></ThemeProvider>)
    expect(screen.getByTestId('theme')).toHaveTextContent('system')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    media.matches = true
    act(() => listener())
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    unmount()
    expect(media.removeEventListener).toHaveBeenCalled()
  })

  it('shows a failure and rolls back a controlled editor input', async () => {
    useSettingsStore.setState({ settings: structuredClone(settings) })
    vi.mocked(updateCategory).mockRejectedValueOnce(new Error('save failed'))
    render(<ThemeProvider><EditorSettings /></ThemeProvider>)
    const fontSize = document.querySelector<HTMLInputElement>('#font-size')!
    fireEvent.change(fontSize, { target: { value: '20' } })
    await waitFor(() => expect(toast.error).toHaveBeenCalled())
    expect(fontSize).toHaveValue(14)
    expect(useSettingsStore.getState().settings?.editor.font_size).toBe(14)
  })

  it('does not show a stale save failure after session termination', async () => {
    useSessionStore.setState({ accessToken: 'old', refreshToken: 'refresh', isGuest: false })
    useSettingsStore.setState({ settings: structuredClone(settings) })
    let rejectSave!: (reason: Error) => void
    vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((_, reject) => { rejectSave = reject }))
    render(<ThemeProvider><EditorSettings /></ThemeProvider>)
    fireEvent.change(document.querySelector<HTMLInputElement>('#font-size')!, { target: { value: '20' } })
    act(() => useSessionStore.getState().clearTokens())
    await act(async () => { rejectSave(new Error('old request failed')); await Promise.resolve(); await Promise.resolve() })
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('clears settings on session termination but not token refresh', async () => {
    useSessionStore.setState({ accessToken: 'old', refreshToken: 'refresh', isGuest: false })
    useSettingsStore.setState({ settings: structuredClone(settings) })
    render(<ThemeProvider><ThemeProbe /></ThemeProvider>)
    act(() => useSessionStore.setState({ accessToken: 'refreshed', refreshToken: 'new-refresh' }))
    expect(useSettingsStore.getState().settings).toEqual(settings)
    act(() => useSessionStore.getState().clearTokens())
    expect(useSettingsStore.getState().settings).toBeNull()
    expect(screen.getByTestId('theme')).toHaveTextContent('light')
    expect(localStorage.getItem('papersage_theme')).toBe('light')
  })

  it('does not flash old GET preferences while an optimistic UI save is queued', async () => {
    useSettingsStore.setState({ settings: structuredClone(settings) })
    let resolveGet!: (value: AllSettings) => void
    let resolveSave!: (value: { category: string; settings: Record<string, unknown> }) => void
    vi.mocked(getSettings).mockImplementationOnce(() => new Promise((resolve) => { resolveGet = resolve }))
    vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveSave = resolve }))
    render(<ThemeProvider><ThemeProbe /></ThemeProvider>)
    let fetch!: Promise<AllSettings>
    let save!: Promise<void>
    act(() => {
      fetch = useSettingsStore.getState().fetchSettings()
      save = useSettingsStore.getState().patchCategory('ui', { theme: 'dark', language: 'en-US' })
    })
    expect(screen.getByTestId('theme')).toHaveTextContent('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(i18n.language).toBe('en-US')
    await act(async () => { resolveGet(structuredClone(settings)); await fetch })
    expect(screen.getByTestId('theme')).toHaveTextContent('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(i18n.language).toBe('en-US')
    await act(async () => {
      resolveSave({ category: 'ui', settings: { ...settings.ui, theme: 'dark', language: 'en-US' } })
      await save
    })
    expect(localStorage.getItem('papersage_theme')).toBe('dark')
    expect(localStorage.getItem(LANG_STORAGE_KEY)).toBe('en-US')
  })

  it('keeps new-session preferences when an old GET resolves after clear', async () => {
    let resolveOld!: (value: AllSettings) => void
    const oldSettings = structuredClone(settings)
    oldSettings.ui = { ...oldSettings.ui, theme: 'dark', language: 'en-US' }
    vi.mocked(getSettings).mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve }))
      .mockResolvedValueOnce(structuredClone(settings))
    render(<ThemeProvider><ThemeProbe /></ThemeProvider>)
    let old!: Promise<AllSettings>
    act(() => { old = useSettingsStore.getState().fetchSettings() })
    const oldResult = expect(old).rejects.toThrow()
    await act(async () => {
      useSettingsStore.getState().clear()
      await useSettingsStore.getState().fetchSettings()
      resolveOld(oldSettings)
      await oldResult
    })
    expect(screen.getByTestId('theme')).toHaveTextContent('light')
    expect(localStorage.getItem('papersage_theme')).toBe('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(localStorage.getItem(LANG_STORAGE_KEY)).toBe('zh-CN')
    expect(i18n.language).toBe('zh-CN')
  })

  it('does not change new-session preferences when an old GET fails', async () => {
    let rejectOld!: (error: Error) => void
    vi.mocked(getSettings).mockImplementationOnce(() => new Promise((_, reject) => { rejectOld = reject }))
      .mockResolvedValueOnce(structuredClone(settings))
    render(<ThemeProvider><ThemeProbe /></ThemeProvider>)
    let old!: Promise<AllSettings>
    act(() => { old = useSettingsStore.getState().fetchSettings() })
    const oldResult = expect(old).rejects.toThrow('old failure')
    await act(async () => {
      useSettingsStore.getState().clear()
      await useSettingsStore.getState().fetchSettings()
      rejectOld(new Error('old failure'))
      await oldResult
    })
    expect(useSettingsStore.getState().error).toBeNull()
    expect(screen.getByTestId('theme')).toHaveTextContent('light')
    expect(localStorage.getItem('papersage_theme')).toBe('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(localStorage.getItem(LANG_STORAGE_KEY)).toBe('zh-CN')
    expect(i18n.language).toBe('zh-CN')
  })

  it('does not apply an old UI save after the new session loads', async () => {
    useSettingsStore.setState({ settings: structuredClone(settings) })
    let resolveOld!: (value: { category: string; settings: Record<string, unknown> }) => void
    vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve }))
    vi.mocked(getSettings).mockResolvedValueOnce(structuredClone(settings))
    render(<ThemeProvider><ThemeProbe /></ThemeProvider>)
    let old!: Promise<void>
    act(() => { old = useSettingsStore.getState().patchCategory('ui', { theme: 'dark', language: 'en-US' }) })
    const oldResult = expect(old).rejects.toThrow()
    await act(async () => {
      useSettingsStore.getState().clear()
      await useSettingsStore.getState().fetchSettings()
      resolveOld({ category: 'ui', settings: { ...settings.ui, theme: 'dark', language: 'en-US' } })
      await oldResult
    })
    expect(screen.getByTestId('theme')).toHaveTextContent('light')
    expect(localStorage.getItem('papersage_theme')).toBe('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(localStorage.getItem(LANG_STORAGE_KEY)).toBe('zh-CN')
    expect(i18n.language).toBe('zh-CN')
  })

  it('keeps context, DOM and language aligned during an optimistic UI edit and rollback', async () => {
    useSettingsStore.setState({ settings: structuredClone(settings) })
    let rejectSave!: (reason: Error) => void
    vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((_, reject) => { rejectSave = reject }))
    render(<ThemeProvider><ThemeProbe /></ThemeProvider>)
    let save!: Promise<void>
    act(() => { save = useSettingsStore.getState().patchCategory('ui', { theme: 'dark', language: 'en-US' }) })
    expect(screen.getByTestId('theme')).toHaveTextContent('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem(LANG_STORAGE_KEY)).toBe('en-US')
    expect(i18n.language).toBe('en-US')
    await act(async () => {
      rejectSave(new Error('save failed'))
      await expect(save).rejects.toThrow()
    })
    expect(screen.getByTestId('theme')).toHaveTextContent('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(localStorage.getItem(LANG_STORAGE_KEY)).toBe('zh-CN')
    expect(i18n.language).toBe('zh-CN')
  })
})
