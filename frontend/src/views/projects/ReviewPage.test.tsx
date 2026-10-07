import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

const mutate = vi.fn()
const state = {
  project_id: 'project-1',
  scenario: 'undergraduate_thesis' as const,
  brief: {
    topic: '可解释的论文辅助', background: '', questions: ['如何验证效果？'], goals: [],
    scope_out: [], objects_and_data: '', methods: '', key_chapters: [], milestones: [],
    constraints: [], known_facts: [], assumptions: [], unverified: [],
  },
  requirements: [],
  phases: { prepare: 'in_progress' as const, write: 'not_started' as const, review: 'not_started' as const },
  current_phase: 'review' as const,
  revision: 2,
}

vi.mock('@tanstack/react-router', () => ({ useParams: () => ({ projectId: 'project-1' }) }))
vi.mock('@/services/writing-state-api', () => ({
  useWritingState: () => ({ data: state, isLoading: false, error: null }),
  useSaveWritingState: () => ({ mutate, isPending: false }),
}))

import { ReviewPage } from './ReviewPage'

function Wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

describe('ReviewPage', () => {
  beforeEach(() => mutate.mockReset())

  it('shows manual phase progress and states that it does not fabricate review results', () => {
    render(<Wrapper><ReviewPage /></Wrapper>)
    expect(screen.getByRole('heading', { name: '完成情况由你确认' })).toBeInTheDocument()
    expect(screen.getByText(/不会自动判定论文质量/)).toBeInTheDocument()
    expect(screen.getByText('可解释的论文辅助')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /点击切换/ })).toHaveLength(3)
  })

  it('persists the next selected manual phase status with the current revision', () => {
    render(<Wrapper><ReviewPage /></Wrapper>)
    fireEvent.click(screen.getByRole('button', { name: /写作准备状态/ }))
    expect(mutate).toHaveBeenCalledWith(expect.objectContaining({
      phases: { ...state.phases, prepare: 'done' },
      current_phase: 'review',
      revision: 2,
    }), expect.any(Object))
  })
})
