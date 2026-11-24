import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { NormLibraryPanel } from './NormLibraryPanel'
import type { NormAdoption, NormEntry } from '@/types/norms'

const projectNorms: { data: NormAdoption[]; isLoading: boolean; error: Error | null; refetch: () => void } = {
  data: [],
  isLoading: false,
  error: null,
  refetch: vi.fn(),
}

vi.mock('@/services/norms-api', async () => {
  const actual = await vi.importActual<typeof import('@/services/norms-api')>('@/services/norms-api')
  return {
    adoptionIndex: actual.adoptionIndex,
    adoptionSummary: actual.adoptionSummary,
    useProjectNorms: () => projectNorms,
    useNormPacks: () => ({ data: [], isLoading: false, error: null }),
    useNormEntries: () => ({
      data: { items: [], total: 0, page: 1, pageSize: 50 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    }),
    useAdoptNorms: () => ({ mutate: vi.fn(), isPending: false, error: null }),
    useUpdateAdoption: () => ({ mutate: vi.fn(), isPending: false, error: null }),
    useUnadoptNorm: () => ({ mutate: vi.fn(), isPending: false, error: null }),
  }
})

function wrap(ui: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={client}>{ui}</QueryClientProvider>
}

function adoptionOf(overrides: Partial<NormEntry> & { entryId: string; state: 'enabled' | 'disabled'; hasNewerVersion?: boolean; latestVersion?: number }): NormAdoption {
  const { entryId, state, hasNewerVersion = false, latestVersion = overrides.version ?? 1, ...entryOverrides } = overrides
  return {
    entryId,
    version: 1,
    state,
    hasNewerVersion,
    latestVersion,
    entry: {
      id: entryId,
      packId: 'p1',
      packName: '本科毕业论文通用学术规范（2026 版）',
      code: 'GBT7714-TYPE-MARKS',
      category: '参考文献',
      ruleKind: 'official',
      binding: 'must',
      scenario: 'undergraduate_thesis',
      version: 1,
      sortOrder: 10,
      state: 'active',
      title: '按文献类型标注类型标识',
      ruleText: '期刊文章用 [J] 标注。',
      sourceRef: 'GB/T 7714—2015',
      applicableSections: ['参考文献'],
      tags: [],
      isBuiltin: true,
      ...entryOverrides,
    },
  }
}

describe('NormLibraryPanel', () => {
  it('没有采用规范时说明影响，而不是留空白', () => {
    projectNorms.data = []
    projectNorms.error = null
    render(wrap(<NormLibraryPanel projectId="project-1" />))
    expect(screen.getByText('学术规范')).toBeInTheDocument()
    expect(screen.getByText(/还没有采用任何学术规范/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '管理' })).toBeInTheDocument()
  })

  it('已采用时给出条数与分类，并提示有新版本', () => {
    projectNorms.data = [
      adoptionOf({ entryId: 'e1', state: 'enabled' }),
      adoptionOf({ entryId: 'e2', state: 'disabled', hasNewerVersion: true, latestVersion: 2, category: '结构逻辑' }),
    ]
    projectNorms.error = null
    render(wrap(<NormLibraryPanel projectId="project-1" />))
    expect(screen.getByText('已采用 2 条 · 已启用 1 条')).toBeInTheDocument()
    expect(screen.getByText(/1 条规范有新版本/)).toBeInTheDocument()
  })

  it('读取失败时给出可重试的错误态', () => {
    projectNorms.data = []
    projectNorms.error = new Error('network down')
    render(wrap(<NormLibraryPanel projectId="project-1" />))
    expect(screen.getByRole('alert')).toHaveTextContent(/没读到已采用的规范/)
    expect(screen.getByRole('button', { name: '重试' })).toBeInTheDocument()
    projectNorms.error = null
  })
})
