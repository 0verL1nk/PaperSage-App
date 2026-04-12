import { cn } from '@/lib/utils'
import { useState } from 'react'
import { useCreateProject } from '@/services/projects-api'
import type { ProjectGroup } from '@/types/projects'
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
import { Textarea } from '@/components/ui/textarea'

interface CreateProjectDialogProps {
  /** Whether the dialog is open */
  open: boolean
  /** Callback when open state changes */
  onOpenChange: (open: boolean) => void
  /** Available groups */
  groups: ProjectGroup[]
}

export function CreateProjectDialog({ open, onOpenChange, groups }: CreateProjectDialogProps) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [groupId, setGroupId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const createProject = useCreateProject()
  const isPending = createProject.isPending

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return

    setError(null)

    try {
      await createProject.mutateAsync({
        name: name.trim(),
        description: description.trim() || undefined,
        group_id: groupId ?? undefined,
      })
      // Reset form
      setName('')
      setDescription('')
      setGroupId(null)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建项目失败')
    }
  }

  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      // Reset form when closing
      setName('')
      setDescription('')
      setGroupId(null)
      setError(null)
    }
    onOpenChange(isOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[440px] p-6 overflow-hidden rounded-2xl border border-border/40 shadow-strong bg-background gap-6">
        <DialogHeader>
          <DialogTitle className="text-xl font-semibold tracking-tight text-foreground">新建项目</DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-5">
          {error && (
            <div className="p-3 rounded-lg bg-destructive/10 text-destructive text-[13px] font-medium">
              {error}
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="name" className="text-[13px] font-medium text-foreground">
              项目名称
            </Label>
            <Input
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如：市场调研分析"
              className="h-10 text-[14px] bg-muted/40 border-border/40 focus-visible:ring-1 focus-visible:ring-foreground/20 focus-visible:bg-background"
              required
              autoFocus
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="description" className="text-[13px] font-medium text-foreground">
              描述 <span className="text-muted-foreground font-normal">(可选)</span>
            </Label>
            <Textarea
              id="description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="简要说明这个项目的目标..."
              className="min-h-[80px] resize-none text-[14px] bg-muted/40 border-border/40 focus-visible:ring-1 focus-visible:ring-foreground/20 focus-visible:bg-background"
            />
          </div>

          {groups.length > 0 && (
            <div className="space-y-2">
              <Label htmlFor="group" className="text-[13px] font-medium text-foreground">
                分组
              </Label>
              <select
                id="group"
                value={groupId ?? ''}
                onChange={(e) => setGroupId(e.target.value || null)}
                className={cn(
                  "flex h-10 w-full items-center justify-between rounded-lg border border-border/40 bg-muted/40 px-3 py-2 text-[14px] ring-offset-background placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-foreground/20 focus:bg-background disabled:cursor-not-allowed disabled:opacity-50 appearance-none transition-all cursor-pointer"
                )}
              >
                <option value="">无分组</option>
                {groups.map((group) => (
                  <option key={group.id} value={group.id}>
                    {group.name}
                  </option>
                ))}
              </select>
            </div>
          )}

          <DialogFooter className="pt-2 sm:justify-end gap-2">
            <Button
              type="button"
              variant="ghost"
              onClick={() => handleOpenChange(false)}
              disabled={isPending}
              className="font-medium text-[13px] rounded-lg"
            >
              取消
            </Button>
            <Button
              type="submit"
              variant="default"
              disabled={isPending || !name.trim()}
              className="font-medium text-[13px] rounded-lg px-6"
            >
              {isPending ? '创建中...' : '创建项目'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
