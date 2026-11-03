import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import axios, { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { apiClient } from '@/services/client'
import { useSessionStore } from '@/store/session-store'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AiPanel } from './ai-panel'
import { AgentSettings } from '@/views/settings/AgentSettings'
import { useSettingsStore } from '@/store/settings-store'
import { emptyChatState, useChatStore } from '@/store/chat-store'
import { getSettings, updateCategory } from '@/services/settings-api'
import { toast } from 'sonner'
import i18n from '@/i18n'
import type { AllSettings } from '@/services/settings-api'

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))
vi.mock('@/services/settings-api', () => ({ getSettings: vi.fn(), updateCategory: vi.fn(), resetCategory: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: [], refetch: vi.fn() }),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}))
vi.mock('@/views/settings/model-selector', () => ({
  ModelSelector: ({ value, onChange, disabled }: { value: { providerId: string; model: string }; onChange: (value: { providerId: string; model: string }) => void; disabled?: boolean }) => (
    <div><span data-testid="selection">{value.providerId}/{value.model}</span><button disabled={disabled} onClick={() => onChange({ providerId: 'p', model: 'B' })}>Choose B</button><button disabled={disabled} onClick={() => onChange({ providerId: 'q', model: 'Q' })}>Choose Q</button></div>
  ),
}))

const settings: AllSettings = {
  ui: { theme: 'system', language: 'zh-CN', sidebar_collapsed: false },
  editor: { font_size: 14, tab_size: 2, auto_save: true, auto_save_interval: 30, word_wrap: true },
  agent: {
    active_provider_id: 'p', temperature: 0.7, writing_style: 'academic', memory_enabled: true,
    providers: [
      { id: 'p', name: 'Provider', type: 'openai_compatible', base_url: 'https://example.invalid', api_key: 'secret', models: ['A', 'B'], active_model: 'A' },
      { id: 'q', name: 'Other', type: 'openai_compatible', base_url: 'https://example.invalid', api_key: 'secret2', models: ['Q'], active_model: 'Q' },
    ],
  },
}

const originalSend = useChatStore.getState().send
const originalCancel = useChatStore.getState().cancel
const originalPatch = useSettingsStore.getState().patchCategory
const originalAdapter = apiClient.defaults.adapter
const originalAxiosAdapter = axios.defaults.adapter

afterEach(() => {
  apiClient.defaults.adapter = originalAdapter
  axios.defaults.adapter = originalAxiosAdapter
  useSessionStore.getState().clearTokens()
})

beforeEach(() => {
  void i18n.changeLanguage('zh-CN')
  useSettingsStore.getState().clear()
  useSettingsStore.setState({ settings: structuredClone(settings), error: null, patchCategory: originalPatch })
  useChatStore.setState({ ...emptyChatState, send: originalSend, cancel: originalCancel })
  vi.mocked(updateCategory).mockReset()
  vi.mocked(toast.error).mockClear()
  Element.prototype.scrollIntoView = vi.fn()
})

describe('AI panel model selection', () => {
  it('shows exactly one error for a real failed settings request', async () => {
    const actual = await vi.importActual<typeof import('@/services/settings-api')>('@/services/settings-api')
    vi.mocked(updateCategory).mockImplementation(actual.updateCategory)
    apiClient.defaults.adapter = async (config) => { throw new AxiosError('offline', 'ERR_NETWORK', config) }
    render(<AiPanel projectId="p" />)
    fireEvent.click(screen.getByText('Choose B'))
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('模型选择保存失败，已恢复原选择，请重试。'))
    expect(toast.error).toHaveBeenCalledTimes(1)
  })

  it('suppresses a real request failure after the account is cleared', async () => {
    const actual = await vi.importActual<typeof import('@/services/settings-api')>('@/services/settings-api')
    vi.mocked(updateCategory).mockImplementation(actual.updateCategory)
    let rejectSave!: () => void
    apiClient.defaults.adapter = (config) => new Promise((_, reject) => {
      rejectSave = () => reject(new AxiosError('offline', 'ERR_NETWORK', config))
    })
    const patch = vi.spyOn(useSettingsStore.getState(), 'patchCategory')
    const view = render(<AiPanel projectId="p" />)
    fireEvent.click(screen.getByText('Choose B'))
    const result = expect(patch.mock.results[0].value).rejects.toMatchObject({ name: 'AbortError' })
    await vi.waitFor(() => expect(rejectSave).toBeTypeOf('function'))
    view.unmount()
    useSettingsStore.getState().clear()
    await act(async () => { rejectSave(); await result })
    expect(toast.error).not.toHaveBeenCalled()
    patch.mockRestore()
  })

  it('still refreshes a 401 and retries the settings patch', async () => {
    const actual = await vi.importActual<typeof import('@/services/settings-api')>('@/services/settings-api')
    vi.mocked(updateCategory).mockImplementation(actual.updateCategory)
    useSessionStore.getState().setTokens({ accessToken: 'old-token', refreshToken: 'old-refresh' })
    const authorization: string[] = []
    const adapter = async (config: InternalAxiosRequestConfig): Promise<AxiosResponse> => {
      if (config.url?.includes('/auth/refresh')) {
        return { config, status: 200, statusText: 'OK', headers: {}, data: { code: 200, data: { accessToken: 'new-token', refreshToken: 'new-refresh' }, info: '' } }
      }
      authorization.push(String(config.headers.Authorization))
      if (authorization.length === 1) {
        const response = { config, status: 401, statusText: 'Unauthorized', headers: {}, data: {} }
        throw new AxiosError('Unauthorized', 'ERR_BAD_RESPONSE', config, undefined, response)
      }
      const saved = structuredClone(settings.agent)
      saved.providers[0].active_model = 'B'
      return { config, status: 200, statusText: 'OK', headers: {}, data: { code: 200, data: { category: 'agent', settings: saved }, info: '' } }
    }
    apiClient.defaults.adapter = adapter
    axios.defaults.adapter = adapter
    render(<AiPanel projectId="p" />)
    fireEvent.click(screen.getByText('Choose B'))
    await waitFor(() => expect(useSessionStore.getState().accessToken).toBe('new-token'))
    await waitFor(() => expect(screen.getByText('Choose B')).toBeEnabled())
    expect(authorization).toEqual(['Bearer old-token', 'Bearer new-token'])
    expect(toast.error).not.toHaveBeenCalled()
  })

  it.each([['Choose B', 'p', 'B'], ['Choose Q', 'q', 'Q']])('persists %s and displays the saved selection', async (button, providerId, model) => {
    vi.mocked(updateCategory).mockImplementation(async (category) => ({
      category,
      settings: { ...settings.agent, active_provider_id: providerId, providers: settings.agent.providers.map((p) => p.id === providerId ? { ...p, active_model: model } : p) },
    }))
    render(<AiPanel projectId="p" />)
    expect(screen.getByTestId('selection')).toHaveTextContent('p/A')
    fireEvent.click(screen.getByText(button))
    await waitFor(() => expect(screen.getByTestId('selection')).toHaveTextContent(`${providerId}/${model}`))
    expect(updateCategory).toHaveBeenCalledWith('agent', { model_selection: { provider_id: providerId, model } })
    expect(useSettingsStore.getState().settings?.agent.providers.find((p) => p.id === providerId)?.active_model).toBe(model)
  })

  it('blocks sending after remount while model selection is still saving', async () => {
    let resolveSave!: (value: { category: string; settings: Record<string, unknown> }) => void
    vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveSave = resolve }))
    const send = vi.fn().mockResolvedValue(undefined)
    useChatStore.setState({ send })
    const view = render(<AiPanel projectId="p" mode="preparation" />)
    fireEvent.click(screen.getByText('Choose B'))
    expect(screen.getByTestId('selection')).toHaveTextContent('p/B')
    view.unmount()
    render(<AiPanel projectId="p" mode="preparation" />)
    const input = screen.getByPlaceholderText('说说你的研究兴趣、初步想法或遇到的困难…')
    fireEvent.change(input, { target: { value: 'test question' } })
    expect(screen.getByTitle('发送')).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Choose B' })).toBeDisabled()
    expect(screen.getByText(/正在保存模型设置/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '我还没确定选题，想从兴趣方向开始梳理。' })).toBeDisabled()
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(send).not.toHaveBeenCalled()
    const saved = structuredClone(settings.agent)
    saved.providers[0].active_model = 'B'
    resolveSave({ category: 'agent', settings: { ...saved } })
    await waitFor(() => expect(screen.getByTitle('发送')).toBeEnabled())
  })

  it('blocks sending when a provider activated in settings is still saving', async () => {
    let resolveSave!: (value: { category: string; settings: Record<string, unknown> }) => void
    vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveSave = resolve }))
    const view = render(<AgentSettings />)
    fireEvent.click(screen.getByText('Other'))
    view.unmount()
    render(<AiPanel projectId="p" />)
    expect(screen.getByTestId('selection')).toHaveTextContent('q/Q')
    fireEvent.change(screen.getByPlaceholderText('问点什么…'), { target: { value: 'test' } })
    expect(screen.getByTitle('发送')).toBeDisabled()
    resolveSave({ category: 'agent', settings: { ...settings.agent, active_provider_id: 'q' } })
    await waitFor(() => expect(screen.getByTitle('发送')).toBeEnabled())
  })

  it('checks pending settings synchronously before React rerenders', async () => {
    let resolveSave!: (value: { category: string; settings: Record<string, unknown> }) => void
    vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveSave = resolve }))
    const send = vi.fn().mockResolvedValue(undefined)
    useChatStore.setState({ send })
    render(<AiPanel projectId="p" />)
    const input = screen.getByPlaceholderText('问点什么…')
    fireEvent.change(input, { target: { value: 'test' } })
    let save!: Promise<void>
    act(() => {
      save = useSettingsStore.getState().patchCategory('agent', { active_provider_id: 'q' })
      fireEvent.keyDown(input, { key: 'Enter' })
    })
    expect(send).not.toHaveBeenCalled()
    await act(async () => {
      resolveSave({ category: 'agent', settings: { ...settings.agent, active_provider_id: 'q' } })
      await save
    })
  })

  it('keeps the stop button available while a model save is pending', async () => {
    let resolveSave!: (value: { category: string; settings: Record<string, unknown> }) => void
    vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveSave = resolve }))
    useChatStore.setState({ streaming: true })
    render(<AiPanel projectId="p" />)
    let save!: Promise<void>
    act(() => { save = useSettingsStore.getState().patchCategory('agent', { active_provider_id: 'q' }) })
    expect(screen.getByTitle('停止')).toBeEnabled()
    await act(async () => {
      resolveSave({ category: 'agent', settings: { ...settings.agent, active_provider_id: 'q' } })
      await save
    })
  })

  it('reports model save failure after the panel unmounts', async () => {
    let rejectSave!: (error: Error) => void
    vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((_, reject) => { rejectSave = reject }))
    const view = render(<AiPanel projectId="p" />)
    fireEvent.click(screen.getByText('Choose B'))
    view.unmount()
    rejectSave(new Error('private detail'))
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('模型选择保存失败，已恢复原选择，请重试。'))
  })

  it('does not report an old account model save after clear', async () => {
    let rejectSave!: (error: Error) => void
    vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((_, reject) => { rejectSave = reject }))
    const patch = vi.spyOn(useSettingsStore.getState(), 'patchCategory')
    const view = render(<AiPanel projectId="p" />)
    fireEvent.click(screen.getByText('Choose B'))
    const result = expect(patch.mock.results[0].value).rejects.toMatchObject({ name: 'AbortError' })
    view.unmount()
    useSettingsStore.getState().clear()
    await act(async () => { rejectSave(new Error('old account')); await result })
    expect(toast.error).not.toHaveBeenCalled()
    patch.mockRestore()
  })

  it('reports settings load failures instead of treating them as an unconfigured model', async () => {
    useSettingsStore.setState({ settings: null, error: null })
    vi.mocked(getSettings).mockRejectedValueOnce(new Error('private database detail'))
    render(<AiPanel projectId="p" />)
    expect(await screen.findByRole('alert')).toHaveTextContent('设置加载失败')
    expect(screen.queryByText('尚未配置 AI 模型')).not.toBeInTheDocument()
    expect(screen.queryByText(/private database detail/)).not.toBeInTheDocument()
  })

  it('keeps the stop action when the last provider disappears during a run', () => {
    const cancel = vi.fn().mockResolvedValue(undefined)
    useChatStore.setState({ streaming: true, cancel })
    render(<AiPanel projectId="p" mode="preparation" />)
    act(() => useSettingsStore.setState({ settings: { ...settings, agent: { ...settings.agent, providers: [], active_provider_id: '' } } }))
    expect(screen.queryByText('尚未配置 AI 模型')).not.toBeInTheDocument()
    fireEvent.click(screen.getByTitle('停止'))
    expect(cancel).toHaveBeenCalledTimes(1)
    act(() => useChatStore.setState({ streaming: false }))
    expect(screen.getByText('尚未配置 AI 模型')).toBeInTheDocument()
    expect(screen.queryByTitle('发送')).not.toBeInTheDocument()
  })

  it('does not start a run if the last provider disappears before rerender', () => {
    const send = vi.fn().mockResolvedValue(undefined)
    useChatStore.setState({ send })
    render(<AiPanel projectId="p" />)
    const input = screen.getByPlaceholderText('问点什么…')
    fireEvent.change(input, { target: { value: 'test' } })
    act(() => {
      useSettingsStore.setState({ settings: { ...settings, agent: { ...settings.agent, providers: [], active_provider_id: '' } } })
      fireEvent.keyDown(input, { key: 'Enter' })
    })
    expect(send).not.toHaveBeenCalled()
    expect(screen.getByText('尚未配置 AI 模型')).toBeInTheDocument()
  })

  it('keeps the active model fixed while the current answer is streaming', () => {
    useChatStore.setState({ streaming: true })
    render(<AiPanel projectId="p" />)
    expect(screen.getByRole('button', { name: 'Choose B' })).toBeDisabled()
  })

  it('displays the historical default for legacy settings without an active provider or model', () => {
    const legacy = structuredClone(settings)
    legacy.agent.active_provider_id = ''
    delete (legacy.agent.providers[0] as Partial<typeof legacy.agent.providers[number]>).active_model
    useSettingsStore.setState({ settings: legacy })
    render(<AiPanel projectId="p" />)
    expect(screen.getByTestId('selection')).toHaveTextContent('p/B')
  })

  it('restores selection and reports a failed save', async () => {
    vi.mocked(updateCategory).mockRejectedValue(new Error('secret must not be displayed'))
    render(<AiPanel projectId="p" />)
    fireEvent.click(screen.getByText('Choose B'))
    await waitFor(() => expect(screen.getByTestId('selection')).toHaveTextContent('p/A'))
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('模型选择保存失败，已恢复原选择，请重试。'))
    expect(screen.queryByText(/secret must not be displayed/)).not.toBeInTheDocument()
  })
})
