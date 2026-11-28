import type { AgentChangeProposal } from '@/services/agent-api'
import { MentorCommentCard } from './mentor-comment-card'

export interface ProposalCardProps {
  proposal: AgentChangeProposal
  onApply: () => Promise<void>
  onReject: (reasonCode?: string, note?: string) => Promise<void>
  onUndo?: () => Promise<void>
}

/**
 * 聊天消息里的建议卡：与导师批注栏共用同一张卡片与展示原子，
 * 不再保留第二套红绿 Diff 与旧文案。
 */
export function ProposalCard({ proposal, onApply, onReject, onUndo }: ProposalCardProps) {
  return (
    <MentorCommentCard
      proposal={proposal}
      active={false}
      locatable
      onActivate={() => {}}
      onApply={onApply}
      onReject={onReject}
      onUndo={onUndo}
    />
  )
}
