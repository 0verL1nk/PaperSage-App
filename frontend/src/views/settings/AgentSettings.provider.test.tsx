import type { ReactNode } from 'react'
import { beforeEach, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import i18n from '@/i18n'
import { AgentSettings } from './AgentSettings'
import { useSettingsStore } from '@/store/settings-store'
import { testAgentConnection } from '@/services/settings-api'
import type { AllSettings } from '@/services/settings-api'

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))
vi.mock('@/services/settings-api', () => ({
  getSettings: vi.fn(),
  updateCategory: vi.fn(),
  resetCategory: vi.fn(),
  testAgentConnection: vi.fn(),
}))

// Radix Select does not open reliably in jsdom; expose each option as a button.
type SelectItemProps = { value: string; children: ReactNode }
vi.mock('@/components/ui/select', () => {
  const flattenItems = (node: ReactNode): SelectItemProps[] => {
    const out: SelectItemProps[] = []
    const walk = (n: ReactNode) => {
      if (Array.isArray(n)) {
        n.forEach(walk)
      } else if (n && typeof n === 'object' && 'props' in n) {
        const el = n as { type?: { displayName?: string }; props?: { value?: string; children?: ReactNode } }
        if (el.type?.displayName === 'SelectItem' && el.props?.value !== undefined) {
          out.push({ value: el.props.value, children: el.props.children as ReactNode })
        } else if (el.props?.children) {
          walk(el.props.children)
        }
      }
    }
    walk(node)
    return out
  }
  const Select = ({ onValueChange, children }: { value?: string; onValueChange?: (v: string) => void; children: ReactNode }) => (
    <div data-testid="mock-select">
      {flattenItems(children).map((item) => (
        <button key={item.value} type="button" data-testid={`select-option-${item.value}`} onClick={() => onValueChange?.(item.value)}>
          {item.children}
        </button>
      ))}
    </div>
  )
  Select.displayName = 'Select'
  const SelectTrigger = ({ children }: { children?: ReactNode }) => <div>{children}</div>
  SelectTrigger.displayName = 'SelectTrigger'
  const SelectValue = ({ children }: { children?: ReactNode }) => <span>{children}</span>
  SelectValue.displayName = 'SelectValue'
  const SelectContent = ({ children }: { children?: ReactNode }) => <>{children}</>
  SelectContent.displayName = 'SelectContent'
  const SelectItem = ({ children }: SelectItemProps) => <>{children}</>
  SelectItem.displayName = 'SelectItem'
  return { Select, SelectTrigger, SelectValue, SelectContent, SelectItem }
})

const settings: AllSettings = {
  ui: { theme: 'system', language: 'zh-CN', sidebar_collapsed: false },
  editor: { font_size: 14, tab_size: 2, auto_save: true, auto_save_interval: 30, word_wrap: true },
  agent: {
    active_provider_id: '',
    temperature: 0.7,
    writing_style: 'academic',
    memory_enabled: true,
    providers: [],
  },
}

const originalPatch = useSettingsStore.getState().patchCategory

beforeEach(() => {
  void i18n.changeLanguage('zh-CN')
  useSettingsStore.getState().clear()
  useSettingsStore.setState({ settings: structuredClone(settings), error: null, patchCategory: originalPatch })
  vi.mocked(testAgentConnection).mockReset()
})

it('shows the configured server fallback model when no provider exists', () => {
  useSettingsStore.setState({
    settings: {
      ...structuredClone(settings),
      agent: { ...settings.agent, server_fallback: { configured: true, model: 'deepseek-chat' } },
    },
  })
  render(<AgentSettings />)
  expect(screen.getAllByText('服务端已配置默认模型：deepseek-chat').length).toBeGreaterThan(0)
})

it('says the server fallback is missing when nothing is configured', () => {
  render(<AgentSettings />)
  expect(screen.getByText('服务端也未配置默认模型。')).toBeInTheDocument()
})

it('fills the base URL and suggested model when a preset is selected', () => {
  render(<AgentSettings />)
  fireEvent.click(screen.getByRole('button', { name: '添加服务商' }))
  fireEvent.click(screen.getByTestId('select-option-deepseek'))

  expect(screen.getByPlaceholderText('https://api.openai.com/v1')).toHaveValue('https://api.deepseek.com/v1')
  expect(screen.getByRole('button', { name: /deepseek-chat/ })).toBeInTheDocument()
})

it('reports a successful connection test without saving the key', async () => {
  vi.mocked(testAgentConnection).mockResolvedValue({ ok: true, models: ['m1', 'm2'] })
  render(<AgentSettings />)
  fireEvent.click(screen.getByRole('button', { name: '添加服务商' }))
  fireEvent.click(screen.getByTestId('select-option-deepseek'))
  fireEvent.change(screen.getByPlaceholderText('sk-...'), { target: { value: 'secret-key' } })

  fireEvent.click(screen.getByRole('button', { name: '测试连接' }))

  await waitFor(() => expect(screen.getByText('连接成功，发现 2 个模型')).toBeInTheDocument())
  expect(testAgentConnection).toHaveBeenCalledWith({
    base_url: 'https://api.deepseek.com/v1',
    api_key: 'secret-key',
    model: 'deepseek-chat',
  })
})

it('shows the failure message without echoing the key', async () => {
  vi.mocked(testAgentConnection).mockRejectedValue(new Error('provider returned HTTP 401'))
  render(<AgentSettings />)
  fireEvent.click(screen.getByRole('button', { name: '添加服务商' }))
  fireEvent.click(screen.getByTestId('select-option-deepseek'))
  fireEvent.change(screen.getByPlaceholderText('sk-...'), { target: { value: 'secret-key' } })

  fireEvent.click(screen.getByRole('button', { name: '测试连接' }))

  await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('provider returned HTTP 401'))
  expect(screen.getByRole('alert').textContent).not.toContain('secret-key')
})
