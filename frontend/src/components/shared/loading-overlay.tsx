import { LoaderCircle } from 'lucide-react'
import { useUiStore } from '@/store'

export function LoadingOverlay() {
  const loadingCount = useUiStore((state) => state.loadingCount)
  const loadingText = useUiStore((state) => state.loadingText)

  if (loadingCount === 0) {
    return null
  }

  return (
    <div className="pointer-events-none fixed inset-0 z-50 flex items-center justify-center bg-slate-950/20 px-4 backdrop-blur-sm">
      <div className="surface-card flex min-w-[16rem] max-w-sm items-center gap-3 rounded-3xl border border-white/50 bg-[color:var(--surface-subtle)] px-5 py-4 text-foreground">
        <div className="flex size-10 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
          <LoaderCircle className="size-5 animate-spin" />
        </div>
        <div className="flex flex-col gap-1">
          <p className="text-sm font-medium">全局请求处理中</p>
          <p className="text-sm text-muted-foreground">{loadingText || '加载中...'}</p>
        </div>
      </div>
    </div>
  )
}
