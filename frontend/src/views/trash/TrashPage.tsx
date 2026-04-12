import { cn } from '@/lib/utils'
import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTrash, useRestoreProject, usePermanentDeleteProject } from '@/services/projects-api'
import type { TrashItem } from '@/services/projects-api'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Trash2, RotateCcw, ArrowLeft, FolderGit2, AlertCircle } from 'lucide-react'

export function TrashPage() {
  const { data: items = [], isLoading, error } = useTrash()
  const restoreProject = useRestoreProject()
  const permanentDelete = usePermanentDeleteProject()
  const [deleteTarget, setDeleteTarget] = useState<TrashItem | null>(null)

  const handleRestore = async (item: TrashItem) => {
    if (item.type === 'project') {
      await restoreProject.mutateAsync(item.id)
    }
  }

  const handlePermanentDelete = async () => {
    if (deleteTarget?.type === 'project') {
      await permanentDelete.mutateAsync(deleteTarget.id)
    }
    setDeleteTarget(null)
  }

  if (isLoading) {
    return (
      <div className="p-8 md:p-12 max-w-5xl mx-auto space-y-8">
        <div className="flex items-center justify-between">
          <Skeleton className="h-10 w-32 rounded-lg" />
          <Skeleton className="h-9 w-32 rounded-full" />
        </div>
        <div className="space-y-3">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-20 w-full rounded-2xl" />
          ))}
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex h-[50vh] items-center justify-center p-8">
        <div className="text-center space-y-4">
          <div className="inline-flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10 text-destructive">
            <AlertCircle className="h-6 w-6" />
          </div>
          <p className="text-sm font-medium text-destructive">加载失败</p>
          <p className="text-xs text-muted-foreground">{error.message}</p>
        </div>
      </div>
    )
  }

  return (
    <div className="p-8 md:p-12 max-w-5xl mx-auto space-y-10 pb-24">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="space-y-1">
          <h1 className="text-3xl font-semibold tracking-tight text-foreground flex items-center gap-3">
            <Trash2 className="h-7 w-7 text-muted-foreground/50 stroke-[1.5]" />
            回收站
          </h1>
          <p className="text-sm text-muted-foreground">在此处管理已删除的项目，您可以选择恢复或永久删除</p>
        </div>
        <Button variant="ghost" size="sm" asChild className="rounded-full text-muted-foreground hover:text-foreground">
          <Link to="/app/projects">
            <ArrowLeft className="mr-2 h-4 w-4" />
            返回项目
          </Link>
        </Button>
      </div>

      {/* Trash List */}
      {items.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="mb-6 flex h-16 w-16 items-center justify-center rounded-2xl bg-muted/30">
            <Trash2 className="h-8 w-8 text-muted-foreground/30 stroke-[1.5]" />
          </div>
          <h3 className="text-lg font-medium text-foreground">回收站是空的</h3>
          <p className="text-sm text-muted-foreground mt-2">在这里您找不到任何被删除的项目。</p>
        </div>
      ) : (
        <div className="grid gap-3">
          {items.map((item) => (
            <div 
              key={item.id} 
              className="group flex items-center justify-between p-4 bg-card rounded-2xl border border-border/40 hover:border-border/80 transition-all"
            >
              <div className="flex items-center gap-4 flex-1 min-w-0">
                <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-muted/50 text-muted-foreground">
                  <FolderGit2 className="h-5 w-5 stroke-[1.5]" />
                </div>
                <div className="flex-1 min-w-0">
                  <h3 className="font-medium text-[15px] truncate text-foreground">{item.name}</h3>
                  <p className="text-[11px] text-muted-foreground mt-0.5">
                    删除于: {new Date(item.deleted_at).toLocaleString('zh-CN')}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleRestore(item)}
                  disabled={restoreProject.isPending}
                  className="rounded-full h-8 text-muted-foreground hover:text-primary hover:bg-primary/5"
                >
                  <RotateCcw className="mr-2 h-3.5 w-3.5" />
                  恢复
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setDeleteTarget(item)}
                  disabled={permanentDelete.isPending}
                  className="rounded-full h-8 text-muted-foreground hover:text-destructive hover:bg-destructive/5"
                >
                  <Trash2 className="mr-2 h-3.5 w-3.5" />
                  彻底删除
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title="永久删除"
        description={
          deleteTarget
            ? `确定要永久删除 "${deleteTarget.name}" 吗？此操作将无法撤销。`
            : ""
        }
        confirmText="永久删除"
        variant="destructive"
        onConfirm={handlePermanentDelete}
        isPending={permanentDelete.isPending}
      />
    </div>
  )
}
