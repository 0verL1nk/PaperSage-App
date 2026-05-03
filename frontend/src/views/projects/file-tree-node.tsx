import { cn } from '@/lib/utils'
import { useRef, useEffect } from 'react'
import {
  FolderOpen,
  FolderClosed,
  Trash2,
  Pencil,
  Plus,
  FolderPlus,
  ChevronRight,
} from 'lucide-react'
import {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
} from '@/components/ui/context-menu'
import type { NodeRendererProps, TreeApi } from 'react-arborist'
import { FileTypeIcon } from './file-tree-icons'
import type { FileNodeData } from './writing-file-tree'

// ---------------------------------------------------------------------------
// Inline edit input (cancel on blur, submit on Enter)
// ---------------------------------------------------------------------------

export function InlineInput({ onSubmit, onCancel, defaultValue }: {
  onSubmit: (value: string) => void
  onCancel: () => void
  defaultValue?: string
}) {
  const freshRef = useRef(true)
  const inputRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    const t = setTimeout(() => { freshRef.current = false }, 150)
    return () => clearTimeout(t)
  }, [])
  useEffect(() => {
    inputRef.current?.focus()
    inputRef.current?.select()
  }, [])

  return (
    <input
      ref={inputRef}
      className="w-full px-1 text-[12px] bg-transparent border-b border-primary outline-none"
      defaultValue={defaultValue}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          const val = (e.target as HTMLInputElement).value.trim()
          if (val) onSubmit(val)
          else onCancel()
        }
        if (e.key === 'Escape') onCancel()
      }}
      onBlur={() => {
        if (freshRef.current) return
        onCancel()
      }}
    />
  )
}

// ---------------------------------------------------------------------------
// Node actions passed from parent
// ---------------------------------------------------------------------------

export interface NodeActions {
  onDeleteFile: (path: string) => void
  onDeleteFolder: (path: string) => void
}

// ---------------------------------------------------------------------------
// Node renderer (file + folder)
// ---------------------------------------------------------------------------

export function NodeRenderer({
  node,
  style,
  dragHandle,
  tree,
  actions,
  activeFilePath,
}: NodeRendererProps<FileNodeData> & { actions: NodeActions; activeFilePath: string | null }) {
  const isDir = node.isInternal
  const isActive = activeFilePath === node.data.id
  const isDragging = node.isDragging
  const willReceiveDrop = node.willReceiveDrop

  if (node.isEditing) {
    return (
      <div style={style} ref={dragHandle} className="px-2 w-full">
        <div className={cn(
          'flex items-center gap-2 text-[12px] rounded-lg',
          isDir ? 'px-2 py-1.5' : 'px-2 py-1.5',
        )}>
          {isDir
            ? <FolderOpen size={14} className="text-yellow-500/80 shrink-0" />
            : <FileTypeIcon name={node.data.name} />}
          <InlineInput
            defaultValue={node.data.name}
            onSubmit={(value) => node.submit(value)}
            onCancel={() => node.reset()}
          />
        </div>
      </div>
    )
  }

  if (isDir) {
    return (
      <FolderNodeRow
        style={style}
        dragHandle={dragHandle}
        node={node}
        tree={tree}
        actions={actions}
        willReceiveDrop={willReceiveDrop}
      />
    )
  }

  return (
    <div style={style} ref={dragHandle} className={cn('px-2 w-full', isDragging && 'opacity-40')}>
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <div
            className={cn(
              'flex items-center gap-2 px-2 py-1.5 text-[12px] cursor-pointer transition-colors rounded-lg group relative w-full',
              willReceiveDrop && 'bg-primary/5 ring-1 ring-primary/30',
              isActive && !willReceiveDrop
                ? 'bg-primary/10 text-primary font-medium shadow-sm'
                : 'text-muted-foreground hover:bg-muted hover:text-foreground',
            )}
            onClick={() => tree.select(node.data.id, { focus: true })}
          >
            {isActive && (
              <div className="absolute left-0 top-1 bottom-1 w-[3px] bg-primary rounded-full" />
            )}
            <FileTypeIcon name={node.data.name} />
            <span className="truncate flex-1">{node.data.name}</span>
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent className="w-40">
          <ContextMenuItem onClick={() => tree.select(node.data.id, { focus: true })}>
            <Pencil size={14} className="mr-2" />打开
          </ContextMenuItem>
          <ContextMenuItem onClick={() => node.edit()}>
            <Pencil size={14} className="mr-2" />重命名
          </ContextMenuItem>
          <ContextMenuSeparator />
          <ContextMenuItem
            className="text-destructive focus:text-destructive"
            onClick={() => actions.onDeleteFile(node.data.fullPath)}
          >
            <Trash2 size={14} className="mr-2" />删除
          </ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Folder row with chevron animation
// ---------------------------------------------------------------------------

function FolderNodeRow({
  style,
  dragHandle,
  node,
  tree,
  actions,
  willReceiveDrop,
}: {
  style: React.CSSProperties
  dragHandle?: (el: HTMLDivElement | null) => void
  node: { data: FileNodeData; isInternal: boolean; isOpen: boolean; toggle: () => void; isDragging?: boolean }
  tree: TreeApi<FileNodeData>
  actions: NodeActions
  willReceiveDrop?: boolean
}) {
  const header = (
    <div
      ref={dragHandle}
      style={style}
      className={cn(
        'flex items-center gap-1 px-2 py-1.5 text-[12px] cursor-pointer text-foreground/80 transition-colors rounded-lg font-medium group w-full',
        willReceiveDrop ? 'bg-primary/5 ring-1 ring-primary/30' : 'hover:bg-muted',
        node.isDragging && 'opacity-40',
      )}
      onClick={() => node.toggle()}
    >
      <ChevronRight
        size={12}
        className={cn(
          'shrink-0 text-muted-foreground transition-transform duration-150',
          node.isOpen && 'rotate-90',
        )}
      />
      {node.isOpen ? (
        <FolderOpen size={14} className="text-yellow-500/80 shrink-0 transition-transform duration-150" />
      ) : (
        <FolderClosed size={14} className="text-yellow-500/80 shrink-0 transition-transform duration-150" />
      )}
      <span className="truncate flex-1">{node.data.name}</span>
    </div>
  )

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{header}</ContextMenuTrigger>
      <ContextMenuContent className="w-44">
        <ContextMenuItem onClick={() => tree.create({ type: 'leaf', parentId: node.data.id })}>
          <Plus size={14} className="mr-2" />新建文件
        </ContextMenuItem>
        <ContextMenuItem onClick={() => tree.create({ type: 'internal', parentId: node.data.id })}>
          <FolderPlus size={14} className="mr-2" />新建文件夹
        </ContextMenuItem>
        <ContextMenuSeparator />
        <ContextMenuItem onClick={() => tree.get(node.data.id)?.edit()}>
          <Pencil size={14} className="mr-2" />重命名
        </ContextMenuItem>
        <ContextMenuSeparator />
        <ContextMenuItem
          className="text-destructive focus:text-destructive"
          onClick={() => actions.onDeleteFolder(node.data.fullPath)}
        >
          <Trash2 size={14} className="mr-2" />删除
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  )
}
