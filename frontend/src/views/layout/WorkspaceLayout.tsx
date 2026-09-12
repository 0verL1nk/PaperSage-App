import { useNavigate, useParams, Link, useLocation } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, FileUp, PenLine, Tags, GitBranch, Sparkles, Settings, ExternalLink } from 'lucide-react'
import { type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { useProject } from '@/services/projects-api'
import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'

interface WorkspaceLayoutProps {
  children: ReactNode
}

const projectTabs = [
  { path: '/project/$projectId/uploading' as const, labelKey: 'nav.tabUploading', icon: FileUp, key: 'uploading' },
  { path: '/project/$projectId/writing' as const, labelKey: 'nav.tabWriting', icon: PenLine, key: 'writing' },
  { path: '/project/$projectId/classifying' as const, labelKey: 'nav.tabClassifying', icon: Tags, key: 'classifying' },
  { path: '/project/$projectId/logic-chain' as const, labelKey: 'nav.tabLogicChain', icon: GitBranch, key: 'logic-chain' },
  { path: '/project/$projectId/generating' as const, labelKey: 'nav.tabGenerating', icon: Sparkles, key: 'generating' },
]

export function WorkspaceLayout({ children }: WorkspaceLayoutProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const { t } = useTranslation()
  const { projectId = 'default' } = useParams({ strict: false })
  const { data: project, isLoading } = useProject(projectId)

  // Find active tab based on current path
  const activeTab = projectTabs.find((tab) => {
    const segments = location.pathname.split('/')
    const lastSegment = segments[segments.length - 1]
    return lastSegment === tab.key
  })?.key || ''

  const handleNavigate = (path: string) => {
    const targetPath = path.replace('$projectId', projectId)
    navigate({ to: targetPath })
  }

  return (
    <div className="flex h-screen flex-col bg-background overflow-hidden">
      {/* Ultra-clean Top Navigation Bar */}
      <header className="flex-none flex h-14 items-center justify-between border-b border-border/40 bg-background/80 backdrop-blur-xl px-4 z-50">
        
        {/* Left: Branding & Breadcrumbs */}
        <div className="flex items-center gap-3">
          <Button 
            variant="ghost" 
            size="icon-xs" 
            className="rounded-full text-muted-foreground hover:text-foreground transition-transform hover:scale-110 active:scale-95"
            onClick={() => navigate({ to: '/app/projects' })}
          >
            <ArrowLeft className="size-4" />
          </Button>
          
          <div className="h-4 w-px bg-border/50 mx-1" />

          <Link to="/" className="flex items-center gap-2 hover:opacity-70 transition-all hover:scale-105 active:scale-95">
            <div className="flex size-5 items-center justify-center rounded-[4px] bg-foreground shadow-sm">
              <span className="text-[10px] font-bold text-background italic">S</span>
            </div>
          </Link>
          
          <span className="text-muted-foreground/30 mx-1">/</span>
          
          {isLoading ? (
            <Skeleton className="h-4 w-24 rounded" />
          ) : (
            <div className="flex items-center gap-2">
              <h2 className="text-[13px] font-medium text-foreground truncate max-w-[200px]">
                {project?.name || '未知项目'}
              </h2>
            </div>
          )}
        </div>

        {/* Center: Clean Pill Tabs with improved Hover */}
        <div className="hidden md:flex absolute left-1/2 -translate-x-1/2 items-center p-1 bg-muted/60 rounded-full border border-border/50 shadow-sm">
          {projectTabs.map((tab) => {
            const Icon = tab.icon
            const isActive = activeTab === tab.key
            return (
              <button
                key={tab.key}
                onClick={() => handleNavigate(tab.path)}
                className={cn(
                  "group flex items-center gap-1.5 px-4 py-1.5 rounded-full text-[13px] font-medium transition-all duration-300 outline-none relative overflow-hidden",
                  isActive 
                    ? "bg-background text-primary shadow-[0_2px_10px_-3px_rgba(0,0,0,0.1)] ring-1 ring-border/50" 
                    : "text-muted-foreground hover:text-foreground hover:bg-muted hover:scale-105 active:scale-95"
                )}
              >
                <Icon className={cn(
                  "size-3.5 transition-all duration-300", 
                  isActive ? "text-primary" : "opacity-60 group-hover:opacity-100 group-hover:rotate-6"
                )} />
                <span className="relative z-10">{t(tab.labelKey)}</span>
              </button>
            )
          })}
        </div>

        {/* Right: Actions */}
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="icon-sm" className="text-muted-foreground rounded-full hover:bg-muted/50 hover:text-foreground transition-all hover:rotate-12">
            <Settings className="size-4" />
          </Button>
          <Button variant="ghost" size="icon-sm" className="text-muted-foreground rounded-full hover:bg-muted/50 hover:text-foreground transition-all hover:-translate-y-0.5">
            <ExternalLink className="size-4" />
          </Button>
        </div>
      </header>

      {/* Mobile Tabs Fallback */}
      <nav className="md:hidden flex-none sticky top-14 z-40 bg-background/80 backdrop-blur-xl border-b border-border/40 px-2">
        <div className="flex h-12 items-center gap-1 overflow-x-auto no-scrollbar">
          {projectTabs.map((tab) => {
            const isActive = activeTab === tab.key
            return (
              <button
                key={tab.key}
                onClick={() => handleNavigate(tab.path)}
                className={cn(
                  "flex-none px-4 h-full text-[13px] font-medium transition-all outline-none whitespace-nowrap border-b-2",
                  isActive 
                    ? "text-primary border-primary bg-primary/5" 
                    : "text-muted-foreground border-transparent hover:text-foreground hover:bg-muted/30"
                )}
              >
                {t(tab.labelKey)}
              </button>
            )
          })}
        </div>
      </nav>

      {/* Main Content Area: Now using flex-1 and overflow-hidden for 3-pane internal scroll */}
      <main className="flex-1 bg-muted/5 overflow-hidden relative">
        {children}
      </main>
    </div>
  )
}
