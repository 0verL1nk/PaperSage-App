import { Link, useLocation } from '@tanstack/react-router'
import {
  FileText,
  Trash2,
  User,
  FolderOpen,
  LogOut,
  Search,
  Bell,
  Settings,
  Plus,
} from 'lucide-react'
import { type ReactNode } from 'react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import { logout } from '@/features/auth'

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
    <div className="flex h-screen flex-col bg-[#ffffff] dark:bg-[#000000] font-sans tracking-tight overflow-hidden">
      {/* Header: Fixed height */}
      <header className="apple-glass flex-none flex h-12 items-center justify-between px-4 border-b border-black/[0.05] dark:border-white/[0.05] z-50">
        {/* Left: Logo */}
        <div className="flex items-center w-64">
          <Link to="/" className="flex items-center gap-2 px-2 hover:opacity-70 transition-opacity">
            <div className="flex size-6 items-center justify-center rounded-[6px] bg-black dark:bg-white">
              <span className="text-[14px] font-bold text-white dark:text-black italic">S</span>
            </div>
            <span className="text-[15px] font-semibold tracking-tight hidden sm:inline">PaperSage</span>
          </Link>
        </div>

        {/* Center: Search */}
        <div className="relative flex-1 max-w-xl px-4">
          <Search className="absolute left-7 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-black/40 dark:text-white/40" />
          <Input
            type="search"
            placeholder="Search documents, projects..."
            className="h-8 w-full bg-black/[0.04] dark:bg-white/[0.06] border-none text-[13px] rounded-lg pl-9 focus-visible:ring-1 focus-visible:ring-black/10"
          />
        </div>

        {/* Right: Actions */}
        <div className="flex items-center justify-end w-64 gap-1">
          <Button variant="ghost" size="icon-sm" className="rounded-full hover:bg-black/10 dark:hover:bg-white/10">
            <Bell className="size-4 text-black/60 dark:text-white/60" />
          </Button>
          <div className="h-4 w-px bg-black/[0.08] dark:bg-white/[0.08] mx-2" />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button className="flex items-center gap-2 pl-1 pr-2 py-1 rounded-full hover:bg-black/10 dark:hover:bg-white/10 transition-colors outline-none">
                <div className="flex size-6 items-center justify-center rounded-full bg-black text-white text-[10px] font-bold">
                  JS
                </div>
                <span className="text-[13px] font-medium hidden lg:inline">User Name</span>
                </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-56">
                <div className="px-3 py-2">
                <p className="text-sm font-semibold">User Name</p>
                <p className="text-xs text-black/50 dark:text-white/50">user@example.com</p>
                </div>
                <DropdownMenuSeparator />

              <DropdownMenuItem asChild>
                <Link to="/app/profile">
                  <User className="mr-2 h-4 w-4" />
                  <span>Profile</span>
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link to="/app/profile">
                  <Settings className="mr-2 h-4 w-4" />
                  <span>Settings</span>
                </Link>
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="text-red-500 focus:text-red-500 cursor-pointer"
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
        <aside className="hidden w-64 flex-none flex-col border-r border-black/[0.05] dark:border-white/[0.05] bg-[#f5f5f7]/50 dark:bg-[#1d1d1f]/50 lg:flex overflow-hidden">
          <div className="p-4 flex items-center justify-between flex-none">
            <span className="px-2 text-[11px] font-bold text-black/40 dark:text-white/40 uppercase tracking-widest">
              Library
            </span>
            <Button variant="ghost" size="icon-xs" className="rounded-md">
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
                    "flex items-center gap-3 rounded-lg px-3 py-2 text-[13px] font-medium transition-all duration-200",
                    isActive
                      ? "bg-black text-white dark:bg-white dark:text-black shadow-sm"
                      : "text-black/70 dark:text-white/70 hover:bg-black/[0.04] dark:hover:bg-white/[0.04]"
                  )}
                >
                  <Icon className={cn("size-4", isActive ? "text-inherit" : "opacity-60")} />
                  {item.label}
                </Link>
              )
            })}
          </nav>

          <div className="p-4 mt-auto border-t border-black/[0.05] dark:border-white/[0.05] flex-none">
            <Link 
              to="/app/profile" 
              className="flex items-center gap-3 px-3 py-2 text-[13px] font-medium text-black/60 dark:text-white/60 hover:text-black dark:hover:text-white transition-colors"
            >
              <Settings className="size-4 opacity-60" />
              <span>System Settings</span>
            </Link>
          </div>
        </aside>

        {/* Main Content: Only this part scrolls */}
        <main className="flex-1 bg-white dark:bg-[#000000] overflow-y-auto relative">
          {children}
        </main>
      </div>
    </div>
  )
}
