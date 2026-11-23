import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { useSessionStore } from '@/store'

// react-pdf pulls in pdfjs, which needs DOMMatrix that jsdom does not provide.
// Same mock as the other suites that render through the PDF layer.
vi.mock('react-pdf', () => ({
  pdfjs: {
    getDocument: vi.fn(),
    GlobalWorkerOptions: {},
    PDFDataRangeTransport: class PDFDataRangeTransport {},
    version: '5.4.296',
  },
}))

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
const writingStateResult: { data: typeof state | undefined; isLoading: boolean; error: Error | null; refetch: () => void } = {
  data: state, isLoading: false, error: null, refetch: vi.fn(),
}

vi.mock('@/services/writing-state-api', () => ({
  useWritingState: () => writingStateResult,
  useSaveWritingState: () => ({ mutate, isPending: false }),
  getConflictState: (error: unknown) => error instanceof Error && 'details' in error ? (error as Error & { details: { data: typeof state } }).details.data : null,
}))
vi.mock('./ai-panel', () => ({
  AiPanel: ({ mode }: { mode: string }) => <div data-testid="research-agent" data-mode={mode} />,
}))
// 研究产物面板会经 use-latex 拉入 react-pdf，jsdom 没有 DOMMatrix；本文件只验证准备页本身。
vi.mock('@/components/domain/ResearchArtifactsPanel', () => ({
  ResearchArtifactsPanel: () => <div data-testid="research-artifacts" />,
}))

import { PreparePage } from './PreparePage'

function Wrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
}

describe('PreparePage', () => {
  it('shows conversational preparation and derived next steps without field-by-field inputs', () => {
    render(<Wrapper><PreparePage /></Wrapper>)
    expect(screen.getByTestId('research-agent')).toHaveAttribute('data-mode', 'preparation')
    expect(screen.getByText('和研究顾问一起理清思路')).toBeInTheDocument()
    expect(screen.getByText('找到一个值得研究的问题')).toBeInTheDocument()
    expect(screen.getByText('还没有研究蓝图。先从你感兴趣的方向聊起，不需要逐项填写表格。')).toBeInTheDocument()
    expect(screen.queryByLabelText('研究主题')).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: '进入资料库' })).toHaveAttribute('href', '/project/$projectId/uploading')
  })

  it('renders the existing research brief as an agent-maintained summary', () => {
    state.brief.topic = '校园阅读习惯研究'
    render(<Wrapper><PreparePage /></Wrapper>)
    expect(screen.getByText('校园阅读习惯研究')).toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    state.brief.topic = ''
  })

  it('shows an actionable error with retry when preparation data cannot load', () => {
    writingStateResult.data = undefined
    writingStateResult.error = new Error('Network request failed.')
    render(<Wrapper><PreparePage /></Wrapper>)

    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('暂时打不开这个项目的准备内容')
    expect(alert).toHaveTextContent('网络连接失败，请检查网络后重试。')
    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(writingStateResult.refetch).toHaveBeenCalled()

    writingStateResult.data = state
    writingStateResult.error = null
  })

  it('does not pretend the local guest workspace has an AI agent', () => {
    useSessionStore.setState((current) => ({ ...current, isGuest: true }))
    render(<Wrapper><PreparePage /></Wrapper>)
    expect(screen.getByText('研究顾问需要登录后使用')).toBeInTheDocument()
    expect(screen.queryByTestId('research-agent')).not.toBeInTheDocument()
    useSessionStore.setState((current) => ({ ...current, isGuest: false }))
  })
})
