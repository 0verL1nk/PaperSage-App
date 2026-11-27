import { useState } from 'react'
import { useDeleteProject } from '@/services/projects-api'
import type { Project } from '@/types/projects'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { StatusBadge } from '@/components/shared/status-badge'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { FolderGit2, MoreHorizontal, PenLine, Trash2 } from 'lucide-react'

interface ProjectCardProps {
  /** The project to display */
  project: Project
  /** Callback when clicked */
  onClick?: () => void
  /** 快捷直达写作页；未传时不渲染，避免调用方被路由依赖污染。 */
  onContinue?: () => void
}

function formatUpdatedAt(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const days = Math.floor((Date.now() - date.getTime()) / 86_400_000)
  if (days <= 0) return '今天更新'
  if (days === 1) return '昨天更新'
  if (days < 7) return `${days} 天前更新`
  return `${date.getMonth() + 1} 月 ${date.getDate()} 日更新`
}

export function ProjectCard({ project, onClick, onContinue }: ProjectCardProps) {
  const deleteProject = useDeleteProject(project.id)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)

  const handleDelete = async () => {
    await deleteProject.mutateAsync()
  }

  const scenarioLabel = project.scenario === 'intro_paper' ? '学术小论文' : '本科毕业论文'
  const updatedLabel = formatUpdatedAt(project.updated_at)

  return (
    <>
      <div className="group relative flex h-full min-h-[200px] flex-col rounded-xl border border-border bg-card p-5 text-left transition-colors hover:border-border-strong hover:shadow-sm">
        <div className="flex items-start justify-between gap-2">
          <div className="flex size-9 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <FolderGit2 className="size-4" strokeWidth={1.5} />
          </div>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={`${project.name} 的更多操作`}
                className="relative z-20 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
              >
                <MoreHorizontal className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem variant="destructive" onSelect={() => setShowDeleteConfirm(true)}>
                <Trash2 className="size-3.5" />删除项目
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>

        <h3 className="mt-4 text-body font-medium tracking-tight text-foreground">
          {/* 标题按钮的伪元素铺满整张卡片，让卡片可点且不把嵌套按钮塞进 role=button */}
          <button
            type="button"
            onClick={onClick}
            className="w-full text-left outline-none after:absolute after:inset-0 after:content-[''] focus-visible:underline focus-visible:decoration-primary"
          >
            <span className="block truncate">{project.name}</span>
          </button>
        </h3>

        <p
          className={
            project.description
              ? 'mt-1.5 line-clamp-2 min-h-[2.5rem] text-body-sm leading-relaxed text-muted-foreground'
              : 'mt-1.5 line-clamp-2 min-h-[2.5rem] text-body-sm leading-relaxed text-muted-foreground italic'
          }
        >
          {project.description || '暂无详细描述'}
        </p>

        <div className="mt-auto space-y-2 border-t border-border/60 pt-3">
          <div className="flex items-center justify-between gap-2">
            <div className="flex min-w-0 items-center gap-1.5">
              <StatusBadge tone="neutral">{scenarioLabel}</StatusBadge>
              {project.group ? (
                <span className="max-w-[120px] truncate rounded-md bg-muted px-2 py-0.5 text-caption text-muted-foreground">
                  {project.group.name}
                </span>
              ) : null}
            </div>
            <span className="shrink-0 text-caption text-muted-foreground">{updatedLabel}</span>
          </div>
          {onContinue ? (
            <Button
              variant="outline"
              size="sm"
              className="relative z-20 w-full"
              onClick={onContinue}
            >
              <PenLine className="mr-1.5 size-3.5" />
              继续写作
            </Button>
          ) : null}
        </div>
      </div>

      <ConfirmDialog
        open={showDeleteConfirm}
        onOpenChange={setShowDeleteConfirm}
        title="删除项目"
        description={`确定要删除 "${project.name}" 吗？删除后可从回收站恢复。`}
        confirmText="删除"
        variant="destructive"
        onConfirm={handleDelete}
        isPending={deleteProject.isPending}
      />
    </>
  )
}
