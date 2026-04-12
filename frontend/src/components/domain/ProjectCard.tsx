import { cn } from '@/lib/utils'
import { useState } from 'react'
import { useDeleteProject } from '@/services/projects-api'
import type { Project } from '@/types/projects'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Trash2, FolderGit2 } from 'lucide-react'

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
        role="button"
        tabIndex={0}
        onClick={onClick}
        onKeyDown={(e) => e.key === 'Enter' && onClick?.()}
        className={cn(
          "group relative flex flex-col justify-between p-5 h-[180px] bg-card rounded-2xl border border-border/40 transition-all duration-500 outline-none text-left",
          "hover:border-primary/20 hover:shadow-[0_8px_30px_rgb(0,0,0,0.04)] hover:-translate-y-1 hover:bg-muted/5",
          "active:scale-[0.98] focus-visible:ring-2 focus-visible:ring-primary/20"
        )}
      >
        <div className="relative z-10">
          <div className="flex items-start justify-between mb-4">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-muted/50 text-muted-foreground transition-all duration-500 group-hover:bg-primary group-hover:text-primary-foreground group-hover:rotate-6 group-hover:scale-110 shadow-sm">
              <FolderGit2 className="h-5 w-5 stroke-[1.5]" />
            </div>
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={(e) => {
                e.stopPropagation()
                setShowDeleteConfirm(true)
              }}
              className="opacity-0 group-hover:opacity-100 transition-all duration-300 text-muted-foreground hover:text-destructive hover:bg-destructive/10 rounded-full"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          </div>
          
          <h3 className="font-semibold text-[15px] tracking-tight truncate group-hover:text-primary transition-colors duration-300 text-foreground">
            {project.name}
          </h3>
          
          {project.description ? (
            <p className="text-[13px] text-muted-foreground line-clamp-2 mt-1.5 leading-relaxed transition-colors duration-300 group-hover:text-muted-foreground/80">
              {project.description}
            </p>
          ) : (
            <p className="text-[13px] text-muted-foreground/30 mt-1.5 italic font-light tracking-wide">暂无详细描述</p>
          )}
        </div>

        <div className="flex items-center justify-between mt-4 relative z-10">
          <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground/60 group-hover:text-muted-foreground transition-colors duration-300">
            {formattedDate}
          </span>
          {project.group && (
            <span className="px-2.5 py-0.5 rounded-full text-[10px] font-bold bg-muted/50 text-muted-foreground/70 group-hover:bg-primary/5 group-hover:text-primary transition-all duration-300">
              {project.group.name}
            </span>
          )}
        </div>
        
        {/* Subtle hover background highlight */}
        <div className="absolute inset-0 rounded-2xl bg-gradient-to-br from-primary/[0.02] to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-500" />
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
