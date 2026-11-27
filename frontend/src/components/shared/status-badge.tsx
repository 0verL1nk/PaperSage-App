import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export type StatusTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger'

const toneClass: Record<StatusTone, string> = {
  neutral: 'border-border bg-muted text-muted-foreground',
  info: 'border-info/20 bg-info/10 text-info',
  success: 'border-success/20 bg-success/10 text-success',
  warning: 'border-warning/25 bg-warning/10 text-warning',
  danger: 'border-destructive/20 bg-destructive/10 text-destructive',
}

export interface StatusBadgeProps {
  tone?: StatusTone
  dot?: boolean
  className?: string
  children: ReactNode
}

/** 语义状态徽标：只允许语义 token，禁止页面再写 amber / emerald / green。 */
export function StatusBadge({ tone = 'neutral', dot = false, className, children }: StatusBadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex h-5 items-center gap-1.5 rounded-md border px-2 text-caption whitespace-nowrap',
        toneClass[tone],
        className,
      )}
    >
      {dot ? <span aria-hidden className="size-1.5 rounded-full bg-current" /> : null}
      {children}
    </span>
  )
}
