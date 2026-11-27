import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Inbox, LoaderCircle, TriangleAlert, type LucideIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

export interface StateBlockProps {
  title: string
  description?: string
  icon?: LucideIcon
  action?: ReactNode
  /** 面板内嵌：去掉外框与大面积留白 */
  compact?: boolean
  className?: string
  children?: ReactNode
}

/** 全站统一的空 / 错 / 载入反馈基座；文案一律由调用方传入。 */
export function StateBlock({
  title,
  description,
  icon: Icon = Inbox,
  action,
  compact = false,
  className,
  children,
}: StateBlockProps) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center text-center',
        compact
          ? 'px-4 py-8'
          : 'rounded-xl border border-border bg-card px-6 py-16',
        className,
      )}
    >
      <div className="mb-4 flex size-12 items-center justify-center rounded-xl bg-muted text-muted-foreground">
        <Icon className="size-6" strokeWidth={1.5} />
      </div>
      <h3 className="text-base font-medium text-foreground">{title}</h3>
      {description ? (
        <p className="mt-1.5 max-w-sm text-body-sm text-muted-foreground">{description}</p>
      ) : null}
      {children}
      {action ? <div className="mt-5">{action}</div> : null}
    </div>
  )
}

export interface EmptyStateProps {
  title: string
  description?: string
  icon?: LucideIcon
  action?: ReactNode
  compact?: boolean
  className?: string
}

export function EmptyState(props: EmptyStateProps) {
  return <StateBlock {...props} />
}

export interface ErrorStateProps {
  title: string
  description?: string
  details?: string
  onRetry?: () => void
  retryLabel?: string
  compact?: boolean
  className?: string
}

export function ErrorState({
  title,
  description,
  details,
  onRetry,
  retryLabel,
  compact,
  className,
}: ErrorStateProps) {
  const { t } = useTranslation()
  return (
    <StateBlock
      title={title}
      description={description}
      icon={TriangleAlert}
      compact={compact}
      className={className}
      action={
        onRetry ? (
          <Button variant="outline" size="sm" onClick={onRetry}>
            {retryLabel ?? t('common.retry')}
          </Button>
        ) : undefined
      }
    >
      {details ? (
        <p className="mt-3 max-w-sm break-words font-mono text-caption text-muted-foreground">
          {details}
        </p>
      ) : null}
    </StateBlock>
  )
}

export interface LoadingStateProps {
  title: string
  description?: string
  variant?: 'skeleton' | 'spinner'
  rows?: number
  compact?: boolean
  className?: string
}

export function LoadingState({
  title,
  description,
  variant = 'skeleton',
  rows = 3,
  compact = false,
  className,
}: LoadingStateProps) {
  if (variant === 'spinner') {
    return (
      <StateBlock
        title={title}
        description={description}
        icon={LoaderCircle}
        compact={compact}
        className={cn('[&_svg]:animate-spin', className)}
      />
    )
  }

  return (
    <div
      role="status"
      aria-live="polite"
      className={cn(
        compact ? 'px-4 py-6' : 'rounded-xl border border-border bg-card p-6',
        className,
      )}
    >
      <div className="mb-4 space-y-1">
        <p className="text-body-sm font-medium text-foreground">{title}</p>
        {description ? (
          <p className="text-caption text-muted-foreground">{description}</p>
        ) : null}
      </div>
      <div className="space-y-3">
        <Skeleton className="h-4 w-2/5" />
        {Array.from({ length: rows }).map((_, index) => (
          <Skeleton key={index} className="h-10 w-full" />
        ))}
      </div>
    </div>
  )
}
