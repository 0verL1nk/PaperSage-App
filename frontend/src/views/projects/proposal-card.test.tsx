import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { ProposalCard } from './proposal-card'
import type { AgentChangeProposal } from '@/services/agent-api'

const proposal: AgentChangeProposal = {
  id: 'p1',
  filePath: 'body/ch1.tex',
  oldText: '研究背景的原文。',
  newText: '研究背景的原文，并补充研究问题来源。',
  reason: '这一段需要交代研究问题如何从材料中导出。',
  suggestionKind: 'logic_gap',
  sectionPath: '绪论 / 研究背景',
  status: 'pending',
  normRefs: [{ cite: 'N-GBT7714@v1', title: '文后参考文献著录规则', rule: '按顺序编码制著录', source: '国家标准' }],
}

const banned = /修改建议|依据充分|依据说明：|依据支持|应用到稿件|待确认|停顿后自动检查|8 秒|不抢光标|检查本段/

describe('ProposalCard (chat)', () => {
  it('uses the shared mentor card and keeps legacy norm refs visible', () => {
    const { container } = render(
      <ProposalCard proposal={proposal} onApply={vi.fn()} onReject={vi.fn()} onUndo={vi.fn()} />,
    )
    expect(screen.getByText('论证')).toBeInTheDocument()
    expect(screen.getByText(proposal.reason!)).toBeInTheDocument()
    expect(screen.getByText(/依据：规范 \[N-GBT7714@v1\]/)).toBeInTheDocument()
    expect(container.textContent).not.toMatch(banned)
  })

  it('applies through the callback', async () => {
    const onApply = vi.fn().mockResolvedValue(undefined)
    render(<ProposalCard proposal={proposal} onApply={onApply} onReject={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: '采纳' }))
    expect(onApply).toHaveBeenCalled()
  })
})
