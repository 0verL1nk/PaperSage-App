/**
 * TrashPage - displays deleted projects
 */
import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTrash, useRestoreProject, usePermanentDeleteProject } from '../api'
import type { TrashItem } from '../api'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Trash2, RotateCcw, ArrowLeft } from 'lucide-react'

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
      <div className="p-6 space-y-6">
        <div className="flex items-center justify-between">
          <Skeleton className="h-8 w-24" />
          <Skeleton className="h-10 w-32" />
        </div>
        <div className="space-y-2">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-20 w-full" />
          ))}
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-6">
        <Card className="p-6 text-center text-destructive">
          <p>Failed to load trash: {error.message}</p>
        </Card>
      </div>
    )
  }

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold tracking-tight flex items-center gap-2">
          <Trash2 className="h-6 w-6" />
          Trash
        </h1>
        <Button variant="ghost" size="sm" asChild>
          <Link to="/app/projects">
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Projects
          </Link>
        </Button>
      </div>

      {/* Trash List */}
      {items.length === 0 ? (
        <Card className="p-12 text-center">
          <div className="flex flex-col items-center gap-4">
            <div className="rounded-full bg-muted p-4">
              <Trash2 className="h-8 w-8 text-muted-foreground" />
            </div>
            <div>
              <p className="font-medium">Trash is empty</p>
              <p className="text-sm text-muted-foreground">
                Deleted projects will appear here
              </p>
            </div>
          </div>
        </Card>
      ) : (
        <div className="space-y-2">
          {items.map((item) => (
            <Card key={item.id} className="p-4">
              <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3 flex-1 min-w-0">
                  <div className="rounded-lg bg-muted p-2">
                    <Trash2 className="h-4 w-4 text-muted-foreground" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <h3 className="font-medium truncate">{item.name}</h3>
                    <p className="text-xs text-muted-foreground">
                      Deleted: {new Date(item.deleted_at).toLocaleDateString()}
                    </p>
                  </div>
                </div>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => handleRestore(item)}
                    disabled={restoreProject.isPending}
                  >
                    <RotateCcw className="mr-2 h-4 w-4" />
                    {restoreProject.isPending ? '恢复中...' : '恢复'}
                  </Button>
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={() => setDeleteTarget(item)}
                    disabled={permanentDelete.isPending}
                  >
                    <Trash2 className="mr-2 h-4 w-4" />
                    {permanentDelete.isPending ? '删除中...' : '删除'}
                  </Button>
                </div>
              </div>
            </Card>
          ))}
        </div>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title="永久删除"
        description={
          deleteTarget
            ? `确定要永久删除 "${deleteTarget.name}" 吗？此操作不可撤销。`
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
