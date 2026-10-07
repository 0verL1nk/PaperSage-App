import { useState } from 'react'
import { Check, Cloud, HardDrive } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import { appEnv, getApiTarget, setApiTarget, type ApiTarget } from '@/lib/env'

/**
 * 环境切换：在线（部署的后端）/ 本地（localhost 后端）。
 *
 * 切换后**整页刷新**——访问令牌与 React Query 缓存都绑定在原来的后端上，
 * 不刷新会拿到「A 后端签发的 token 去请求 B 后端」这种半坏状态。
 */
export function ApiTargetToggle({ prominent = false }: { prominent?: boolean }) {
  const [target, setTarget] = useState<ApiTarget>(() => getApiTarget())

  const choose = (next: ApiTarget) => {
    if (next === target) return
    setApiTarget(next)
    setTarget(next)
    if (typeof window !== 'undefined') window.location.reload()
  }

  const isLocal = target === 'local'

  const options: { value: ApiTarget; label: string; hint: string; icon: typeof Cloud }[] = [
    { value: 'online', label: '在线', hint: appEnv.apiBaseUrl || '同源（前端由后端一起提供）', icon: Cloud },
    { value: 'local', label: '本地', hint: appEnv.localApiBaseUrl, icon: HardDrive },
  ]

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className={cn(
            'gap-1.5 rounded-full border-border/55 bg-background/75 px-2.5 font-medium text-foreground shadow-sm backdrop-blur-md transition-colors hover:bg-background',
            prominent ? 'h-[44px] px-3 text-xs sm:px-3.5' : 'h-8 text-xs sm:px-3',
          )}
          title={isLocal ? `接口环境：本地（${appEnv.localApiBaseUrl}）` : '接口环境：在线'}
          aria-label={`切换后端，当前：${isLocal ? '本地' : '在线'}`}
        >
          {isLocal ? <HardDrive className="size-3.5" /> : <Cloud className="size-3.5" />}
          {prominent && <span>后端</span>}
          <span>{isLocal ? '本地' : '在线'}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuLabel>后端环境</DropdownMenuLabel>
        {options.map((opt) => {
          const Icon = opt.icon
          const active = opt.value === target
          return (
            <DropdownMenuItem
              key={opt.value}
              onSelect={() => choose(opt.value)}
              className="flex items-start gap-2"
            >
              <Icon className="mt-0.5 size-4 shrink-0 opacity-70" />
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-medium">{opt.label}</span>
                <span className="block truncate text-xs text-muted-foreground">{opt.hint}</span>
              </span>
              <Check className={cn('mt-0.5 size-4 shrink-0', active ? 'opacity-100' : 'opacity-0')} />
            </DropdownMenuItem>
          )
        })}
        <DropdownMenuSeparator />
        <p className="px-3 py-2 text-xs text-muted-foreground">切换后页面会刷新</p>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
