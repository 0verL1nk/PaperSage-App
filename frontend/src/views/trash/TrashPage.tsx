import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { ArrowLeft, FolderGit2, RotateCcw, Trash2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState, ErrorState, LoadingState, PageContainer, PageHeader } from '@/components/shared'
import { useTrash, useRestoreProject, usePermanentDeleteProject } from '@/services/projects-api'
import type { TrashItem } from '@/services/projects-api'

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
      <PageContainer size="default">
        <LoadingState title="正在加载回收站" rows={3} />
      </PageContainer>
    )
  }

  if (error) {
    return (
      <PageContainer size="default">
        <ErrorState title="加载失败" description={error.message} />
      </PageContainer>
    )
  }

  return (
    <PageContainer size="default" className="space-y-8 pb-24">
      <PageHeader
        title="回收站"
        description="在此处管理已删除的项目，您可以选择恢复或永久删除"
        icon={Trash2}
        back={
          <Button variant="ghost" size="sm" asChild className="text-muted-foreground">
            <Link to="/app/projects">
              <ArrowLeft className="mr-1.5 size-4" />
              返回项目
            </Link>
          </Button>
        }
      />

      {items.length === 0 ? (
        <EmptyState
          title="回收站是空的"
          description="在这里您找不到任何被删除的项目。"
          icon={Trash2}
        />
      ) : (
        <div className="grid gap-3">
          {items.map((item) => (
            <div
              key={item.id}
              className="group flex items-center justify-between gap-3 rounded-lg border border-border bg-card p-4 transition-colors hover:border-border-strong"
            >
              <div className="flex min-w-0 flex-1 items-center gap-4">
                <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                  <FolderGit2 className="size-5" strokeWidth={1.5} />
                </div>
                <div className="min-w-0 flex-1">
                  <h3 className="truncate text-body font-medium text-foreground">{item.name}</h3>
                  <p className="mt-0.5 text-caption text-muted-foreground">
                    删除于: {new Date(item.deleted_at).toLocaleString('zh-CN')}
                  </p>
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleRestore(item)}
                  disabled={restoreProject.isPending}
                  className="min-h-11 text-muted-foreground hover:text-primary"
                >
                  <RotateCcw className="mr-2 size-3.5" />
                  恢复
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setDeleteTarget(item)}
                  disabled={permanentDelete.isPending}
                  className="min-h-11 text-muted-foreground hover:text-destructive"
                >
                  <Trash2 className="mr-2 size-3.5" />
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
            : ''
        }
        confirmText="永久删除"
        variant="destructive"
        onConfirm={handlePermanentDelete}
        isPending={permanentDelete.isPending}
      />
    </PageContainer>
  )
}
