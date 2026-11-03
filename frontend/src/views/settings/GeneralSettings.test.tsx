import type { ReactNode } from 'react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { useSettingsStore } from '@/store/settings-store'
import { toast } from 'sonner'
vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))
import type { AllSettings, CategorySettings } from '@/services/settings-api'

// Mock the store before importing the component
vi.mock('@/store/settings-store', async () => {
  const actual = await vi.importActual<typeof import('@/store/settings-store')>(
    '@/store/settings-store',
  )
  return {
    ...actual,
    useSettingsStore: vi.fn(),
  }
})

// Replace the Radix Select with a test-friendly version that exposes
// each option as a clickable button.
type SelectItemProps = {
  value: string
  children: ReactNode
}
type SelectContentProps = {
  children: ReactNode
}
type SelectRootProps = {
  value: string
  onValueChange?: (value: string) => void
  children: ReactNode
}

vi.mock('@/components/ui/select', () => {
  // Recreate the structure of the real component: Select wraps everything,
  // SelectContent wraps items, SelectItem is what we click. Walking the
  // tree and rendering a button per item mirrors onValueChange behaviour.
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

  const Select = ({ value, onValueChange, children }: SelectRootProps) => {
    const items = flattenItems(children)
    return (
      <div data-testid="mock-select" data-value={value}>
        {items.map((item) => (
          <button
            key={item.value}
            data-testid={`select-option-${item.value}`}
            onClick={() => onValueChange?.(item.value)}
            type="button"
          >
            {item.children}
          </button>
        ))}
      </div>
    )
  }
  Select.displayName = 'Select'

  const SelectTrigger = ({ children }: { children?: ReactNode }) => <div>{children}</div>
  SelectTrigger.displayName = 'SelectTrigger'

  const SelectValue = ({ children }: { children?: ReactNode }) => <span>{children}</span>
  SelectValue.displayName = 'SelectValue'

  const SelectContent = ({ children }: SelectContentProps) => <>{children}</>
  SelectContent.displayName = 'SelectContent'

  const SelectItem = ({ children }: SelectItemProps) => <>{children}</>
  SelectItem.displayName = 'SelectItem'

  return { Select, SelectTrigger, SelectValue, SelectContent, SelectItem }
})

// Stub other UI components we don't exercise here
vi.mock('@/components/ui/card', () => ({
  Card: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  CardHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  CardTitle: ({ children }: { children: ReactNode }) => <h3>{children}</h3>,
  CardContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))
vi.mock('@/components/ui/label', () => ({
  Label: ({ children, htmlFor }: { children: ReactNode; htmlFor?: string }) => (
    <label htmlFor={htmlFor}>{children}</label>
  ),
}))
vi.mock('@/components/ui/checkbox', () => ({
  Checkbox: ({ onCheckedChange }: { onCheckedChange?: (v: boolean) => void }) => (
    <button
      data-testid="checkbox"
      type="button"
      onClick={() => onCheckedChange?.(true)}
    >
      toggle
    </button>
  ),
}))

import { GeneralSettings } from './GeneralSettings'

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

type StoreState = ReturnType<typeof useSettingsStore.getState>

function installMockStore(): ReturnType<typeof vi.fn> {
  const patchCategory = vi.fn().mockResolvedValue(undefined)
  vi.mocked(useSettingsStore).mockImplementation((selector?: (s: StoreState) => unknown) => {
    const state: StoreState = {
      settings: fixtureSettings,
      loading: false,
      error: null,
      pendingAgentModelChanges: 0,
      fetchSettings: vi.fn(),
      patchCategory,
      resetCategory: vi.fn(),
      clear: vi.fn(),
    }
    return (selector ? selector(state) : state) as ReturnType<typeof useSettingsStore>
  })
  return patchCategory
}

describe('GeneralSettings', () => {
  beforeEach(() => {
    vi.mocked(useSettingsStore).mockReset()
  })

  it('renders nothing when ui settings are missing', () => {
    const settingsWithoutUi: AllSettings = {
      ...fixtureSettings,
      ui: undefined as unknown as AllSettings['ui'],
    }
    vi.mocked(useSettingsStore).mockImplementation((selector?: (s: StoreState) => unknown) => {
      const state: StoreState = {
        settings: settingsWithoutUi,
        loading: false,
        error: null,
        pendingAgentModelChanges: 0,
        fetchSettings: vi.fn(),
        patchCategory: vi.fn().mockResolvedValue(undefined),
        resetCategory: vi.fn(),
        clear: vi.fn(),
      }
      return (selector ? selector(state) : state) as ReturnType<typeof useSettingsStore>
    })
    const { container } = render(<GeneralSettings />)
    expect(container.firstChild).toBeNull()
  })

  it('shows the current theme value', () => {
    installMockStore()
    render(<GeneralSettings />)
    const selects = document.querySelectorAll('[data-testid="mock-select"]')
    const themeSelect = Array.from(selects).find(
      (el) => el.getAttribute('data-value') === 'system',
    )
    expect(themeSelect).toBeTruthy()
  })

  it('calls patchCategory with the new theme when Dark is selected', async () => {
    const patchCategory = installMockStore()
    const user = userEvent.setup()
    render(<GeneralSettings />)

    await user.click(screen.getByTestId('select-option-dark'))

    expect(patchCategory).toHaveBeenCalledWith('ui', { theme: 'dark' })
  })

  it('calls patchCategory with the new language when English is selected', async () => {
    const patchCategory = installMockStore()
    const user = userEvent.setup()
    render(<GeneralSettings />)

    await user.click(screen.getByTestId('select-option-en-US'))

    const expected: Partial<CategorySettings> = { language: 'en-US' }
    expect(patchCategory).toHaveBeenCalledWith('ui', expected)
  })

  it('shows a visible error when saving fails', async () => {
    const patchCategory = installMockStore()
    patchCategory.mockRejectedValueOnce(new Error('save failed'))
    render(<GeneralSettings />)
    await userEvent.setup().click(screen.getByTestId('select-option-dark'))
    expect(toast.error).toHaveBeenCalled()
  })

  it('calls patchCategory when the sidebar checkbox is toggled', async () => {
    const patchCategory = installMockStore()
    const user = userEvent.setup()
    render(<GeneralSettings />)

    await user.click(screen.getByTestId('checkbox'))

    const expected: Partial<CategorySettings> = { sidebar_collapsed: true }
    expect(patchCategory).toHaveBeenCalledWith('ui', expected)
  })
})
