/* eslint-disable react-refresh/only-export-components */
import { forwardRef, useCallback, useEffect, useMemo, useRef, useState, useImperativeHandle } from 'react'
import { Tree, type TreeApi, type RenameHandler, type MoveHandler } from 'react-arborist'
import { Check, FolderPlus, Plus, X } from 'lucide-react'
import { toast } from 'sonner'
import {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
} from '@/components/ui/context-menu'
import type { LatexFileItem } from '@/services/latex-api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NodeRenderer, type NodeActions } from './file-tree-node'
import { FileTreeSearch } from './file-tree-search'

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

/** Two-pass tree construction: build all nodes first, then link parents. */
export function toArboristData(items: LatexFileItem[]): FileNodeData[] {
  const nodeMap = new Map<string, FileNodeData>()

  // Pass 1: create all nodes
  for (const item of items) {
    const parts = item.file_path.split('/').filter(Boolean)
    for (let i = 0; i < parts.length; i++) {
      const fullPath = parts.slice(0, i + 1).join('/')
      const fileName = parts[parts.length - 1]
      const isLeaf = i === parts.length - 1
      if (isLeaf && HIDDEN_LEAF_NAMES.has(fileName)) continue

      if (!nodeMap.has(fullPath)) {
        nodeMap.set(fullPath, {
          id: fullPath,
          name: parts[i],
          fullPath,
          ...(isLeaf ? {} : { children: [] }),
        })
      } else if (isLeaf) {
        const existing = nodeMap.get(fullPath)!
        delete existing.children
      }
    }
  }

  // Pass 2: link children to parents
  for (const node of nodeMap.values()) {
    const lastSlash = node.fullPath.lastIndexOf('/')
    const parentPath = lastSlash >= 0 ? node.fullPath.substring(0, lastSlash) : ''
    if (!parentPath) continue
    const parent = nodeMap.get(parentPath)
    if (parent && parent.children) {
      parent.children.push(node)
    }
  }

  // Collect roots
  const roots: FileNodeData[] = []
  for (const node of nodeMap.values()) {
    const lastSlash = node.fullPath.lastIndexOf('/')
    const parentPath = lastSlash >= 0 ? node.fullPath.substring(0, lastSlash) : ''
    if (!parentPath || !nodeMap.has(parentPath)) {
      roots.push(node)
    }
  }

  return sortNodes(roots)
}

function sortNodes(nodes: FileNodeData[]): FileNodeData[] {
  const sorted = [...nodes].sort((a, b) => {
    const aDir = a.children != null
    const bDir = b.children != null
    if (aDir !== bDir) return aDir ? -1 : 1
    return a.name.localeCompare(b.name)
  })
  for (const node of sorted) {
    if (node.children?.length) {
      node.children = sortNodes(node.children)
    }
  }
  return sorted
}

// ---------------------------------------------------------------------------
// Public component
// ---------------------------------------------------------------------------

export interface FileTreeHandle {
  createLeaf: () => void
  createInternal: () => void
}

type CreationType = 'leaf' | 'internal'
type PendingCreation = { type: CreationType; parentPath: string; name: string }

interface FileTreeProps {
  files: LatexFileItem[]
  activeFilePath: string | null
  onSelect: (filePath: string) => void
  onDeleteFile: (filePath: string) => void
  onDeleteFolder: (filePath: string) => void
  onCreateFile: (path: string, content?: string) => Promise<LatexFileItem>
  onRenameFile: (oldPath: string, newPath: string) => Promise<LatexFileItem>
  onRenameFolder: (oldFolder: string, newFolder: string) => Promise<unknown>
  onMoveFile: (filePath: string, newDir: string) => Promise<LatexFileItem>
  onExternalDrop?: (files: FileList, targetFolder?: string) => void
}

export const WritingFileTree = forwardRef<FileTreeHandle, FileTreeProps>(function WritingFileTree({
  files,
  activeFilePath,
  onSelect,
  onDeleteFile,
  onDeleteFolder,
  onCreateFile,
  onRenameFile,
  onRenameFolder,
  onMoveFile,
  onExternalDrop,
}, ref) {
  const treeRef = useRef<TreeApi<FileNodeData>>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const [treeWidth, setTreeWidth] = useState(300)
  const [searchTerm, setSearchTerm] = useState('')
  const [openState, setOpenState] = useState<Record<string, boolean>>({})
  const [pendingCreation, setPendingCreation] = useState<PendingCreation | null>(null)
  const [creating, setCreating] = useState(false)

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const ro = new ResizeObserver(([entry]) => {
      setTreeWidth(Math.floor(entry.contentRect.width))
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const beginCreate = useCallback((type: CreationType, parentPath = '') => {
    setPendingCreation({ type, parentPath, name: type === 'internal' ? 'new_folder' : 'new_file.tex' })
  }, [])

  useImperativeHandle(ref, () => ({
    createLeaf: () => beginCreate('leaf'),
    createInternal: () => beginCreate('internal'),
  }), [beginCreate])

  const submitCreate = useCallback(async (event: React.FormEvent) => {
    event.preventDefault()
    if (!pendingCreation || !pendingCreation.name.trim() || creating) return
    const name = pendingCreation.name.trim()
    const path = pendingCreation.parentPath ? `${pendingCreation.parentPath}/${name}` : name
    setCreating(true)
    try {
      if (pendingCreation.type === 'internal') await onCreateFile(`${path}/.gitkeep`, '')
      else await onCreateFile(path, '')
      setPendingCreation(null)
    } catch (error) {
      toast.error(pendingCreation.type === 'internal' ? '文件夹创建失败' : '文件创建失败', {
        description: error instanceof Error ? error.message : '请检查网络和项目权限后重试。',
      })
    } finally {
      setCreating(false)
    }
  }, [creating, onCreateFile, pendingCreation])

  const data = useMemo(() => toArboristData(files), [files])

  const actions = useMemo<NodeActions>(
    () => ({
      onDeleteFile,
      onDeleteFolder,
      onStartCreateFile: (parentPath) => beginCreate('leaf', parentPath),
      onStartCreateFolder: (parentPath) => beginCreate('internal', parentPath),
    }),
    [beginCreate, onDeleteFile, onDeleteFolder],
  )

  const handleActivate = useCallback(
    (node: { data: FileNodeData }) => {
      if (!('children' in node.data) || node.data.children == null) onSelect(node.data.fullPath)
    },
    [onSelect],
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
        if (node.isInternal) {
          await onRenameFolder(oldPath, newPath).catch(() => {})
        } else {
          await onRenameFile(oldPath, newPath).catch(() => {})
        }
      }
    },
    [onRenameFile, onRenameFolder],
  )

  const handleMove: MoveHandler<FileNodeData> = useCallback(
    async ({ dragIds, parentId }) => {
      const dragId = dragIds[0]
      if (!dragId) return
      const newDir = parentId ?? ''
      await onMoveFile(dragId, newDir).catch(() => {})
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

  const handleToggle = useCallback((id: string) => {
    setOpenState((prev) => {
      const current = prev[id] ?? true
      return { ...prev, [id]: !current }
    })
  }, [])

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    const tree = treeRef.current
    if (!tree) return
    const selectedId = tree.selectedNodes[0]?.id
    if (!selectedId) return

    if (e.key === 'Delete') {
      e.preventDefault()
      const node = tree.get(selectedId)
      if (node) {
        if (node.isInternal) onDeleteFolder(node.data.fullPath)
        else onDeleteFile(node.data.fullPath)
      }
    } else if (e.key === 'F2') {
      e.preventDefault()
      tree.get(selectedId)?.edit()
    }
  }, [onDeleteFile, onDeleteFolder])

  const searchMatch = useCallback((node: { data: FileNodeData }, term: string) => {
    return node.data.name.toLowerCase().includes(term.toLowerCase())
  }, [])

  const hasNoResults = searchTerm && !dataHasMatch(data, searchTerm.toLowerCase())

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <FileTreeSearch value={searchTerm} onChange={setSearchTerm} />
      {hasNoResults ? (
        <div className="flex-1 overflow-y-auto py-3 scrollbar-none">
          <p className="text-[11px] text-muted-foreground text-center py-8">无匹配文件</p>
        </div>
      ) : (
        <div
          ref={containerRef}
          className="flex-1 overflow-y-auto overflow-x-hidden py-3 scrollbar-none"
          onDragOver={handleTreeDragOver}
          onDrop={handleTreeDrop}
          onKeyDown={handleKeyDown}
          tabIndex={0}
        >
          <ContextMenu>
            <ContextMenuTrigger asChild>
              <div className="relative min-h-full">
                {pendingCreation && (
                  <form onSubmit={submitCreate} className="mx-2 mb-2 rounded-lg border border-primary/20 bg-background p-2 shadow-sm" data-testid="tree-create-form">
                    <label htmlFor="tree-create-name" className="mb-1.5 block text-[11px] font-medium text-muted-foreground">
                      {pendingCreation.type === 'internal' ? '新建文件夹' : '新建文件'}
                      {pendingCreation.parentPath && <span className="ml-1">· {pendingCreation.parentPath}</span>}
                    </label>
                    <div className="flex items-center gap-1.5">
                      <Input
                        id="tree-create-name"
                        autoFocus
                        value={pendingCreation.name}
                        onChange={(event) => setPendingCreation({ ...pendingCreation, name: event.target.value })}
                        onKeyDown={(event) => { if (event.key === 'Escape') setPendingCreation(null) }}
                        className="h-9 min-w-0 text-xs"
                        aria-label={pendingCreation.type === 'internal' ? '文件夹名称' : '文件名称'}
                      />
                      <Button type="submit" size="icon-sm" aria-label="确认创建" disabled={creating || !pendingCreation.name.trim()}><Check className="size-4" /></Button>
                      <Button type="button" size="icon-sm" variant="ghost" aria-label="取消创建" disabled={creating} onClick={() => setPendingCreation(null)}><X className="size-4" /></Button>
                    </div>
                  </form>
                )}
                <Tree<FileNodeData>
                  ref={treeRef}
                  width={treeWidth}
                  data={data}
                  initialOpenState={openState}
                  onToggle={handleToggle}
                  rowHeight={32}
                  indent={16}
                  padding={0}
                  onActivate={handleActivate}
                  selection={activeFilePath ?? undefined}
                  onRename={handleRename}
                  onMove={handleMove}
                  searchTerm={searchTerm}
                  searchMatch={searchMatch}
                >
                  {(props) => <NodeRenderer {...props} actions={actions} activeFilePath={activeFilePath} />}
                </Tree>
                {files.length === 0 && (
                  <div className="pointer-events-none absolute inset-x-0 top-8 text-center text-[11px] text-muted-foreground">
                    暂无文件，点击上方 + 或文件夹按钮开始
                  </div>
                )}
              </div>
            </ContextMenuTrigger>
            <ContextMenuContent className="w-44">
              <ContextMenuItem onClick={() => beginCreate('leaf')}>
                <Plus size={14} className="mr-2" />新建文件
              </ContextMenuItem>
              <ContextMenuItem onClick={() => beginCreate('internal')}>
                <FolderPlus size={14} className="mr-2" />新建文件夹
              </ContextMenuItem>
            </ContextMenuContent>
          </ContextMenu>
        </div>
      )}
    </div>
  )
})

function dataHasMatch(nodes: FileNodeData[], term: string): boolean {
  for (const node of nodes) {
    if (node.name.toLowerCase().includes(term)) return true
    if (node.children?.length && dataHasMatch(node.children, term)) return true
  }
  return false
}
