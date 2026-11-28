import { Switch as RadixSwitch } from 'radix-ui'

import { cn } from '@/lib/utils'

/** 语义化开关：复用项目已安装的 radix-ui 原语，键盘与 aria 交给原语。 */
export function Switch({
  checked,
  onCheckedChange,
  disabled,
  id,
  'aria-label': ariaLabel,
  className,
}: {
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  disabled?: boolean
  id?: string
  'aria-label'?: string
  className?: string
}) {
  return (
    <RadixSwitch.Root
      id={id}
      checked={checked}
      onCheckedChange={onCheckedChange}
      disabled={disabled}
      aria-label={ariaLabel}
      className={cn(
        'relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border border-transparent bg-muted transition-colors',
        'data-[state=checked]:bg-primary disabled:cursor-not-allowed disabled:opacity-50',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        className,
      )}
    >
      <RadixSwitch.Thumb className="block size-4 translate-x-0.5 rounded-full bg-background shadow transition-transform data-[state=checked]:translate-x-[18px]" />
    </RadixSwitch.Root>
  )
}
