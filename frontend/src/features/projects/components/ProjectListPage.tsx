/**
 * ProjectListPage - displays list of projects with filtering and actions
 */
import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useProjects, useGroups } from '../api'
import { ProjectCard } from './ProjectCard'
import { CreateProjectDialog } from './CreateProjectDialog'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Trash2, Plus, FolderOpen } from 'lucide-react'

export function ProjectListPage() {
  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null)
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false)

  const { data: projects = [], isLoading, error } = useProjects(selectedGroupId ?? undefined)
  const { data: groups = [] } = useGroups()

  if (isLoading) {
    return (
      <div className="p-6 space-y-6">
        <div className="flex items-center justify-between">
          <Skeleton className="h-8 w-32" />
          <Skeleton className="h-10 w-36" />
        </div>
        <div className="flex gap-2">
          <Skeleton className="h-8 w-16" />
          <Skeleton className="h-8 w-24" />
        </div>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-32 w-full" />
          ))}
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-6">
        <Card className="p-6 text-center text-destructive">
          <p>Failed to load projects: {error.message}</p>
        </Card>
      </div>
    )
  }

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold tracking-tight">My Projects</h1>
        <div className="flex gap-2">
          <Button variant="ghost" size="sm" asChild>
            <Link to="/app/trash">
              <Trash2 className="mr-2 h-4 w-4" />
              Trash
            </Link>
          </Button>
          <Button variant="liquid" size="sm" onClick={() => setIsCreateDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            New Project
          </Button>
        </div>
      </div>

      {/* Group Filter */}
      {groups.length > 0 && (
        <div className="flex gap-2 flex-wrap">
          <Button
            variant={selectedGroupId === null ? 'default' : 'secondary'}
            size="sm"
            onClick={() => setSelectedGroupId(null)}
          >
            All
          </Button>
          {groups.map((group) => (
            <Button
              key={group.id}
              variant={selectedGroupId === group.id ? 'default' : 'secondary'}
              size="sm"
              onClick={() => setSelectedGroupId(group.id)}
            >
              {group.name}
            </Button>
          ))}
        </div>
      )}

      {/* Project List */}
      {projects.length === 0 ? (
        <Card className="p-12 text-center">
          <div className="flex flex-col items-center gap-4">
            <div className="rounded-full bg-muted p-4">
              <FolderOpen className="h-8 w-8 text-muted-foreground" />
            </div>
            <div>
              <p className="font-medium">No projects yet</p>
              <p className="text-sm text-muted-foreground">
                Create your first project to get started
              </p>
            </div>
            <Button variant="liquid" onClick={() => setIsCreateDialogOpen(true)}>
              <Plus className="mr-2 h-4 w-4" />
              Create Project
            </Button>
          </div>
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 items-start">
          {projects.map((project) => (
            <ProjectCard
              key={project.id}
              project={project}
              onClick={() => {/* Navigate to project */}}
            />
          ))}
        </div>
      )}

      {/* Create Dialog */}
      <CreateProjectDialog
        open={isCreateDialogOpen}
        onOpenChange={setIsCreateDialogOpen}
        groups={groups}
      />
    </div>
  )
}
