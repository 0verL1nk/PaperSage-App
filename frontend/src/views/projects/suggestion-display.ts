import type { AgentChangeProposal, AgentSuggestionKind, ProposalBasis } from '@/services/agent-api'

/** 静态枚举映射与文案摘要；不含组件，避免 fast-refresh 与展示层耦合。 */

const KIND_LABEL: Record<AgentSuggestionKind, string> = {
  evidence_gap: '材料',
  logic_gap: '论证',
  norm: '规范',
  citation: '引用',
  language: '表达',
  structure: '结构',
}

export function suggestionKindLabel(kind?: string): string {
  if (kind && kind in KIND_LABEL) return KIND_LABEL[kind as AgentSuggestionKind]
  return '批注'
}

/** 旧聊天建议只有 norm_refs：折算成同一套 basis，避免第二套展示。 */
export function proposalBasis(proposal: AgentChangeProposal): ProposalBasis[] | undefined {
  if (proposal.basis?.length) return proposal.basis
  if (!proposal.normRefs?.length) return undefined
  return proposal.normRefs.map((ref) => ({
    type: 'norm',
    stance: 'supports',
    cite: ref.cite,
    normTitle: ref.title,
    normRule: ref.rule,
    normSource: ref.source,
  }))
}

export function formatBasisSummary(basis?: ProposalBasis[]): string {
  if (!basis?.length) return '暂无依据'
  const first = basis[0]
  const lead =
    first.type === 'plan'
      ? `章节计划${first.planRevision ? ` v${first.planRevision}` : ''}${first.sectionTitle ? ` · ${first.sectionTitle}` : ''}`
      : first.type === 'evidence'
        ? `材料《${first.sourceName || '项目资料'}》`
        : `规范 [${first.cite}]${first.normTitle ? ` ${first.normTitle}` : ''}`
  return basis.length > 1 ? `${lead}（共 ${basis.length} 条）` : lead
}
