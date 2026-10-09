import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AiPanel } from '@/views/projects/ai-panel'
import { useSettingsStore } from '@/store/settings-store'
import { useChatStore } from '@/store/chat-store'
import { emptyChatState } from '@/store/chat-store'

const query = vi.hoisted(() => ({ evidenceData: [] as Array<Record<string, string>>, evidenceLoading: false, evidenceError: false, refetch: vi.fn() }))

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('@tanstack/react-query', () => ({
  useQuery: (options: { queryKey: unknown[] }) => options.queryKey[0] === 'agent-run-evidence'
    ? { data: query.evidenceData, isLoading: query.evidenceLoading, isError: query.evidenceError, refetch: query.refetch }
    : { data: [], refetch: vi.fn() },
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}))

beforeEach(() => {
  useChatStore.setState({ ...emptyChatState })
  query.evidenceData = []
  query.evidenceLoading = false
  query.evidenceError = false
  query.refetch.mockClear()
  // jsdom 没实现 scrollIntoView；面板挂载时会调用它
  Element.prototype.scrollIntoView = vi.fn()
})

describe('AiPanel', () => {
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
