import { Link } from '@tanstack/react-router'
import { HardDrive, UploadCloud } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useSessionStore } from '@/store'

export const OPEN_GUEST_IMPORT_EVENT = 'papersage:open-guest-import'

export function GuestSessionBadge() {
  const isGuest = useSessionStore((state) => state.isGuest)
  const importPending = useSessionStore((state) => state.guestImportPending)

  if (!isGuest && !importPending) return null

  if (isGuest) {
    return (
      <div className="flex min-w-0 flex-wrap items-center gap-2 rounded-xl border border-amber-500/25 bg-amber-500/5 px-3 py-2">
        <Badge variant="outline" className="shrink-0 gap-1.5 border-amber-600/25 bg-amber-500/10 text-amber-800 dark:text-amber-300">
          <HardDrive className="size-3" /> 本机游客 · 未同步
        </Badge>
        <span className="min-w-0 flex-1 text-xs leading-5 text-muted-foreground">内容只保存在此浏览器，清除站点数据可能无法恢复。</span>
        <Button asChild size="sm" variant="outline" className="min-h-10 shrink-0 rounded-lg">
          <Link to="/auth"><UploadCloud className="mr-1.5 size-4" />登录并导入</Link>
        </Button>
      </div>
    )
  }

  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2 rounded-xl border border-primary/20 bg-primary/5 px-3 py-2">
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
  )
}
