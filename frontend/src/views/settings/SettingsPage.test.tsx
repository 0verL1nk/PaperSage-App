import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render as rtlRender, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactElement } from 'react'
import userEvent from '@testing-library/user-event'

import { apiClient } from '@/services/client'
import { useSettingsStore } from '@/store/settings-store'
import type { AllSettings } from '@/services/settings-api'
import '@/i18n'

// Mock the store before importing the component so the component picks up the mock
vi.mock('@/store/settings-store', async () => {
  const actual = await vi.importActual<typeof import('@/store/settings-store')>(
    '@/store/settings-store',
  )
  return {
    ...actual,
    useSettingsStore: vi.fn(),
  }
})

import { SettingsPage } from './SettingsPage'

function render(ui: ReactElement) {
  return rtlRender(<QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>)
}

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

function installMockStore(overrides: Partial<ReturnType<typeof useSettingsStore.getState>>) {
  const fetchSettings = vi.fn().mockResolvedValue(fixtureSettings)
  vi.mocked(useSettingsStore).mockImplementation((selector?: (s: ReturnType<typeof useSettingsStore.getState>) => unknown) => {
    const state = {
      settings: null as AllSettings | null,
      loading: false,
      error: null as string | null,
      pendingAgentModelChanges: 0,
      fetchSettings,
      patchCategory: vi.fn(),
      resetCategory: vi.fn(),
      clear: vi.fn(),
      ...overrides,
    }
    return (selector ? selector(state) : state) as ReturnType<typeof useSettingsStore>
  })
  return fetchSettings
}

describe('SettingsPage', () => {
  beforeEach(() => {
    vi.mocked(useSettingsStore).mockReset()
    vi.spyOn(apiClient, 'get').mockRejectedValue(new Error('offline'))
  })

  it('keeps settings interactive', async () => {
    const patchCategory = vi.fn().mockResolvedValue(undefined)
    installMockStore({ settings: fixtureSettings, patchCategory })
    render(<SettingsPage />)

    await userEvent.click(screen.getByRole('checkbox', { name: '默认收起侧边栏' }))
    expect(patchCategory).toHaveBeenCalledWith('ui', { sidebar_collapsed: true })
  })

  it('renders the three tab triggers', () => {
    installMockStore({ settings: fixtureSettings, loading: false })
    render(<SettingsPage />)
    expect(screen.getByRole('tab', { name: '常规' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '编辑器' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'AI 助手' })).toBeInTheDocument()
  })

  it('shows loading state when no settings and loading=true', () => {
    installMockStore({ settings: null, loading: true })
    render(<SettingsPage />)
    expect(screen.getByText('正在加载设置…')).toBeInTheDocument()
  })

  it('shows localized AI provider settings in Chinese', async () => {
    installMockStore({ settings: fixtureSettings, loading: false })
    render(<SettingsPage />)
    await userEvent.click(screen.getByRole('tab', { name: 'AI 助手' }))
    expect(screen.getByText('模型服务商')).toBeInTheDocument()
    expect(screen.getByText('尚未配置服务商。点击 + 添加一个。')).toBeInTheDocument()
    expect(screen.getByText('尚未配置模型，请先添加服务商。')).toBeInTheDocument()
  })

  it('invokes fetchSettings on mount when settings are not loaded', async () => {
    const fetchSettings = installMockStore({ settings: null, loading: false })
    render(<SettingsPage />)
    await waitFor(() => {
      expect(fetchSettings).toHaveBeenCalled()
    })
  })

  it('does not call fetchSettings on mount when settings are already loaded', () => {
    const fetchSettings = installMockStore({ settings: fixtureSettings, loading: false })
    render(<SettingsPage />)
    expect(fetchSettings).not.toHaveBeenCalled()
  })
})
