/** Move document(s) to a different folder dialog. */

import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { useMoveDocument } from '@/services/files-api'
import type { Folder } from '@/types/files'

interface MoveDocumentDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  projectId: string
  currentFolderId: string | null
  documentIds: string[]
  folders: Folder[]
}

export function MoveDocumentDialog({
  open,
  onOpenChange,
  projectId,
  currentFolderId,
  documentIds,
  folders,
}: MoveDocumentDialogProps) {
  const [targetFolderId, setTargetFolderId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const moveMutation = useMoveDocument(projectId, currentFolderId)
  const isPending = moveMutation.isPending

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setError(null)

    try {
      const folderId = targetFolderId === '_root' ? null : targetFolderId
      for (const docId of documentIds) {
        await moveMutation.mutateAsync({ documentId: docId, data: { folderId } })
      }
      setTargetFolderId(null)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : '移动失败')
    }
  }

  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      setTargetFolderId(null)
      setError(null)
    }
    onOpenChange(isOpen)
  }

  const count = documentIds.length
  const label = count === 1 ? '文档' : `${count} 篇文档`

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[400px] p-6 rounded-2xl border border-border/40 shadow-strong bg-background gap-6">
        <DialogHeader>
          <DialogTitle className="text-lg font-semibold tracking-tight">
            移动{label}
          </DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="p-3 rounded-lg bg-destructive/10 text-destructive text-[13px] font-medium">
              {error}
            </div>
          )}

          <div className="space-y-2">
            <Label className="text-[13px] font-medium">目标文件夹</Label>
            <select
              value={targetFolderId ?? '_root'}
              onChange={(e) => setTargetFolderId(e.target.value === '_root' ? null : e.target.value)}
              className="flex h-10 w-full rounded-lg border border-border/40 bg-muted/40 px-3 py-2 text-[14px] focus:outline-none focus:ring-1 focus:ring-foreground/20 focus:bg-background cursor-pointer"
            >
              <option value="_root">根目录（无文件夹）</option>
              {folders.map((f) => (
                <option key={f.id} value={f.id}>{f.name}</option>
              ))}
            </select>
          </div>

          <DialogFooter className="pt-2 sm:justify-end gap-2">
            <Button type="button" variant="ghost" onClick={() => handleOpenChange(false)} disabled={isPending} className="text-[13px] rounded-lg">
              取消
            </Button>
            <Button type="submit" disabled={isPending} className="text-[13px] rounded-lg px-6">
              {isPending ? '移动中...' : '移动'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
