/**
 * Upload dropzone — 对齐 Google Drive 上传面板的交互结构：
 *
 * - 空态：大拖拽区（视觉引导）
 * - 有文件后：拖拽区收缩为一条「继续添加」窄带，文件列表成为主体
 *   （双行：文件名 + 大小·状态；上传中的行有细进度条）
 * - 页脚：左侧汇总（就绪/上传中/重复），右侧「取消」+「添加 N 个」
 * - 添加成功后自动关闭；关闭即放弃（服务端暂存行随卸载清理）
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import { useDropzone } from 'react-dropzone'
import { Upload, Loader2, Check, X, AlertTriangle, RotateCw, Plus } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { useUploadDocument, useDeleteDocument, useConfirmDocument } from '@/services/files-api'
import { useUploadStore } from '@/store/upload-store'
import { ACCEPTED_FILE_TYPES, MAX_FILE_SIZE_BYTES } from './constants'
import { FileTypeIcon } from '../projects/file-tree-icons'

interface UploadDropzoneProps {
  projectId: string
  folderId: string | null
  onClose: () => void
}

type ItemStatus = 'uploading' | 'staged' | 'error'

interface UploadItem {
  id: number
  file: File
  status: ItemStatus
  error?: string
  duplicate?: boolean
  documentId?: string
}

function fmtSize(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} KB`
  return `${bytes} B`
}

export function UploadDropzone({ projectId, folderId, onClose }: UploadDropzoneProps) {
  const [items, setItems] = useState<UploadItem[]>([])
  const idRef = useRef(0)
  const uploadMutation = useUploadDocument(projectId, folderId)
  const deleteMutation = useDeleteDocument(projectId, folderId)
  const confirmMutation = useConfirmDocument(projectId, folderId)
  const { setUploading } = useUploadStore()
  const progressMap = useUploadStore((s) => s.uploadProgress)

  const uploadFile = useCallback(
    async (file: File) => {
      try {
        const doc = await uploadMutation.mutateAsync({
          file,
          staged: true,
          onUploadProgress: (e) => {
            if (e.total) {
              useUploadStore.getState().setProgress(file.name, Math.round((e.loaded / e.total) * 100))
            }
          },
        })
        setItems((prev) =>
          prev.map((u) =>
            u.file === file && u.status === 'uploading'
              ? { ...u, status: 'staged', documentId: doc?.id, duplicate: doc?.duplicate === true }
              : u,
          ),
        )
      } catch (err) {
        setItems((prev) =>
          prev.map((u) =>
            u.file === file && u.status === 'uploading'
              ? { ...u, status: 'error', error: err instanceof Error ? err.message : '上传失败' }
              : u,
          ),
        )
      }
    },
    [uploadMutation],
  )

  const onDrop = useCallback(
    (acceptedFiles: File[]) => {
      const batch = acceptedFiles.map((file) => ({ id: ++idRef.current, file, status: 'uploading' as const }))
      setItems((prev) => [...prev, ...batch])
      void (async () => {
        setUploading(true)
        for (const b of batch) {
          await uploadFile(b.file)
        }
        setUploading(false)
      })()
    },
    [setUploading, uploadFile],
  )

  const retry = useCallback(
    (file: File) => {
      setItems((prev) => prev.map((u) => (u.file === file ? { ...u, status: 'uploading', error: undefined } : u)))
      useUploadStore.getState().setProgress(file.name, 0)
      void (async () => {
        setUploading(true)
        await uploadFile(file)
        setUploading(false)
      })()
    },
    [setUploading, uploadFile],
  )

  // 丢弃：staged 行删除服务端暂存；error 行仅移出列表
  const discard = useCallback(
    (item: UploadItem) => {
      if (item.status === 'staged' && item.documentId) {
        deleteMutation.mutate(item.documentId)
      }
      setItems((prev) => prev.filter((u) => u.id !== item.id))
    },
    [deleteMutation],
  )

  // 添加：所有已就绪行入库；全部成功则自动关闭（Drive 行为）。
  // 已入库 id 记入 committedIdsRef：commitAll 末尾同步调用 onClose，
  // React 尚未重渲染、stagedIdsRef 还是旧值——卸载清理若不排除
  // 已入库行，会把刚 confirm 的文件当暂存删掉（实测丢过第 6 个文件）
  const committedIdsRef = useRef<Set<string>>(new Set())
  const commitAll = useCallback(async () => {
    const staged = items.filter((u) => u.status === 'staged' && u.documentId)
    if (!staged.length) return
    let failed = 0
    setUploading(true)
    for (const u of staged) {
      try {
        await confirmMutation.mutateAsync(u.documentId!)
        committedIdsRef.current.add(u.documentId!)
        setItems((prev) => prev.filter((x) => x.id !== u.id))
      } catch {
        failed++
      }
    }
    setUploading(false)
    if (failed === 0) onClose()
  }, [items, setUploading, confirmMutation, onClose])

  useEffect(
    () => () =>
      discardByIdsRef.current(
        stagedIdsRef.current.filter((id) => !committedIdsRef.current.has(id)),
      ),
    [],
  )

  // 关闭即放弃：卸载时清理尚未「添加」的暂存行（ref 持最新集合防闭包过期）
  const stagedIdsRef = useRef<string[]>([])
  stagedIdsRef.current = items.filter((u) => u.status === 'staged' && u.documentId).map((u) => u.documentId!)
  const discardByIdsRef = useRef<(ids: string[]) => void>(() => {})
  discardByIdsRef.current = (ids) => {
    for (const id of ids) deleteMutation.mutate(id)
  }
  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    onDrop,
    accept: ACCEPTED_FILE_TYPES,
    maxSize: MAX_FILE_SIZE_BYTES,
    multiple: true,
  })

  const stagedItems = items.filter((u) => u.status === 'staged')
  const uploadingCount = items.filter((u) => u.status === 'uploading').length
  const errorCount = items.filter((u) => u.status === 'error').length
  const dupCount = stagedItems.filter((u) => u.duplicate).length
  const hasItems = items.length > 0

  // 页脚汇总：就绪 / 上传中 / 失败
  const summary: string[] = []
  if (stagedItems.length > 0) summary.push(`${stagedItems.length} 个已就绪`)
  if (uploadingCount > 0) summary.push(`${uploadingCount} 个上传中`)
  if (errorCount > 0) summary.push(`${errorCount} 个失败`)
  if (dupCount > 0) summary.push(`${dupCount} 个疑似重复`)

  return (
    <div className="flex flex-col">
      {/* 拖拽区：空态大引导，有文件后收缩为窄带 */}
      <div
        {...getRootProps()}
        className={cn(
          'rounded-xl text-center cursor-pointer border-2 border-dashed transition-all',
          isDragActive ? 'border-accent bg-accent/5' : 'border-accent/20 hover:border-accent/40 hover:bg-accent/5',
          hasItems ? 'py-3' : 'py-10',
        )}
      >
        <input {...getInputProps()} />
        {hasItems ? (
          <p className="text-xs text-muted-foreground flex items-center justify-center gap-1.5">
            <Plus size={13} />继续拖拽或点击添加文件
          </p>
        ) : (
          <div>
            <div className="w-12 h-12 rounded-2xl bg-accent/10 flex items-center justify-center mx-auto mb-3">
              <Upload size={22} className="text-accent-foreground/70" />
            </div>
            <p className="text-sm text-muted-foreground">
              {isDragActive ? '松开以上传文件...' : '拖拽文件到此处，或点击选择'}
            </p>
            <p className="text-[10px] text-muted-foreground/50 mt-1.5">
              支持 PDF、DOCX、TXT — 最大 50 MB
            </p>
          </div>
        )}
      </div>

      {/* 文件列表（主体） */}
      {hasItems && (
        <div className="mt-4 -mx-1 max-h-[280px] overflow-y-auto px-1 divide-y divide-border/40">
          {items.map((u) => {
            const pct = u.status === 'uploading' ? (progressMap.get(u.file.name) ?? 0) : 0
            return (
              <div key={u.id} className="group flex items-center gap-3 py-2.5">
                <FileTypeIcon name={u.file.name} />
                <div className="flex-1 min-w-0">
                  <p className="text-[13px] font-medium truncate leading-tight">{u.file.name}</p>
                  <p className="text-[11px] text-muted-foreground/80 mt-0.5 flex items-center gap-1.5 min-w-0">
                    <span className="flex-none">{fmtSize(u.file.size)}</span>
                    {u.status === 'staged' && u.duplicate && (
                      <span
                        className="text-amber-600 flex items-center gap-0.5 flex-none"
                        title="项目中已有同名且同大小的文件"
                      >
                        <AlertTriangle size={10} />与已有文件重复
                      </span>
                    )}
                    {u.status === 'error' && (
                      <span className="text-destructive truncate" title={u.error}>
                        · {u.error ?? '上传失败'}
                      </span>
                    )}
                  </p>
                  {u.status === 'uploading' && (
                    <div className="mt-1.5 h-[3px] rounded-full bg-muted overflow-hidden">
                      <div
                        className="h-full bg-accent rounded-full transition-[width] duration-150"
                        style={{ width: `${pct}%` }}
                      />
                    </div>
                  )}
                </div>
                <div className="flex items-center gap-1.5 flex-none">
                  {u.status === 'uploading' && (
                    <Loader2 size={14} className="animate-spin text-accent-foreground" />
                  )}
                  {u.status === 'staged' && (
                    <span className="w-5 h-5 rounded-full bg-green-500/10 flex items-center justify-center">
                      <Check size={12} className="text-green-600" />
                    </span>
                  )}
                  {u.status === 'error' && (
                    <button
                      type="button"
                      onClick={() => retry(u.file)}
                      className="flex items-center gap-1 px-2 h-6 rounded-lg text-[11px] text-accent-foreground hover:bg-accent/15 transition-colors"
                      aria-label={`重试上传 ${u.file.name}`}
                    >
                      <RotateCw size={11} />重试
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => discard(u)}
                    className="opacity-0 group-hover:opacity-100 text-muted-foreground/50 hover:text-foreground transition-all p-1"
                    aria-label={u.status === 'staged' ? `丢弃 ${u.file.name}` : `移除 ${u.file.name}`}
                  >
                    <X size={13} />
                  </button>
                </div>
              </div>
            )
          })}
        </div>
      )}

      {/* 页脚：汇总 + 操作 */}
      {hasItems && (
        <div className="mt-4 pt-3 border-t border-border/40 flex items-center justify-between gap-3">
          <span className="text-[11px] text-muted-foreground/80 truncate">
            {summary.join(' · ') || '全部完成'}
          </span>
          <div className="flex items-center gap-2 flex-none">
            <Button
              variant="ghost"
              size="sm"
              onClick={onClose}
              disabled={confirmMutation.isPending}
              className="h-8 text-xs rounded-lg"
            >
              取消
            </Button>
            <Button
              size="sm"
              onClick={commitAll}
              disabled={stagedItems.length === 0 || uploadingCount > 0 || confirmMutation.isPending}
              className="h-8 text-xs gap-1.5 bg-accent hover:bg-accent/90 text-accent-foreground rounded-lg px-4"
            >
              <Plus size={13} />
              {confirmMutation.isPending
                ? '添加中...'
                : stagedItems.length > 0
                  ? `添加 ${stagedItems.length} 个`
                  : '添加'}
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
