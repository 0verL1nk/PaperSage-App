import { Link, useLocation } from '@tanstack/react-router'
import {
  FileText,
  Trash2,
  User,
  FolderOpen,
  LogOut,
  Bell,
  Search,
  ChevronDown,
  PlusCircle,
} from 'lucide-react'
import { type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

interface MainLayoutProps {
  children: ReactNode
}

const mainNavItems = [
  { path: '/app/projects', label: 'Projects', icon: FolderOpen },
  { path: '/app/documents', label: 'Documents', icon: FileText },
  { path: '/app/trash', label: 'Trash', icon: Trash2 },
]

export function MainLayout({ children }: MainLayoutProps) {
  const location = useLocation()
  const currentPath = location.pathname

  return (
    <div className="flex min-h-screen flex-col">
      {/* Header */}
      <header className="sticky top-0 z-50 flex h-16 items-center justify-between gap-4 border-b bg-background px-6">
        {/* Left section: Logo */}
        <div className="flex items-center gap-4">
          <Link to="/" className="flex items-center gap-2 font-semibold shrink-0">
            <div className="flex size-8 items-center justify-center rounded-lg bg-primary">
              <span className="text-lg font-bold text-primary-foreground">P</span>
            </div>
            <span className="text-lg hidden sm:inline">PaperSage</span>
          </Link>
        </div>

        {/* Center: Search */}
        <div className="relative flex-1 max-w-md mx-4">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="search"
            placeholder="Search..."
            className="pl-9 h-9 bg-muted/50"
          />
        </div>

        {/* Right actions */}
        <div className="flex items-center gap-1">
          {/* Create new */}
          <Button variant="liquid" size="sm" className="gap-2">
            <PlusCircle className="size-4" />
            <span className="hidden sm:inline">New</span>
          </Button>

          {/* Notifications */}
          <Button variant="ghost" size="icon" className="relative">
            <Bell className="size-5" />
            <span className="absolute -top-1 -right-1 size-4 rounded-full bg-destructive text-[10px] font-medium text-destructive-foreground flex items-center justify-center">
              0
            </span>
          </Button>

          {/* User menu */}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="sm" className="gap-2 h-9 px-2">
                <div className="flex size-6 items-center justify-center rounded-full bg-muted text-xs font-medium">
                  U
                </div>
                <span className="hidden sm:inline text-sm font-medium">User</span>
                <ChevronDown className="size-3 text-muted-foreground" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <div className="px-2 py-1.5 text-sm font-medium">User Name</div>
              <div className="px-2 py-1.5 text-xs text-muted-foreground">
                user@example.com
              </div>
              <DropdownMenuSeparator />
              <DropdownMenuItem asChild>
                <Link to="/app/profile">
                  <User className="mr-2 h-4 w-4" />
                  Profile
                </Link>
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem asChild>
                <Link to="/auth" className="text-destructive focus:text-destructive">
                  <LogOut className="mr-2 h-4 w-4" />
                  Logout
                </Link>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </header>

      <div className="flex flex-1">
        {/* Sidebar */}
        <aside className="hidden w-64 flex-col border-r bg-background lg:flex">
          <div className="p-4">
            <p className="px-3 text-xs font-semibold text-muted-foreground uppercase tracking-wider">
              Navigation
            </p>
          </div>
          <nav className="flex-1 space-y-1 px-3">
            {mainNavItems.map((item) => {
              const isActive = currentPath === item.path
              const Icon = item.icon
              return (
                <Link
                  key={item.path}
                  to={item.path}
                  className={`flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors ${
                    isActive
                      ? 'bg-primary/10 text-primary'
                      : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                  }`}
                >
                  <Icon className="size-5" />
                  {item.label}
                </Link>
              )
            })}
          </nav>

          <div className="p-4 mt-auto border-t">
            <Button variant="outline" className="w-full justify-start" asChild>
              <Link to="/app/profile">
                <User className="mr-2 h-4 w-4" />
                Profile
              </Link>
            </Button>
          </div>
        </aside>

        {/* Main Content */}
        <main className="flex-1 bg-muted/30">{children}</main>
      </div>
    </div>
  )
}
