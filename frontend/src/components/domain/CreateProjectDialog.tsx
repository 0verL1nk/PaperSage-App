import { useState } from 'react'
import { useCreateProject } from '@/services/projects-api'
import type { ProjectGroup } from '@/types/projects'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { ProjectScenario } from '@/types/projects'
import { useSessionStore } from '@/store'

interface CreateProjectDialogProps {
  /** Whether the dialog is open */
  open: boolean
  /** Callback when open state changes */
  onOpenChange: (open: boolean) => void
  /** Available groups */
  groups: ProjectGroup[]
}

const NO_GROUP = '__no_group__'

export function CreateProjectDialog({ open, onOpenChange, groups }: CreateProjectDialogProps) {
  const isGuest = useSessionStore((session) => session.isGuest)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [groupId, setGroupId] = useState<string | null>(null)
  const [scenario, setScenario] = useState<ProjectScenario>('undergraduate_thesis')
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
        scenario,
      })
      // Reset form
      setName('')
      setDescription('')
      setGroupId(null)
      setScenario('undergraduate_thesis')
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
      setScenario('undergraduate_thesis')
      setError(null)
    }
    onOpenChange(isOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-h-[90dvh] w-[calc(100vw-1rem)] overflow-y-auto rounded-2xl border border-border/40 bg-background p-5 shadow-strong sm:max-w-[440px] sm:p-6">
        <DialogHeader>
          <DialogTitle className="text-xl font-semibold tracking-tight text-foreground">新建项目</DialogTitle>
          <DialogDescription className="text-sm leading-5 text-muted-foreground">填写项目名称和写作场景，之后可以在工作台继续编辑。</DialogDescription>
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
              className="h-12 text-[14px] bg-muted/40 border-border/40 focus-visible:ring-1 focus-visible:ring-foreground/20 focus-visible:bg-background"
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
              className="min-h-[88px] resize-none text-[14px] bg-muted/40 border-border/40 focus-visible:ring-1 focus-visible:ring-foreground/20 focus-visible:bg-background"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="scenario" className="text-[13px] font-medium text-foreground">论文场景</Label>
            <Select value={scenario} onValueChange={(value) => setScenario(value as ProjectScenario)}>
              <SelectTrigger id="scenario" aria-label="论文场景" className="h-12 w-full rounded-lg border-border/40 bg-muted/40 px-3 text-sm focus:bg-background">
                <SelectValue />
              </SelectTrigger>
              <SelectContent position="popper" align="start" className="w-[var(--radix-select-trigger-width)]">
                <SelectItem value="undergraduate_thesis">本科毕业论文</SelectItem>
                <SelectItem value="intro_paper">入门级学术小论文</SelectItem>
              </SelectContent>
            </Select>
            <p className="text-[11px] leading-5 text-muted-foreground">场景用于展示对应写作流程，之后仍可自由往返各阶段。</p>
          </div>

          {isGuest && (
            <p className="rounded-lg border border-border/60 bg-muted/40 px-3 py-2 text-xs leading-5 text-muted-foreground">
              游客模式暂不提供项目分组管理。登录账号后可创建和整理分组；本地项目可以照常创建与编辑。
            </p>
          )}

          {groups.length > 0 && (
            <div className="space-y-2">
              <Label htmlFor="group" className="text-[13px] font-medium text-foreground">
                分组
              </Label>
              <Select value={groupId ?? NO_GROUP} onValueChange={(value) => setGroupId(value === NO_GROUP ? null : value)}>
                <SelectTrigger id="group" aria-label="分组" className="h-10 w-full rounded-lg border-border/40 bg-muted/40 px-3 text-sm focus:bg-background">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent position="popper" align="start" className="w-[var(--radix-select-trigger-width)]">
                  <SelectItem value={NO_GROUP}>无分组</SelectItem>
                  {groups.map((group) => <SelectItem key={group.id} value={group.id}>{group.name}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
          )}

          <DialogFooter className="pt-2 sm:justify-end gap-2">
            <Button
              type="button"
              variant="ghost"
              onClick={() => handleOpenChange(false)}
              disabled={isPending}
              className="min-h-12 font-medium text-[13px] rounded-lg"
            >
              取消
            </Button>
            <Button
              type="submit"
              variant="default"
              disabled={isPending || !name.trim()}
              className="min-h-12 font-medium text-[13px] rounded-lg px-6"
            >
              {isPending ? '创建中...' : '创建项目'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
