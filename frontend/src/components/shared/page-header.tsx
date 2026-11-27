import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

export interface PageHeaderProps {
  title: ReactNode
  description?: ReactNode
  eyebrow?: ReactNode
  icon?: LucideIcon
  actions?: ReactNode
  /** 已渲染的返回链接（如 TanStack <Link>）；目标与文案由调用方决定。 */
  back?: ReactNode
  className?: string
}

/** 页面标题区：统一 h1 字阶、说明文案与右侧操作区，移动端纵向堆叠。 */
export function PageHeader({
  title,
  description,
  eyebrow,
  icon: Icon,
  actions,
  back,
  className,
}: PageHeaderProps) {
  return (
    <div
      className={cn(
        'flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between',
        className,
      )}
    >
      <div className="flex min-w-0 items-start gap-3">
        {back ? <div className="mt-0.5 flex-none">{back}</div> : null}
        {Icon ? (
          <div className="mt-0.5 flex size-9 flex-none items-center justify-center rounded-lg border border-border bg-card text-muted-foreground">
            <Icon className="size-4" />
          </div>
        ) : null}
        <div className="min-w-0 space-y-1">
          {eyebrow ? (
            <p className="text-caption font-medium tracking-wide text-muted-foreground uppercase">
              {eyebrow}
            </p>
          ) : null}
          <h1 className="text-h1 text-foreground">{title}</h1>
          {description ? (
            <p className="text-body-sm text-muted-foreground">{description}</p>
          ) : null}
        </div>
      </div>
      {actions ? (
        <div className="flex flex-none flex-wrap items-center gap-2">{actions}</div>
      ) : null}
    </div>
  )
}
