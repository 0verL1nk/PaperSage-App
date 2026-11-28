import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MentorCommentRail } from './mentor-comment-rail'
import type { AgentChangeProposal } from '@/services/agent-api'

const base: AgentChangeProposal = {
  id: 'a', filePath: 'main.tex', oldText: '旧文', newText: '新文',
  reason: '补充研究问题的来源。', suggestionKind: 'logic_gap',
  sectionPath: '绪论 / 研究背景', status: 'pending',
  basis: [{ type: 'plan', stance: 'supports', sectionTitle: '研究背景', planRevision: 2 }],
}

function railProps(proposals: AgentChangeProposal[]) {
  return {
    proposals,
    activeId: null,
    anchorOf: (proposal: AgentChangeProposal) => ({
      start: proposal.id === 'a' ? 0 : 10, end: 5, locatable: true,
    }),
    notice: null,
    checking: false,
    isLoading: false,
    isError: false,
    onRetry: vi.fn(),
    onActivate: vi.fn(),
    onApply: vi.fn().mockResolvedValue(undefined),
    onReject: vi.fn().mockResolvedValue(undefined),
    onUndo: vi.fn().mockResolvedValue(undefined),
    onRecheck: vi.fn(),
    onClose: vi.fn(),
  }
}

describe('MentorCommentRail', () => {
  it('groups pending comments by section and shows processed separately', () => {
    render(
      <MentorCommentRail
        {...railProps([
          base,
          { ...base, id: 'b', sectionPath: '绪论 / 研究方法', reason: '方法部分需要补采样范围。' },
          { ...base, id: 'c', status: 'applied', reason: '已处理的一条。' },
        ])}
      />,
    )
    expect(screen.getByText('写作批注')).toBeInTheDocument()
    expect(screen.getByText('2 条待处理')).toBeInTheDocument()
    expect(screen.getByText('绪论 / 研究背景')).toBeInTheDocument()
    expect(screen.getByText('绪论 / 研究方法')).toBeInTheDocument()
    expect(screen.getByText('已处理')).toBeInTheDocument()
    expect(screen.getByText('已处理的一条。')).toBeInTheDocument()
  })

  it('activates a comment when its body is clicked', () => {
    const props = railProps([base])
    render(<MentorCommentRail {...props} />)
    fireEvent.click(screen.getByRole('button', { name: /定位到这条批注/ }))
    expect(props.onActivate).toHaveBeenCalledWith('a')
  })

  it('shows the empty state and closes from the header', () => {
    const props = railProps([])
    render(<MentorCommentRail {...props} />)
    expect(screen.getByText('还没有写作批注')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '收起写作批注' }))
    expect(props.onClose).toHaveBeenCalled()
  })
})
