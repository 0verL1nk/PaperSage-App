import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { RecheckMarksPanel } from './RecheckMarksPanel'
import type { RecheckMark } from '@/types/research'

const resolveMark = vi.fn().mockResolvedValue({})
const refetch = vi.fn()
const query: { marks: RecheckMark[]; isLoading: boolean; error: Error | null } = {
  marks: [],
  isLoading: false,
  error: null,
}

vi.mock('@/services/recheck-api', () => ({
  useRecheckMarks: () => ({ data: query.marks, isLoading: query.isLoading, error: query.error, refetch }),
  useResolveRecheckMark: () => ({ mutateAsync: resolveMark, isPending: false }),
}))

function mark(overrides: Partial<RecheckMark> = {}): RecheckMark {
  return {
    id: 'm1',
    projectId: 'p1',
    targetType: 'artifact_revision',
    targetId: 'r1',
    targetTitle: '章节计划（1 节）',
    targetDetail: '修订 v1',
    targetMissing: false,
    triggerKind: 'source_deleted',
    triggerLabel: '资料已删除',
    reason: '资料《论文.pdf》已删除',
    createdAt: '2026-11-26T00:00:00Z',
    ...overrides,
  }
}

describe('RecheckMarksPanel', () => {
  beforeEach(() => {
    query.marks = []
    query.isLoading = false
    query.error = null
    vi.clearAllMocks()
  })

  it('renders nothing when there is nothing to recheck', () => {
    const { container } = render(<RecheckMarksPanel projectId="p1" />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the affected target, the reason and the three actions', () => {
    query.marks = [mark()]
    render(<RecheckMarksPanel projectId="p1" />)
    expect(screen.getByText('章节计划（1 节）')).toBeInTheDocument()
    expect(screen.getByText('资料《论文.pdf》已删除')).toBeInTheDocument()
    expect(screen.getByText(/系统不会自动改写正文或产物/)).toBeInTheDocument()
    for (const label of ['已复查', '已重建', '忽略']) {
      expect(screen.getByRole('button', { name: label })).toBeInTheDocument()
    }
  })

  it('records the chosen resolution for the mark', () => {
    query.marks = [mark()]
    render(<RecheckMarksPanel projectId="p1" />)
    fireEvent.click(screen.getByRole('button', { name: '已复查' }))
    expect(resolveMark).toHaveBeenCalledWith({ markId: 'm1', resolution: 'rechecked' })
  })

  it('degrades honestly when the target no longer exists', () => {
    query.marks = [mark({ targetMissing: true, targetTitle: '目标已不存在', targetDetail: undefined })]
    render(<RecheckMarksPanel projectId="p1" />)
    expect(screen.getAllByText('目标已不存在').length).toBeGreaterThan(0)
  })

  it('surfaces load failures with a retry instead of pretending empty', () => {
    query.error = new Error('offline')
    render(<RecheckMarksPanel projectId="p1" />)
    expect(screen.getByRole('alert')).toHaveTextContent('待复查列表加载失败')
    fireEvent.click(screen.getByRole('button', { name: /重试/ }))
    expect(refetch).toHaveBeenCalled()
  })

  it('shows the resolution label on the processed tab', () => {
    query.marks = [mark({ resolution: 'rebuilt', resolvedAt: '2026-11-26T01:00:00Z' })]
    render(<RecheckMarksPanel projectId="p1" />)
    fireEvent.click(screen.getByRole('tab', { name: '已处理' }))
    expect(screen.getByText('已重建')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '忽略' })).not.toBeInTheDocument()
  })
})
