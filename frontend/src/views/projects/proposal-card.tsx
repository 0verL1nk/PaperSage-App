import { useState } from 'react'
import { ChevronDown, ChevronRight, Loader2 } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { AgentChangeProposal, AgentSuggestionKind, ProposalBasis } from '@/services/agent-api'

/**
 * 修改建议卡：聊天与「建议」列表共用同一份渲染。
 *
 * 防自动化偏见：依据默认展开、应用按钮不默认聚焦、不提供「全部应用」；
 * 卡片只陈述「改动 + 依据 + 不确定点」，不写「已修正/已符合规范」这类结论。
 */

const KIND_LABEL: Record<AgentSuggestionKind, string> = {
  evidence_gap: '缺少材料依据',
  logic_gap: '论证缺口',
  norm: '学术规范',
  citation: '引用问题',
  language: '表达',
  structure: '结构',
}

const STATUS_LABEL: Record<AgentChangeProposal['status'], string> = {
  pending: '待确认',
  applied: '已应用',
  rejected: '已忽略',
  stale: '已过期（稿件已变化）',
  reverted: '已撤销',
  dropped: '已丢弃',
}

const REJECT_REASONS = [
  { code: 'too_vague', label: '太泛' },
  { code: 'out_of_context', label: '不符合语境' },
  { code: 'no_change_needed', label: '无需修改' },
  { code: 'other', label: '其他' },
] as const

export interface ProposalCardProps {
  proposal: AgentChangeProposal
  onApply: () => Promise<void>
  onReject: (reasonCode?: string, note?: string) => Promise<void>
  onUndo?: () => Promise<void>
}

export function ProposalCard({ proposal, onApply, onReject, onUndo }: ProposalCardProps) {
  const [busy, setBusy] = useState(false)
  const [rejecting, setRejecting] = useState(false)
  const [reason, setReason] = useState<string>('')
  const [note, setNote] = useState('')

  const pending = proposal.status === 'pending'
  const basis = proposal.basis ?? []

  const run = async (action: () => Promise<void>) => {
    setBusy(true)
    try {
      await action()
    } finally {
      setBusy(false)
    }
  }

  return (
    <article className="min-w-0 overflow-hidden rounded-2xl border border-border/40 bg-card">
      <header className="flex flex-wrap items-center gap-x-2 gap-y-1 border-b border-border/40 px-3 py-2">
        <span className="text-[11px] font-bold uppercase tracking-widest text-muted-foreground">修改建议</span>
        {proposal.suggestionKind && (
          <span className="rounded-md bg-primary/10 px-1.5 py-0.5 text-[10px] text-primary">
            {KIND_LABEL[proposal.suggestionKind] ?? proposal.suggestionKind}
          </span>
        )}
        {proposal.confidence === 'high' && (
          <span className="rounded-md bg-green-500/10 px-1.5 py-0.5 text-[10px] text-green-700 dark:text-green-400">依据充分</span>
        )}
        {(proposal.confidence === 'medium' || proposal.confidence === 'low') && (
          <span className="rounded-md bg-amber-500/10 px-1.5 py-0.5 text-[10px] text-amber-700 dark:text-amber-400">依据存疑</span>
        )}
        <span className="ml-auto text-[10px] text-muted-foreground">{STATUS_LABEL[proposal.status]}</span>
      </header>

      <div className="space-y-2 px-3 py-2">
        <p className="truncate text-[10px] text-muted-foreground">
          {proposal.sectionPath ? `${proposal.sectionPath} · ${proposal.filePath}` : proposal.filePath}
        </p>
        <pre className="overflow-x-auto whitespace-pre-wrap rounded-lg bg-destructive/5 p-2 text-[11px] text-destructive/90">
          {proposal.oldText}
        </pre>
        <pre className="overflow-x-auto whitespace-pre-wrap rounded-lg bg-green-500/5 p-2 text-[11px] text-green-700 dark:text-green-400">
          {proposal.newText}
        </pre>
        {proposal.reason && <p className="text-[11px] leading-5 text-muted-foreground">{proposal.reason}</p>}
        {proposal.confidenceReason && (
          <p className="text-[10px] leading-4 text-muted-foreground">依据说明：{proposal.confidenceReason}</p>
        )}

        {basis.length > 0
          ? <BasisList basis={basis} />
          : proposal.normRefs && proposal.normRefs.length > 0 && <NormRefList refs={proposal.normRefs} />}

        {proposal.status === 'stale' && (
          <p className="text-[11px] text-muted-foreground">稿件已变化，这条建议不能再应用；请基于当前内容重新检查本段。</p>
        )}
        {proposal.status === 'reverted' && <p className="text-[11px] text-muted-foreground">这次修改已撤销，稿件恢复了原文。</p>}
        {proposal.status === 'rejected' && proposal.rejectReason && (
          <p className="text-[11px] text-muted-foreground">
            已忽略（{REJECT_REASONS.find((r) => r.code === proposal.rejectReason)?.label ?? proposal.rejectReason}）
            {proposal.rejectNote ? `：${proposal.rejectNote}` : ''}
          </p>
        )}
      </div>

      {pending && !rejecting && (
        <div className="flex flex-wrap gap-2 px-3 pb-3">
          <Button
            size="sm"
            className="min-h-11 rounded-lg px-3 text-[11px] sm:min-h-8"
            disabled={busy}
            onClick={() =>
              void run(async () => {
                try {
                  await onApply()
                  toast.success('已应用到稿件，可在卡片上撤销')
                } catch (error) {
                  toast.error(error instanceof Error ? error.message : '应用建议失败，请重试')
                }
              })
            }
          >
            {busy ? <Loader2 className="size-3 animate-spin" /> : null}
            应用到稿件
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="min-h-11 rounded-lg px-3 text-[11px] sm:min-h-8"
            disabled={busy}
            onClick={() => setRejecting(true)}
          >
            忽略
          </Button>
        </div>
      )}

      {pending && rejecting && (
        <div className="space-y-2 border-t border-border/40 px-3 py-3">
          <p className="text-[11px] font-medium text-muted-foreground">忽略原因（可选，用于改进建议质量）</p>
          <div className="flex flex-wrap gap-1.5">
            {REJECT_REASONS.map((item) => (
              <Button
                key={item.code}
                type="button"
                size="sm"
                variant={reason === item.code ? 'default' : 'outline'}
                className="min-h-11 rounded-lg px-2.5 text-[11px] sm:min-h-8"
                onClick={() => setReason(reason === item.code ? '' : item.code)}
              >
                {item.label}
              </Button>
            ))}
          </div>
          <input
            value={note}
            onChange={(event) => setNote(event.target.value)}
            placeholder="补充说明（可选）"
            aria-label="忽略原因补充说明"
            className="min-h-11 w-full rounded-lg border border-border/60 bg-background px-2.5 text-[11px] focus:outline-none focus:ring-2 focus:ring-primary/10"
          />
          <div className="flex gap-2">
            <Button
              size="sm"
              className="min-h-11 rounded-lg px-3 text-[11px] sm:min-h-8"
              disabled={busy}
              onClick={() =>
                void run(async () => {
                  try {
                    await onReject(reason, note.trim())
                    setRejecting(false)
                  } catch (error) {
                    toast.error(error instanceof Error ? error.message : '忽略失败，请重试')
                  }
                })
              }
            >
              确认忽略
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="min-h-11 rounded-lg px-3 text-[11px] sm:min-h-8"
              disabled={busy}
              onClick={() => setRejecting(false)}
            >
              取消
            </Button>
          </div>
        </div>
      )}

      {proposal.status === 'applied' && onUndo && (
        <div className="px-3 pb-3">
          <Button
            size="sm"
            variant="outline"
            className="min-h-11 rounded-lg px-3 text-[11px] sm:min-h-8"
            disabled={busy}
            onClick={() =>
              void run(async () => {
                try {
                  await onUndo()
                  toast.success('已撤销这次修改')
                } catch (error) {
                  toast.error(error instanceof Error ? error.message : '撤销失败；如稿件已继续修改，请手动回退')
                }
              })
            }
          >
            撤销这次修改
          </Button>
        </div>
      )}
    </article>
  )
}

/** 旧聊天建议只有 norm_refs：沿用合并前 main 的标题 chip，避免信息回退。 */
function NormRefList({ refs }: { refs: NonNullable<AgentChangeProposal['normRefs']> }) {
  return (
    <div className="space-y-1">
      <div className="flex flex-wrap gap-1">
        {refs.map((ref) => (
          <span key={ref.cite} title={ref.cite} className="rounded-md border border-border/60 px-1.5 py-0.5 text-[10px] text-muted-foreground">
            {ref.title}
          </span>
        ))}
      </div>
      <details>
        <summary className="min-h-11 cursor-pointer text-[11px] text-primary">
          查看依据的规范条款（{refs.length} 条）
        </summary>
        <ul className="mt-1 space-y-1">
          {refs.map((ref) => (
            <li key={ref.cite} className="text-[11px] leading-5 text-muted-foreground">
              <span className="font-medium text-foreground">[{ref.cite}] {ref.title}</span>
              <span className="block">{ref.rule}</span>
              <span className="block">出处：{ref.source}</span>
            </li>
          ))}
        </ul>
      </details>
    </div>
  )
}

function BasisList({ basis }: { basis: ProposalBasis[] }) {
  return (
    <div className="space-y-1.5">
      <p className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground">依据</p>
      {basis.map((item, index) => (
        <BasisItem key={`${item.type}-${index}`} item={item} />
      ))}
    </div>
  )
}

function BasisItem({ item }: { item: ProposalBasis }) {
  const [open, setOpen] = useState(false)
  const summary =
    item.type === 'evidence'
      ? `材料原文 · ${item.sourceName || '项目资料'}`
      : item.type === 'norm'
        ? `规范条款 · [${item.cite}] ${item.normTitle ?? ''}`
        : `章节计划 · ${item.sectionTitle ?? item.sectionId ?? ''}`
  return (
    <div className="rounded-lg border border-border/40 bg-muted/15">
      <button
        type="button"
        className="flex min-h-11 w-full items-center gap-1.5 px-2.5 py-2 text-left text-[11px] text-muted-foreground hover:text-foreground sm:min-h-8"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
        <span className="min-w-0 flex-1 truncate">{summary}</span>
        <span className={cn('shrink-0 text-[10px]', item.stance === 'uncertain' ? 'text-amber-600 dark:text-amber-400' : 'text-green-700 dark:text-green-400')}>
          {item.stance === 'uncertain' ? '依据存疑' : '依据支持'}
        </span>
      </button>
      {open && (
        <div className="space-y-1.5 border-t border-border/40 px-2.5 py-2 text-[11px] leading-5 text-muted-foreground">
          {item.note && <p>{item.note}</p>}
          {item.quote && <blockquote className="whitespace-pre-wrap break-words border-l-2 border-primary/30 pl-2.5">{item.quote}</blockquote>}
          {item.type === 'norm' && item.normRule && <p>{item.normRule}</p>}
          {item.type === 'norm' && item.normSource && <p>出处：{item.normSource}</p>}
          {item.type === 'plan' && item.gap && <p>计划缺口：{item.gap}</p>}
          {item.locator && <p className="text-[10px]">位置：{item.locator}</p>}
        </div>
      )}
    </div>
  )
}
