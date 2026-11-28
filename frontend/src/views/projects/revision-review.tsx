import { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'

import { cn } from '@/lib/utils'
import type { ProposalBasis } from '@/services/agent-api'
import { formatBasisSummary, suggestionKindLabel } from './suggestion-display'

/**
 * 批注与聊天建议共用的展示原子。
 *
 * 只做静态枚举映射与排版，不生成/改写任何评语——评语始终是模型 reason 原文，
 * 避免前端模板冒充导师口吻。
 */

export function SuggestionTypeLabel({ kind, className }: { kind?: string; className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-1 text-[11px] font-medium text-muted-foreground', className)}>
      <span className="size-1.5 rounded-full bg-primary/70" aria-hidden />
      {suggestionKindLabel(kind)}
    </span>
  )
}

/** 修订对比：原文 / 改为。浅色底 + 左线，不用红绿代码块。 */
export function RevisionDiff({ oldText, newText }: { oldText: string; newText: string }) {
  return (
    <div className="space-y-1.5 text-[12px] leading-5">
      <div>
        <p className="mb-0.5 text-[10px] text-muted-foreground">原文</p>
        <p className="whitespace-pre-wrap break-words rounded-md border-l-2 border-border bg-muted/30 px-2 py-1.5 font-mono">
          {oldText}
        </p>
      </div>
      <div>
        <p className="mb-0.5 text-[10px] text-muted-foreground">改为</p>
        <p className="whitespace-pre-wrap break-words rounded-md border-l-2 border-primary/40 bg-primary/5 px-2 py-1.5 font-mono">
          {newText}
        </p>
      </div>
    </div>
  )
}

/** 依据摘要：始终可见的一行，防止自动化偏见。 */
export function BasisSummary({ basis }: { basis?: ProposalBasis[] }) {
  return <p className="truncate text-[11px] text-muted-foreground">依据：{formatBasisSummary(basis)}</p>
}

/** 依据详情：默认折叠；只有 uncertain 才显示「依据存疑」。 */
export function BasisDetails({ basis }: { basis?: ProposalBasis[] }) {
  const [open, setOpen] = useState(false)
  if (!basis?.length) return null
  return (
    <div className="rounded-md border border-border/40">
      <button
        type="button"
        className="flex min-h-11 w-full items-center gap-1 px-2 py-1.5 text-left text-[11px] text-muted-foreground hover:text-foreground sm:min-h-8"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
        参考依据
      </button>
      {open && (
        <ul className="space-y-1.5 border-t border-border/40 px-2 py-2">
          {basis.map((item, index) => (
            <li key={`${item.type}-${index}`} className="space-y-1 text-[11px] leading-5 text-muted-foreground">
              <p className="font-medium text-foreground">
                {item.type === 'plan'
                  ? `章节计划 · ${item.sectionTitle ?? item.sectionId ?? ''}`
                  : item.type === 'evidence'
                    ? `材料 · ${item.sourceName || '项目资料'}`
                    : `规范 · [${item.cite}] ${item.normTitle ?? ''}`}
                {item.stance === 'uncertain' && (
                  <span className="ml-1.5 rounded bg-amber-500/10 px-1 py-0.5 text-[10px] text-amber-700 dark:text-amber-400">依据存疑</span>
                )}
              </p>
              {item.note && <p>{item.note}</p>}
              {item.quote && (
                <blockquote className="whitespace-pre-wrap break-words border-l-2 border-primary/30 pl-2">{item.quote}</blockquote>
              )}
              {item.type === 'norm' && item.normRule && <p>{item.normRule}</p>}
              {item.type === 'plan' && item.gap && <p>计划缺口：{item.gap}</p>}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/** 只在非 high 时提示存疑；high 不额外贴标签。 */
export function ConfidenceNote({ confidence, reason }: { confidence?: string; reason?: string }) {
  if (!confidence || confidence === 'high') return null
  return (
    <p className="text-[11px] text-amber-700 dark:text-amber-400">
      依据存疑{reason ? `：${reason}` : ''}
    </p>
  )
}
