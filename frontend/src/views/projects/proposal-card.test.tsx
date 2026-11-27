import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { ProposalCard } from './proposal-card'
import type { AgentChangeProposal } from '@/services/agent-api'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

const proposal: AgentChangeProposal = {
  id: 'p1',
  filePath: 'body/ch1.tex',
  oldText: '研究背景的原文。',
  newText: '研究背景的原文，并补充研究问题来源。',
  reason: '章节计划要求交代研究问题来源。',
  suggestionKind: 'logic_gap',
  confidence: 'high',
  confidenceReason: '已采用章节计划明确要求。',
  sectionPath: '绪论 / 研究背景',
  status: 'pending',
  basis: [
    {
      type: 'plan', stance: 'supports', note: '第一章计划要求',
      sectionId: 'ch1.1', sectionTitle: '研究背景', gap: '未交代研究问题来源',
    },
    {
      type: 'evidence', stance: 'supports', sourceName: '文献A', quote: '材料中的原句',
    },
  ],
}

describe('ProposalCard provenance', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows the three-source basis expanded on demand and never hides uncertainty', () => {
    render(<ProposalCard proposal={proposal} onApply={vi.fn()} onReject={vi.fn()} onUndo={vi.fn()} />)
    expect(screen.getByText('依据充分')).toBeInTheDocument()
    expect(screen.getByText('章节计划 · 研究背景')).toBeInTheDocument()
    expect(screen.getByText('材料原文 · 文献A')).toBeInTheDocument()

    const planButton = screen.getByRole('button', { name: /章节计划 · 研究背景/ })
    expect(planButton).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(planButton)
    expect(planButton).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('计划缺口：未交代研究问题来源')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /材料原文 · 文献A/ }))
    expect(screen.getByText('材料中的原句')).toBeInTheDocument()
  })

  it('sends the optional reject reason before dismissing the card', async () => {
    const onReject = vi.fn().mockResolvedValue(undefined)
    render(<ProposalCard proposal={proposal} onApply={vi.fn()} onReject={onReject} onUndo={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: '忽略' }))
    fireEvent.click(screen.getByRole('button', { name: '太泛' }))
    fireEvent.change(screen.getByLabelText('忽略原因补充说明'), { target: { value: '没有落到具体句' } })
    fireEvent.click(screen.getByRole('button', { name: '确认忽略' }))
    expect(onReject).toHaveBeenCalledWith('too_vague', '没有落到具体句')
  })

  it('offers one-click undo only after the proposal was applied', () => {
    const { rerender } = render(
      <ProposalCard proposal={proposal} onApply={vi.fn()} onReject={vi.fn()} onUndo={vi.fn()} />,
    )
    expect(screen.queryByRole('button', { name: '撤销这次修改' })).not.toBeInTheDocument()
    rerender(
      <ProposalCard
        proposal={{ ...proposal, status: 'applied' }}
        onApply={vi.fn()}
        onReject={vi.fn()}
        onUndo={vi.fn()}
      />,
    )
    expect(screen.getByRole('button', { name: '撤销这次修改' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '应用到稿件' })).not.toBeInTheDocument()
  })
})
