import { act, fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mutate = vi.fn()
const state = {
  project_id: 'project-1', scenario: 'undergraduate_thesis' as const,
  brief: {
    topic: '', background: '', questions: [], goals: [], scope_out: [], objects_and_data: '', methods: '',
    key_chapters: [], milestones: [], constraints: [], known_facts: [], assumptions: [], unverified: [],
  },
  requirements: [],
  phases: { prepare: 'in_progress' as const, write: 'not_started' as const, review: 'not_started' as const },
  current_phase: 'prepare' as const, revision: 0,
}

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ projectId: 'project-1' }),
  Link: ({ children, to, ...props }: { children: ReactNode; to: string; [key: string]: unknown }) => <a href={to} {...props}>{children}</a>,
}))
vi.mock('@/services/files-api', () => ({ useDocuments: () => ({ data: [] }) }))
vi.mock('@/services/writing-state-api', () => ({
  useWritingState: () => ({ data: state, isLoading: false, error: null }),
  useSaveWritingState: () => ({ mutate, isPending: false }),
  getConflictState: (error: unknown) => error instanceof Error && 'details' in error ? (error as Error & { details: { data: typeof state } }).details.data : null,
}))

import { PreparePage } from './PreparePage'

function Wrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
}

describe('PreparePage', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mutate.mockReset()
  })

  afterEach(() => vi.useRealTimers())

  it('shows derived missing items without blocking access to work', () => {
    render(<Wrapper><PreparePage /></Wrapper>)
    expect(screen.getByText('补充研究主题')).toBeInTheDocument()
    expect(screen.getByText('明确核心研究问题')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /进入资料库/ })).toHaveAttribute('href', '/project/$projectId/uploading')
    expect(screen.getByText('待补项')).toBeInTheDocument()
  })

  it('debounces and saves the edited blueprint with its current revision', () => {
    render(<Wrapper><PreparePage /></Wrapper>)
    fireEvent.change(screen.getByPlaceholderText('用一句话概括研究对象与问题'), { target: { value: '新的研究主题' } })
    expect(mutate).not.toHaveBeenCalled()
    act(() => vi.advanceTimersByTime(799))
    expect(mutate).not.toHaveBeenCalled()
    act(() => vi.advanceTimersByTime(1))
    expect(mutate).toHaveBeenCalledWith(expect.objectContaining({
      brief: expect.objectContaining({ topic: '新的研究主题' }), revision: 0, current_phase: 'prepare',
    }), expect.any(Object))
  })

  it('keeps local input and offers latest or overwrite actions on a revision conflict', () => {
    const latest = { ...state, brief: { ...state.brief, topic: '其他标签页主题' }, revision: 4 }
    const conflictError = Object.assign(new Error('conflict'), { details: { data: latest } })
    mutate.mockImplementation((_request, options: { onError: (error: Error) => void }) => options.onError(conflictError))
    render(<Wrapper><PreparePage /></Wrapper>)
    const topic = screen.getByPlaceholderText('用一句话概括研究对象与问题') as HTMLInputElement
    fireEvent.change(topic, { target: { value: '当前输入仍需保留' } })
    act(() => vi.advanceTimersByTime(800))
    expect(screen.getByRole('alert')).toHaveTextContent('内容已在其他位置更新')
    expect(topic).toHaveValue('当前输入仍需保留')
    fireEvent.click(screen.getByRole('button', { name: '加载最新版本' }))
    expect(topic).toHaveValue('其他标签页主题')
  })
})
