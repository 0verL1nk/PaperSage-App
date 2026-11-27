import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export interface PanelToolbarProps {
  title?: ReactNode
  subtitle?: ReactNode
  start?: ReactNode
  end?: ReactNode
  sticky?: boolean
  bordered?: boolean
  className?: string
}

/** 面板工具条：统一高度、留白、分割线与左右操作位，替代各面板手写头。 */
export function PanelToolbar({
  title,
  subtitle,
  start,
  end,
  sticky = false,
  bordered = true,
  className,
}: PanelToolbarProps) {
  return (
    <div
      className={cn(
        'flex min-h-11 items-center gap-2 px-3 py-2',
        bordered && 'border-b border-border',
        sticky && 'sticky top-0 z-10 bg-background/95 backdrop-blur',
        className,
      )}
    >
      {start ? <div className="flex flex-none items-center gap-1.5">{start}</div> : null}
      <div className="min-w-0 flex-1">
        {title ? (
          <div className="truncate text-body-sm font-medium text-foreground">{title}</div>
        ) : null}
        {subtitle ? (
          <div className="truncate text-caption text-muted-foreground">{subtitle}</div>
        ) : null}
      </div>
      {end ? <div className="flex flex-none items-center gap-1.5">{end}</div> : null}
    </div>
  )
}
