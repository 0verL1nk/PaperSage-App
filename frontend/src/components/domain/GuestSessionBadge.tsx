import { Link } from '@tanstack/react-router'
import { HardDrive, UploadCloud } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { isDesktop } from '@/lib/desktop'
import { useSessionStore } from '@/store'

export const OPEN_GUEST_IMPORT_EVENT = 'papersage:open-guest-import'

// Persistent guest state belongs in the chrome, not in a full-width banner:
// nothing is wrong and there is no action to take until the user wants an account.
export function GuestModeChip({ className }: { className?: string }) {
  const isGuest = useSessionStore((state) => state.isGuest)
  if (!isGuest) return null
  return (
    <Button
      asChild
      variant="ghost"
      size="sm"
      className={className}
      title={isDesktop() ? '内容只保存在此设备，登录后可导入云端' : '内容只保存在此浏览器，登录后可导入云端'}
    >
      <Link to="/auth" aria-label="本机游客，登录并导入" className="gap-1.5 text-muted-foreground">
        <HardDrive className="size-3.5" />
        <span className="hidden text-xs sm:inline">本机</span>
      </Link>
    </Button>
  )
}

export function GuestSessionBadge({ compact = false }: { compact?: boolean }) {
  const isGuest = useSessionStore((state) => state.isGuest)
  const importPending = useSessionStore((state) => state.guestImportPending)

  if (!isGuest && !importPending) return null

  if (compact) {
    return (
      <div className="flex min-w-0 items-center gap-2">
        <Badge variant="outline" className="shrink-0 gap-1.5 border-amber-600/25 bg-amber-500/10 text-amber-800 dark:text-amber-300">
          <HardDrive className="size-3" /> {isGuest ? '本机游客 · 未同步' : '本机游客数据待导入'}
        </Badge>
        <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
          {isGuest ? '内容只保存在此设备' : '尚未上传到当前账号'}
        </span>
        {isGuest ? (
          <Button asChild size="sm" variant="outline" className="h-8 shrink-0 rounded-lg">
            <Link to="/auth"><UploadCloud className="mr-1.5 size-3.5" />登录并导入</Link>
          </Button>
        ) : (
          <Button
            size="sm"
            variant="outline"
            className="h-8 shrink-0 rounded-lg"
            onClick={() => window.dispatchEvent(new CustomEvent(OPEN_GUEST_IMPORT_EVENT))}
          >
            <UploadCloud className="mr-1.5 size-3.5" />检查并导入
          </Button>
        )}
      </div>
    )
  }

  if (isGuest) {
    return (
      <>
        <Link to="/auth" className="md:hidden" aria-label="本机游客，登录并导入">
          <Badge variant="outline" className="shrink-0 gap-1 border-amber-600/25 bg-amber-500/10 text-amber-800 dark:text-amber-300">
            <HardDrive className="size-3" /> 游客
          </Badge>
        </Link>
        <div className="hidden min-w-0 flex-wrap items-center gap-2 rounded-xl border border-amber-500/25 bg-amber-500/5 px-3 py-2 md:flex">
          <Badge variant="outline" className="shrink-0 gap-1.5 border-amber-600/25 bg-amber-500/10 text-amber-800 dark:text-amber-300">
            <HardDrive className="size-3" /> 本机游客 · 未同步
          </Badge>
          <span className="min-w-0 flex-1 text-xs leading-5 text-muted-foreground">
            {isDesktop() ? '内容只保存在此设备，卸载或清除应用数据可能无法恢复。' : '内容只保存在此浏览器，清除站点数据可能无法恢复。'}
          </span>
          <Button asChild size="sm" variant="outline" className="min-h-10 shrink-0 rounded-lg">
            <Link to="/auth"><UploadCloud className="mr-1.5 size-4" />登录并导入</Link>
          </Button>
        </div>
      </>
    )
  }

  return (
    <>
      <Button
        size="sm"
        variant="outline"
        className="min-h-10 shrink-0 rounded-lg md:hidden"
        aria-label="本机游客数据待导入"
        onClick={() => window.dispatchEvent(new CustomEvent(OPEN_GUEST_IMPORT_EVENT))}
      >
        <HardDrive className="size-3" /> 待导入
      </Button>
      <div className="hidden min-w-0 flex-wrap items-center gap-2 rounded-xl border border-primary/20 bg-primary/5 px-3 py-2 md:flex">
        <Badge variant="outline" className="shrink-0 gap-1.5 border-primary/25 bg-primary/5 text-primary">
          <HardDrive className="size-3" /> 本机游客数据待导入
        </Badge>
        <span className="min-w-0 flex-1 text-xs leading-5 text-muted-foreground">尚未上传到当前账号。</span>
        <Button
          size="sm"
          variant="outline"
          className="min-h-10 shrink-0 rounded-lg"
          onClick={() => window.dispatchEvent(new CustomEvent(OPEN_GUEST_IMPORT_EVENT))}
        >
          <UploadCloud className="mr-1.5 size-4" />检查并导入
        </Button>
      </div>
    </>
  )
}
