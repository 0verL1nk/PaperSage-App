import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import * as agentApi from '@/services/agent-api'
import { useSessionStore } from '@/store/session-store'
import type { AllSettings } from '@/services/settings-api'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AiPanel } from '@/views/projects/ai-panel'
import { useSettingsStore } from '@/store/settings-store'
import { useChatStore } from '@/store/chat-store'
import { emptyChatState } from '@/store/chat-store'

const query = vi.hoisted(() => ({ real: false, evidenceData: [] as Array<Record<string, string>>, evidenceLoading: false, evidenceError: false, refetch: vi.fn() }))

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('@tanstack/react-query', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-query')>()
  return {
    ...actual,
    useQuery: (options: Parameters<typeof actual.useQuery>[0]) => query.real
      ? actual.useQuery(options)
      : options.queryKey[0] === 'agent-run-evidence'
        ? { data: query.evidenceData, isLoading: query.evidenceLoading, isError: query.evidenceError, refetch: query.refetch }
        : { data: [], refetch: vi.fn() },
    useQueryClient: () => query.real ? actual.useQueryClient() : { invalidateQueries: vi.fn() },
  }
})

const originalFetchSettings = useSettingsStore.getState().fetchSettings
const originalSend = useChatStore.getState().send

beforeEach(() => {
  vi.restoreAllMocks()
  query.real = false
  useSessionStore.setState({ isGuest: false })
  useSettingsStore.setState({ fetchSettings: originalFetchSettings, error: null })
  useChatStore.setState({ ...emptyChatState, send: originalSend })
  query.evidenceData = []
  query.evidenceLoading = false
  query.evidenceError = false
  query.refetch.mockClear()
  // jsdom 没实现 scrollIntoView；面板挂载时会调用它
  Element.prototype.scrollIntoView = vi.fn()
})

const noProviderSettings: AllSettings = {
  ui: { theme: 'system', language: 'zh-CN', sidebar_collapsed: false },
  editor: { font_size: 14, tab_size: 2, auto_save: true, auto_save_interval: 30, word_wrap: true },
  agent: { providers: [], active_provider_id: '', temperature: 0.7, writing_style: 'academic', memory_enabled: true },
}

function renderWithRealQuery() {
  query.real = true
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><AiPanel projectId="p1" mode="preparation" /></QueryClientProvider>)
}

describe('AiPanel', () => {
  it('reads and switches history without a model provider', async () => {
    useSettingsStore.setState({ settings: structuredClone(noProviderSettings), error: null })
    const list = vi.spyOn(agentApi, 'listConversations').mockResolvedValue([{ id: 'c1', title: 'First history' }, { id: 'c2', title: 'Second history' }])
    vi.spyOn(agentApi, 'listMessages').mockImplementation(async (id) => [{ id, role: 'user', status: 'completed', createdAt: '2025-01-01T00:00:00Z', parts: [{ id: 'p', ordinal: 0, type: 'text', status: 'completed', createdAt: '2025-01-01T00:00:00Z', text: `History ${id}` }] }])
    const send = vi.spyOn(useChatStore.getState(), 'send')
    renderWithRealQuery()
    expect(await screen.findByText('History c1')).toBeInTheDocument()
    expect(list).toHaveBeenCalledWith('p1')
    expect(screen.getByText('尚未配置 AI 模型')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '去设置' })).toBeInTheDocument()
    const input = screen.getByPlaceholderText('说说你的研究兴趣、初步想法或遇到的困难…')
    expect(input).toBeDisabled()
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(send).not.toHaveBeenCalled()
    fireEvent.pointerDown(screen.getByRole('button', { name: /First history/ }), { button: 0, ctrlKey: false })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Second history' }))
    expect(await screen.findByText('History c2')).toBeInTheDocument()
  })

  it('keeps the setup prompt when there is no provider or history', async () => {
    useSettingsStore.setState({ settings: structuredClone(noProviderSettings), error: null })
    const list = vi.spyOn(agentApi, 'listConversations').mockResolvedValue([])
    renderWithRealQuery()
    await waitFor(() => expect(list).toHaveBeenCalledWith('p1'))
    expect(await screen.findByText('尚未配置 AI 模型')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '去设置' })).toBeInTheDocument()
  })

  it('shows a retry when the history list fails instead of claiming it is empty', async () => {
    useSettingsStore.setState({ settings: structuredClone(noProviderSettings), error: null })
    const list = vi.spyOn(agentApi, 'listConversations').mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce([])
    renderWithRealQuery()
    expect(await screen.findByText('会话加载失败，请重试。')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '重试会话' }))
    await waitFor(() => expect(list).toHaveBeenCalledTimes(2))
    expect(await screen.findByText('尚未配置 AI 模型')).toBeInTheDocument()
  })

  it('shows history even if settings failed to load', async () => {
    const fetchSettings = vi.fn().mockRejectedValue(new Error('offline'))
    useSettingsStore.setState({ settings: null, error: 'offline', fetchSettings })
    vi.spyOn(agentApi, 'listConversations').mockResolvedValue([{ id: 'c1', title: 'History' }])
    vi.spyOn(agentApi, 'listMessages').mockResolvedValue([{ id: 'm1', role: 'user', status: 'completed', createdAt: '2025-01-01T00:00:00Z', parts: [{ id: 'p', ordinal: 0, type: 'text', status: 'completed', createdAt: '2025-01-01T00:00:00Z', text: 'Saved message' }] }])
    renderWithRealQuery()
    expect(await screen.findByText('Saved message')).toBeInTheDocument()
    expect(screen.getByText('设置加载失败，请重试。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '重试设置' })).toBeInTheDocument()
    expect(screen.getByPlaceholderText('说说你的研究兴趣、初步想法或遇到的困难…')).toBeDisabled()
  })

  it('does not request account history in guest mode', async () => {
    useSessionStore.setState({ isGuest: true })
    useSettingsStore.setState({ settings: structuredClone(noProviderSettings), error: null })
    const list = vi.spyOn(agentApi, 'listConversations')
    renderWithRealQuery()
    await act(async () => { await Promise.resolve() })
    expect(list).not.toHaveBeenCalled()
    expect(screen.getByText('登录后可使用 AI 助手')).toBeInTheDocument()
  })

  it('settings 未加载时面板自己发起加载（否则永远停在「尚未配置」）', () => {
    // 回归：面板曾依赖 ModelSelector 去加载 settings，而 ModelSelector 只在
    // hasProvider 为真时渲染 → 没设置就永远不加载，用户永远看到「尚未配置」。
    const fetchSettings = vi.fn().mockResolvedValue(undefined)
    useSettingsStore.setState({ settings: null, fetchSettings } as never)

    render(<AiPanel projectId="p1" />)

    expect(fetchSettings).toHaveBeenCalled()
  })

  it('未配置模型时给出设置入口，而不是一个发不出去的空输入框', () => {
    useSettingsStore.setState({
      settings: { ui: {}, editor: {}, agent: { providers: [], active_provider_id: '' } },
    } as never)

    render(<AiPanel projectId="p1" />)

    expect(screen.getByText('尚未配置 AI 模型')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '去设置' })).toBeInTheDocument()
    // 关键：这种情况下不该出现输入框，避免用户白打一段话
    expect(screen.queryByPlaceholderText('问点什么…')).not.toBeInTheDocument()
  })

  it('已配置模型时展示输入框与发送按钮', () => {
    useSettingsStore.setState({
      settings: {
        ui: {},
        editor: {},
        agent: { providers: [{ id: 'p', name: 'P', models: ['m'], active_model: 'm' }], active_provider_id: 'p' },
      },
    } as never)

    render(<AiPanel projectId="p1" />)

    expect(screen.getByPlaceholderText('问点什么…')).toBeInTheDocument()
    expect(screen.queryByText('尚未配置 AI 模型')).not.toBeInTheDocument()
  })

  it('写作准备模式提供对话入口和研究起步提示', () => {
    useSettingsStore.setState({
      settings: {
        ui: {},
        editor: {},
        agent: { providers: [{ id: 'p', name: 'P', models: ['m'], active_model: 'm' }], active_provider_id: 'p' },
      },
    } as never)

    render(<AiPanel projectId="p1" mode="preparation" />)

    expect(screen.getByPlaceholderText('说说你的研究兴趣、初步想法或遇到的困难…')).toBeInTheDocument()
    expect(screen.getByText(/一次聚焦一个问题/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /我还没确定选题/ })).toBeInTheDocument()
  })

  it('历史助手消息展示可展开的参考材料和原文摘录', async () => {
    useSettingsStore.setState({ settings: { ui: {}, editor: {}, agent: { providers: [{ id: 'p', name: 'P', models: ['m'], active_model: 'm' }], active_provider_id: 'p' } } } as never)
    useChatStore.setState({
      ...emptyChatState,
      conversationId: 'c1',
      messages: [{ id: 'm1', runId: 'r1', role: 'assistant', status: 'completed', parts: [{ id: 'text', type: 'text', text: '回答', status: 'completed' }] }],
    })
    query.evidenceData = [{ id: 'e1', sourceName: '研究计划.pdf', quote: '原文摘录' }]

    render(<AiPanel projectId="p1" />)

    const disclosure = await screen.findByText('参考材料 · 1')
    expect(disclosure).toBeInTheDocument()
    expect(screen.getByText('研究计划.pdf')).toBeInTheDocument()
    expect(screen.getByText('原文摘录')).toBeInTheDocument()
  })

  it('证据加载失败时提供重试入口', () => {
    useSettingsStore.setState({ settings: { ui: {}, editor: {}, agent: { providers: [{ id: 'p', name: 'P', models: ['m'], active_model: 'm' }], active_provider_id: 'p' } } } as never)
    useChatStore.setState({
      ...emptyChatState,
      messages: [{ id: 'm1', runId: 'r1', role: 'assistant', status: 'completed', parts: [{ id: 'text', type: 'text', text: '回答', status: 'completed' }] }],
    })
    query.evidenceError = true

    render(<AiPanel projectId="p1" />)

    expect(screen.getByText('依据暂时无法读取')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '重试' })).toBeInTheDocument()
  })

  it('区分证据加载中和无证据的成功结果', () => {
    useSettingsStore.setState({ settings: { ui: {}, editor: {}, agent: { providers: [{ id: 'p', name: 'P', models: ['m'], active_model: 'm' }], active_provider_id: 'p' } } } as never)
    useChatStore.setState({
      ...emptyChatState,
      messages: [{ id: 'm1', runId: 'r1', role: 'assistant', status: 'completed', parts: [{ id: 'text', type: 'text', text: '回答', status: 'completed' }] }],
    })
    query.evidenceLoading = true
    const { rerender } = render(<AiPanel projectId="p1" />)
    expect(screen.getByText('正在读取本次回答使用的依据…')).toBeInTheDocument()

    query.evidenceLoading = false
    rerender(<AiPanel projectId="p1" />)
    expect(screen.queryByText('参考材料 · 0')).not.toBeInTheDocument()
    expect(screen.queryByText('依据暂时无法读取')).not.toBeInTheDocument()
  })
})
