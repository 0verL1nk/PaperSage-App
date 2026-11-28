import { createContext, useContext, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

interface WorkspaceChromeSlots {
  actions: HTMLElement | null
  menuExtra: HTMLElement | null
  closeMenu: () => void
}

interface WorkspaceChromeRegistry {
  setActions: (node: HTMLElement | null) => void
  setMenuExtra: (node: HTMLElement | null) => void
}

const SlotsContext = createContext<WorkspaceChromeSlots>({ actions: null, menuExtra: null, closeMenu: () => {} })
const RegistryContext = createContext<WorkspaceChromeRegistry | null>(null)

/** 工作区壳：页面把主操作投递到移动顶栏，把低频项投递到「更多」面板。 */
export function WorkspaceChromeProvider({ children, closeMenu }: { children: ReactNode; closeMenu: () => void }) {
  const [actions, setActions] = useState<HTMLElement | null>(null)
  const [menuExtra, setMenuExtra] = useState<HTMLElement | null>(null)
  return (
    <RegistryContext.Provider value={{ setActions, setMenuExtra }}>
      <SlotsContext.Provider value={{ actions, menuExtra, closeMenu }}>{children}</SlotsContext.Provider>
    </RegistryContext.Provider>
  )
}

export function WorkspaceHeaderActionsSlot() {
  const registry = useContext(RegistryContext)
  return <div ref={registry?.setActions} className="flex items-center gap-1" />
}

export function WorkspaceMenuExtraSlot() {
  const registry = useContext(RegistryContext)
  return <div ref={registry?.setMenuExtra} className="flex flex-col" />
}

/** 页面主操作：挂在工作区壳里就投递到顶栏；独立渲染（单测）时就地显示。 */
export function WorkspacePageActions({ children }: { children: ReactNode }) {
  const registry = useContext(RegistryContext)
  const slots = useContext(SlotsContext)
  if (!registry) return <>{children}</>
  return slots.actions ? createPortal(children, slots.actions) : null
}

/** 页面低频项：只在工作区壳的「更多」面板里显示，不需要壳时什么也不渲染。 */
export function WorkspaceMenuExtra({ children }: { children: ReactNode }) {
  const registry = useContext(RegistryContext)
  const slots = useContext(SlotsContext)
  if (!registry) return null
  return slots.menuExtra ? createPortal(children, slots.menuExtra) : null
}

/** 关掉「更多」面板，再打开页面自己的弹层（避免两层弹层叠在一起）。 */
// eslint-disable-next-line react-refresh/only-export-components -- 与插槽同属一个上下文，拆文件反而多一层
export function useCloseWorkspaceMenu() {
  return useContext(SlotsContext).closeMenu
}
