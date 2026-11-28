import { useState } from 'react'
import { ChevronDown, ChevronRight, Loader2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { AgentChangeProposal } from '@/services/agent-api'
import {
  BasisDetails,
  BasisSummary,
  ConfidenceNote,
  RevisionDiff,
  SuggestionTypeLabel,
} from './revision-review'
import { proposalBasis } from './suggestion-display'

const REJECT_REASONS = [
  { code: 'too_vague', label: '太泛' },
  { code: 'out_of_context', label: '不符合语境' },
  { code: 'no_change_needed', label: '无需修改' },
  { code: 'other', label: '其他' },
] as const

export interface MentorCommentCardProps {
  proposal: AgentChangeProposal
  active: boolean
  locatable: boolean
  onActivate: () => void
  onApply: () => Promise<void>
  onReject: (reasonCode?: string, note?: string) => Promise<void>
  onUndo?: () => Promise<void>
  onRecheck?: () => void
}

/** 导师批注卡：首屏只留「类型 + 评语 + 查看修改 + 依据摘要 + 采纳/忽略」。 */
export function MentorCommentCard({
  proposal, active, locatable, onActivate, onApply, onReject, onUndo, onRecheck,
}: MentorCommentCardProps) {
  const [busy, setBusy] = useState(false)
  const [reasonExpanded, setReasonExpanded] = useState(false)
  const [showDiff, setShowDiff] = useState(false)
  const [rejecting, setRejecting] = useState(false)
  const [rejectReason, setRejectReason] = useState('')
  const [rejectNote, setRejectNote] = useState('')
  const [error, setError] = useState('')

  const basis = proposalBasis(proposal)
  const reason = proposal.reason?.trim() || '导师没有给出具体评语。'
  const pending = proposal.status === 'pending'
  const canRecheck = proposal.status === 'stale' || !locatable

  const run = async (action: () => Promise<void>) => {
    setBusy(true)
    setError('')
    try {
      await action()
    } catch (err) {
      setError(err instanceof Error ? err.message : '操作失败，请稍后再试。')
    } finally {
      setBusy(false)
    }
  }

  return (
    <article
      className={cn(
        'border-b border-border/40 transition-colors',
        active && 'border-l-2 border-l-primary bg-primary/5',
      )}
      data-proposal-id={proposal.id}
    >
      <div className="space-y-1.5 px-3 py-2.5">
        <button
          type="button"
          className="block w-full space-y-1 text-left"
          onClick={onActivate}
          aria-label={`定位到这条批注：${reason.slice(0, 40)}`}
        >
          <span className="flex items-center justify-between gap-2">
            <SuggestionTypeLabel kind={proposal.suggestionKind} />
            {proposal.status === 'applied' && <span className="text-[10px] text-muted-foreground">已采纳</span>}
            {proposal.status === 'reverted' && <span className="text-[10px] text-muted-foreground">已撤销</span>}
            {proposal.status === 'rejected' && <span className="text-[10px] text-muted-foreground">已忽略</span>}
          </span>
          <p className={cn('text-[12px] leading-5 text-foreground', !reasonExpanded && 'line-clamp-3')}>{reason}</p>
        </button>
        {reason.length > 60 && (
          <button
            type="button"
            className="text-[11px] text-primary"
            onClick={() => setReasonExpanded((value) => !value)}
          >
            {reasonExpanded ? '收起' : '展开评语'}
          </button>
        )}

        {!locatable && <p className="text-[11px] text-muted-foreground">正文已变化，暂时无法定位。</p>}
        {proposal.status === 'stale' && <p className="text-[11px] text-muted-foreground">稿件已变化，这条批注不能再采纳。</p>}
        {proposal.status === 'rejected' && proposal.rejectReason && (
          <p className="text-[11px] text-muted-foreground">
            已忽略（{REJECT_REASONS.find((item) => item.code === proposal.rejectReason)?.label ?? proposal.rejectReason}）
            {proposal.rejectNote ? `：${proposal.rejectNote}` : ''}
          </p>
        )}

        <BasisSummary basis={basis} />
        <ConfidenceNote confidence={proposal.confidence} reason={proposal.confidenceReason} />

        <button
          type="button"
          className="flex min-h-11 items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground sm:min-h-8"
          aria-expanded={showDiff}
          onClick={() => setShowDiff((value) => !value)}
        >
          {showDiff ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
          查看修改
        </button>
        {showDiff && <RevisionDiff oldText={proposal.oldText} newText={proposal.newText} />}
        {showDiff && basis?.length ? <BasisDetails basis={basis} /> : null}
      </div>

      {pending && !rejecting && (
        <div className="flex flex-wrap gap-2 px-3 pb-3">
          <Button
            size="sm"
            className="min-h-11 rounded-lg px-3 text-[11px] sm:min-h-8"
            disabled={busy || !locatable}
            onClick={() => void run(onApply)}
          >
            {busy ? <Loader2 className="size-3 animate-spin" /> : null}
            采纳
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
        <div className="space-y-2 border-t border-border/40 px-3 py-2.5">
          <p className="text-[11px] text-muted-foreground">忽略原因（可选）</p>
          <div className="flex flex-wrap gap-1.5">
            {REJECT_REASONS.map((item) => (
              <Button
                key={item.code}
                type="button"
                size="sm"
                variant={rejectReason === item.code ? 'default' : 'outline'}
                className="min-h-11 rounded-lg px-2.5 text-[11px] sm:min-h-8"
                onClick={() => setRejectReason(rejectReason === item.code ? '' : item.code)}
              >
                {item.label}
              </Button>
            ))}
          </div>
          <input
            value={rejectNote}
            onChange={(event) => setRejectNote(event.target.value)}
            placeholder="补充说明（可选）"
            aria-label="忽略原因补充说明"
            className="min-h-11 w-full rounded-lg border border-border/60 bg-background px-2.5 text-[11px] focus:outline-none focus:ring-2 focus:ring-primary/10"
          />
          <div className="flex gap-2">
            <Button
              size="sm"
              className="min-h-11 rounded-lg px-3 text-[11px] sm:min-h-8"
              disabled={busy}
              onClick={() => void run(async () => {
                await onReject(rejectReason, rejectNote.trim())
                setRejecting(false)
              })}
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
            onClick={() => void run(onUndo)}
          >
            撤销
          </Button>
        </div>
      )}

      {canRecheck && onRecheck && (
        <div className="px-3 pb-3">
          <Button
            size="sm"
            variant="outline"
            className="min-h-11 rounded-lg px-3 text-[11px] sm:min-h-8"
            disabled={busy}
            onClick={onRecheck}
          >
            重新批注本段
          </Button>
        </div>
      )}

      {error && <p role="alert" className="px-3 pb-3 text-[11px] text-destructive">{error}</p>}
    </article>
  )
}
