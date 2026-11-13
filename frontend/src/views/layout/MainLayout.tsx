import { Link, useLocation, useNavigate } from '@tanstack/react-router'
import { ShieldCheck,
  FileText,
  Trash2,
  User,
  FolderOpen,
  LogOut,
  Search,
  Bell,
  Settings,
  Plus,
  Menu,
} from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { Events } from '@wailsio/runtime'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Sheet, SheetContent, SheetTrigger, SheetTitle } from '@/components/ui/sheet'
import { ApiTargetToggle } from '@/components/shared/api-target-toggle'
import { cn } from '@/lib/utils'
import { logout } from '@/services/auth-api'
import { useGroups } from '@/services/projects-api'
import { CreateProjectDialog } from '@/components/domain/CreateProjectDialog'
import { GuestModeChip, GuestSessionBadge } from '@/components/domain/GuestSessionBadge'
import { DesktopAppMenu, WindowControls } from '@/components/desktop/desktop-titlebar'
import { useSessionStore } from '@/store'

interface MainLayoutProps {
  children: ReactNode
}

const mainNavItems = [
  { path: '/app/projects', labelKey: 'nav.projects', icon: FolderOpen },
  { path: '/app/documents', labelKey: 'nav.documents', icon: FileText },
  { path: '/app/trash', labelKey: 'nav.trash', icon: Trash2 },
]

import { useMe } from '@/hooks/use-me'

function avatarInitials(username: string | undefined) {
  const value = username?.trim()
  if (!value) return '·'
  const parts = value.split(/[\s._\-@]+/).filter(Boolean)
  if (parts.length > 1) return `${Array.from(parts[0])[0]}${Array.from(parts[1])[0]}`.toLocaleUpperCase()
  return Array.from(value).slice(0, 2).join('').toLocaleUpperCase()
}

export function MainLayout({ children }: MainLayoutProps) {
  const isGuest = useSessionStore((session) => session.isGuest)
  const guestImportPending = useSessionStore((session) => session.guestImportPending)
  const { t } = useTranslation()
  const { data: me, isLoading: isMeLoading, isError: isMeError, refetch: refetchMe } = useMe()
  const accountLabel = isGuest ? t('account.localGuest') : me?.username ?? t(isMeLoading ? 'account.loading' : 'account.unavailable')
  const location = useLocation()
  const navigate = useNavigate()
  const currentPath = location.pathname
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const [createProjectOpen, setCreateProjectOpen] = useState(false)
  const { data: groups = [] } = useGroups()

  useEffect(() => Events.On('menu:new-project', () => {
    sessionStorage.setItem('papersage:open-create-project', '1')
    void navigate({ to: '/app/projects' })
  }), [navigate])

  return (
    <div className="flex h-full flex-col bg-background font-sans tracking-tight overflow-hidden">
      {/* Header: Fixed height */}
      <header className="apple-glass desktop-drag relative z-50 flex h-12 flex-none select-none items-center justify-between border-b border-border pl-4">
        {/* Left: Logo and Mobile Menu */}
        <div className="desktop-no-drag flex items-center lg:w-64">
          <DesktopAppMenu className="mr-1" />
          <div className="lg:hidden flex items-center mr-2">
            <Sheet open={mobileMenuOpen} onOpenChange={setMobileMenuOpen}>
              <SheetTrigger asChild>
                <Button variant="ghost" size="icon-sm" className="rounded-md">
                  <Menu className="size-4" />
                </Button>
              </SheetTrigger>
              <SheetContent side="left" className="p-0 w-64 flex flex-col">
                <SheetTitle className="sr-only">{t('layout.navigationMenu')}</SheetTitle>
                <div className="p-4 flex items-center justify-between flex-none border-b border-border">
                  <span className="px-2 text-xs font-bold text-muted-foreground uppercase tracking-widest">
                    {t('layout.library')}
                  </span>
                  <Button variant="ghost" size="icon-xs" className="rounded-md" onClick={() => { setMobileMenuOpen(false); setCreateProjectOpen(true); }}>
                    <Plus className="size-3" />
                  </Button>
                </div>
                
                <nav className="flex-1 px-2 py-2 space-y-0.5 overflow-y-auto no-scrollbar">
                  {mainNavItems.map((item) => {
                    const isActive = currentPath === item.path
                    const Icon = item.icon
                    return (
                      <Link
                        key={item.path}
                        to={item.path}
                        onClick={() => setMobileMenuOpen(false)}
                        className={cn(
                          "flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-all duration-200",
                          isActive
                            ? "bg-primary text-primary-foreground shadow-sm"
                            : "text-muted-foreground hover:bg-muted hover:text-foreground"
                        )}
                      >
                        <Icon className={cn("size-4", isActive ? "text-inherit" : "opacity-60")} />
                        {t(item.labelKey)}
                      </Link>
                    )
                  })}
                  {me?.isSuperuser && (
                    <Link
                      to="/app/admin"
                      className={cn(
                        "flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-all duration-200",
                        location.pathname === '/app/admin'
                          ? "bg-primary text-primary-foreground shadow-sm"
                          : "text-muted-foreground hover:bg-muted hover:text-foreground"
                      )}
                    >
                      <ShieldCheck className="size-4 opacity-60" />
                      Admin
                    </Link>
                  )}
                </nav>

                <div className="p-4 mt-auto border-t border-border flex-none">
                  <Link 
                    to="/app/settings" 
                    onClick={() => setMobileMenuOpen(false)}
                    className="flex items-center gap-3 px-3 py-2 text-sm font-medium text-muted-foreground hover:text-foreground transition-colors"
                  >
                    <Settings className="size-4 opacity-60" />
                    <span>{t('layout.systemSettings')}</span>
                  </Link>
                </div>
              </SheetContent>
            </Sheet>
          </div>

          <Link to="/" className="flex items-center gap-2 px-2 hover:opacity-70 transition-opacity">
            <div className="flex size-6 items-center justify-center rounded-md bg-primary">
              <span className="text-sm font-bold text-primary-foreground italic">S</span>
            </div>
            <span className="text-sm font-semibold tracking-tight hidden sm:inline">PaperSage</span>
          </Link>
        </div>

        {/* Center: Search */}
        <div className="desktop-no-drag relative hidden max-w-xl flex-1 px-4 md:block">
          <Search className="absolute left-7 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="search"
            aria-label={t('layout.search')}
            placeholder={t('layout.searchPlaceholder')}
            className="h-8 w-full bg-muted border-none text-sm rounded-lg pl-9 focus-visible:ring-1 focus-visible:ring-ring"
          />
        </div>

        {/* Right: Actions */}
        <div className="desktop-no-drag flex items-center justify-end gap-1 lg:w-64">
          <Button variant="ghost" size="icon-sm" className="rounded-full hover:bg-muted md:hidden" aria-label={t('layout.search')}>
            <Search className="size-4 text-muted-foreground" />
          </Button>
          <GuestModeChip className="rounded-full" />
          {!isGuest && <ApiTargetToggle />}
          <Button variant="ghost" size="icon-sm" className="rounded-full hover:bg-muted" aria-label={t('layout.notifications')}>
            <Bell className="size-4 text-muted-foreground" />
          </Button>
          <div className="h-4 w-px bg-border mx-1 md:mx-2" />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button aria-label={`${t('account.menu')}: ${accountLabel}`} className="flex items-center gap-2 pl-1 pr-2 py-1 rounded-full hover:bg-muted transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring">
                <div aria-hidden="true" className="flex size-7 items-center justify-center rounded-full bg-primary text-primary-foreground text-[10px] font-bold">
                  {isGuest ? t('account.guestAvatar') : avatarInitials(me?.username)}
                </div>
                <span className="max-w-36 truncate text-sm font-medium hidden lg:inline">{accountLabel}</span>
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <div className="px-3 py-2">
                <p className="truncate text-sm font-semibold">{accountLabel}</p>
                <p className="truncate text-xs text-muted-foreground">{isGuest ? t('account.guestStorage') : me?.email ?? (isMeLoading ? t('account.loading') : t('account.noEmail'))}</p>
              </div>
              <DropdownMenuSeparator />
              {isGuest ? (
                <DropdownMenuItem asChild>
                  <Link to="/auth"><User className="mr-2 h-4 w-4" /><span>{t('account.loginOrRegister')}</span></Link>
                </DropdownMenuItem>
              ) : (
                <>
              <DropdownMenuItem asChild>
                <Link to="/app/settings">
                  <User className="mr-2 h-4 w-4" />
                  <span>{t('account.profile')}</span>
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link to="/app/settings">
                  <Settings className="mr-2 h-4 w-4" />
                  <span>{t('account.settings')}</span>
                </Link>
              </DropdownMenuItem>
                </>
              )}
              {isMeError && !isGuest && <DropdownMenuItem onSelect={() => void refetchMe()}>{t('account.retry')}</DropdownMenuItem>}
              {!isGuest && <>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="text-destructive focus:text-destructive cursor-pointer"
                onClick={() => logout()}
              >
                <LogOut className="mr-2 h-4 w-4" />
                <span>{t('account.logout')}</span>
              </DropdownMenuItem>
              </>}
            </DropdownMenuContent>
          </DropdownMenu>
          <WindowControls className="ml-1" />
        </div>
      </header>

      <div className="flex flex-1 overflow-hidden">
        {/* Sidebar: Fixed, but internally scrollable if needed */}
        <aside className="hidden w-64 flex-none flex-col border-r border-border bg-muted/30 lg:flex overflow-hidden">
          <div className="p-4 flex items-center justify-between flex-none">
            <span className="px-2 text-xs font-bold text-muted-foreground uppercase tracking-widest">
              {t('layout.library')}
            </span>
            <Button variant="ghost" size="icon-xs" className="rounded-md" onClick={() => setCreateProjectOpen(true)}>
              <Plus className="size-3" />
            </Button>
          </div>
          
          <nav className="flex-1 px-2 space-y-0.5 overflow-y-auto no-scrollbar">
            {mainNavItems.map((item) => {
              const isActive = currentPath === item.path
              const Icon = item.icon
              return (
                <Link
                  key={item.path}
                  to={item.path}
                  className={cn(
                    "flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-all duration-200",
                    isActive
                      ? "bg-primary text-primary-foreground shadow-sm"
                      : "text-muted-foreground hover:bg-muted hover:text-foreground"
                  )}
                >
                  <Icon className={cn("size-4", isActive ? "text-inherit" : "opacity-60")} />
                  {t(item.labelKey)}
                </Link>
              )
            })}
          </nav>

          <div className="p-4 mt-auto border-t border-border flex-none">
            <Link 
              to="/app/settings" 
              className="flex items-center gap-3 px-3 py-2 text-sm font-medium text-muted-foreground hover:text-foreground transition-colors"
            >
              <Settings className="size-4 opacity-60" />
              <span>{t('layout.systemSettings')}</span>
            </Link>
          </div>
        </aside>

        {/* Main Content: Only this part scrolls */}
        <main className="flex-1 bg-background overflow-y-auto relative">
          {!isGuest && guestImportPending && (
            <div className="px-8 pt-6 md:px-12">
              <GuestSessionBadge />
            </div>
          )}
          {children}
        </main>
      </div>

      <CreateProjectDialog
        open={createProjectOpen}
        onOpenChange={setCreateProjectOpen}
        groups={groups}
      />
    </div>
  )
}
