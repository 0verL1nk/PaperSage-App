import { useState } from 'react'
import { cn } from '@/lib/utils'
import { FileText, FileCode, FolderOpen, FolderClosed, Trash2, Pencil } from 'lucide-react'
import {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
} from '@/components/ui/context-menu'
import type { LatexFileItem } from '@/services/latex-api'

// --- Tree structure builder ---

interface TreeNode {
  name: string
  fullPath: string
  isDir: boolean
  children: TreeNode[]
}

function buildTree(items: LatexFileItem[]): TreeNode[] {
  const root: TreeNode[] = []

  for (const item of items) {
    const segments = item.file_path.split('/').filter(Boolean)
    let current = root

    for (let i = 0; i < segments.length; i++) {
      const seg = segments[i]
      const isLeaf = i === segments.length - 1
      const fullPath = segments.slice(0, i + 1).join('/')

      let node = current.find((n) => n.name === seg)
      if (!node) {
        node = { name: seg, fullPath, isDir: !isLeaf, children: [] }
        current.push(node)
      }
      current = node.children
    }
  }

  sortTree(root)
  return root
}

function sortTree(nodes: TreeNode[]) {
  nodes.sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
    return a.name.localeCompare(b.name)
  })
  for (const n of nodes) sortTree(n.children)
}

function getFileIcon(name: string) {
  const ext = name.split('.').pop()?.toLowerCase() ?? ''
  if (['.bib', '.cls', '.sty', '.bst'].includes(`.${ext}`))
    return <FileCode size={14} className="text-orange-500" />
  return <FileText size={14} className="text-primary" />
}

// --- Component ---

interface FileTreeProps {
  files: LatexFileItem[]
  activeFilePath: string | null
  onSelect: (filePath: string) => void
  onDeleteFile: (filePath: string) => void
  onRenameFile?: (filePath: string) => void
}

export function WritingFileTree({
  files,
  activeFilePath,
  onSelect,
  onDeleteFile,
  onRenameFile,
}: FileTreeProps) {
  const tree = buildTree(files)

  return (
    <div className="flex-1 overflow-y-auto py-3 scrollbar-none">
      {tree.map((node) => (
        <TreeNodeView
          key={node.fullPath}
          node={node}
          depth={0}
          activeFilePath={activeFilePath}
          onSelect={onSelect}
          onDeleteFile={onDeleteFile}
          onRenameFile={onRenameFile}
        />
      ))}
      {files.length === 0 && (
        <p className="text-[11px] text-muted-foreground text-center py-8">
          暂无文件
        </p>
      )}
    </div>
  )
}

function TreeNodeView({
  node,
  depth,
  activeFilePath,
  onSelect,
  onDeleteFile,
  onRenameFile,
}: {
  node: TreeNode
  depth: number
  activeFilePath: string | null
  onSelect: (path: string) => void
  onDeleteFile: (path: string) => void
  onRenameFile?: (path: string) => void
}) {
  if (node.isDir) {
    return <FolderNode node={node} depth={depth} activeFilePath={activeFilePath} onSelect={onSelect} onDeleteFile={onDeleteFile} onRenameFile={onRenameFile} />
  }
  return (
    <FileNode
      node={node}
      depth={depth}
      active={activeFilePath === node.fullPath}
      onSelect={onSelect}
      onDelete={onDeleteFile}
      onRename={onRenameFile}
    />
  )
}

function FolderNode({
  node,
  depth,
  activeFilePath,
  onSelect,
  onDeleteFile,
  onRenameFile,
}: {
  node: TreeNode
  depth: number
  activeFilePath: string | null
  onSelect: (path: string) => void
  onDeleteFile: (path: string) => void
  onRenameFile?: (path: string) => void
}) {
  const [open, setOpen] = useState(true)

  return (
    <div className="mb-0.5">
      <div className="px-2">
        <div
          onClick={() => setOpen(!open)}
          className="flex items-center gap-2 px-2 py-1.5 text-[12px] cursor-pointer text-foreground/80 hover:bg-muted transition-all rounded-lg font-bold group"
          style={{ paddingLeft: `${depth * 16 + 8}px` }}
        >
          {open ? (
            <FolderOpen size={14} className="text-yellow-500/80 transition-transform duration-300 group-hover:scale-110" />
          ) : (
            <FolderClosed size={14} className="text-yellow-500/80 transition-transform duration-300 group-hover:scale-110" />
          )}
          <span>{node.name}</span>
        </div>
      </div>
      {open &&
        node.children.map((child) => (
          <TreeNodeView
            key={child.fullPath}
            node={child}
            depth={depth + 1}
            activeFilePath={activeFilePath}
            onSelect={onSelect}
            onDeleteFile={onDeleteFile}
            onRenameFile={onRenameFile}
          />
        ))}
    </div>
  )
}

function FileNode({
  node,
  depth,
  active,
  onSelect,
  onDelete,
  onRename,
}: {
  node: TreeNode
  depth: number
  active: boolean
  onSelect: (path: string) => void
  onDelete: (path: string) => void
  onRename?: (path: string) => void
}) {
  return (
    <div className="px-2 w-full mb-0.5">
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <div
            className={cn(
              "flex items-center gap-2 px-2 py-1.5 text-[12px] cursor-pointer transition-all duration-200 rounded-lg group relative",
              active ? "bg-primary/10 text-primary font-bold shadow-sm" : "text-muted-foreground hover:bg-muted hover:text-foreground",
            )}
            style={{ paddingLeft: `${depth * 16 + 8}px` }}
            onClick={() => onSelect(node.fullPath)}
          >
            <span className="shrink-0 transition-transform group-hover:scale-110">
              {getFileIcon(node.name)}
            </span>
            <span className="truncate flex-1">{node.name}</span>
            {active && <div className="absolute left-0 top-1.5 bottom-1.5 w-0.5 bg-primary rounded-full" />}
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent className="w-40">
          <ContextMenuItem onClick={() => onSelect(node.fullPath)}>
            <Pencil size={14} className="mr-2" />
            打开
          </ContextMenuItem>
          {onRename && (
            <ContextMenuItem onClick={() => onRename(node.fullPath)}>
              <Pencil size={14} className="mr-2" />
              重命名
            </ContextMenuItem>
          )}
          <ContextMenuSeparator />
          <ContextMenuItem
            className="text-destructive focus:text-destructive"
            onClick={() => onDelete(node.fullPath)}
          >
            <Trash2 size={14} className="mr-2" />
            删除
          </ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>
    </div>
  )
}
