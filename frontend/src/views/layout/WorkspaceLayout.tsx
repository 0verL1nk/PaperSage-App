import { useNavigate, useParams, Link, useLocation } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, BookOpenCheck, FileUp, PenLine, ClipboardCheck, Settings } from 'lucide-react'
import { type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { useProject } from '@/services/projects-api'
import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import { GuestModeChip, GuestSessionBadge } from '@/components/domain/GuestSessionBadge'
import { useSessionStore } from '@/store'
import { DesktopAppMenu, WindowControls } from '@/components/desktop/desktop-titlebar'

interface WorkspaceLayoutProps {
  children: ReactNode
}

const projectTabs = [
  { path: '/project/$projectId/prepare' as const, labelKey: 'nav.tabPrepare', icon: BookOpenCheck, key: 'prepare' },
  { path: '/project/$projectId/writing' as const, labelKey: 'nav.tabWriting', icon: PenLine, key: 'writing' },
  { path: '/project/$projectId/review' as const, labelKey: 'nav.tabReview', icon: ClipboardCheck, key: 'review' },
  { path: '/project/$projectId/uploading' as const, labelKey: 'nav.tabUploading', icon: FileUp, key: 'uploading' },
]

export function WorkspaceLayout({ children }: WorkspaceLayoutProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const { t } = useTranslation()
  const { projectId = 'default' } = useParams({ strict: false })
  const { data: project, isLoading } = useProject(projectId)
  const isGuest = useSessionStore((state) => state.isGuest)
  const guestImportPending = useSessionStore((state) => state.guestImportPending)

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
    <div className="flex h-full flex-col bg-background overflow-hidden">
      {/* Ultra-clean Top Navigation Bar */}
      <header className="desktop-drag relative z-50 flex h-14 flex-none select-none items-center gap-4 border-b border-border/40 bg-background/80 pl-4 backdrop-blur-xl">

        {/* Left: Branding & Breadcrumbs */}
        <div className="desktop-no-drag flex min-w-0 flex-1 items-center gap-3">
          <Button
            variant="ghost"
            size="icon-xs"
            className="rounded-full text-muted-foreground hover:text-foreground transition-transform hover:scale-110 active:scale-95"
            onClick={() => navigate({ to: '/app/projects' })}
            aria-label={t('nav.backToProjects')}
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
            <div className="flex min-w-0 items-center gap-2">
              <h2 className="max-w-[240px] truncate text-[13px] font-medium text-foreground">
                {project?.name || '未知项目'}
              </h2>
            </div>
          )}
        </div>

        {/* Center: Clean Pill Tabs with improved Hover */}
        <div className="desktop-no-drag hidden flex-none items-center rounded-full border border-border/50 bg-muted/60 p-1 shadow-sm lg:flex">
          {projectTabs.map((tab) => {
            const Icon = tab.icon
            const isActive = activeTab === tab.key
            return (
              <button
                key={tab.key}
                aria-current={isActive ? 'page' : undefined}
                onClick={() => handleNavigate(tab.path)}
                className={cn(
                  "group relative flex min-h-12 items-center gap-1.5 overflow-hidden rounded-full px-4 py-2 text-[13px] font-medium outline-none transition-all duration-300",
                  isActive
                    ? "bg-background text-primary shadow-[0_2px_10px_-3px_rgba(0,0,0,0.1)] ring-1 ring-border/50"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground"
                )}
              >
                <Icon className={cn(
                  "size-3.5 transition-all duration-300",
                  isActive ? "text-primary" : "opacity-60 group-hover:opacity-100"
                )} />
                <span className="relative z-10">{t(tab.labelKey)}</span>
              </button>
            )
          })}
        </div>

        {/* Right: Actions */}
        <div className="desktop-no-drag flex min-w-0 flex-1 items-center justify-end gap-1">
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('layout.systemSettings')}
            title={t('layout.systemSettings')}
            onClick={() => navigate({ to: '/app/settings' })}
            className="rounded-full text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground"
          >
            <Settings className="size-4" />
          </Button>
          <GuestModeChip className="rounded-full" />
          <DesktopAppMenu />
          <WindowControls className="ml-1" />
        </div>
      </header>

      {!isGuest && guestImportPending && (
        <div className="flex-none border-b border-amber-500/20 bg-amber-500/[0.04] px-4 py-2">
          <GuestSessionBadge compact />
        </div>
      )}

      {/* Mobile Tabs Fallback（<1024 横向滚动，不换行也不压标题） */}
      <nav className="lg:hidden flex-none sticky top-14 z-40 bg-background/80 backdrop-blur-xl border-b border-border/40 px-2">
        <div className="flex h-12 items-center gap-1 overflow-x-auto no-scrollbar">
          {projectTabs.map((tab) => {
            const isActive = activeTab === tab.key
            return (
              <button
                key={tab.key}
                aria-current={isActive ? 'page' : undefined}
                onClick={() => handleNavigate(tab.path)}
                className={cn(
                  "flex-none min-h-11 px-4 h-full text-[13px] font-medium transition-all outline-none whitespace-nowrap border-b-2",
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
