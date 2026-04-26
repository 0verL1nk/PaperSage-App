/** Uploading page — reference document management with on-demand preview. */

import { useState, useCallback } from 'react'
import { useParams } from '@tanstack/react-router'
import * as Resizable from 'react-resizable-panels'
import { Upload, FolderPlus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useUploadStore } from '@/store/upload-store'
import { useDocuments, useDeleteDocument } from '@/services/files-api'
import { useProjectFolders } from '@/services/folders-api'
import type { ProjectDocument } from '@/types/files'
import { PANEL_SIZES } from './constants'
import { DataTable } from './DataTable'
import { FilterBar } from './FilterBar'
import { BatchActionsBar } from './BatchActionsBar'
import { UploadDropzone } from './UploadDropzone'
import { DocumentPreviewPanel } from './DocumentPreviewPanel'
import { MoveDocumentDialog } from './MoveDocumentDialog'
import { EditRemarkDialog } from './EditRemarkDialog'
import { CreateFolderDialog } from './CreateFolderDialog'

export function UploadingPage() {
  const { projectId = '' } = useParams({ strict: false })

  const [typeFilter, setTypeFilter] = useState('all')
  const [searchQuery, setSearchQuery] = useState('')

  const { selectedFolderId, selectedFileIds, setSelectedFolder, clearSelection } = useUploadStore()

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

  const filteredDocs = documents.filter((doc) => {
    if (typeFilter !== 'all' && doc.type !== typeFilter) return false
    return true
  })

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

  const actions = { onMove: handleMove, onRemark: handleRemark, onDelete: handleDelete, onPreview: handlePreview }

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
        <DataTable data={filteredDocs} actions={actions} globalFilter={searchQuery} />
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

  return (
    <div className="flex h-full w-full bg-background overflow-hidden">
      {previewDoc ? (
        <Resizable.Group orientation="horizontal" id="uploading-layout">
          <Resizable.Panel defaultSize={PANEL_SIZES.left} minSize={PANEL_SIZES.minLeft}>
            {tableContent}
          </Resizable.Panel>

          <Resizable.Separator className="w-px bg-border/40 hover:bg-accent/30 transition-colors" />

          <Resizable.Panel defaultSize={PANEL_SIZES.right} minSize={PANEL_SIZES.minRight}>
            <aside className="h-full border-l border-border/30 bg-muted/10">
              <DocumentPreviewPanel document={previewDoc} onClose={handleClosePreview} />
            </aside>
          </Resizable.Panel>
        </Resizable.Group>
      ) : (
        tableContent
      )}

      {uploadOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 backdrop-blur-sm" onClick={() => setUploadOpen(false)}>
          <div className="w-full max-w-md p-6 bg-background rounded-2xl border border-border/40 shadow-strong" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-sm font-semibold mb-4">上传文档</h3>
            <UploadDropzone projectId={projectId} folderId={selectedFolderId} />
            <div className="mt-4 flex justify-end">
              <Button variant="ghost" size="sm" onClick={() => setUploadOpen(false)} className="text-xs">
                关闭
              </Button>
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
