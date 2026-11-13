import { Check, Cloud, Copy, HardDrive, Maximize2, Menu, Minus, Power, RefreshCw, Square, X } from 'lucide-react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { getApiTarget, setApiTarget, type ApiTarget } from '@/lib/env'
import { getDesktopService, hasCustomTitlebar } from '@/lib/desktop'
import { cn } from '@/lib/utils'

/** App menu hosted by the page header instead of a separate title strip. */
export function DesktopAppMenu({ className }: { className?: string }) {
  if (!hasCustomTitlebar()) return null
  return <DesktopAppMenuInner className={className} />
}

/** Minimise / maximise / close, sized to sit inside a page header. */
export function WindowControls({ className }: { className?: string }) {
  const service = getDesktopService()
  const [maximised, setMaximised] = useState(false)

  useEffect(() => {
    let alive = true
    void service?.windowIsMaximised().then((value) => {
      if (alive) setMaximised(value)
    })
    return () => {
      alive = false
    }
  }, [service])

  if (!hasCustomTitlebar()) return null

  const refresh = async () => {
    const value = await service?.windowIsMaximised()
    setMaximised(Boolean(value))
  }

  return (
    <div
      className={cn('desktop-no-drag flex h-full items-stretch', className)}
      onDoubleClick={(event) => event.stopPropagation()}
    >
      <WindowButton label="最小化" onClick={() => void service?.windowMinimise()}>
        <Minus className="size-3.5" />
      </WindowButton>
      <WindowButton
        label={maximised ? '还原' : '最大化'}
        onClick={() => void service?.windowToggleMaximise().then(refresh)}
      >
        {maximised ? <Copy className="size-3 -scale-x-100" /> : <Square className="size-3" />}
      </WindowButton>
      <WindowButton label="关闭" danger onClick={() => void service?.windowClose()}>
        <X className="size-3.5" />
      </WindowButton>
    </div>
  )
}

function DesktopAppMenuInner({ className }: { className?: string }) {
  const service = getDesktopService()
  const [menuOpen, setMenuOpen] = useState(false)
  const [menuPos, setMenuPos] = useState<{ left: number; top: number } | null>(null)
  const [target, setTarget] = useState<ApiTarget>(() => getApiTarget())
  const triggerRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!menuOpen) return
    const first = menuRef.current?.querySelector<HTMLElement>('[role^="menuitem"]')
    first?.focus()
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setMenuOpen(false)
        triggerRef.current?.focus()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [menuOpen])

  const toggleMenu = () => {
    const rect = triggerRef.current?.getBoundingClientRect()
    if (!menuOpen && rect) {
      setMenuPos({ left: rect.left, top: rect.bottom + 4 })
    }
    setMenuOpen((value) => !value)
  }

  const closeMenu = (action?: () => void) => {
    setMenuOpen(false)
    action?.()
  }

  const switchTarget = (next: ApiTarget) => {
    if (next === target) {
      closeMenu()
      return
    }
    setApiTarget(next)
    setTarget(next)
    window.location.reload()
  }

  return (
    <div className={cn('desktop-no-drag relative', className)}>
      <button
        ref={triggerRef}
        type="button"
        aria-label="应用菜单"
        title="应用菜单"
        aria-haspopup="menu"
        aria-expanded={menuOpen}
        onClick={toggleMenu}
        className="desktop-no-drag flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-foreground/[0.06] hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <Menu className="size-4" />
      </button>

      {menuOpen && menuPos && createPortal(
        <>
          <div className="fixed inset-0 z-[100]" onClick={() => setMenuOpen(false)} />
          <div
            ref={menuRef}
            role="menu"
            aria-label="应用菜单"
            style={{ left: menuPos.left, top: menuPos.top }}
            className="fixed z-[110] w-56 rounded-xl border border-border/60 bg-popover/95 p-1 shadow-[0_16px_40px_-12px_rgba(0,0,0,.4)] backdrop-blur-xl"
          >
            <MenuAction onSelect={() => closeMenu(() => void service?.windowToggleFullscreen())}>
              <Maximize2 className="size-4" />
              切换全屏
            </MenuAction>
            <MenuAction onSelect={() => closeMenu(() => void service?.checkForUpdates())}>
              <RefreshCw className="size-4" />
              检查更新
            </MenuAction>
            <div className="my-1 h-px bg-border/60" />
            <div className="px-2.5 pb-1 pt-1.5 text-[10px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">
              服务器
            </div>
            <MenuRadio label="在线" icon={<Cloud className="size-4" />} checked={target === 'online'} onSelect={() => switchTarget('online')} />
            <MenuRadio label="本机" icon={<HardDrive className="size-4" />} checked={target === 'local'} onSelect={() => switchTarget('local')} />
            <div className="my-1 h-px bg-border/60" />
            <MenuAction onSelect={() => closeMenu(() => void service?.quitApp())}>
              <Power className="size-4" />
              退出 PaperSage
            </MenuAction>
          </div>
        </>,
        document.body,
      )}
    </div>
  )
}

function MenuAction({ onSelect, children }: { onSelect: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onSelect}
      className="flex h-9 w-full items-center gap-2 rounded-lg px-2.5 text-left text-[12.5px] text-foreground/85 transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
    >
      {children}
    </button>
  )
}

function MenuRadio({
  label,
  icon,
  checked,
  onSelect,
}: {
  label: string
  icon: ReactNode
  checked: boolean
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      role="menuitemradio"
      aria-checked={checked}
      onClick={onSelect}
      className="flex h-9 w-full items-center gap-2 rounded-lg px-2.5 text-left text-[12.5px] text-foreground/85 transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
    >
      {icon}
      <span className="flex-1">{label}</span>
      <Check className={cn('size-4', checked ? 'opacity-100' : 'opacity-0')} />
    </button>
  )
}

function WindowButton({
  label,
  onClick,
  danger,
  children,
}: {
  label: string
  onClick: () => void
  danger?: boolean
  children: ReactNode
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      className={cn(
        'desktop-no-drag flex w-11 items-center justify-center text-muted-foreground transition-colors',
        'hover:bg-foreground/[0.07] hover:text-foreground',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring',
        danger && 'hover:bg-[#c42b1c] hover:text-white',
      )}
    >
      {children}
    </button>
  )
}
