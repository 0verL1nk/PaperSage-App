import { useCallback, useEffect, useRef, useState } from 'react'
import { Check, Download, HardDrive, Loader2, Pause, RotateCcw, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import {
  clearLatexResourceCache, downloadLatexResource, getBrowserStorageStatus, getLatexResourceStatus,
  pruneTexliveCacheExcept, readLatexFormatSummary, readTexliveCacheSummary,
  type BrowserStorageStatus, type CachedFormatSummary, type LatexDownloadProgress, type LatexResource,
  type TexliveCacheSummary,
} from '@/lib/latex-resource-cache'
import { browserResourceOrigin, discoverResourceProfile } from '@/lib/latex/resource-profile'
import { WARMUP_SAMPLES, warmupBrowserLatex, type WarmupSample } from '@/lib/latex/warmup'
import { isDesktop } from '@/lib/desktop'
import browserEngineManifest from '@/lib/latex/browser-engine-manifest.json'

const ENGINE_RESOURCES: LatexResource[] = browserEngineManifest.resources
const ENGINE_PROFILES = browserEngineManifest.profiles
type EngineProfileId = typeof ENGINE_PROFILES[number]['id']

const FORMAT_LABELS: Record<string, string> = {
  'swiftlatex-pdftex-format': 'pdfLaTeX 格式文件',
  'swiftlatex-xetex-format': 'XeLaTeX 格式文件',
}

function formatBytes(bytes: number): string {
  return bytes < 1024 * 1024 ? `${Math.ceil(bytes / 1024)} KB` : `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function formatTime(value: number | null): string {
  return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '尚未使用'
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
  const [profileId, setProfileId] = useState<EngineProfileId>('xelatex')
  const controllerRef = useRef<AbortController | null>(null)

  const [texlive, setTexlive] = useState<TexliveCacheSummary | null>(null)
  const [formats, setFormats] = useState<CachedFormatSummary[] | null>(null)
  const [storage, setStorage] = useState<BrowserStorageStatus | null>(null)
  const [resourceProfile, setResourceProfile] = useState<string | null>(null)
  const [profileError, setProfileError] = useState<string | null>(null)
  const [summaryLoading, setSummaryLoading] = useState(false)
  const [warmingId, setWarmingId] = useState<WarmupSample['id'] | null>(null)
  const [warmupMessage, setWarmupMessage] = useState('')
  const warmupControllerRef = useRef<AbortController | null>(null)

  const [isClearConfirmOpen, setIsClearConfirmOpen] = useState(false)
  const [isPruneConfirmOpen, setIsPruneConfirmOpen] = useState(false)
  const [isClearing, setIsClearing] = useState(false)
  const [isPruning, setIsPruning] = useState(false)
  const [cacheNotice, setCacheNotice] = useState<string | null>(null)

  const engineProfile = ENGINE_PROFILES.find((candidate) => candidate.id === profileId) ?? ENGINE_PROFILES[0]
  const profileResourceIds: readonly string[] = engineProfile.resourceIds
  const profileResources = ENGINE_RESOURCES.filter((resource) => profileResourceIds.includes(resource.id))
  const totalBytes = profileResources.reduce((total, resource) => total + resource.size, 0)

  const refreshEngine = useCallback(async () => {
    const entries = await Promise.all(ENGINE_RESOURCES.map(async (resource) => [resource.id, await getLatexResourceStatus(resource)] as const))
    setStatuses(Object.fromEntries(entries.map(([id, value]) => [id, value.downloaded])))
    setCachedIds(new Set(entries.filter(([, value]) => value.cached).map(([id]) => id)))
  }, [])

  const refreshCache = useCallback(async () => {
    setSummaryLoading(true)
    setProfileError(null)
    try {
      const [profile, texliveSummary, formatSummary, storageStatus] = await Promise.all([
        discoverResourceProfile(browserResourceOrigin()).catch((cause: unknown) => {
          setProfileError(cause instanceof Error ? cause.message : '无法获取 TeX Live 资源版本')
          return null
        }),
        readTexliveCacheSummary().catch(() => null),
        readLatexFormatSummary().catch(() => [] as CachedFormatSummary[]),
        getBrowserStorageStatus(),
      ])
      setResourceProfile(profile)
      setTexlive(texliveSummary)
      setFormats(formatSummary)
      setStorage(storageStatus)
    } finally {
      setSummaryLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!open) return
    void refreshEngine()
    void refreshCache()
  }, [open, refreshEngine, refreshCache])

  const storageUnavailable = storage !== null && !storage.available
  const actionsBlocked = disabled || busy || isClearing || isPruning || warmingId !== null || storageUnavailable

  const start = async () => {
    setBusy(true)
    setError(null)
    setCurrent(null)
    try {
      if (!('serviceWorker' in navigator)) throw new Error('此浏览器不支持 Service Worker，无法让编译引擎从本地缓存加载')
      await navigator.storage?.persist?.()
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
      await Promise.all([refreshEngine(), refreshCache()])
    } catch (cause) {
      if (!(cause instanceof DOMException && cause.name === 'AbortError')) {
        const message = cause instanceof Error ? cause.message : '资源下载失败；已下载分块已保留，再试会续传'
        setError(cause instanceof TypeError ? `网络请求失败。请检查资源源的 CORS 与 HTTP Range 配置；${message}` : message)
      }
      setCurrent(null)
      await refreshEngine()
    } finally {
      setBusy(false)
      controllerRef.current = null
    }
  }

  const pause = () => controllerRef.current?.abort()

  const startWarmup = async (sample: WarmupSample) => {
    setWarmingId(sample.id)
    setWarmupMessage(`正在准备${sample.name}…`)
    setError(null)
    setCacheNotice(null)
    const controller = new AbortController()
    warmupControllerRef.current = controller
    try {
      await warmupBrowserLatex(sample, {
        signal: controller.signal,
        onProgress: (progress) => setWarmupMessage(progress.message),
      })
      setCacheNotice(`${sample.name}预热完成，相关宏包和字体已保存在本机缓存。`)
    } catch (cause) {
      if (cause instanceof DOMException && cause.name === 'AbortError') {
        setCacheNotice('已取消预热；已经下载的资源仍然保留。')
      } else {
        setError(cause instanceof Error ? cause.message : '预热编译失败，请稍后重试')
      }
    } finally {
      setWarmingId(null)
      setWarmupMessage('')
      warmupControllerRef.current = null
      await refreshCache()
    }
  }

  const pruneOtherProfiles = async () => {
    if (!resourceProfile) return
    setIsPruning(true)
    setError(null)
    setCacheNotice(null)
    try {
      const pruned = await pruneTexliveCacheExcept(resourceProfile)
      setCacheNotice(pruned.entries > 0
        ? `已清理其他版本缓存：${pruned.entries} 个文件（${formatBytes(pruned.bytes)}）`
        : '没有其他版本的 TeX Live 缓存需要清理。')
      await refreshCache()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '清理旧版本缓存失败')
    } finally {
      setIsPruning(false)
    }
  }

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
      await refreshCache()
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
  const otherProfiles = texlive?.byProfile.filter((profile) => profile.profile !== resourceProfile && profile.profile !== '内置资源') ?? []
  const otherProfileBytes = otherProfiles.reduce((total, profile) => total + profile.bytes, 0)
  const otherProfileEntries = otherProfiles.reduce((total, profile) => total + profile.entries, 0)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2"><HardDrive size={18} />浏览器编译环境</DialogTitle>
          <DialogDescription>管理浏览器编译引擎、TeX Live 宏包字体与格式文件缓存，全部保存在本机浏览器。</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          {!isDesktop() && (
            <>
              <div className="grid grid-cols-2 gap-2" role="group" aria-label="选择要初始化的 LaTeX 引擎">
                {ENGINE_PROFILES.map((candidate) => (
                  <button
                    key={candidate.id}
                    type="button"
                    aria-pressed={profileId === candidate.id}
                    disabled={busy || disabled || isClearing || isPruning || warmingId !== null}
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
                        {readyCount === profileResources.length ? `${engineProfile.name} 引擎已就绪` : busy ? `正在准备 ${engineProfile.name}` : `首次使用需要下载 ${engineProfile.name}`}
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
            </>
          )}

          <section className="rounded-xl border p-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h3 className="text-sm font-medium">TeX Live 宏包与字体</h3>
                <p className="mt-0.5 text-xs text-muted-foreground">编译时按需下载的宏包、字体与字体映射，全部保存在本机浏览器缓存。</p>
              </div>
              <span className="shrink-0 pt-0.5 text-xs text-muted-foreground">
                {summaryLoading ? '读取中…' : texlive ? `${texlive.entries} 个文件 · ${formatBytes(texlive.bytes)}` : '尚无缓存'}
              </span>
            </div>
            <dl className="mt-3 grid grid-cols-2 gap-2 text-xs sm:grid-cols-4">
              <div>
                <dt className="text-muted-foreground">当前版本</dt>
                <dd className="mt-0.5 break-all font-medium">{profileError ?? resourceProfile ?? '读取中…'}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">宏包 / 字体</dt>
                <dd className="mt-0.5 font-medium">{texlive ? `${texlive.packages} / ${texlive.fonts}` : '—'}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">其他版本占用</dt>
                <dd className="mt-0.5 font-medium">{otherProfiles.length > 0 ? `${otherProfileEntries} 个 · ${formatBytes(otherProfileBytes)}` : '无'}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">最近使用</dt>
                <dd className="mt-0.5 font-medium">{formatTime(texlive?.lastUsed ?? null)}</dd>
              </div>
            </dl>
            {texlive && texlive.topPackages.length > 0 && (
              <details className="mt-3 text-xs text-muted-foreground">
                <summary className="w-fit cursor-pointer select-none text-primary/90 hover:text-primary">占用最大的宏包</summary>
                <ul className="mt-1.5 space-y-1">
                  {texlive.topPackages.map((entry) => (
                    <li key={entry.package}><span className="font-mono">{entry.package}</span> · {entry.entries} 个文件 · {formatBytes(entry.bytes)}</li>
                  ))}
                </ul>
              </details>
            )}
            <div className="mt-3 flex flex-wrap gap-2">
              {WARMUP_SAMPLES.map((sample) => (
                <Button
                  key={sample.id}
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={actionsBlocked}
                  onClick={() => void startWarmup(sample)}
                >
                  {warmingId === sample.id
                    ? <Loader2 size={14} className="mr-2 animate-spin" />
                    : <Download size={14} className="mr-2" />}
                  {warmingId === sample.id ? '预热中…' : `预热${sample.name}`}
                </Button>
              ))}
              {warmingId !== null && (
                <Button type="button" variant="outline" size="sm" onClick={() => warmupControllerRef.current?.abort()}>
                  <Pause size={14} className="mr-2" />取消预热
                </Button>
              )}
              {otherProfiles.length > 0 && (
                <Button type="button" variant="outline" size="sm" disabled={actionsBlocked} onClick={() => setIsPruneConfirmOpen(true)}>
                  <Trash2 size={14} className="mr-2" />清理其他版本（{formatBytes(otherProfileBytes)}）
                </Button>
              )}
            </div>
            {warmingId !== null && <p className="mt-2 text-xs text-muted-foreground" role="status">{warmupMessage}</p>}
          </section>

          <section className="rounded-xl border p-4">
            <h3 className="text-sm font-medium">格式文件缓存</h3>
            <p className="mt-0.5 text-xs text-muted-foreground">首次编译时自动生成，之后直接载入，不再重新生成。</p>
            {formats === null ? (
              <p className="mt-2 text-xs text-muted-foreground">正在读取…</p>
            ) : formats.length === 0 ? (
              <p className="mt-2 text-xs text-muted-foreground">还没有生成格式文件；第一次编译会自动生成并缓存。</p>
            ) : (
              <ul className="mt-2 divide-y rounded-lg border text-xs">
                {formats.map((format) => (
                  <li key={format.id} className="flex flex-wrap items-center justify-between gap-2 px-3 py-2">
                    <span className="font-medium">{FORMAT_LABELS[format.id] ?? format.id}</span>
                    <span className="text-muted-foreground">
                      {format.engineVersion === browserEngineManifest.engineVersion
                        ? `${formatBytes(format.bytes)} · ${formatTime(format.savedAt)}`
                        : '引擎已更新，下次编译会重新生成'}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </section>

          <section className="rounded-xl border p-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h3 className="text-sm font-medium">浏览器存储</h3>
                <p className="mt-0.5 text-xs text-muted-foreground">本机已用空间与浏览器上限；授权持久存储后浏览器不会自动清理。</p>
              </div>
              {storage?.persisted === true && (
                <span className="shrink-0 rounded-full bg-primary/10 px-2 py-0.5 text-xs text-primary">已授权持久存储</span>
              )}
            </div>
            {storage && storage.available ? (
              <div className="mt-2 space-y-1 text-xs">
                <p>已用 {formatBytes(storage.usage)}{storage.quota > 0 ? ` / 上限 ${formatBytes(storage.quota)}` : ''}</p>
                {storage.persisted === false && (
                  <p className="text-amber-800 dark:text-amber-300">浏览器暂未授予持久存储；清理站点数据时缓存可能被移除。</p>
                )}
              </div>
            ) : (
              <p className="mt-2 rounded-lg bg-amber-500/10 px-3 py-2 text-xs leading-5 text-amber-800 dark:text-amber-300" role="status">
                {storage?.unavailableReason ?? '正在读取浏览器存储状态…'}
              </p>
            )}
          </section>

          {error && <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
          {cacheNotice && <p role="status" className="rounded-md bg-primary/5 p-3 text-sm text-muted-foreground">{cacheNotice}</p>}
          <div className="space-y-2 text-xs text-muted-foreground">
            <p>宏包和字体会在编译时按需加载并缓存在此浏览器；格式文件在首次编译时生成。</p>
            <details className="group">
              <summary className="w-fit cursor-pointer select-none text-primary/90 hover:text-primary">下载中断后会怎样？</summary>
              <p className="mt-1.5 leading-5">已完成的 1 MiB 分块会保存在本机。重新打开此处继续时会请求剩余部分；正在传输的分块可能需要重下。</p>
            </details>
          </div>
          <div className="flex flex-wrap justify-end gap-2">
            {busy && <Button variant="outline" onClick={pause}><Pause size={14} className="mr-2" />暂停并保留进度</Button>}
            <Button type="button" variant="outline" disabled={actionsBlocked} onClick={() => setIsClearConfirmOpen(true)}>
              <Trash2 size={14} className="mr-2" />清理全部缓存
            </Button>
            {!isDesktop() && (
              <Button onClick={() => void start()} disabled={actionsBlocked || readyCount === profileResources.length}>
                {busy ? <Loader2 size={14} className="mr-2 animate-spin" /> : readyCount === profileResources.length ? <Check size={14} className="mr-2" /> : readyCount ? <RotateCcw size={14} className="mr-2" /> : <Download size={14} className="mr-2" />}
                {busy ? '正在下载…' : readyCount === profileResources.length ? '已完成' : readyCount ? '继续下载' : '下载并准备'}
              </Button>
            )}
          </div>
        </div>
      </DialogContent>
      <ConfirmDialog
        open={isPruneConfirmOpen}
        onOpenChange={setIsPruneConfirmOpen}
        title="清理其他版本的 TeX Live 缓存？"
        description={`将删除旧版本 profile 的 ${otherProfileEntries} 个缓存文件（${formatBytes(otherProfileBytes)}），当前版本的缓存保留。再次编译旧版本文档时会重新下载。`}
        confirmText="确认清理"
        cancelText="保留"
        variant="destructive"
        onConfirm={pruneOtherProfiles}
        isPending={isPruning}
      />
      <ConfirmDialog
        open={isClearConfirmOpen}
        onOpenChange={setIsClearConfirmOpen}
        title="清理浏览器 LaTeX 缓存？"
        description="这会删除已下载引擎、宏包、字体、格式文件和未完成的下载分块。项目源码与已上传的 PDF 不受影响；再次编译时会重新下载需要的资源。"
        confirmText="清理全部缓存"
        cancelText="保留缓存"
        variant="destructive"
        onConfirm={clearCache}
        isPending={isClearing}
      />
    </Dialog>
  )
}
