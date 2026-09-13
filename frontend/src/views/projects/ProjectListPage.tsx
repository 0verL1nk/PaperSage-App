import { cn } from '@/lib/utils'
import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useProjects, useGroups } from '@/services/projects-api'
import { ProjectCard } from '@/components/domain/ProjectCard'
import { CreateProjectDialog } from '@/components/domain/CreateProjectDialog'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Trash2, Plus, FolderGit2 } from 'lucide-react'

export function ProjectListPage() {
  const navigate = useNavigate()
  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null)
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false)

  const { data: projects = [], isLoading, error } = useProjects(selectedGroupId ?? undefined)
  const { data: groups = [] } = useGroups()

  if (isLoading) {
    return (
      <div className="p-8 md:p-12 max-w-7xl mx-auto space-y-8">
        <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
          <div className="space-y-3">
            <Skeleton className="h-10 w-40 rounded-lg" />
            <Skeleton className="h-4 w-64 rounded-md" />
          </div>
        </div>
        <div className="flex gap-2">
          <Skeleton className="h-9 w-20 rounded-full" />
          <Skeleton className="h-9 w-24 rounded-full" />
        </div>
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {[1, 2, 3, 4].map((i) => (
            <Skeleton key={i} className="h-44 w-full rounded-2xl" />
          ))}
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex h-[50vh] items-center justify-center p-8">
        <div className="text-center space-y-4">
          <div className="inline-flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10 text-destructive">
            <FolderGit2 className="h-6 w-6" />
          </div>
          <p className="text-sm font-medium text-destructive">项目加载失败</p>
          <p className="text-xs text-muted-foreground">{error.message}</p>
          <Button variant="outline" size="sm" onClick={() => window.location.reload()}>
            重试
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="p-8 md:p-12 max-w-7xl mx-auto space-y-10 pb-24">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-6">
        <div className="space-y-2">
          <h1 className="text-3xl font-semibold tracking-tight text-foreground">所有项目</h1>
          <p className="text-sm text-muted-foreground">管理并组织您的研究文献与分析工作区</p>
        </div>
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" asChild className="text-muted-foreground hover:text-foreground">
            <Link to="/app/trash">
              <Trash2 className="mr-2 h-4 w-4" />
              回收站
            </Link>
          </Button>
          <Button variant="default" size="sm" onClick={() => setIsCreateDialogOpen(true)} className="px-4">
            <Plus className="mr-2 h-4 w-4" />
            新建项目
          </Button>
        </div>
      </div>

      {/* Filter - Segmented Control Style */}
      {groups.length > 0 && (
        <div className="flex items-center gap-1 p-1 bg-muted/40 rounded-full w-fit border border-border/40">
          <button
            onClick={() => setSelectedGroupId(null)}
            className={cn(
              "px-4 py-1.5 text-sm font-medium rounded-full transition-all duration-200 outline-none",
              selectedGroupId === null
                ? "bg-background text-foreground shadow-sm ring-1 ring-border/50"
                : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
            )}
          >
            全部
          </button>
          {groups.map((group) => (
            <button
              key={group.id}
              onClick={() => setSelectedGroupId(group.id)}
              className={cn(
                "px-4 py-1.5 text-sm font-medium rounded-full transition-all duration-200 outline-none",
                selectedGroupId === group.id
                  ? "bg-background text-foreground shadow-sm ring-1 ring-border/50"
                  : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
              )}
            >
              {group.name}
            </button>
          ))}
        </div>
      )}

      {/* Project List */}
      {projects.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="mb-6 flex h-16 w-16 items-center justify-center rounded-2xl bg-muted/30">
            <FolderGit2 className="h-8 w-8 text-muted-foreground/40 stroke-[1.5]" />
          </div>
          <h3 className="mb-2 text-lg font-medium text-foreground">暂无项目</h3>
          <p className="mb-8 max-w-[280px] text-sm text-muted-foreground leading-relaxed">
            您还没有创建任何项目，新建一个项目开始您的研究之旅。
          </p>
          <Button variant="default" onClick={() => setIsCreateDialogOpen(true)} >
            <Plus className="mr-2 h-4 w-4" />
            创建第一个项目
          </Button>
        </div>
      ) : (
        <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 animate-in fade-in duration-500">
          {projects.map((project) => (
            <ProjectCard
              key={project.id}
              project={project}
              onClick={() => navigate({ to: '/project/$projectId', params: { projectId: project.id } })}
            />
          ))}
        </div>
      )}

      <CreateProjectDialog
        open={isCreateDialogOpen}
        onOpenChange={setIsCreateDialogOpen}
        groups={groups}
      />
    </div>
  )
}
