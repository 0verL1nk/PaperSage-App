import { useNavigate, useParams, Link, useLocation } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeft, BookOpenCheck, ChevronDown, ClipboardCheck, FileUp, MoreHorizontal, PenLine, Settings, User,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { useProject } from '@/services/projects-api'
import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import { GuestModeChip, GuestSessionBadge } from '@/components/domain/GuestSessionBadge'
import { useSessionStore } from '@/store'
import { DesktopAppMenu, WindowControls } from '@/components/desktop/desktop-titlebar'
import { Sheet, SheetContent, SheetDescription, SheetTitle } from '@/components/ui/sheet'
import { WorkspaceChromeProvider, WorkspaceHeaderActionsSlot, WorkspaceMenuExtraSlot } from './workspace-chrome'

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
  const { data: project, isLoading, isError: projectFailed } = useProject(projectId)
  const isGuest = useSessionStore((state) => state.isGuest)
  const guestImportPending = useSessionStore((state) => state.guestImportPending)
  const [stageSheetOpen, setStageSheetOpen] = useState(false)
  const [moreSheetOpen, setMoreSheetOpen] = useState(false)

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

  const projectName = project?.name || '项目'

  return (
    <WorkspaceChromeProvider closeMenu={() => setMoreSheetOpen(false)}>
      <div className="flex h-full flex-col bg-background overflow-hidden">
        {/* 手机端 48px 单层顶栏；桌面端仍是 56px，DOM 不变 */}
        <header
          data-mobile-chrome="topbar"
          className="desktop-drag relative z-50 flex h-[48px] flex-none select-none items-center gap-1 border-b border-border/40 bg-background/80 pl-1 pr-1 backdrop-blur-xl lg:h-14 lg:gap-4 lg:pl-4"
        >
          {/* Left: Branding & Breadcrumbs */}
          <div className="desktop-no-drag flex min-w-0 flex-1 items-center gap-1 lg:gap-3">
            <Button
              variant="ghost"
              size="icon-sm"
              className="size-[44px] shrink-0 rounded-full text-muted-foreground transition-transform hover:scale-110 hover:text-foreground active:scale-95 lg:size-8"
              onClick={() => navigate({ to: '/app/projects' })}
              aria-label={t('nav.backToProjects')}
            >
              <ArrowLeft className="size-4" />
            </Button>

            <div className="mx-1 hidden h-4 w-px bg-border/50 lg:block" />

            <Link to="/" className="hidden items-center gap-2 transition-all hover:scale-105 hover:opacity-70 active:scale-95 lg:flex">
              <div className="flex size-5 items-center justify-center rounded-[4px] bg-foreground shadow-sm">
                <span className="text-[10px] font-bold text-background italic">S</span>
              </div>
            </Link>

            <span className="mx-1 hidden text-muted-foreground/30 lg:inline">/</span>

            {isLoading ? (
              <Skeleton className="h-4 w-24 rounded" />
            ) : projectFailed ? (
              // 读取失败时不要用「未知项目」冒充项目名：等页面自己的错误态给出重试入口。
              <span className="text-[13px] text-muted-foreground">项目信息暂不可用</span>
            ) : (
              <>
                <h2 className="hidden max-w-[240px] truncate text-[13px] font-medium text-foreground lg:block">
                  {projectName}
                </h2>
                {/* 手机端：项目名即阶段入口，点开底部面板切换阶段 */}
                <button
                  type="button"
                  aria-label={`切换阶段：${projectName}`}
                  onClick={() => setStageSheetOpen(true)}
                  className="flex min-h-[44px] min-w-0 items-center gap-1 rounded-lg px-1 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring lg:hidden"
                >
                  <span className="truncate text-[13px] font-medium text-foreground">{projectName}</span>
                  <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
                </button>
              </>
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
            <div className="flex items-center gap-1 lg:hidden">
              {/* 当前页唯一主操作（如写作页「编译」）由页面投递到这里 */}
              <WorkspaceHeaderActionsSlot />
              <Button
                variant="ghost"
                size="icon"
                aria-label="更多选项"
                className="size-[44px] rounded-full text-muted-foreground"
                onClick={() => setMoreSheetOpen(true)}
              >
                <MoreHorizontal className="size-5" />
              </Button>
            </div>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('layout.systemSettings')}
              title={t('layout.systemSettings')}
              onClick={() => navigate({ to: '/app/settings' })}
              className="hidden rounded-full text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground lg:inline-flex"
            >
              <Settings className="size-4" />
            </Button>
            <GuestModeChip className="hidden rounded-full lg:flex" />
            <DesktopAppMenu />
            <WindowControls className="ml-1" />
          </div>
        </header>

        {/* 手机端第二阶段：44px 四等分阶段导航；写作页是单焦点工作台，不占这一层 */}
        {activeTab !== 'writing' && (
          <nav
            aria-label="项目阶段"
            data-mobile-chrome="stage-nav"
            className="lg:hidden flex-none border-b border-border/40 bg-background/80 backdrop-blur-xl"
          >
            <div className="grid h-[44px] grid-cols-4">
              {projectTabs.map((tab) => {
                const isActive = activeTab === tab.key
                return (
                  <button
                    key={tab.key}
                    aria-current={isActive ? 'page' : undefined}
                    onClick={() => handleNavigate(tab.path)}
                    className={cn(
                      'relative flex min-h-[44px] items-center justify-center whitespace-nowrap text-[13px] font-medium outline-none transition-colors',
                      isActive ? 'text-primary' : 'text-muted-foreground hover:text-foreground',
                    )}
                  >
                    {t(tab.labelKey)}
                    {isActive && <span aria-hidden="true" className="absolute inset-x-4 bottom-0 h-0.5 rounded-full bg-primary" />}
                  </button>
                )
              })}
            </div>
          </nav>
        )}

        {/* Main Content Area: Now using flex-1 and overflow-hidden for 3-pane internal scroll */}
        <main className="flex-1 bg-muted/5 overflow-hidden relative flex flex-col">
          {!isGuest && guestImportPending && (
            <div className="flex-none border-b border-amber-500/20 bg-amber-500/[0.04] px-4 py-2">
              <GuestSessionBadge compact />
            </div>
          )}
          <div className="relative min-h-0 flex-1">{children}</div>
        </main>
      </div>

      {/* 阶段面板：项目名点开 */}
      <Sheet open={stageSheetOpen} onOpenChange={setStageSheetOpen}>
        <SheetContent side="bottom" className="pb-[calc(env(safe-area-inset-bottom)+12px)]">
          <SheetTitle className="px-4 pt-4 text-left text-base">切换阶段</SheetTitle>
          <SheetDescription className="sr-only">选择写作准备、论文撰写、检查完善或资料库。</SheetDescription>
          <div className="flex flex-col gap-1 px-4">
            {projectTabs.map((tab) => {
              const Icon = tab.icon
              const isActive = activeTab === tab.key
              return (
                <button
                  key={tab.key}
                  type="button"
                  aria-current={isActive ? 'page' : undefined}
                  onClick={() => {
                    setStageSheetOpen(false)
                    handleNavigate(tab.path)
                  }}
                  className={cn(
                    'flex min-h-[56px] items-center gap-3 rounded-xl px-3 text-left text-[15px] outline-none transition-colors',
                    isActive ? 'bg-primary/10 font-medium text-primary' : 'text-foreground hover:bg-muted',
                  )}
                >
                  <Icon className="size-4 shrink-0" />
                  <span className="flex-1">{t(tab.labelKey)}</span>
                  {isActive && <span className="text-[11px] text-primary">当前</span>}
                </button>
              )
            })}
          </div>
        </SheetContent>
      </Sheet>

      {/* 更多面板：低频项（含页面自己投递的编译器、排版组件等） */}
      <Sheet open={moreSheetOpen} onOpenChange={setMoreSheetOpen}>
        <SheetContent side="bottom" className="pb-[calc(env(safe-area-inset-bottom)+12px)]">
          <SheetTitle className="px-4 pt-4 text-left text-base">更多</SheetTitle>
          <SheetDescription className="sr-only">切换编译器、准备排版组件与打开系统设置。</SheetDescription>
          <div className="flex flex-col gap-1 px-4">
            {isGuest && <GuestModeChip className="mb-1 self-start rounded-full" />}
            <WorkspaceMenuExtraSlot />
            <button
              type="button"
              onClick={() => {
                setMoreSheetOpen(false)
                navigate({ to: '/app/settings' })
              }}
              className="flex min-h-[52px] items-center gap-3 rounded-xl px-3 text-left text-sm text-foreground outline-none transition-colors hover:bg-muted"
            >
              <Settings className="size-4 shrink-0 opacity-60" />
              系统设置
            </button>
            {isGuest && (
              <button
                type="button"
                onClick={() => {
                  setMoreSheetOpen(false)
                  navigate({ to: '/auth' })
                }}
                className="flex min-h-[52px] items-center gap-3 rounded-xl px-3 text-left text-sm text-foreground outline-none transition-colors hover:bg-muted"
              >
                <User className="size-4 shrink-0 opacity-60" />
                {t('account.loginOrRegister')}
              </button>
            )}
          </div>
        </SheetContent>
      </Sheet>
    </WorkspaceChromeProvider>
  )
}
