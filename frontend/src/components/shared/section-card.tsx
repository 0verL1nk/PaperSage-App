import type { ReactNode } from 'react'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { cn } from '@/lib/utils'

export interface SectionCardProps {
  title?: ReactNode
  description?: ReactNode
  actions?: ReactNode
  size?: 'default' | 'sm'
  className?: string
  contentClassName?: string
  children: ReactNode
}

/**
 * 区块卡片：统一标题 / 说明 / 操作区与内容留白。
 * 规则：SectionCard 内不再嵌 SectionCard；内部分区用 Separator 或浅色 div。
 */
export function SectionCard({
  title,
  description,
  actions,
  size = 'default',
  className,
  contentClassName,
  children,
}: SectionCardProps) {
  const hasHeader = Boolean(title || description || actions)
  return (
    <Card size={size} className={cn('gap-3', className)}>
      {hasHeader ? (
        <CardHeader>
          {title ? <CardTitle>{title}</CardTitle> : null}
          {description ? <CardDescription>{description}</CardDescription> : null}
          {actions ? <CardAction>{actions}</CardAction> : null}
        </CardHeader>
      ) : null}
      <CardContent className={contentClassName}>{children}</CardContent>
    </Card>
  )
}
