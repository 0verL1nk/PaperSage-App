import { useState } from 'react'
import { Check, Cloud, HardDrive, Link2 } from 'lucide-react'

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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  appEnv,
  getApiTarget,
  getCustomApiBaseUrl,
  normalizeApiBaseUrl,
  setApiTarget,
  setCustomApiBaseUrl,
  type ApiTarget,
} from '@/lib/env'

/**
 * 环境切换：在线（部署的后端）/ 本地（localhost 后端）。
 *
 * 切换后**整页刷新**——访问令牌与 React Query 缓存都绑定在原来的后端上，
 * 不刷新会拿到「A 后端签发的 token 去请求 B 后端」这种半坏状态。
 */
export function ApiTargetToggle(props: { prominent?: boolean }) {
  // 生产构建只暴露部署后端；切换入口是开发/联调能力，不对普通用户展示
  if (import.meta.env.PROD) return null
  return <ApiTargetToggleMenu {...props} />
}

function ApiTargetToggleMenu({ prominent = false }: { prominent?: boolean }) {
  const [target, setTarget] = useState<ApiTarget>(() => getApiTarget())
  const [customDialogOpen, setCustomDialogOpen] = useState(false)
  const [customAddress, setCustomAddress] = useState(() => getCustomApiBaseUrl())
  const [customAddressError, setCustomAddressError] = useState('')

  const choose = (next: ApiTarget) => {
    if (next === target) return
    if (next === 'custom' && !getCustomApiBaseUrl()) {
      setCustomDialogOpen(true)
      return
    }
    setApiTarget(next)
    setTarget(next)
    if (typeof window !== 'undefined') window.location.reload()
  }

  const isLocal = target === 'local'
  const isCustom = target === 'custom'
  const customAddressHint = getCustomApiBaseUrl() || '尚未配置'

  const saveCustomAddress = () => {
    try {
      setCustomApiBaseUrl(normalizeApiBaseUrl(customAddress))
      setApiTarget('custom')
      setTarget('custom')
      setCustomAddressError('')
      setCustomDialogOpen(false)
      if (typeof window !== 'undefined') window.location.reload()
    } catch (error) {
      setCustomAddressError(error instanceof Error ? error.message : '后端地址无效')
    }
  }

  const options: { value: ApiTarget; label: string; hint: string; icon: typeof Cloud }[] = [
    { value: 'online', label: '在线', hint: appEnv.apiBaseUrl || '同源（前端由后端一起提供）', icon: Cloud },
    { value: 'local', label: '本地', hint: appEnv.localApiBaseUrl, icon: HardDrive },
    { value: 'custom', label: '自定义', hint: customAddressHint, icon: Link2 },
  ]

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant={prominent ? 'outline' : 'ghost'}
          size={prominent ? 'sm' : 'icon'}
          className={cn(
            prominent
              ? 'h-[44px] gap-1.5 rounded-full border-border/55 bg-background/75 px-3 text-xs font-medium text-foreground shadow-sm backdrop-blur-md transition-colors hover:bg-background sm:px-3.5'
              : 'relative size-12 rounded-full text-muted-foreground/75 hover:bg-muted/70 hover:text-foreground',
          )}
          title={isLocal ? `接口环境：本地（${appEnv.localApiBaseUrl}）` : isCustom ? `接口环境：自定义（${customAddressHint}）` : '接口环境：在线'}
          aria-label={`切换后端，当前：${isLocal ? '本地' : isCustom ? '自定义' : '在线'}`}
        >
          {isLocal ? <HardDrive className="size-3.5" /> : isCustom ? <Link2 className="size-3.5" /> : <Cloud className="size-3.5" />}
          {prominent ? (
            <><span>后端</span><span>{isLocal ? '本地' : isCustom ? '自定义' : '在线'}</span></>
          ) : (
            <span aria-hidden="true" className={cn(
              'absolute bottom-1 right-1 size-1.5 rounded-full ring-2 ring-background',
              isLocal ? 'bg-amber-500' : isCustom ? 'bg-sky-500' : 'bg-emerald-500',
            )} />
          )}
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
        <DropdownMenuItem
          onSelect={() => {
            setCustomAddress(getCustomApiBaseUrl())
            setCustomAddressError('')
            setCustomDialogOpen(true)
          }}
          className="flex items-start gap-2"
        >
          <Link2 className="mt-0.5 size-4 shrink-0 opacity-70" />
          <span className="min-w-0 flex-1">
            <span className="block text-sm font-medium">配置自定义地址</span>
            <span className="block truncate text-xs text-muted-foreground">{customAddressHint}</span>
          </span>
          <Check className={cn('mt-0.5 size-4 shrink-0', isCustom ? 'opacity-100' : 'opacity-0')} />
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <p className="px-3 py-2 text-xs text-muted-foreground">切换后页面会刷新</p>
      </DropdownMenuContent>
      <Dialog open={customDialogOpen} onOpenChange={setCustomDialogOpen}>
        <DialogContent className="w-[calc(100vw-1rem)] rounded-2xl border border-border/40 bg-background p-5 shadow-strong sm:max-w-[440px] sm:p-6">
          <DialogHeader>
            <DialogTitle className="text-xl font-semibold tracking-tight text-foreground">自定义后端</DialogTitle>
            <DialogDescription className="text-sm leading-5">填写 Go API 地址。切换后页面会刷新。</DialogDescription>
          </DialogHeader>
          <div className="space-y-2.5">
            <Label htmlFor="custom-api-base-url" className="text-[13px] font-medium text-foreground">API 地址</Label>
            <Input
              id="custom-api-base-url"
              inputMode="url"
              autoComplete="url"
              autoFocus
              className="h-11 bg-muted/40 text-sm focus-visible:bg-background"
              placeholder="https://api.example.com"
              value={customAddress}
              onChange={(event) => {
                setCustomAddress(event.target.value)
                setCustomAddressError('')
              }}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.nativeEvent.isComposing) saveCustomAddress()
              }}
              aria-invalid={Boolean(customAddressError)}
              aria-describedby={customAddressError ? 'custom-api-error' : undefined}
            />
            {customAddressError && <p id="custom-api-error" role="alert" className="text-[13px] text-destructive">{customAddressError}</p>}
          </div>
          <DialogFooter className="-mx-5 -mb-5 mt-1 rounded-b-2xl px-5 py-4 sm:-mx-6 sm:-mb-6 sm:px-6">
            <Button variant="ghost" className="rounded-lg" onClick={() => setCustomDialogOpen(false)}>取消</Button>
            <Button className="rounded-lg" onClick={saveCustomAddress}>连接</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DropdownMenu>
  )
}
