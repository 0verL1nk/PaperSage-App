import { Link, useLocation } from '@tanstack/react-router'
import { FileText, Home, Trash2, User } from 'lucide-react'
import { type ReactNode } from 'react'
import { Button } from '@/components/ui/button'

interface MainLayoutProps {
  children: ReactNode
}

const navItems = [
  { path: '/app/documents', label: 'Documents', icon: FileText },
  { path: '/app/trash', label: 'Trash', icon: Trash2 },
  { path: '/app/profile', label: 'Profile', icon: User },
]

export function MainLayout({ children }: MainLayoutProps) {
  const location = useLocation()
  const currentPath = location.pathname

  return (
    <div className="flex min-h-screen flex-col">
      {/* Header */}
      <header className="sticky top-0 z-50 flex h-16 items-center border-b bg-background/95 px-6 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="flex items-center gap-2">
          <Link to="/" className="flex items-center gap-2">
            <Home className="size-5" />
            <span className="text-lg font-semibold">PaperSage</span>
          </Link>
        </div>
      </header>

      <div className="flex flex-1">
        {/* Sidebar */}
        <aside className="hidden w-64 flex-col border-r bg-background lg:flex">
          <nav className="flex-1 space-y-1 p-4">
            {navItems.map((item) => {
              const isActive = currentPath === item.path
              const Icon = item.icon
              return (
                <Link
                  key={item.path}
                  to={item.path}
                  className={`flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
                    isActive
                      ? 'bg-accent text-accent-foreground'
                      : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                  }`}
                >
                  <Icon className="size-4" />
                  {item.label}
                </Link>
              )
            })}
          </nav>

          <div className="border-t p-4">
            <Button variant="outline" size="sm" className="w-full" asChild>
              <Link to="/auth">Logout</Link>
            </Button>
          </div>
        </aside>

        {/* Main Content */}
        <main className="flex-1">{children}</main>
      </div>
    </div>
  )
}