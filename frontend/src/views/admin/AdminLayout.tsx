/**
 * 管理后台二级导航：系统设置 + 编译错误日志（仅 superuser，接口 403 兜底）。
 */
import { Link, Outlet, useLocation } from '@tanstack/react-router'
import { ShieldCheck } from 'lucide-react'

import { cn } from '@/lib/utils'

const TABS = [
  { to: '/app/admin', label: '系统设置' },
  { to: '/app/admin/compile-diagnostics', label: '编译错误日志' },
] as const

export function AdminLayout() {
  const location = useLocation()

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6 p-4 sm:p-6">
      <div className="flex items-center gap-2">
        <ShieldCheck className="size-5 text-primary/70" />
        <h1 className="text-lg font-semibold">管理后台</h1>
      </div>

      <nav aria-label="管理后台导航" className="-mx-1 flex gap-1 overflow-x-auto px-1 pb-1">
        {TABS.map((tab) => {
          const isActive = tab.to === '/app/admin'
            ? location.pathname === '/app/admin' || location.pathname === '/app/admin/'
            : location.pathname.startsWith(tab.to)
          return (
            <Link
              key={tab.to}
              to={tab.to}
              className={cn(
                'shrink-0 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                isActive
                  ? 'bg-primary text-primary-foreground shadow-sm'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground',
              )}
            >
              {tab.label}
            </Link>
          )
        })}
      </nav>

      <Outlet />
    </div>
  )
}
