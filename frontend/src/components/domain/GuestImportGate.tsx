import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, HardDrive, LoaderCircle, RotateCw, UploadCloud } from 'lucide-react'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { clearGuestWorkspace } from '@/services/guest-local-workspace'
import { getGuestImportBundle, getAccountImportIdentity, importGuestWorkspace, type GuestImportProgress, type GuestImportSummary } from '@/services/guest-workspace-import'
import type { GuestWorkspaceBundle } from '@/services/guest-local-workspace'
import { setGuestImportPending, useSessionStore } from '@/store'
import { OPEN_GUEST_IMPORT_EVENT } from '@/components/domain/GuestSessionBadge'

function projectContentCount(bundle: GuestWorkspaceBundle) {
  return bundle.projects.reduce((count, entry) => count + entry.folders.length + entry.documents.length + entry.latexFiles.length, 0)
}

export function GuestImportGate() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const accessToken = useSessionStore((state) => state.accessToken)
  const pending = useSessionStore((state) => state.guestImportPending)
  const [open, setOpen] = useState(false)
  const [dismissed, setDismissed] = useState(false)
  const [loading, setLoading] = useState(false)
  const [bundle, setBundle] = useState<GuestWorkspaceBundle | null>(null)
  const [identity, setIdentity] = useState<{ accountId: string; username: string } | null>(null)
  const [error, setError] = useState('')
  const [progress, setProgress] = useState<GuestImportProgress | null>(null)
  const [completedSummary, setCompletedSummary] = useState<GuestImportSummary | null>(null)
  const [confirmDeleteBackup, setConfirmDeleteBackup] = useState(false)
  const loadingRef = useRef(false)

  const loadReview = useCallback(async () => {
    if (loadingRef.current || bundle) return
    loadingRef.current = true
    setLoading(true)
    setError('')
    try {
      const [workspace, account] = await Promise.all([getGuestImportBundle(), getAccountImportIdentity()])
      setBundle(workspace)
      setIdentity(account)
      if (workspace.projects.length === 0) {
        setGuestImportPending(false)
        setOpen(false)
        setDismissed(true)
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '读取本机游客数据失败。')
    } finally {
      loadingRef.current = false
      setLoading(false)
    }
  }, [bundle])

  useEffect(() => {
    if (!accessToken || !pending || dismissed) return
    // Query keys are shared between local and cloud adapters; evict guest
    // snapshots before account-backed pages reuse those keys.
    for (const key of ['projects', 'project', 'writing-state', 'documents', 'folders', 'latex-files', 'latex-file-content']) {
      queryClient.removeQueries({ queryKey: [key] })
    }
    setOpen(true)
    void loadReview()
  }, [accessToken, pending, dismissed, loadReview, queryClient])

  useEffect(() => {
    const openReview = () => {
      if (!accessToken || !pending) return
      setDismissed(false)
      setOpen(true)
      void loadReview()
    }
    window.addEventListener(OPEN_GUEST_IMPORT_EVENT, openReview)
    return () => window.removeEventListener(OPEN_GUEST_IMPORT_EVENT, openReview)
  }, [accessToken, pending, loadReview])

  const startImport = async () => {
    if (!bundle || !identity || loading) return
    setLoading(true)
    setError('')
    setProgress(null)
    try {
      const summary = await importGuestWorkspace(bundle, identity.accountId, setProgress)
      setCompletedSummary(summary)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['projects'] }),
        queryClient.invalidateQueries({ queryKey: ['project'] }),
        queryClient.invalidateQueries({ queryKey: ['folders'] }),
        queryClient.invalidateQueries({ queryKey: ['documents'] }),
        queryClient.invalidateQueries({ queryKey: ['writing-state'] }),
        queryClient.invalidateQueries({ queryKey: ['latex-files'] }),
      ])
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '导入中断。游客原始数据和已完成项目的续传记录都保留在本机。')
    } finally {
      setLoading(false)
    }
  }

  const finishImportChoice = async (keepBackup: boolean) => {
    if (!completedSummary) return
    setLoading(true)
    setError('')
    try {
      if (!keepBackup) await clearGuestWorkspace()
      setGuestImportPending(false)
      setOpen(false)
      toast.success('游客项目已导入账号', {
        description: keepBackup
          ? `${completedSummary.importedProjects.length} 个云端项目已建立；本机备份已保留。`
          : `${completedSummary.importedProjects.length} 个云端项目已建立；本机游客副本已删除。`,
      })
      await navigate({ to: '/app/projects' })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '本机副本未能删除，仍会保留在当前浏览器。')
    } finally {
      setLoading(false)
    }
  }

  const closeReview = (next: boolean) => {
    if (!next && (loading || completedSummary)) return
    setOpen(next)
    if (!next) setDismissed(true)
  }

  const count = bundle ? projectContentCount(bundle) : 0
  const totalBytes = bundle?.projects.reduce((sum, entry) => sum + entry.documents.reduce((docSum, item) => docSum + item.blob.size, 0) + entry.latexFiles.reduce((latexSum, item) => latexSum + (item.blob?.size ?? new Blob([item.content]).size), 0), 0) ?? 0

  return (
    <Dialog open={open} onOpenChange={closeReview}>
      <DialogContent className="max-h-[min(90dvh,760px)] w-[calc(100vw-1rem)] max-w-xl overflow-y-auto rounded-2xl p-5 sm:w-full sm:p-6">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2"><UploadCloud className="size-5 text-primary" />导入本机游客项目</DialogTitle>
          <DialogDescription className="leading-6">
            先核对将复制到账号的内容。确认后会按项目和文件逐项上传；完成后本机游客数据库会作为备份保留。
          </DialogDescription>
        </DialogHeader>

        {completedSummary && (
          <Alert className="border-emerald-600/25 bg-emerald-600/5">
            <CheckCircle2 className="size-4 text-emerald-700" />
            <AlertTitle>云端副本已完整建立</AlertTitle>
            <AlertDescription className="leading-5">
              已完成 {completedSummary.completed}/{completedSummary.total} 项，包含项目内容与回收站状态。请选择保留本机备份，或确认删除本机游客数据库后继续。
            </AlertDescription>
          </Alert>
        )}

        {error && <Alert variant="destructive"><AlertTitle>{completedSummary ? '本机备份仍保留' : '导入尚未完成'}</AlertTitle><AlertDescription className="break-words">{error}</AlertDescription></Alert>}

        {loading && !bundle && <div className="flex min-h-24 items-center justify-center gap-2 text-sm text-muted-foreground"><LoaderCircle className="size-4 animate-spin" />正在读取本机项目…</div>}

        {bundle && identity && (
          <div className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-border/60 bg-muted/40 p-3 text-sm">
              <span>导入到账号 <strong>{identity.username}</strong></span>
              <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground"><HardDrive className="size-3.5" />本机副本保留</span>
            </div>
            <div className="max-h-56 space-y-2 overflow-y-auto pr-1">
              {bundle.projects.map((entry) => (
                <section key={entry.project.id} className="rounded-xl border border-border/60 p-3">
                  <h3 className="flex flex-wrap items-center gap-2 break-words text-sm font-semibold">{entry.project.name}{entry.project.is_deleted && <span className="rounded-full bg-muted px-2 py-0.5 text-[10px] font-medium text-muted-foreground">原在回收站</span>}</h3>
                  {entry.project.description && <p className="mt-1 line-clamp-2 break-words text-xs leading-5 text-muted-foreground">{entry.project.description}</p>}
                  <p className="mt-2 text-xs text-muted-foreground">
                    {entry.folders.length} 个文件夹 · {entry.documents.length} 篇文档 · {entry.latexFiles.length} 个 LaTeX 文件{entry.writingState ? ' · 含写作计划' : ''}
                  </p>
                </section>
              ))}
            </div>
            <p className="text-xs leading-5 text-muted-foreground">
              共 {bundle.projects.length} 个项目、{count} 个文件/文件夹，约 {(totalBytes / (1024 * 1024)).toFixed(1)} MB。云端创建与上传完成前不会标记导入成功。
            </p>
            {loading && progress && (
              <div className="space-y-2" aria-live="polite">
                <div className="flex justify-between gap-3 text-xs text-muted-foreground"><span className="truncate">{progress.projectName} · {progress.itemName}</span><span className="shrink-0">{progress.completed}/{progress.total}</span></div>
                <progress
                  aria-label="游客项目导入进度"
                  className="h-2 w-full overflow-hidden rounded-full accent-primary"
                  max={progress.total || 1}
                  value={progress.completed}
                />
              </div>
            )}
            {error && progress && <p className="text-xs leading-5 text-muted-foreground">已完成 {progress.completed}/{progress.total} 项。检查点已保存；可以重试继续，游客原始数据仍在本机。</p>}
            <Alert className="border-primary/20 bg-primary/5">
              <CheckCircle2 className="size-4 text-primary" />
              <AlertTitle className="text-sm">默认保留本机备份</AlertTitle>
              <AlertDescription className="text-xs leading-5">导入会在账号下创建云端副本。完成后你可以保留本机数据，或通过二次确认删除本机游客数据库。</AlertDescription>
            </Alert>
          </div>
        )}

        <DialogFooter className="flex-col-reverse gap-2 sm:flex-row sm:justify-between">
          {completedSummary ? (
            <>
              <Button variant="destructive" className="min-h-12" disabled={loading} onClick={() => setConfirmDeleteBackup(true)}>删除本机备份…</Button>
              <Button className="min-h-12" disabled={loading} onClick={() => void finishImportChoice(true)}>
                {loading ? <><LoaderCircle className="mr-2 size-4 animate-spin" />正在完成…</> : '保留本机备份并继续'}
              </Button>
            </>
          ) : (
            <>
              <Button variant="ghost" className="min-h-12" disabled={loading} onClick={() => closeReview(false)}>稍后处理</Button>
              <Button className="min-h-12" disabled={loading || (!bundle && !error) || Boolean(bundle && !identity)} onClick={() => {
                if (!bundle) void loadReview()
                else void startImport()
              }}>
                {loading ? <><LoaderCircle className="mr-2 size-4 animate-spin" />{bundle ? '正在导入…' : '正在读取…'}</> : !bundle ? <><RotateCw className="mr-2 size-4" />重试读取本机数据</> : error && progress ? <><RotateCw className="mr-2 size-4" />重试未完成项</> : <><UploadCloud className="mr-2 size-4" />确认导入到账号</>}
              </Button>
            </>
          )}
        </DialogFooter>
        {bundle && <span className="sr-only">本次导入包含 {bundle.projects.length} 个项目</span>}
      </DialogContent>
      <ConfirmDialog
        open={confirmDeleteBackup}
        onOpenChange={setConfirmDeleteBackup}
        title="删除本机游客备份？"
        description="云端副本已完整建立。确认后会删除此浏览器中的整个游客数据库，无法从 PaperSage 恢复；请先确认云端项目都已导入完成。"
        confirmText="确认删除本机副本"
        cancelText="继续保留"
        variant="destructive"
        isPending={loading}
        onConfirm={() => finishImportChoice(false)}
      />
    </Dialog>
  )
}
