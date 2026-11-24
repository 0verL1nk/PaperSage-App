import { cn } from '@/lib/utils'
import { useState } from 'react'
import { useDeleteProject } from '@/services/projects-api'
import type { Project } from '@/types/projects'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { FolderGit2, MoreHorizontal, Trash2 } from 'lucide-react'

interface ProjectCardProps {
  /** The project to display */
  project: Project
  /** Callback when clicked */
  onClick?: () => void
}

export function ProjectCard({ project, onClick }: ProjectCardProps) {
  const deleteProject = useDeleteProject(project.id)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)

  const handleDelete = async () => {
    await deleteProject.mutateAsync()
  }

  const formattedDate = new Date(project.created_at).toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  })

  return (
    <>
      <div
        className={cn(
          "group relative flex flex-col justify-between p-5 h-[180px] bg-card rounded-2xl border border-border/40 transition-all duration-500 text-left",
          "hover:border-primary/20 hover:shadow-[0_8px_30px_rgb(0,0,0,0.04)] hover:-translate-y-1 hover:bg-muted/5",
        )}
      >
        <div>
          <div className="flex items-start justify-between mb-4">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-muted/50 text-muted-foreground transition-all duration-500 group-hover:bg-primary group-hover:text-primary-foreground group-hover:rotate-6 group-hover:scale-110 shadow-sm">
              <FolderGit2 className="h-5 w-5 stroke-[1.5]" />
            </div>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`${project.name} 的更多操作`}
                  className="relative z-20 rounded-full text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
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

          <h3 className="text-[15px] font-semibold tracking-tight text-foreground transition-colors duration-300 group-hover:text-primary">
            {/* 标题按钮的伪元素铺满整张卡片，让卡片可点且不把嵌套按钮塞进 role=button */}
            <button
              type="button"
              onClick={onClick}
              className="w-full text-left outline-none after:absolute after:inset-0 after:content-[''] focus-visible:underline focus-visible:decoration-primary"
            >
              <span className="block truncate">{project.name}</span>
            </button>
          </h3>

          {project.description ? (
            <p className="text-[13px] text-muted-foreground line-clamp-2 mt-1.5 leading-relaxed">
              {project.description}
            </p>
          ) : (
            <p className="text-[13px] text-muted-foreground mt-1.5 italic tracking-wide">暂无详细描述</p>
          )}
        </div>

        <div className="flex items-center justify-between gap-2 mt-4">
          <span className="shrink-0 text-xs font-medium tracking-wide text-muted-foreground">
            {project.scenario === 'intro_paper' ? '入门级学术小论文' : '本科毕业论文'}
          </span>
          <div className="flex min-w-0 items-center gap-2">
            {project.group && (
              <span className="max-w-[120px] truncate px-2.5 py-0.5 rounded-full text-xs font-semibold bg-muted/50 text-muted-foreground group-hover:bg-primary/5 group-hover:text-primary transition-all duration-300">
                {project.group.name}
              </span>
            )}
            <span className="shrink-0 text-xs text-muted-foreground">{formattedDate}</span>
          </div>
        </div>

        {/* Subtle hover background highlight */}
        <div className="pointer-events-none absolute inset-0 rounded-2xl bg-gradient-to-br from-primary/[0.02] to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-500" />
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
