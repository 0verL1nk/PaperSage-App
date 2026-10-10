import { useCallback, useEffect, useRef, useState } from 'react'
import { Check, Download, HardDrive, Loader2, Pause, RotateCcw, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import {
  clearLatexResourceCache, downloadLatexResource, getLatexResourceStatus,
  type LatexDownloadProgress, type LatexResource,
} from '@/lib/latex-resource-cache'
import browserEngineManifest from '@/lib/latex/browser-engine-manifest.json'

const ENGINE_RESOURCES: LatexResource[] = browserEngineManifest.resources
const ENGINE_PROFILES = browserEngineManifest.profiles
type EngineProfileId = typeof ENGINE_PROFILES[number]['id']

function formatBytes(bytes: number): string {
  return bytes < 1024 * 1024 ? `${Math.ceil(bytes / 1024)} KB` : `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

export function LatexEnvironmentSetup({ open, onOpenChange, disabled = false }: {
  open: boolean
  onOpenChange: (open: boolean) => void
  disabled?: boolean
}) {
  const [statuses, setStatuses] = useState<Record<string, number>>({})
  const [cachedIds, setCachedIds] = useState<Set<string>>(() => new Set())
  const [current, setCurrent] = useState<LatexDownloadProgress | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [persistentStorage, setPersistentStorage] = useState<boolean | null>(null)
  const [isClearConfirmOpen, setIsClearConfirmOpen] = useState(false)
  const [isClearing, setIsClearing] = useState(false)
  const [cacheNotice, setCacheNotice] = useState<string | null>(null)
  const [profileId, setProfileId] = useState<EngineProfileId>('xelatex')
  const controllerRef = useRef<AbortController | null>(null)
  const profile = ENGINE_PROFILES.find((candidate) => candidate.id === profileId) ?? ENGINE_PROFILES[0]
  const profileResourceIds: readonly string[] = profile.resourceIds
  const profileResources = ENGINE_RESOURCES.filter((resource) => profileResourceIds.includes(resource.id))
  const totalBytes = profileResources.reduce((total, resource) => total + resource.size, 0)

  const refresh = useCallback(async () => {
    const entries = await Promise.all(ENGINE_RESOURCES.map(async (resource) => [resource.id, await getLatexResourceStatus(resource)] as const))
    setStatuses(Object.fromEntries(entries.map(([id, value]) => [id, value.downloaded])))
    setCachedIds(new Set(entries.filter(([, value]) => value.cached).map(([id]) => id)))
  }, [])

  useEffect(() => {
    if (open) void refresh()
  }, [open, refresh])

  const start = async () => {
    setBusy(true)
    setError(null)
    setCurrent(null)
    try {
      if (!('serviceWorker' in navigator)) throw new Error('此浏览器不支持 Service Worker，无法让编译引擎从本地缓存加载')
      if (navigator.storage?.persist) setPersistentStorage(await navigator.storage.persist())
      else setPersistentStorage(false)
      await navigator.serviceWorker.register('/latex-resource-sw.js', { scope: '/' })
      await navigator.serviceWorker.ready
      const controller = new AbortController()
      controllerRef.current = controller
      for (const resource of profileResources) {
        if (cachedIds.has(resource.id)) continue
        await downloadLatexResource(resource, {
          signal: controller.signal,
          onProgress: (progress) => {
            setCurrent(progress)
            setStatuses((previous) => ({ ...previous, [resource.id]: progress.downloaded }))
          },
        })
        setCachedIds((previous) => new Set(previous).add(resource.id))
      }
      setCurrent(null)
      await refresh()
    } catch (cause) {
      if (!(cause instanceof DOMException && cause.name === 'AbortError')) {
        const message = cause instanceof Error ? cause.message : '资源下载失败；已下载分块已保留，再试会续传'
        setError(cause instanceof TypeError ? `网络请求失败。请检查资源源的 CORS 与 HTTP Range 配置；${message}` : message)
      }
      setCurrent(null)
      await refresh()
    } finally {
      setBusy(false)
      controllerRef.current = null
    }
  }

  const pause = () => controllerRef.current?.abort()
  const clearCache = async () => {
    if (disabled) return
    setIsClearing(true)
    setError(null)
    setCacheNotice(null)
    try {
      await clearLatexResourceCache()
      setStatuses({})
      setCachedIds(new Set())
      setCurrent(null)
      setCacheNotice('浏览器 LaTeX 缓存已清理；下次编译会重新下载所需资源。')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '清理浏览器 LaTeX 缓存失败')
    } finally {
      setIsClearing(false)
    }
  }
  const readyCount = profileResources.filter((resource) => cachedIds.has(resource.id)).length
  const downloaded = profileResources.reduce((total, resource) => total + (cachedIds.has(resource.id) ? resource.size : statuses[resource.id] ?? 0), 0)
  const percent = Math.floor(downloaded * 100 / totalBytes)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2"><HardDrive size={18} />浏览器编译环境</DialogTitle>
          <DialogDescription>选择要预载的浏览器引擎。宏包和字体仍会在编译时按需加载。</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-2" role="group" aria-label="选择要初始化的 LaTeX 引擎">
            {ENGINE_PROFILES.map((candidate) => (
              <button
                key={candidate.id}
                type="button"
                aria-pressed={profileId === candidate.id}
                disabled={busy || disabled || isClearing}
                onClick={() => setProfileId(candidate.id)}
                className={`rounded-xl border p-3 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-60 ${profileId === candidate.id ? 'border-primary bg-primary/5' : 'hover:bg-muted/40'}`}
              >
                <span className="block text-sm font-medium">{candidate.name}</span>
                <span className="mt-1 block text-xs text-muted-foreground">{candidate.description}</span>
              </button>
            ))}
          </div>
          <section className="rounded-xl border bg-muted/20 p-4" aria-live="polite">
            <div className="flex items-start justify-between gap-3">
              <div className="flex items-start gap-3">
                <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                  {readyCount === profileResources.length
                    ? <Check size={17} />
                    : busy ? <Loader2 size={17} className="animate-spin" /> : <Download size={17} />}
                </div>
                <div className="min-w-0">
                  <p className="text-sm font-medium">
                    {readyCount === profileResources.length ? `${profile.name} 环境已就绪` : busy ? `正在准备 ${profile.name}` : `首次使用需要下载 ${profile.name}`}
                  </p>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {readyCount}/{profileResources.length} 项已验证 · 本配置约 {formatBytes(totalBytes)}
                  </p>
                </div>
              </div>
              <span className="shrink-0 pt-1 text-xs tabular-nums text-muted-foreground">{percent}%</span>
            </div>
            <progress className="mt-3 h-1.5 w-full accent-primary" max={100} value={percent} aria-label="浏览器编译引擎总体下载进度" />
            {current?.resumed && <p className="mt-2 text-xs text-primary">已从上次中断的位置继续</p>}
          </section>

          <div className="divide-y rounded-xl border" aria-live="polite">
            {profileResources.map((resource) => {
              const resourceProgress = statuses[resource.id] ?? 0
              const isCached = cachedIds.has(resource.id)
              const percentDone = isCached ? 100 : Math.floor(resourceProgress * 100 / resource.size)
              return (
                <div key={resource.id} className="px-3 py-2.5">
                  <div className="flex items-center justify-between gap-3">
                    <span className="flex min-w-0 items-center gap-2 text-sm font-medium">
                      {isCached
                        ? <Check size={14} className="shrink-0 text-green-600" />
                        : busy && current?.resource.id === resource.id
                          ? <Loader2 size={14} className="shrink-0 animate-spin text-primary" />
                          : <span className="size-3.5 shrink-0 rounded-full border border-border" />}
                      <span className="truncate">{resource.name}</span>
                    </span>
                    <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                      {isCached ? '已验证' : `${formatBytes(resourceProgress)} / ${formatBytes(resource.size)}`}
                    </span>
                  </div>
                  <progress className="ml-5 mt-2 block h-1 w-[calc(100%-1.25rem)] accent-primary" max={100} value={percentDone} aria-label={`${resource.name} 下载进度`} />
                </div>
              )
            })}
          </div>
          {persistentStorage === false && <p role="status" className="rounded-lg bg-amber-500/10 px-3 py-2 text-xs leading-5 text-amber-800 dark:text-amber-300">浏览器没有授予持久存储。刷新后通常仍可续传，但浏览器清理站点数据时可能会移除已缓存资源。</p>}
          {error && <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
          {cacheNotice && <p role="status" className="rounded-md bg-primary/5 p-3 text-sm text-muted-foreground">{cacheNotice}</p>}
          <div className="space-y-2 text-xs text-muted-foreground">
            <p>宏包和字体会在编译时按需加载，并缓存在此浏览器；首次编译还会生成格式文件。</p>
            <details className="group">
              <summary className="w-fit cursor-pointer select-none text-primary/90 hover:text-primary">下载中断后会怎样？</summary>
              <p className="mt-1.5 leading-5">已完成的 1 MiB 分块会保存在本机。重新打开此处继续时会请求剩余部分；正在传输的分块可能需要重下。</p>
            </details>
          </div>
          <div className="flex justify-end gap-2">
            {busy && <Button variant="outline" onClick={pause}><Pause size={14} className="mr-2" />暂停并保留进度</Button>}
            <Button type="button" variant="outline" disabled={busy || disabled || isClearing} onClick={() => setIsClearConfirmOpen(true)}>
              <Trash2 size={14} className="mr-2" />清理缓存
            </Button>
            <Button onClick={() => void start()} disabled={busy || disabled || isClearing || readyCount === profileResources.length}>
              {busy ? <Loader2 size={14} className="mr-2 animate-spin" /> : readyCount === profileResources.length ? <Check size={14} className="mr-2" /> : readyCount ? <RotateCcw size={14} className="mr-2" /> : <Download size={14} className="mr-2" />}
              {busy ? '正在下载…' : readyCount === profileResources.length ? '已完成' : readyCount ? '继续下载' : '下载并准备'}
            </Button>
          </div>
        </div>
      </DialogContent>
      <ConfirmDialog
        open={isClearConfirmOpen}
        onOpenChange={setIsClearConfirmOpen}
        title="清理浏览器 LaTeX 缓存？"
        description="这会删除已下载引擎、宏包、字体、格式文件和未完成的下载分块。项目源码与已上传的 PDF 不受影响；再次编译时会重新下载需要的资源。"
        confirmText="清理缓存"
        cancelText="保留缓存"
        variant="destructive"
        onConfirm={clearCache}
        isPending={isClearing}
      />
    </Dialog>
  )
}
