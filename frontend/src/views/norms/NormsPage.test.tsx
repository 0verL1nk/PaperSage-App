import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { NormsPage } from './NormsPage'
import type { NormEntryPage, NormPack } from '@/types/norms'

const builtinPack: NormPack = {
  id: 'p-builtin',
  key: 'cn-undergrad-thesis-general-2026',
  name: '本科毕业论文通用学术规范（2026 版）',
  authority: 'standard',
  edition: '2026',
  scenario: 'undergraduate_thesis',
  isBuiltin: true,
  state: 'active',
  entryCount: 15,
}

const ownPack: NormPack = {
  id: 'p-own',
  key: 'my-lab-rules',
  name: '实验室补充要求',
  authority: 'user',
  edition: '',
  scenario: 'undergraduate_thesis',
  isBuiltin: false,
  state: 'active',
  entryCount: 0,
}

const emptyPage: NormEntryPage = { items: [], total: 0, page: 1, pageSize: 50 }
const packs: { data: NormPack[]; isLoading: boolean; error: Error | null; refetch: () => void } = {
  data: [builtinPack, ownPack],
  isLoading: false,
  error: null,
  refetch: vi.fn(),
}
const entries: { data: NormEntryPage; isLoading: boolean; error: Error | null; refetch: () => void } = {
  data: emptyPage,
  isLoading: false,
  error: null,
  refetch: vi.fn(),
}

vi.mock('@/services/norms-api', () => ({
  useNormPacks: () => packs,
  useNormEntries: () => entries,
  useNormEntry: () => ({ data: undefined, isLoading: false, error: null, refetch: vi.fn() }),
  useCreateEntry: () => ({ mutate: vi.fn(), isPending: false, error: null }),
  useUpdateEntry: () => ({ mutate: vi.fn(), isPending: false, error: null }),
  useDeleteEntry: () => ({ mutate: vi.fn(), isPending: false, error: null }),
  useCreatePack: () => ({ mutate: vi.fn(), isPending: false, error: null }),
  useDeletePack: () => ({ mutate: vi.fn(), isPending: false, error: null }),
}))

function wrap(ui: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={client}>{ui}</QueryClientProvider>
}

describe('NormsPage', () => {
  beforeEach(() => {
    packs.data = [builtinPack, ownPack]
    entries.data = emptyPage
  })

  it('列出规范包并说明内置只读', () => {
    render(wrap(<NormsPage />))
    expect(screen.getByRole('heading', { name: '规范库' })).toBeInTheDocument()
    expect(screen.getByText('本科毕业论文通用学术规范（2026 版）')).toBeInTheDocument()
    expect(screen.getByText(/内置规范包只读/)).toBeInTheDocument()
  })

  it('未选包时不能新增条目，并给出原因', () => {
    render(wrap(<NormsPage />))
    const addButton = screen.getByRole('button', { name: /新增条目/ })
    expect(addButton).toBeDisabled()
    expect(addButton).toHaveAttribute('title', expect.stringContaining('内置规范只读'))
  })

  it('选中自建包后可以打开新增条目表单', () => {
    render(wrap(<NormsPage />))
    fireEvent.click(screen.getByText('实验室补充要求'))
    const addButton = screen.getByRole('button', { name: /新增条目/ })
    expect(addButton).toBeEnabled()
    fireEvent.click(addButton)
    expect(screen.getByText('新增规范条目')).toBeInTheDocument()
    expect(screen.getByText('规则说明（必填）')).toBeInTheDocument()
  })

  it('没有条目时给出空态而不是空白列表', () => {
    render(wrap(<NormsPage />))
    // 未选包、无筛选条件：库里就是空的，不要用「没有匹配」误导用户去改筛选。
    expect(screen.getByText('这个库里还没有条目。')).toBeInTheDocument()
  })

  it('自建包为空时提示去新增条目，而不是让用户换筛选条件', () => {
    render(wrap(<NormsPage />))
    fireEvent.click(screen.getByText('实验室补充要求'))
    expect(screen.getByText(/这个包还没有条目/)).toBeInTheDocument()
  })
})
