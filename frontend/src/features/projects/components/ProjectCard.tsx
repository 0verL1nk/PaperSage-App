/**
 * ProjectCard - displays a single project
 */
import { useDeleteProject } from '../api'
import type { Project } from '../types'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Trash2, FileText } from 'lucide-react'

interface ProjectCardProps {
  /** The project to display */
  project: Project
  /** Callback when clicked */
  onClick?: () => void
}

export function ProjectCard({ project, onClick }: ProjectCardProps) {
  const deleteProject = useDeleteProject(project.id)

  const handleDelete = async (e: React.MouseEvent) => {
    e.stopPropagation()
    if (window.confirm(`Are you sure you want to delete "${project.name}"?`)) {
      await deleteProject.mutateAsync()
    }
  }

  return (
    <Card
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => e.key === 'Enter' && onClick?.()}
      className="p-4 cursor-pointer hover:shadow-md transition-shadow"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-start gap-3 flex-1 min-w-0">
          <div className="rounded-lg bg-primary/10 p-2">
            <FileText className="h-4 w-4 text-primary" />
          </div>
          <div className="flex-1 min-w-0">
            <h3 className="font-medium truncate">{project.name}</h3>
            {project.description && (
              <p className="text-sm text-muted-foreground line-clamp-2 mt-1">
                {project.description}
              </p>
            )}
            {project.group && (
              <Badge variant="secondary" className="mt-2 text-xs">
                {project.group.name}
              </Badge>
            )}
          </div>
        </div>
        <Button
          variant="ghost"
          size="icon"
          onClick={handleDelete}
          className="text-muted-foreground hover:text-destructive h-8 w-8"
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
    </Card>
  )
}
