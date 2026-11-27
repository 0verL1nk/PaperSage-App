import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

const sizeClass = {
  narrow: 'max-w-3xl',
  default: 'max-w-5xl',
  wide: 'max-w-7xl',
} as const

export interface PageContainerProps {
  children: ReactNode
  /** narrow = max-w-3xl · default = max-w-5xl · wide = max-w-7xl */
  size?: keyof typeof sizeClass
  className?: string
}

/** 页面级留白与最大宽度：替代各页手写的 mx-auto / max-w-* / p-*。 */
export function PageContainer({ children, size = 'default', className }: PageContainerProps) {
  return (
    <div className={cn('mx-auto w-full px-4 py-6 sm:px-6 lg:px-8', sizeClass[size], className)}>
      {children}
    </div>
  )
}
