import { forwardRef, useCallback, useMemo, useRef, useImperativeHandle } from 'react'
import { Tree, type NodeRendererProps, type TreeApi, type CreateHandler, type RenameHandler, type MoveHandler } from 'react-arborist'
import { cn } from '@/lib/utils'
import {
  FileText,
  FileCode,
  FolderOpen,
  FolderClosed,
  Trash2,
  Pencil,
  Plus,
  FolderPlus,
} from 'lucide-react'
import {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
} from '@/components/ui/context-menu'
import type { LatexFileItem } from '@/services/latex-api'

// ---------------------------------------------------------------------------
// Data transform: LatexFileItem[] → react-arborist tree data
// ---------------------------------------------------------------------------

const HIDDEN_LEAF_NAMES = new Set(['.gitkeep'])

export interface FileNodeData {
  id: string
  name: string
  fullPath: string
  children?: FileNodeData[]
}

function toArboristData(items: LatexFileItem[]): FileNodeData[] {
  const dirs = new Map<string, FileNodeData[]>()
  const roots: FileNodeData[] = []

  for (const item of items) {
    const parts = item.file_path.split('/').filter(Boolean)
    const fileName = parts[parts.length - 1]
    const isHidden = HIDDEN_LEAF_NAMES.has(fileName)

    for (let i = 0; i < parts.length; i++) {
      const fullPath = parts.slice(0, i + 1).join('/')
      const isLeaf = i === parts.length - 1

      if (isLeaf && isHidden) continue

      const parentPath = parts.slice(0, i).join('/')
      const siblings = parentPath ? (dirs.get(parentPath) ?? roots) : roots

      if (!siblings.some((n) => n.id === fullPath)) {
        const node: FileNodeData = { id: fullPath, name: parts[i], fullPath }
        siblings.push(node)

        if (!isLeaf && !dirs.has(fullPath)) {
          dirs.set(fullPath, [])
        }
      }
    }
  }

  for (const [parentPath, children] of dirs) {
    const find = (list: FileNodeData[]): FileNodeData | undefined =>
      list.find((n) => n.id === parentPath)
    const parent = find(roots) ?? find([...dirs.values()].flat())
    if (parent) {
      ;(parent as FileNodeData & { children?: FileNodeData[] }).children = sortNodes(children)
    }
  }

  return sortNodes(roots)
}

function sortNodes(nodes: FileNodeData[]): FileNodeData[] {
  return [...nodes].sort((a, b) => {
    const aDir = a.children != null
    const bDir = b.children != null
    if (aDir !== bDir) return aDir ? -1 : 1
    return a.name.localeCompare(b.name)
  })
}

// ---------------------------------------------------------------------------
// Icon helper
// ---------------------------------------------------------------------------

function fileIcon(name: string) {
  const ext = `.${name.split('.').pop()?.toLowerCase() ?? ''}`
  if (['.bib', '.cls', '.sty', '.bst'].includes(ext))
    return <FileCode size={14} className="text-orange-500 shrink-0" />
  return <FileText size={14} className="text-primary shrink-0" />
}

// ---------------------------------------------------------------------------
// Inline edit input (used for both create and rename)
// ---------------------------------------------------------------------------

function InlineInput({ onSubmit, onCancel, defaultValue }: {
  onSubmit: (value: string) => void
  onCancel: () => void
  defaultValue?: string
}) {
  return (
    <input
      autoFocus
      className="w-full px-1.5 py-0.5 text-[12px] bg-background border border-primary rounded outline-none"
      defaultValue={defaultValue}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          const val = (e.target as HTMLInputElement).value.trim()
          if (val) onSubmit(val)
          else onCancel()
        }
        if (e.key === 'Escape') onCancel()
      }}
      onBlur={(e) => {
        const val = e.target.value.trim()
        if (val) onSubmit(val)
        else onCancel()
      }}
    />
  )
}

// ---------------------------------------------------------------------------
// Node renderer
// ---------------------------------------------------------------------------

interface NodeActions {
  onDeleteFile: (path: string) => void
}

function NodeRenderer({
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
      <div style={style} className="px-2 w-full">
        <div className="flex items-center gap-2 px-2 py-1.5">
          {isDir
            ? <FolderOpen size={14} className="text-yellow-500/80 shrink-0" />
            : fileIcon(node.data.name)}
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
              'flex items-center gap-2 px-2 py-1.5 text-[12px] cursor-pointer transition-all duration-200 rounded-lg group relative',
              willReceiveDrop && 'bg-primary/5 ring-1 ring-primary/30',
              isActive && !willReceiveDrop
                ? 'bg-primary/10 text-primary font-bold shadow-sm'
                : 'text-muted-foreground hover:bg-muted hover:text-foreground',
            )}
            onClick={() => tree.select(node.data.id, { focus: true })}
          >
            {fileIcon(node.data.name)}
            <span className="truncate flex-1">{node.data.name}</span>
            {isActive && (
              <div className="absolute left-0 top-1.5 bottom-1.5 w-0.5 bg-primary rounded-full" />
            )}
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
        'flex items-center gap-2 px-2 py-1.5 text-[12px] cursor-pointer text-foreground/80 transition-all rounded-lg font-bold group',
        willReceiveDrop ? 'bg-primary/5 ring-1 ring-primary/30' : 'hover:bg-muted',
        node.isDragging && 'opacity-40',
      )}
      onClick={() => node.toggle()}
    >
      {node.isOpen ? (
        <FolderOpen size={14} className="text-yellow-500/80 transition-transform duration-300 group-hover:scale-110 shrink-0" />
      ) : (
        <FolderClosed size={14} className="text-yellow-500/80 transition-transform duration-300 group-hover:scale-110 shrink-0" />
      )}
      <span>{node.data.name}</span>
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
          onClick={() => actions.onDeleteFile(node.data.fullPath)}
        >
          <Trash2 size={14} className="mr-2" />删除
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  )
}

// ---------------------------------------------------------------------------
// Public component (forwardRef to expose imperative tree actions)
// ---------------------------------------------------------------------------

export interface FileTreeHandle {
  createLeaf: () => void
  createInternal: () => void
}

interface FileTreeProps {
  files: LatexFileItem[]
  activeFilePath: string | null
  onSelect: (filePath: string) => void
  onDeleteFile: (filePath: string) => void
  onCreateFile: (path: string, content?: string) => Promise<LatexFileItem>
  onRenameFile: (oldPath: string, newPath: string) => Promise<LatexFileItem>
  onMoveFile: (filePath: string, newDir: string) => Promise<LatexFileItem>
  onExternalDrop?: (files: FileList) => void
}

export const WritingFileTree = forwardRef<FileTreeHandle, FileTreeProps>(function WritingFileTree({
  files,
  activeFilePath,
  onSelect,
  onDeleteFile,
  onCreateFile,
  onRenameFile,
  onMoveFile,
  onExternalDrop,
}, ref) {
  const treeRef = useRef<TreeApi<FileNodeData>>(null)

  useImperativeHandle(ref, () => ({
    createLeaf: () => treeRef.current?.createLeaf(),
    createInternal: () => treeRef.current?.createInternal(),
  }), [])
  const data = useMemo(() => toArboristData(files), [files])

  const actions = useMemo<NodeActions>(
    () => ({ onDeleteFile }),
    [onDeleteFile],
  )

  const handleActivate = useCallback(
    (node: { data: FileNodeData }) => {
      if (!('children' in node.data) || node.data.children == null) onSelect(node.data.fullPath)
    },
    [onSelect],
  )

  const handleCreate: CreateHandler<FileNodeData> = useCallback(
    async ({ parentId, type }) => {
      const editingNode = treeRef.current?.get(treeRef.current.editingId ?? null)
      const rawName = editingNode?.data?.name ?? (type === 'internal' ? 'new_folder' : 'new_file.tex')

      if (!rawName.trim()) return null

      if (type === 'internal') {
        const dirPath = parentId ? `${parentId}/${rawName}` : rawName
        const keepPath = `${dirPath}/.gitkeep`
        await onCreateFile(keepPath, '')
        return { id: dirPath }
      } else {
        const filePath = parentId ? `${parentId}/${rawName}` : rawName
        await onCreateFile(filePath, '')
        return { id: filePath }
      }
    },
    [onCreateFile],
  )

  const handleRename: RenameHandler<FileNodeData> = useCallback(
    async ({ id, name }) => {
      const node = treeRef.current?.get(id)
      if (!node) return

      const oldPath = node.data.fullPath
      const parts = oldPath.split('/')
      parts[parts.length - 1] = name
      const newPath = parts.join('/')

      if (oldPath !== newPath) {
        await onRenameFile(oldPath, newPath)
      }
    },
    [onRenameFile],
  )

  const handleMove: MoveHandler<FileNodeData> = useCallback(
    async ({ dragIds, parentId }) => {
      const dragId = dragIds[0]
      if (!dragId) return
      const newDir = parentId ?? ''
      await onMoveFile(dragId, newDir)
    },
    [onMoveFile],
  )

  const handleTreeDragOver = useCallback((e: React.DragEvent) => {
    if (!e.dataTransfer.types.includes('Files')) return
    e.preventDefault()
    e.stopPropagation()
  }, [])

  const handleTreeDrop = useCallback((e: React.DragEvent) => {
    if (!e.dataTransfer.types.includes('Files') || !onExternalDrop) return
    if (e.dataTransfer.files.length === 0) return
    e.preventDefault()
    e.stopPropagation()
    onExternalDrop(e.dataTransfer.files)
  }, [onExternalDrop])

  if (files.length === 0) {
    return (
      <div className="flex-1 overflow-y-auto py-3 scrollbar-none">
        <p className="text-[11px] text-muted-foreground text-center py-8">暂无文件</p>
      </div>
    )
  }

  return (
    <div
      className="flex-1 overflow-y-auto py-3 scrollbar-none"
      onDragOver={handleTreeDragOver}
      onDrop={handleTreeDrop}
    >
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <div className="min-h-full">
            <Tree<FileNodeData>
              ref={treeRef}
              data={data}
              openByDefault
              rowHeight={32}
              indent={16}
              padding={0}
              onActivate={handleActivate}
              selection={activeFilePath ?? undefined}
              onCreate={handleCreate}
              onRename={handleRename}
              onMove={handleMove}
            >
              {(props) => <NodeRenderer {...props} actions={actions} activeFilePath={activeFilePath} />}
            </Tree>
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent className="w-44">
          <ContextMenuItem onClick={() => treeRef.current?.createLeaf()}>
            <Plus size={14} className="mr-2" />新建文件
          </ContextMenuItem>
          <ContextMenuItem onClick={() => treeRef.current?.createInternal()}>
            <FolderPlus size={14} className="mr-2" />新建文件夹
          </ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>
    </div>
  )
})
