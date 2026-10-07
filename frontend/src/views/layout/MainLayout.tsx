import { Link, useLocation } from '@tanstack/react-router'
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
import { useState, type ReactNode } from 'react'
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
import { GuestSessionBadge } from '@/components/domain/GuestSessionBadge'

interface MainLayoutProps {
  children: ReactNode
}

const mainNavItems = [
  { path: '/app/projects', label: 'Projects', icon: FolderOpen },
  { path: '/app/documents', label: 'Documents', icon: FileText },
  { path: '/app/trash', label: 'Trash', icon: Trash2 },
]

import { useMe } from '@/hooks/use-me'

export function MainLayout({ children }: MainLayoutProps) {
  const { data: me } = useMe()
  const location = useLocation()
  const currentPath = location.pathname
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const [createProjectOpen, setCreateProjectOpen] = useState(false)
  const { data: groups = [] } = useGroups()

  return (
    <div className="flex h-screen flex-col bg-background font-sans tracking-tight overflow-hidden">
      {/* Header: Fixed height */}
      <header className="apple-glass flex-none flex h-12 items-center justify-between px-4 border-b border-border z-50">
        {/* Left: Logo and Mobile Menu */}
        <div className="flex items-center lg:w-64">
          <div className="lg:hidden flex items-center mr-2">
            <Sheet open={mobileMenuOpen} onOpenChange={setMobileMenuOpen}>
              <SheetTrigger asChild>
                <Button variant="ghost" size="icon-sm" className="rounded-md">
                  <Menu className="size-4" />
                </Button>
              </SheetTrigger>
              <SheetContent side="left" className="p-0 w-64 flex flex-col">
                <SheetTitle className="sr-only">Navigation Menu</SheetTitle>
                <div className="p-4 flex items-center justify-between flex-none border-b border-border">
                  <span className="px-2 text-xs font-bold text-muted-foreground uppercase tracking-widest">
                    Library
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
                        {item.label}
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
                    <span>System Settings</span>
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
        <div className="relative flex-1 max-w-xl px-4 hidden md:block">
          <Search className="absolute left-7 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="search"
            placeholder="Search documents, projects..."
            className="h-8 w-full bg-muted border-none text-sm rounded-lg pl-9 focus-visible:ring-1 focus-visible:ring-ring"
          />
        </div>

        {/* Right: Actions */}
        <div className="flex items-center justify-end lg:w-64 gap-1">
          <Button variant="ghost" size="icon-sm" className="rounded-full hover:bg-muted md:hidden">
            <Search className="size-4 text-muted-foreground" />
          </Button>
          <ApiTargetToggle prominent />
          <Button variant="ghost" size="icon-sm" className="rounded-full hover:bg-muted">
            <Bell className="size-4 text-muted-foreground" />
          </Button>
          <div className="h-4 w-px bg-border mx-1 md:mx-2" />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button className="flex items-center gap-2 pl-1 pr-2 py-1 rounded-full hover:bg-muted transition-colors outline-none">
                <div className="flex size-6 items-center justify-center rounded-full bg-primary text-primary-foreground text-xs font-bold">
                  JS
                </div>
                <span className="text-sm font-medium hidden lg:inline">User Name</span>
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <div className="px-3 py-2">
                <p className="text-sm font-semibold">User Name</p>
                <p className="text-xs text-muted-foreground">user@example.com</p>
              </div>
              <DropdownMenuSeparator />

              <DropdownMenuItem asChild>
                <Link to="/app/settings">
                  <User className="mr-2 h-4 w-4" />
                  <span>Profile</span>
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link to="/app/settings">
                  <Settings className="mr-2 h-4 w-4" />
                  <span>Settings</span>
                </Link>
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="text-destructive focus:text-destructive cursor-pointer"
                onClick={() => logout()}
              >
                <LogOut className="mr-2 h-4 w-4" />
                <span>Logout</span>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </header>

      <div className="flex flex-1 overflow-hidden">
        {/* Sidebar: Fixed, but internally scrollable if needed */}
        <aside className="hidden w-64 flex-none flex-col border-r border-border bg-muted/30 lg:flex overflow-hidden">
          <div className="p-4 flex items-center justify-between flex-none">
            <span className="px-2 text-xs font-bold text-muted-foreground uppercase tracking-widest">
              Library
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
                  {item.label}
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
              <span>System Settings</span>
            </Link>
          </div>
        </aside>

        {/* Main Content: Only this part scrolls */}
        <main className="flex-1 bg-background overflow-y-auto relative">
          <GuestSessionBadge />
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
