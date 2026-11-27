import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { ToggleGroup } from 'radix-ui'
import { cn } from '@/lib/utils'

export interface SegmentedControlItem<T extends string> {
  value: T
  label: ReactNode
  icon?: LucideIcon
}

export interface SegmentedControlProps<T extends string> {
  items: SegmentedControlItem<T>[]
  value: T
  onValueChange: (value: T) => void
  ariaLabel: string
  size?: 'sm' | 'md'
  className?: string
}

/** 基于 Radix ToggleGroup 的分段筛选；键盘 / ARIA 交给原语，不自写。 */
export function SegmentedControl<T extends string>({
  items,
  value,
  onValueChange,
  ariaLabel,
  size = 'md',
  className,
}: SegmentedControlProps<T>) {
  return (
    <ToggleGroup.Root
      type="single"
      value={value}
      onValueChange={(next) => {
        if (next) onValueChange(next as T)
      }}
      aria-label={ariaLabel}
      className={cn(
        'inline-flex w-fit max-w-full items-center gap-1 overflow-x-auto rounded-lg border border-border bg-muted/50 p-1 no-scrollbar',
        className,
      )}
    >
      {items.map((item) => {
        const Icon = item.icon
        return (
          <ToggleGroup.Item
            key={item.value}
            value={item.value}
            className={cn(
              'inline-flex flex-none items-center gap-1.5 rounded-md px-3 font-medium text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring data-[state=on]:bg-background data-[state=on]:text-foreground data-[state=on]:shadow-sm',
              size === 'sm' ? 'h-7 text-body-sm' : 'h-8 text-sm',
            )}
          >
            {Icon ? <Icon className="size-3.5" /> : null}
            {item.label}
          </ToggleGroup.Item>
        )
      })}
    </ToggleGroup.Root>
  )
}
