import { useEffect, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { Plus } from 'lucide-react'
import { Events } from '@wailsio/runtime'

import { ProjectCard } from '@/components/domain/ProjectCard'
import { CreateProjectDialog } from '@/components/domain/CreateProjectDialog'
import { Button } from '@/components/ui/button'
import {
  EmptyState,
  ErrorState,
  LoadingState,
  PageContainer,
  PageHeader,
  SegmentedControl,
} from '@/components/shared'
import { friendlyErrorMessage } from '@/services/errors'
import { useProjects, useGroups } from '@/services/projects-api'
import { isDesktop } from '@/lib/desktop'

export function ProjectListPage() {
  const navigate = useNavigate()
  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null)
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false)

  const { data: projects = [], isLoading, error, refetch } = useProjects(selectedGroupId ?? undefined)
  const { data: groups = [] } = useGroups()

  useEffect(() => {
    const open = () => {
      sessionStorage.removeItem('papersage:open-create-project')
      setIsCreateDialogOpen(true)
    }
    if (sessionStorage.getItem('papersage:open-create-project')) open()
    if (!isDesktop()) return
    return Events.On('menu:new-project', open)
  }, [])

  if (isLoading) {
    return (
      <PageContainer size="wide" className="max-w-[100rem]">
        <LoadingState title="正在加载项目" description="稍候即可继续写作" rows={4} />
      </PageContainer>
    )
  }

  if (error) {
    return (
      <PageContainer size="wide" className="max-w-[100rem]">
        <ErrorState
          title="项目加载失败"
          description={friendlyErrorMessage(error)}
          onRetry={() => void refetch()}
        />
      </PageContainer>
    )
  }

  return (
    <PageContainer size="wide" className="max-w-[100rem] space-y-8 pb-24">
      <PageHeader
        title="所有项目"
        description="从选题准备到成稿检查，继续你的论文写作"
        actions={
          <Button onClick={() => setIsCreateDialogOpen(true)} className="min-h-11 px-4">
            <Plus className="mr-2 h-4 w-4" />
            新建项目
          </Button>
        }
      />

      {groups.length > 0 ? (
        <SegmentedControl
          ariaLabel="按分组筛选项目"
          value={selectedGroupId ?? 'all'}
          onValueChange={(value) => setSelectedGroupId(value === 'all' ? null : value)}
          items={[
            { value: 'all', label: '全部' },
            ...groups.map((group) => ({ value: group.id, label: group.name })),
          ]}
        />
      ) : null}

      {projects.length === 0 ? (
        <EmptyState
          title="暂无项目"
          description="您还没有创建任何项目，新建一个项目开始您的研究之旅。"
          action={
            <Button onClick={() => setIsCreateDialogOpen(true)} className="min-h-11">
              <Plus className="mr-2 h-4 w-4" />
              创建第一个项目
            </Button>
          }
        />
      ) : (
        <div className="grid items-stretch gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
          {projects.map((project) => (
            <ProjectCard
              key={project.id}
              project={project}
              onClick={() => navigate({ to: '/project/$projectId', params: { projectId: project.id } })}
              onContinue={() => navigate({ to: '/project/$projectId/writing', params: { projectId: project.id } })}
            />
          ))}
        </div>
      )}

      <CreateProjectDialog
        open={isCreateDialogOpen}
        onOpenChange={setIsCreateDialogOpen}
        groups={groups}
      />
    </PageContainer>
  )
}
