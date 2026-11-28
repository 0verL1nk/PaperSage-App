import { Eye, Files, PenLine, Sparkles } from 'lucide-react'
import { cn } from '@/lib/utils'

export type WritingPaneKey = 'files' | 'editor' | 'preview' | 'ai'

const PANES: { key: WritingPaneKey; label: string; icon: typeof Files; testId: string }[] = [
  { key: 'files', label: '文件', icon: Files, testId: 'writing-pane-files' },
  { key: 'editor', label: '编辑', icon: PenLine, testId: 'writing-pane-editor' },
  { key: 'preview', label: '预览', icon: Eye, testId: 'writing-pane-preview' },
  { key: 'ai', label: 'AI 助手', icon: Sparkles, testId: 'writing-pane-ai' },
]

interface WritingPaneBarProps {
  active: WritingPaneKey
  onChange: (pane: WritingPaneKey) => void
  aiPendingCount?: number
  compileActive?: boolean
  compileFailed?: boolean
}

/**
 * 写作页移动端底部四分栏：52px + iOS 安全区。
 * 作为 flex 子项而非 fixed，软键盘弹出时不会被顶起或遮住光标。
 */
export function WritingPaneBar({
  active,
  onChange,
  aiPendingCount = 0,
  compileActive = false,
  compileFailed = false,
}: WritingPaneBarProps) {
  const handleKeyDown = (event: React.KeyboardEvent, index: number) => {
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
    event.preventDefault()
    const delta = event.key === 'ArrowRight' ? 1 : -1
    onChange(PANES[(index + delta + PANES.length) % PANES.length].key)
  }

  return (
    <nav
      role="tablist"
      aria-label="写作面板"
      data-writing-pane-bar="true"
      className="flex h-[52px] flex-none items-stretch border-t border-border/40 bg-background pb-[env(safe-area-inset-bottom)]"
    >
      {PANES.map((pane, index) => {
        const Icon = pane.icon
        const selected = active === pane.key
        return (
          <button
            key={pane.key}
            type="button"
            role="tab"
            aria-selected={selected}
            data-testid={pane.testId}
            tabIndex={selected ? 0 : -1}
            onClick={() => onChange(pane.key)}
            onKeyDown={(event) => handleKeyDown(event, index)}
            className={cn(
              'relative flex min-h-[52px] flex-1 flex-col items-center justify-center gap-0.5 px-1 text-[11px] transition-colors',
              selected ? 'text-primary' : 'text-muted-foreground',
            )}
          >
            <Icon className="size-5" />
            <span className="whitespace-nowrap">{pane.label}</span>
            {pane.key === 'ai' && aiPendingCount > 0 && (
              <span className="absolute right-[18%] top-1.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[9px] font-bold leading-none text-primary-foreground">
                {aiPendingCount > 9 ? '9+' : aiPendingCount}
              </span>
            )}
            {pane.key === 'preview' && (compileActive || compileFailed) && (
              <span
                aria-hidden="true"
                className={cn(
                  'absolute right-[24%] top-2 size-2 rounded-full',
                  compileActive ? 'animate-pulse bg-primary' : 'bg-destructive',
                )}
              />
            )}
            {pane.key === 'preview' && compileActive && <span className="sr-only">（编译中，点此查看进度）</span>}
            {pane.key === 'preview' && compileFailed && !compileActive && <span className="sr-only">（编译失败，点此查看原因）</span>}
          </button>
        )
      })}
    </nav>
  )
}
