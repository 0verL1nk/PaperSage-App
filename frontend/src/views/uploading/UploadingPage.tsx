/** Uploading page — reference document management with on-demand preview. */

import { useState, useCallback, useEffect } from 'react'
import { useParams } from '@tanstack/react-router'
import * as Resizable from 'react-resizable-panels'
import { Upload, FolderPlus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useUploadStore } from '@/store/upload-store'
import { useDocuments, useDeleteDocument } from '@/services/files-api'
import { useProjectFolders } from '@/services/folders-api'
import type { ProjectDocument } from '@/types/files'
import { PANEL_SIZES, MOBILE_BREAKPOINT } from './constants'
import { FolderTree } from './FolderTree'
import { DataTable } from './DataTable'
import { FilterBar } from './FilterBar'
import { BatchActionsBar } from './BatchActionsBar'
import { UploadDropzone } from './UploadDropzone'
import { DocumentPreviewPanel } from './DocumentPreviewPanel'
import { MoveDocumentDialog } from './MoveDocumentDialog'
import { EditRemarkDialog } from './EditRemarkDialog'
import { CreateFolderDialog } from './CreateFolderDialog'

/** Hook to detect mobile viewport. */
function useIsMobile() {
  const [isMobile, setIsMobile] = useState(
    typeof window !== 'undefined' ? window.innerWidth < MOBILE_BREAKPOINT : false,
  )
  useEffect(() => {
    const handler = () => setIsMobile(window.innerWidth < MOBILE_BREAKPOINT)
    window.addEventListener('resize', handler)
    return () => window.removeEventListener('resize', handler)
  }, [])
  return isMobile
}

export function UploadingPage() {
  const { projectId = '' } = useParams({ strict: false })

  const [typeFilter, setTypeFilter] = useState('all')
  const [searchQuery, setSearchQuery] = useState('')

  const {
    selectedFolderId, selectedFileIds,
    setSelectedFolder, clearSelection, selectAllFiles,
  } = useUploadStore()

  const [previewDoc, setPreviewDoc] = useState<ProjectDocument | null>(null)

  const [uploadOpen, setUploadOpen] = useState(false)
  const [moveOpen, setMoveOpen] = useState(false)
  const [remarkOpen, setRemarkOpen] = useState(false)
  const [createFolderOpen, setCreateFolderOpen] = useState(false)
  const [moveDocIds, setMoveDocIds] = useState<string[]>([])
  const [remarkDocs, setRemarkDocs] = useState<ProjectDocument[]>([])

  const { data: documents = [] } = useDocuments(projectId, selectedFolderId)
  const { data: folders = [] } = useProjectFolders(projectId)
  const deleteMutation = useDeleteDocument(projectId, selectedFolderId)

  const normalizedSearch = searchQuery.trim().toLowerCase()
  const filteredDocs = documents.filter((doc) => {
    if (typeFilter !== 'all' && doc.type !== typeFilter) return false
    if (!normalizedSearch) return true
    return doc.name.toLowerCase().includes(normalizedSearch)
      || (doc.remark ?? '').toLowerCase().includes(normalizedSearch)
  })

  const clearFilters = useCallback(() => {
    setTypeFilter('all')
    setSearchQuery('')
  }, [])

  // 筛选后已不在结果中的文档不再占据预览区
  const visiblePreviewDoc = previewDoc && filteredDocs.some((d) => d.id === previewDoc.id)
    ? previewDoc
    : null

  const handlePreview = useCallback((doc: ProjectDocument) => {
    setPreviewDoc(doc)
  }, [])

  const handleClosePreview = useCallback(() => {
    setPreviewDoc(null)
  }, [])

  const openMoveDialog = useCallback((ids: string[]) => {
    setMoveDocIds(ids)
    setMoveOpen(true)
  }, [])

  const handleMove = useCallback((doc: ProjectDocument) => {
    openMoveDialog([doc.id])
  }, [openMoveDialog])

  const openRemarkDialog = useCallback((docs: ProjectDocument[]) => {
    setRemarkDocs(docs)
    setRemarkOpen(true)
  }, [])

  const handleRemark = useCallback((doc: ProjectDocument) => {
    openRemarkDialog([doc])
  }, [openRemarkDialog])

  const handleDelete = useCallback((doc: ProjectDocument) => {
    deleteMutation.mutate(doc.id)
    if (previewDoc?.id === doc.id) setPreviewDoc(null)
  }, [deleteMutation, previewDoc])

  const handleBatchDelete = useCallback(() => {
    for (const id of selectedFileIds) {
      deleteMutation.mutate(id)
    }
    clearSelection()
  }, [selectedFileIds, deleteMutation, clearSelection])

  const handleBatchMove = useCallback(() => {
    openMoveDialog(selectedFileIds)
  }, [selectedFileIds, openMoveDialog])

  const handleBatchRemark = useCallback(() => {
    const selected = documents.filter((d) => selectedFileIds.includes(d.id))
    openRemarkDialog(selected)
  }, [selectedFileIds, documents, openRemarkDialog])

  /** Sync DataTable row selection changes to Zustand store. */
  const handleRowSelectionChange = useCallback(
    (selectedIds: string[]) => selectAllFiles(selectedIds),
    [selectAllFiles],
  )

  const actions = {
    onMove: handleMove,
    onRemark: handleRemark,
    onDelete: handleDelete,
    onPreview: handlePreview,
  }

  const folderTreePanel = (
    <aside className="h-full border-r border-border/30 bg-muted/5">
      <FolderTree
        projectId={projectId}
        folders={folders}
        onCreateFolder={() => setCreateFolderOpen(true)}
      />
    </aside>
  )

  const tableContent = (
    <div className="flex flex-col h-full w-full">
      {/* Header */}
      <div className="flex items-center justify-between px-5 py-3 border-b border-border/40 backdrop-blur-xl bg-background/80">
        <h2 className="text-sm font-semibold text-foreground/90">参考文献</h2>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="ghost" className="h-7 text-xs gap-1.5 text-amber-700 dark:text-amber-400" onClick={() => setCreateFolderOpen(true)}>
            <FolderPlus size={13} /> 文件夹
          </Button>
          <Button size="sm" className="h-7 text-xs gap-1.5 bg-accent hover:bg-accent/90 text-accent-foreground rounded-full" onClick={() => setUploadOpen(true)}>
            <Upload size={13} /> 上传
          </Button>
        </div>
      </div>

      <FilterBar
        typeFilter={typeFilter}
        onTypeFilterChange={setTypeFilter}
        folderId={selectedFolderId}
        folders={folders}
        onFolderChange={setSelectedFolder}
        searchQuery={searchQuery}
        onSearchChange={setSearchQuery}
      />

      <div className="flex-1 min-h-0">
        <DataTable
          data={filteredDocs}
          actions={actions}
          totalCount={documents.length}
          onClearFilters={clearFilters}
          onRowSelectionChange={handleRowSelectionChange}
        />
      </div>

      <BatchActionsBar
        selectedCount={selectedFileIds.length}
        onBatchMove={handleBatchMove}
        onBatchRemark={handleBatchRemark}
        onBatchDelete={handleBatchDelete}
        onClearSelection={clearSelection}
      />
    </div>
  )

  const previewPanel = visiblePreviewDoc ? (
    <aside className="h-full border-l border-border/30 bg-muted/10">
      <DocumentPreviewPanel document={visiblePreviewDoc} onClose={handleClosePreview} />
    </aside>
  ) : null

  const isMobile = useIsMobile()

  return (
    <div className="flex h-full w-full bg-background overflow-hidden">
      {isMobile ? (
        /* Mobile: stacked layout */
        <div className="flex flex-col h-full w-full">
          <div className="h-48 shrink-0 border-b border-border/30 overflow-hidden">{folderTreePanel}</div>
          <div className="flex-1 min-h-0">{tableContent}</div>
          {previewPanel && <div className="h-96 shrink-0 border-t border-border/30">{previewPanel}</div>}
        </div>
      ) : previewPanel ? (
        /* Desktop with preview: three-pane resizable */
        <Resizable.Group orientation="horizontal" id="uploading-layout-3">
          <Resizable.Panel defaultSize={PANEL_SIZES.folder} minSize={PANEL_SIZES.minFolder}>
            {folderTreePanel}
          </Resizable.Panel>
          <Resizable.Separator className="w-px bg-border/40 hover:bg-accent/30 transition-colors" />
          <Resizable.Panel defaultSize={PANEL_SIZES.table} minSize={PANEL_SIZES.minTable}>
            {tableContent}
          </Resizable.Panel>
          <Resizable.Separator className="w-px bg-border/40 hover:bg-accent/30 transition-colors" />
          <Resizable.Panel defaultSize={PANEL_SIZES.preview} minSize={PANEL_SIZES.minPreview}>
            {previewPanel}
          </Resizable.Panel>
        </Resizable.Group>
      ) : (
        /* Desktop without preview: two-pane (folder tree + table) */
        <Resizable.Group orientation="horizontal" id="uploading-layout-2">
          <Resizable.Panel defaultSize={PANEL_SIZES.folder} minSize={PANEL_SIZES.minFolder}>
            {folderTreePanel}
          </Resizable.Panel>
          <Resizable.Separator className="w-px bg-border/40 hover:bg-accent/30 transition-colors" />
          <Resizable.Panel defaultSize={100 - PANEL_SIZES.folder} minSize={PANEL_SIZES.minTable}>
            {tableContent}
          </Resizable.Panel>
        </Resizable.Group>
      )}

      {uploadOpen && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 backdrop-blur-sm"
          onClick={() => {
            // 上传进行中不允许误触关闭（丢进度）；完成后任意处点击可关
            if (!useUploadStore.getState().isUploading) setUploadOpen(false)
          }}
        >
          <div
            className="w-full max-w-md bg-background rounded-2xl border border-border/40 shadow-strong overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between px-5 py-4 border-b border-border/40">
              <h3 className="text-sm font-semibold">上传文档</h3>
              <button
                type="button"
                onClick={() => setUploadOpen(false)}
                className="text-muted-foreground/60 hover:text-foreground transition-colors p-1 rounded-lg hover:bg-muted/50"
                aria-label="关闭上传对话框"
              >
                <X size={16} />
              </button>
            </div>
            <div className="px-5 py-4">
              <UploadDropzone projectId={projectId} folderId={selectedFolderId} onClose={() => setUploadOpen(false)} />
            </div>
          </div>
        </div>
      )}

      <MoveDocumentDialog
        open={moveOpen}
        onOpenChange={setMoveOpen}
        projectId={projectId}
        currentFolderId={selectedFolderId}
        documentIds={moveDocIds}
        folders={folders}
      />
      <EditRemarkDialog
        open={remarkOpen}
        onOpenChange={setRemarkOpen}
        projectId={projectId}
        folderId={selectedFolderId}
        documents={remarkDocs}
      />
      <CreateFolderDialog
        open={createFolderOpen}
        onOpenChange={setCreateFolderOpen}
        projectId={projectId}
        parentId={selectedFolderId}
      />
    </div>
  )
}
