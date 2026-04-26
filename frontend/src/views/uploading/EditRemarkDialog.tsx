/** Edit remark for document(s) dialog. */

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
import { Textarea } from '@/components/ui/textarea'
import { useUpdateRemark } from '@/services/files-api'
import type { ProjectDocument } from '@/types/files'

interface EditRemarkDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  projectId: string
  folderId: string | null
  documents: ProjectDocument[]
}

export function EditRemarkDialog({
  open,
  onOpenChange,
  projectId,
  folderId,
  documents,
}: EditRemarkDialogProps) {
  const existingRemark = documents.length === 1 ? (documents[0].remark ?? '') : ''
  const [remark, setRemark] = useState(existingRemark)
  const [error, setError] = useState<string | null>(null)

  const remarkMutation = useUpdateRemark(projectId, folderId)
  const isPending = remarkMutation.isPending

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setError(null)

    try {
      for (const doc of documents) {
        await remarkMutation.mutateAsync({ documentId: doc.id, data: { remark } })
      }
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : '更新失败')
    }
  }

  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      setRemark('')
      setError(null)
    }
    onOpenChange(isOpen)
  }

  const count = documents.length
  const label = count === 1 ? documents[0]?.name : `${count} 篇文档`

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[440px] p-6 rounded-2xl border border-border/40 shadow-strong bg-background gap-6">
        <DialogHeader>
          <DialogTitle className="text-lg font-semibold tracking-tight">
            备注 — {label}
          </DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="p-3 rounded-lg bg-destructive/10 text-destructive text-[13px] font-medium">
              {error}
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="remark" className="text-[13px] font-medium">备注</Label>
            <Textarea
              id="remark"
              value={remark}
              onChange={(e) => setRemark(e.target.value)}
              placeholder="添加关于此文档的备注..."
              className="min-h-[100px] resize-none text-[14px] bg-muted/40 border-border/40 focus-visible:ring-1 focus-visible:ring-foreground/20 focus-visible:bg-background"
              autoFocus
            />
          </div>

          <DialogFooter className="pt-2 sm:justify-end gap-2">
            <Button type="button" variant="ghost" onClick={() => handleOpenChange(false)} disabled={isPending} className="text-[13px] rounded-lg">
              取消
            </Button>
            <Button type="submit" disabled={isPending} className="text-[13px] rounded-lg px-6">
              {isPending ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
