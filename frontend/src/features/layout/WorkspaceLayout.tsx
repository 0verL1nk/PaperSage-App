import { useNavigate } from '@tanstack/react-router'
import { ArrowLeft, Home, FileUp, FileCode, Tags, GitBranch, Sparkles } from 'lucide-react'
import { type ReactNode } from 'react'
import { Button } from '@/components/ui/button'

interface WorkspaceLayoutProps {
  children: ReactNode
}

const projectTabs = [
  { path: '/project/$projectId/uploading' as const, label: 'Uploading', icon: FileUp, key: 'uploading' },
  { path: '/project/$projectId/coding' as const, label: 'Coding', icon: FileCode, key: 'coding' },
  { path: '/project/$projectId/classifying' as const, label: 'Classifying', icon: Tags, key: 'classifying' },
  { path: '/project/$projectId/logic-chain' as const, label: 'Logic Chain', icon: GitBranch, key: 'logic-chain' },
  { path: '/project/$projectId/generating' as const, label: 'Generating', icon: Sparkles, key: 'generating' },
]

export function WorkspaceLayout({ children }: WorkspaceLayoutProps) {
  const navigate = useNavigate()
  const currentPath = typeof window !== 'undefined' ? window.location.pathname : ''

  // Extract projectId from current path
  const projectIdMatch = currentPath.match(/\/project\/([^/]+)/)
  const projectId = projectIdMatch ? projectIdMatch[1] : 'default'

  // Find active tab based on current path
  const activeTab = projectTabs.find((tab) => currentPath.includes(tab.path.replace('/$projectId/', '/')))?.
key || 'uploading'

  const handleNavigate = (path: string) => {
    const targetPath = path.replace('$projectId', projectId)
    navigate({ to: targetPath })
  }

  return (
    <div className="flex min-h-screen flex-col">
      {/* Header */}
      <header className="sticky top-0 z-50 flex h-16 items-center border-b bg-background/95 px-6 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="flex items-center gap-4">
          <Button variant="ghost" size="sm" onClick={() => navigate({ to: '/app/documents' })}>
            <ArrowLeft className="size-4" />
          </Button>
          <Button variant="ghost" size="sm" onClick={() => navigate({ to: '/project/$projectId', params: { projectId } })}>
            <Home className="size-4" />
          </Button>
          <span className="text-lg font-semibold">Project: {projectId}</span>
        </div>
      </header>

      {/* Tab Navigation */}
      <div className="border-b bg-background">
        <div className="flex h-12 items-center gap-1 px-4">
          {projectTabs.map((tab) => {
            const Icon = tab.icon
            const isActive = activeTab === tab.key
            return (
              <Button
                key={tab.key}
                variant={isActive ? 'secondary' : 'ghost'}
                size="sm"
                className="gap-2"
                onClick={() => handleNavigate(tab.path)}
              >
                <Icon className="size-4" />
                {tab.label}
              </Button>
            )
          })}
        </div>
      </div>

      {/* Main Content */}
      <main className="flex-1">{children}</main>
    </div>
  )
}