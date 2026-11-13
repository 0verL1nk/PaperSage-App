import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { FolderOpen, FolderSync, RefreshCw, Unlink } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { getDesktopService } from '@/lib/desktop'
import {
  base64ToText,
  flushPendingWrites,
  folderSync,
  textToBase64,
  type SyncBindingInfo,
  type SyncChange,
} from '@/lib/folder-sync'
import { useQueryClient } from '@tanstack/react-query'
import { createLatexFile, fetchLatexFileContent, saveLatexFile, type LatexFileItem } from '@/services/latex-api'
import { latexKeys } from '@/lib/query-keys'
import { fileSyncStatus, useFolderSyncStore, type FileSyncStatus } from '@/store/folder-sync-store'

const SYNC_TEXT_EXTENSIONS = new Set(['tex', 'bib', 'cls', 'sty', 'bst', 'def', 'ist', 'txt', 'md', 'cfg', 'fd'])

const KIND_LABEL: Record<SyncChange['kind'], string> = {
  created: '本机新增',
  modified: '本机修改',
  deleted: '本机删除',
  renamed: '本机重命名',
}

const STATUS_LABEL: Record<FileSyncStatus, string> = {
  synced: '已同步',
  paused: '已暂停',
  conflict: '冲突',
  pending: '云端改动',
  created: '本机新增',
  modified: '本机修改',
  deleted: '本机删除',
  renamed: '本机重命名',
}

const STATUS_DOT: Record<FileSyncStatus, string> = {
  synced: 'bg-muted-foreground/25',
  paused: 'bg-muted-foreground/40',
  conflict: 'bg-red-500',
  pending: 'bg-sky-500',
  created: 'bg-emerald-500',
  modified: 'bg-amber-500',
  deleted: 'bg-amber-500',
  renamed: 'bg-amber-500',
}

export function FolderSyncDialog({
  open,
  onOpenChange,
  projectId,
  files,
  onApplied,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  projectId: string
  files: LatexFileItem[]
  onApplied: () => void
}) {
  const [info, setInfo] = useState<SyncBindingInfo | null>(null)
  const [changes, setChanges] = useState<SyncChange[]>([])
  const [busy, setBusy] = useState(false)
  const [comparing, setComparing] = useState(false)
  const applyStore = useFolderSyncStore()
  const storeProjects = useFolderSyncStore((state) => state.projectId)
  const storeChanges = useFolderSyncStore((state) => state.changes)
  const storeConflicts = useFolderSyncStore((state) => state.conflicts)
  const storePending = useFolderSyncStore((state) => state.pending)
  const applyingRef = useRef(false)
  const filesRef = useRef(files)
  filesRef.current = files
  const queryClient = useQueryClient()

  const refresh = async () => {
    const next = await folderSync.info(projectId)
    setInfo(next)
    applyStore.setBinding(projectId, next.bound, next.paused)
    if (next.bound && !next.paused) {
      try {
        const scanned = await folderSync.scan(projectId) ?? []
        setChanges(scanned)
        applyStore.setChanges(projectId, scanned)
      } catch {
        setChanges([])
        applyStore.setChanges(projectId, [])
      }
    } else {
      setChanges([])
      applyStore.setChanges(projectId, [])
    }
  }

  // Editor saves made while the binding was paused leave the app ahead of the
  // disk. Replay the queue in order so "cloud change" is transient.
  const retryPending = async () => {
    if ((useFolderSyncStore.getState().pending[projectId]?.length ?? 0) === 0) return
    const written = await flushPendingWrites(projectId, async (path) => {
      const content = await fetchLatexFileContent(projectId, path, { skipGlobalLoading: true })
      return content.content
    })
    if (written > 0) toast.success(`已把 ${written} 个项目改动写入本机`)
  }

  const applyDiskChanges = async (list: SyncChange[]) => {
    if (applyingRef.current) return
    const applicable = list.filter((change) => change.kind !== 'deleted')
    if (applicable.length === 0) {
      await refresh()
      return
    }
    applyingRef.current = true
    setBusy(true)
    let applied = 0
    let failed = 0
    try {
      for (const change of applicable) {
        const data = await folderSync.read(projectId, change.path)
        if (!data) {
          failed += 1
          continue
        }
        const content = base64ToText(data)
        try {
          if (filesRef.current.some((file) => file.file_path === change.path)) {
            await saveLatexFile(projectId, change.path, content, { skipGlobalLoading: true })
          } else {
            await createLatexFile(projectId, change.path, content)
          }
          await folderSync.markSynced(projectId, change.path)
          if (change.kind === 'renamed' && change.oldPath) {
            await folderSync.markSynced(projectId, change.oldPath)
          }
          void queryClient.invalidateQueries({ queryKey: latexKeys.content(projectId, change.path) })
          applied += 1
        } catch {
          failed += 1
        }
      }
    } finally {
      applyingRef.current = false
      setBusy(false)
    }
    if (applied > 0) onApplied()
    if (applied > 0 || failed > 0) {
      toast[applied > 0 ? 'success' : 'error'](
        `本机改动已同步 ${applied} 个文件${failed > 0 ? `，${failed} 个失败` : ''}`,
      )
    }
    await refresh()
  }

  useEffect(() => {
    if (!open) return
    void refresh().then(() => retryPending())
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, projectId])

  useEffect(() => {
    if (!open || !info?.bound || info.paused) return
    return folderSync.onChanges((changedProject, nextChanges) => {
      if (changedProject !== projectId) return
      setChanges(nextChanges)
      void applyDiskChanges(nextChanges)
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, info?.bound, info?.paused, projectId])

  const bindFolder = async () => {
    const root = await getDesktopService()?.pickFolder('选择本地项目文件夹')
    if (!root) return
    setBusy(true)
    try {
      await folderSync.bind(projectId, root)
      await folderSync.watch(projectId)
      await refresh()
      toast.success('已绑定本地文件夹')
      const initial = await folderSync.scan(projectId) ?? []
      // Empty folder: write the project out first so the binding is a real
      // mirror instead of a one-way import.
      if (initial.length === 0 && filesRef.current.length > 0) {
        let exported = 0
        for (const file of filesRef.current) {
          const extension = file.file_path.split('.').pop()?.toLowerCase() ?? ''
          if (!SYNC_TEXT_EXTENSIONS.has(extension)) continue
          try {
            const content = await fetchLatexFileContent(projectId, file.file_path, { skipGlobalLoading: true })
            await folderSync.write(projectId, file.file_path, textToBase64(content.content))
            exported += 1
          } catch {
            // Skip unreadable files; the import pass below reports the rest.
          }
        }
        if (exported > 0) toast.success(`已导出 ${exported} 个文件到本地文件夹`)
      }
      await applyDiskChanges(initial)
    } catch (error) {
      toast.error('绑定失败', { description: error instanceof Error ? error.message : String(error) })
    } finally {
      setBusy(false)
    }
  }

  const unbind = async () => {
    setBusy(true)
    try {
      await folderSync.unwatch(projectId)
      await folderSync.unbind(projectId)
      setInfo(null)
      setChanges([])
      toast.success('已断开本地文件夹')
    } catch (error) {
      toast.error('断开失败', { description: error instanceof Error ? error.message : String(error) })
    } finally {
      setBusy(false)
    }
  }

  const togglePause = async () => {
    if (!info?.bound) return
    setBusy(true)
    try {
      await folderSync.setPaused(projectId, !info.paused)
      if (!info.paused) await folderSync.unwatch(projectId)
      else {
        await folderSync.watch(projectId)
        await retryPending()
      }
      await refresh()
    } catch (error) {
      toast.error('切换失败', { description: error instanceof Error ? error.message : String(error) })
    } finally {
      setBusy(false)
    }
  }

  const ignoreChange = async (change: SyncChange) => {
    try {
      await folderSync.markSynced(projectId, change.kind === 'renamed' && change.oldPath ? change.oldPath : change.path)
      await refresh()
    } catch (error) {
      toast.error('标记失败', { description: error instanceof Error ? error.message : String(error) })
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[calc(100vw-1rem)] rounded-2xl border border-border/40 bg-background p-5 shadow-strong sm:max-w-[520px] sm:p-6">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-xl font-semibold tracking-tight">
            <FolderSync className="size-5 text-primary" />
            本地目录同步
          </DialogTitle>
          <DialogDescription className="text-sm leading-5">
            把项目文件与本机文件夹双向同步。本机改动会自动导入项目；项目内的保存会写回文件夹，冲突时不会静默覆盖。
          </DialogDescription>
        </DialogHeader>

        {!info?.bound ? (
          <div className="rounded-xl border border-dashed border-border/60 p-5 text-center">
            <p className="text-sm text-muted-foreground">尚未绑定本地文件夹</p>
            <Button className="mt-4 gap-2 rounded-lg" onClick={() => void bindFolder()} disabled={busy}>
              <FolderOpen className="size-4" />
              选择文件夹
            </Button>
          </div>
        ) : (
          <div className="space-y-3">
            <div className="rounded-xl border border-border/50 bg-muted/30 p-3">
              <div className="flex items-center justify-between gap-3">
                <p className="truncate font-mono text-xs text-foreground" title={info.root}>{info.root}</p>
                <span className="shrink-0 text-[11px] text-muted-foreground">{info.files} 个已跟踪文件</span>
              </div>
              <div className="mt-2 flex flex-wrap gap-2">
                <Button
                  size="sm"
                  variant="outline"
                  className="h-8 gap-1.5"
                  disabled={busy}
                  onClick={() => {
                    void (async () => {
                      setBusy(true)
                      try {
                        await retryPending()
                        await applyDiskChanges(changes)
                      } finally {
                        setBusy(false)
                      }
                    })()
                  }}
                >
                  <RefreshCw className={busy ? 'size-3.5 animate-spin' : 'size-3.5'} />
                  立即同步
                </Button>
                <Button size="sm" variant="outline" className="h-8" disabled={busy} onClick={() => void togglePause()}>
                  {info.paused ? '恢复监听' : '暂停监听'}
                </Button>
                <Button size="sm" variant="ghost" className="h-8" disabled={busy} onClick={() => setComparing((value) => !value)}>
                  {comparing ? '收起比对' : '全量比对'}
                </Button>
                <Button size="sm" variant="ghost" className="h-8 gap-1.5 text-destructive hover:text-destructive" disabled={busy} onClick={() => void unbind()}>
                  <Unlink className="size-3.5" />
                  断开
                </Button>
              </div>
              {info.paused && (
                <p className="mt-2 text-[11px] text-muted-foreground">
                  监听已暂停：本机改动不会自动导入，项目内的保存也不会写回文件夹。
                </p>
              )}
            </div>

            {comparing ? (
              <ul className="max-h-56 space-y-1 overflow-y-auto">
                {files
                  .filter((file) => SYNC_TEXT_EXTENSIONS.has(file.file_path.split('.').pop()?.toLowerCase() ?? ''))
                  .map((file) => {
                    const status = fileSyncStatus(
                      {
                        projectId: storeProjects,
                        paused: info.paused,
                        changes: storeProjects === projectId ? storeChanges : {},
                        conflicts: storeProjects === projectId ? storeConflicts : {},
                        pending: storeProjects === projectId ? storePending : {},
                      },
                      file.file_path,
                    )
                    return (
                      <li key={file.file_path} className="flex items-center gap-2 rounded-lg border border-border/40 px-2.5 py-1.5 text-xs">
                        <span aria-hidden="true" className={`size-1.5 shrink-0 rounded-full ${STATUS_DOT[status]}`} />
                        <span className="min-w-0 flex-1 truncate" title={file.file_path}>{file.file_path}</span>
                        <span className="shrink-0 text-[11px] text-muted-foreground">{STATUS_LABEL[status]}</span>
                      </li>
                    )
                  })}
              </ul>
            ) : changes.length === 0 ? (
              <p className="py-2 text-center text-xs text-muted-foreground">本机没有待同步的改动</p>
            ) : (
              <ul className="max-h-56 space-y-1 overflow-y-auto">
                {changes.map((change) => (
                  <li key={`${change.kind}:${change.path}`} className="flex items-center justify-between gap-2 rounded-lg border border-border/40 px-2.5 py-1.5 text-xs">
                    <span className="min-w-0 flex-1 truncate" title={change.path}>
                      <span className="mr-2 text-muted-foreground">{KIND_LABEL[change.kind]}</span>
                      {change.path}
                    </span>
                    {change.kind === 'deleted' && (
                      <Button size="sm" variant="ghost" className="h-6 shrink-0 px-2 text-[11px]" onClick={() => void ignoreChange(change)}>
                        忽略
                      </Button>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}

        <DialogFooter className="-mx-5 -mb-5 mt-1 rounded-b-2xl px-5 py-4 sm:-mx-6 sm:-mb-6 sm:px-6">
          <Button variant="ghost" className="rounded-lg" onClick={() => onOpenChange(false)}>关闭</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
