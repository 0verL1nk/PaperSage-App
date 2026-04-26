/** Create folder dialog with name input. */

import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useCreateFolder } from '@/services/folders-api'

interface CreateFolderDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  projectId: string
  parentId: string | null
}

export function CreateFolderDialog({ open, onOpenChange, projectId, parentId }: CreateFolderDialogProps) {
  const [name, setName] = useState('')
  const [error, setError] = useState<string | null>(null)

  const createMutation = useCreateFolder()
  const isPending = createMutation.isPending

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (!name.trim()) return

    setError(null)

    try {
      await createMutation.mutateAsync({ projectId, parentId, name: name.trim() })
      setName('')
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建文件夹失败')
    }
  }

  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      setName('')
      setError(null)
    }
    onOpenChange(isOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[400px] p-6 rounded-2xl border border-border/40 shadow-strong bg-background gap-6">
        <DialogHeader>
          <DialogTitle className="text-lg font-semibold tracking-tight">新建文件夹</DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="p-3 rounded-lg bg-destructive/10 text-destructive text-[13px] font-medium">
              {error}
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="folder-name" className="text-[13px] font-medium">文件夹名称</Label>
            <Input
              id="folder-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如：会议论文"
              className="h-10 text-[14px] bg-muted/40 border-border/40 focus-visible:ring-1 focus-visible:ring-foreground/20 focus-visible:bg-background"
              required
              autoFocus
            />
          </div>

          <DialogFooter className="pt-2 sm:justify-end gap-2">
            <Button type="button" variant="ghost" onClick={() => handleOpenChange(false)} disabled={isPending} className="text-[13px] rounded-lg">
              取消
            </Button>
            <Button type="submit" variant="default" disabled={isPending || !name.trim()} className="text-[13px] rounded-lg px-6">
              {isPending ? '创建中...' : '创建'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
