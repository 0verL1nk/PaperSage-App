import { Loader2, MessageSquare, X } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { StateBlock } from '@/components/shared/state-block'
import { cn } from '@/lib/utils'
import type { AgentChangeProposal } from '@/services/agent-api'
import type { AnchorLocation } from '@/lib/latex/proposal-anchor'
import type { SuggestionNotice } from '@/hooks/use-writing-suggestion'
import { MentorCommentCard } from './mentor-comment-card'

export interface MentorCommentRailProps {
  proposals: AgentChangeProposal[]
  activeId: string | null
  anchorOf: (proposal: AgentChangeProposal) => AnchorLocation
  notice: SuggestionNotice | null
  checking: boolean
  isLoading: boolean
  isError: boolean
  onRetry: () => void
  onActivate: (id: string) => void
  onApply: (proposal: AgentChangeProposal) => Promise<void>
  onReject: (proposal: AgentChangeProposal, reasonCode?: string, note?: string) => Promise<void>
  onUndo: (proposal: AgentChangeProposal) => Promise<void>
  onRecheck: () => void
  onClose?: () => void
  className?: string
}

/** 写作批注栏：桌面右栏 / 窄屏抽屉 / 移动 Sheet 共用同一份内容。 */
export function MentorCommentRail({
  proposals, activeId, anchorOf, notice, checking, isLoading, isError, onRetry,
  onActivate, onApply, onReject, onUndo, onRecheck, onClose, className,
}: MentorCommentRailProps) {
  const pending = proposals
    .filter((proposal) => proposal.status === 'pending')
    .sort((a, b) => {
      const left = anchorOf(a)
      const right = anchorOf(b)
      if (left.locatable && right.locatable) return left.start - right.start
      if (left.locatable) return -1
      if (right.locatable) return 1
      return 0
    })
  const processed = proposals.filter((proposal) => proposal.status !== 'pending')
  const grouped = new Map<string, AgentChangeProposal[]>()
  for (const proposal of pending) {
    const key = proposal.sectionPath || '未分节'
    const list = grouped.get(key) ?? []
    list.push(proposal)
    grouped.set(key, list)
  }

  return (
    <div className={cn('flex h-full min-h-0 flex-col bg-card', className)}>
      <header className="flex min-h-11 flex-none items-center gap-2 border-b border-border/40 px-3">
        <MessageSquare size={14} className="text-muted-foreground" />
        <span className="text-[12px] font-medium">写作批注</span>
        <span className="text-[11px] text-muted-foreground">{pending.length} 条待处理</span>
        {onClose && (
          <Button
            variant="ghost"
            size="icon-xs"
            className="ml-auto size-9 rounded-md text-muted-foreground"
            aria-label="收起写作批注"
            onClick={onClose}
          >
            <X size={14} />
          </Button>
        )}
      </header>

      {(checking || notice) && (
        <div className="flex-none space-y-1 border-b border-border/40 px-3 py-2">
          {checking && (
            <p role="status" className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
              <Loader2 className="size-3 animate-spin" /> 正在批注这一段…
            </p>
          )}
          {notice && (
            <p
              role="status"
              className={cn(
                'text-[11px]',
                notice.kind === 'error'
                  ? 'text-destructive'
                  : notice.kind === 'skip'
                    ? 'text-amber-700 dark:text-amber-400'
                    : 'text-muted-foreground',
              )}
            >
              {notice.text}
            </p>
          )}
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto">
        {isLoading && <p className="px-3 py-3 text-[11px] text-muted-foreground">正在读取写作批注…</p>}
        {isError && (
          <StateBlock
            compact
            title="批注加载失败"
            description="网络或服务暂时不可用。"
            action={<Button size="sm" variant="outline" onClick={onRetry}>重试</Button>}
          />
        )}
        {!isLoading && !isError && proposals.length === 0 && (
          <StateBlock
            compact
            title="还没有写作批注"
            description="把光标放在正文段落里，点上方「批注本段」。"
          />
        )}

        {[...grouped.entries()].map(([section, items]) => (
          <section key={section}>
            <h3 className="bg-muted/20 px-3 py-1 text-[10px] text-muted-foreground">{section}</h3>
            {items.map((proposal) => (
              <MentorCommentCard
                key={proposal.id}
                proposal={proposal}
                active={activeId === proposal.id}
                locatable={anchorOf(proposal).locatable}
                onActivate={() => onActivate(proposal.id)}
                onApply={() => onApply(proposal)}
                onReject={(reason, note) => onReject(proposal, reason, note)}
                onUndo={() => onUndo(proposal)}
                onRecheck={onRecheck}
              />
            ))}
          </section>
        ))}

        {processed.length > 0 && (
          <section>
            <h3 className="bg-muted/20 px-3 py-1 text-[10px] text-muted-foreground">已处理</h3>
            {processed.map((proposal) => (
              <MentorCommentCard
                key={proposal.id}
                proposal={proposal}
                active={activeId === proposal.id}
                locatable={anchorOf(proposal).locatable}
                onActivate={() => onActivate(proposal.id)}
                onApply={() => onApply(proposal)}
                onReject={(reason, note) => onReject(proposal, reason, note)}
                onUndo={() => onUndo(proposal)}
                onRecheck={onRecheck}
              />
            ))}
          </section>
        )}
      </div>
    </div>
  )
}
