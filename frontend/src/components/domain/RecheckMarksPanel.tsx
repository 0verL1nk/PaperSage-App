import { useState } from 'react'
import { LoaderCircle, RefreshCw, ShieldQuestion } from 'lucide-react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import type { RecheckStatus } from '@/services/recheck-api'
import { useRecheckMarks, useResolveRecheckMark } from '@/services/recheck-api'
import type { RecheckMark, RecheckResolution } from '@/types/research'

const RESOLUTION_LABEL: Record<RecheckResolution, string> = {
  rechecked: '已复查',
  rebuilt: '已重建',
  dismissed: '忽略',
}

const ACTIONS: Array<{ resolution: RecheckResolution; label: string; hint: string }> = [
  { resolution: 'rechecked', label: '已复查', hint: '我核对过，原内容仍然成立' },
  { resolution: 'rebuilt', label: '已重建', hint: '我已重新生成一版内容' },
  { resolution: 'dismissed', label: '忽略', hint: '这条提醒不需要处理' },
]

function formatTime(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}

/** 准备页：资料变更后受影响内容的逐条确认入口。系统只提醒，不自动改写任何内容。 */
export function RecheckMarksPanel({ projectId }: { projectId: string }) {
  const [status, setStatus] = useState<RecheckStatus>('open')
  // 两个状态都读：待处理清零后仍要能让用户切到「已处理」回看记录。
  const open = useRecheckMarks(projectId, 'open')
  const resolved = useRecheckMarks(projectId, 'resolved')
  const active = status === 'open' ? open : resolved
  const marks = active.data ?? []
  const { isLoading, error, refetch } = active
  const resolve = useResolveRecheckMark(projectId)

  const handle = (mark: RecheckMark, resolution: RecheckResolution) => {
    resolve.mutateAsync({ markId: mark.id, resolution }).catch((err) => {
      toast.error('处理失败', { description: err instanceof Error ? err.message : String(err) })
    })
  }

  // 完全没有待处理、也没有处理记录时才不占准备页版面；
  // 有历史记录（或读取出错）时保留面板，用户仍可切到「已处理」回看。
  const hasAny =
    (open.data ?? []).length > 0 ||
    (resolved.data ?? []).length > 0 ||
    Boolean(open.error) ||
    Boolean(resolved.error)
  if (!open.isLoading && !resolved.isLoading && !hasAny) return null

  return (
    <Card className="rounded-2xl border-amber-500/30 bg-amber-500/5 shadow-sm">
      <CardHeader className="pb-2">
        <CardTitle className="flex items-center gap-2 text-sm">
          <ShieldQuestion className="size-4 text-amber-700 dark:text-amber-400" />待复查
        </CardTitle>
        <CardDescription>
          资料被删除或出了新版本后，依赖它的产物、证据和知识页会在这里提醒。待复查只表示依据变了，是否仍然成立需要你确认；系统不会自动改写正文或产物。
        </CardDescription>
        <div role="tablist" aria-label="待复查状态" className="flex gap-1 pt-2">
          {(['open', 'resolved'] as const).map((item) => (
            <button
              key={item}
              type="button"
              role="tab"
              aria-selected={status === item}
              onClick={() => setStatus(item)}
              className={`min-h-[44px] rounded-full px-3 text-[11px] ${status === item ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'}`}
            >
              {item === 'open' ? '待处理' : '已处理'}
            </button>
          ))}
        </div>
      </CardHeader>
      <CardContent className="space-y-2 px-4 pb-4">
        {isLoading ? (
          <p role="status" className="flex items-center gap-2 text-xs text-muted-foreground">
            <LoaderCircle className="size-3.5 animate-spin" />正在加载待复查项…
          </p>
        ) : null}
        {error ? (
          <div role="alert" className="flex flex-wrap items-center gap-2 text-xs text-destructive">
            <span>待复查列表加载失败：{error.message}</span>
            <Button size="sm" variant="outline" className="min-h-[44px]" onClick={() => void refetch()}>
              <RefreshCw className="mr-1 size-3.5" />重试
            </Button>
          </div>
        ) : null}
        {!isLoading && !error && marks.length === 0 ? (
          <p className="text-xs text-muted-foreground">{status === 'open' ? '没有待复查项。' : '还没有处理记录。'}</p>
        ) : null}
        {marks.map((mark) => (
          <div key={mark.id} className="rounded-xl border border-border/60 bg-background/80 p-3">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="outline" className="font-normal">{mark.triggerLabel}</Badge>
              <span className="text-xs font-medium">{mark.targetTitle}</span>
              {mark.targetDetail ? <span className="text-[11px] text-muted-foreground">{mark.targetDetail}</span> : null}
              {mark.targetMissing ? <Badge variant="secondary" className="font-normal">目标已不存在</Badge> : null}
            </div>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">{mark.reason}</p>
            <p className="mt-1 text-[10px] text-muted-foreground">
              {formatTime(mark.createdAt)}
              {mark.resolvedAt ? ` · 处理于 ${formatTime(mark.resolvedAt)}` : ''}
            </p>
            {status === 'open' ? (
              <div className="mt-2 flex flex-wrap gap-2">
                {ACTIONS.map((action) => (
                  <Button
                    key={action.resolution}
                    size="sm"
                    variant={action.resolution === 'rechecked' ? 'default' : 'outline'}
                    className="min-h-[44px]"
                    title={action.hint}
                    disabled={resolve.isPending}
                    onClick={() => handle(mark, action.resolution)}
                  >
                    {action.label}
                  </Button>
                ))}
              </div>
            ) : mark.resolution ? (
              <Badge variant="secondary" className="mt-2 font-normal">{RESOLUTION_LABEL[mark.resolution]}</Badge>
            ) : null}
          </div>
        ))}
      </CardContent>
    </Card>
  )
}

export default RecheckMarksPanel
