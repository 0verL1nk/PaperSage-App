/** Folder tree — hierarchical folder list for the uploading page. */

import { useMemo, useCallback } from 'react'
import { Folder, FolderOpen, FolderPlus, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useUploadStore } from '@/store/upload-store'
import { useDeleteFolder } from '@/services/folders-api'
import type { Folder as FolderType } from '@/types/files'

interface FolderTreeProps {
  projectId: string
  folders: FolderType[]
  onCreateFolder: () => void
}

/** Build a tree from a flat folder list using parentId. */
function buildTree(folders: FolderType[]): FolderType[] {
  const map = new Map<string, FolderType>()
  const roots: FolderType[] = []

  for (const f of folders) {
    map.set(f.id, { ...f, children: [] })
  }

  for (const f of folders) {
    const node = map.get(f.id)!
    if (f.parentId && map.has(f.parentId)) {
      map.get(f.parentId)!.children!.push(node)
    } else {
      roots.push(node)
    }
  }

  return roots
}

function FolderTreeNode({
  folder,
  depth,
  selectedFolderId,
  onSelect,
  onDelete,
}: {
  folder: FolderType
  depth: number
  selectedFolderId: string | null
  onSelect: (id: string | null) => void
  onDelete: (id: string) => void
}) {
  const isSelected = folder.id === selectedFolderId
  const hasChildren = folder.children && folder.children.length > 0

  return (
    <>
      <button
        type="button"
        className={`
          group w-full flex items-center gap-1.5 px-2 py-1.5 rounded-md text-left text-[12px]
          transition-colors cursor-pointer
          ${isSelected
            ? 'bg-primary/10 text-primary font-medium'
            : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground'
          }
        `}
        style={{ paddingLeft: `${depth * 16 + 8}px` }}
        onClick={() => onSelect(folder.id)}
      >
        {isSelected
          ? <FolderOpen size={13} className="shrink-0 text-primary" />
          : <Folder size={13} className="shrink-0" />
        }
        <span className="truncate flex-1">{folder.name}</span>
        <span
          role="button"
          tabIndex={0}
          className="opacity-0 group-hover:opacity-100 p-0.5 rounded hover:bg-destructive/10 hover:text-destructive transition-opacity"
          onClick={(e) => { e.stopPropagation(); onDelete(folder.id) }}
          onKeyDown={(e) => { if (e.key === 'Enter') { e.stopPropagation(); onDelete(folder.id) } }}
        >
          <Trash2 size={11} />
        </span>
      </button>

      {hasChildren && folder.children!.map((child) => (
        <FolderTreeNode
          key={child.id}
          folder={child}
          depth={depth + 1}
          selectedFolderId={selectedFolderId}
          onSelect={onSelect}
          onDelete={onDelete}
        />
      ))}
    </>
  )
}

export function FolderTree({ projectId, folders, onCreateFolder }: FolderTreeProps) {
  const { selectedFolderId, setSelectedFolder } = useUploadStore()
  const deleteFolderMutation = useDeleteFolder(projectId)

  const tree = useMemo(() => buildTree(folders), [folders])

  const handleDelete = useCallback((folderId: string) => {
    deleteFolderMutation.mutate(folderId)
    if (selectedFolderId === folderId) {
      setSelectedFolder(null)
    }
  }, [deleteFolderMutation, selectedFolderId, setSelectedFolder])

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-border/40">
        <span className="text-[11px] font-bold text-muted-foreground uppercase tracking-widest">
          文件夹
        </span>
        <Button
          variant="ghost"
          size="icon-xs"
          className="h-6 w-6 rounded-md"
          title="新建文件夹"
          onClick={onCreateFolder}
        >
          <FolderPlus size={13} />
        </Button>
      </div>

      {/* All documents option */}
      <div className="flex-1 overflow-y-auto py-1">
        <button
          type="button"
          className={`
            w-full flex items-center gap-1.5 px-3 py-1.5 rounded-md text-left text-[12px]
            transition-colors cursor-pointer
            ${selectedFolderId === null
              ? 'bg-primary/10 text-primary font-medium'
              : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground'
            }
          `}
          onClick={() => setSelectedFolder(null)}
        >
          <FolderOpen size={13} className="shrink-0" />
          <span className="truncate">全部文档</span>
        </button>

        {tree.map((folder) => (
          <FolderTreeNode
            key={folder.id}
            folder={folder}
            depth={0}
            selectedFolderId={selectedFolderId}
            onSelect={setSelectedFolder}
            onDelete={handleDelete}
          />
        ))}

        {tree.length === 0 && (
          <p className="text-[11px] text-muted-foreground/60 px-3 py-4 text-center">
            暂无文件夹
          </p>
        )}
      </div>
    </div>
  )
}
