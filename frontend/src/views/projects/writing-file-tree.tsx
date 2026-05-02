/* eslint-disable react-refresh/only-export-components */
import { forwardRef, useCallback, useMemo, useRef, useState, useImperativeHandle } from 'react'
import { Tree, type TreeApi, type CreateHandler, type RenameHandler, type MoveHandler } from 'react-arborist'
import { Plus, FolderPlus } from 'lucide-react'
import {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
} from '@/components/ui/context-menu'
import type { LatexFileItem } from '@/services/latex-api'
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

interface FileTreeProps {
  files: LatexFileItem[]
  activeFilePath: string | null
  onSelect: (filePath: string) => void
  onDeleteFile: (filePath: string) => void
  onDeleteFolder: (filePath: string) => void
  onCreateFile: (path: string, content?: string) => Promise<LatexFileItem>
  onRenameFile: (oldPath: string, newPath: string) => Promise<LatexFileItem>
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
  onMoveFile,
  onExternalDrop,
}, ref) {
  const treeRef = useRef<TreeApi<FileNodeData>>(null)
  const [searchTerm, setSearchTerm] = useState('')
  const [openState, setOpenState] = useState<Record<string, boolean>>({})

  useImperativeHandle(ref, () => ({
    createLeaf: () => treeRef.current?.createLeaf(),
    createInternal: () => treeRef.current?.createInternal(),
  }), [])

  const data = useMemo(() => toArboristData(files), [files])

  const actions = useMemo<NodeActions>(
    () => ({ onDeleteFile, onDeleteFolder }),
    [onDeleteFile, onDeleteFolder],
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
        await onCreateFile(keepPath, '').catch(() => {})
        return { id: dirPath }
      } else {
        const filePath = parentId ? `${parentId}/${rawName}` : rawName
        await onCreateFile(filePath, '').catch(() => {})
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
        await onRenameFile(oldPath, newPath).catch(() => {})
      }
    },
    [onRenameFile],
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

  if (files.length === 0) {
    return (
      <div className="flex-1 overflow-y-auto py-3 scrollbar-none">
        <p className="text-[11px] text-muted-foreground text-center py-8">暂无文件</p>
      </div>
    )
  }

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
          className="flex-1 overflow-y-auto py-3 scrollbar-none"
          onDragOver={handleTreeDragOver}
          onDrop={handleTreeDrop}
          onKeyDown={handleKeyDown}
          tabIndex={0}
        >
          <ContextMenu>
            <ContextMenuTrigger asChild>
              <div className="min-h-full">
                <Tree<FileNodeData>
                  ref={treeRef}
                  data={data}
                  initialOpenState={openState}
                  onToggle={handleToggle}
                  rowHeight={32}
                  indent={16}
                  padding={0}
                  onActivate={handleActivate}
                  selection={activeFilePath ?? undefined}
                  onCreate={handleCreate}
                  onRename={handleRename}
                  onMove={handleMove}
                  searchTerm={searchTerm}
                  searchMatch={searchMatch}
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
