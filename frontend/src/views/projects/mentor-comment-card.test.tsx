import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MentorCommentCard } from './mentor-comment-card'
import type { AgentChangeProposal } from '@/services/agent-api'

const proposal: AgentChangeProposal = {
  id: 'p1',
  filePath: 'body/ch1.tex',
  oldText: '研究背景的原文。',
  newText: '研究背景的原文，并补充研究问题来源。',
  reason: '这一段需要交代研究问题如何从材料中导出。',
  suggestionKind: 'logic_gap',
  confidence: 'high',
  sectionPath: '绪论 / 研究背景',
  status: 'pending',
  basis: [{
    type: 'plan', stance: 'supports', sectionId: 'ch1.1', sectionTitle: '研究背景',
    gap: '未交代研究问题来源', planRevision: 2,
  }],
}

const banned = /修改建议|依据充分|依据说明：|依据支持|应用到稿件|待确认|停顿后自动检查|8 秒|不抢光标|检查本段/

describe('MentorCommentCard', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows type, mentor reason and one-line basis without code-diff chrome', () => {
    const { container } = render(
      <MentorCommentCard
        proposal={proposal}
        active={false}
        locatable
        onActivate={vi.fn()}
        onApply={vi.fn()}
        onReject={vi.fn()}
      />,
    )
    expect(screen.getByText('论证')).toBeInTheDocument()
    expect(screen.getByText(proposal.reason!)).toBeInTheDocument()
    expect(screen.getByText(/依据：章节计划 v2 · 研究背景/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /查看修改/ })).toBeInTheDocument()
    expect(container.textContent).not.toMatch(banned)
  })

  it('opens the revision diff on demand', () => {
    render(
      <MentorCommentCard proposal={proposal} active={false} locatable
        onActivate={vi.fn()} onApply={vi.fn()} onReject={vi.fn()} />,
    )
    expect(screen.queryByText('原文')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /查看修改/ }))
    expect(screen.getByText('原文')).toBeInTheDocument()
    expect(screen.getByText('改为')).toBeInTheDocument()
  })

  it('sends the optional ignore reason', async () => {
    const onReject = vi.fn().mockResolvedValue(undefined)
    render(
      <MentorCommentCard proposal={proposal} active={false} locatable
        onActivate={vi.fn()} onApply={vi.fn()} onReject={onReject} />,
    )
    fireEvent.click(screen.getByRole('button', { name: '忽略' }))
    fireEvent.click(screen.getByRole('button', { name: '太泛' }))
    fireEvent.change(screen.getByLabelText('忽略原因补充说明'), { target: { value: '没有落到具体句' } })
    fireEvent.click(screen.getByRole('button', { name: '确认忽略' }))
    expect(onReject).toHaveBeenCalledWith('too_vague', '没有落到具体句')
  })

  it('shows 依据存疑 only for uncertain basis', () => {
    render(
      <MentorCommentCard
        proposal={{ ...proposal, basis: [{ ...proposal.basis![0], stance: 'uncertain' }] }}
        active={false} locatable onActivate={vi.fn()} onApply={vi.fn()} onReject={vi.fn()} />,
    )
    fireEvent.click(screen.getByRole('button', { name: /查看修改/ }))
    fireEvent.click(screen.getByRole('button', { name: '参考依据' }))
    expect(screen.getByText('依据存疑')).toBeInTheDocument()
  })

  it('offers undo after the comment was accepted', () => {
    render(
      <MentorCommentCard proposal={{ ...proposal, status: 'applied' }} active={false} locatable
        onActivate={vi.fn()} onApply={vi.fn()} onReject={vi.fn()} onUndo={vi.fn()} />,
    )
    expect(screen.getByText('已采纳')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '撤销' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '采纳' })).not.toBeInTheDocument()
  })
})
