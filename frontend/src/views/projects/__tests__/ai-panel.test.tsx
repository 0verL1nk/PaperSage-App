import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AiPanel } from '@/views/projects/ai-panel'
import { useSettingsStore } from '@/store/settings-store'
import { useChatStore } from '@/store/chat-store'
import { emptyChatState } from '@/store/chat-store'

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: [], refetch: vi.fn() }),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}))

beforeEach(() => {
  useChatStore.setState({ ...emptyChatState })
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
})
