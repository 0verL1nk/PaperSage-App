import { toast } from 'sonner'

import { notifyWhenBackground } from '@/lib/desktop'
import { compileLatexInBrowser } from '@/lib/latex/swiftlatex-browser-compiler'
import type { BrowserLatexFile, BrowserLatexProgress } from '@/lib/latex/swiftlatex-browser-compiler'
import {
  BROWSER_COMPILE_HEARTBEAT_MS,
  cancelCompile,
  createBrowserCompileJob,
  reportBrowserCompileFailure,
  sendBrowserCompileHeartbeat,
  uploadBrowserCompileResult,
} from '@/services/latex-api'
import {
  hasUnfinishedCompile,
  useCompileStore,
  type PendingBrowserUpload,
  type ProjectCompileState,
} from '@/store/compile-store'

/**
 * Module-level owner of the browser compile. It lives outside React so that
 * routing away from the writing page never aborts the Web Worker or drops the
 * progress/result; only `cancelProjectCompile` stops a running compile.
 */

export interface StartCompileOptions {
  projectId: string
  projectName: string
  rootFile: string
  compiler: string
  isGuest: boolean
  /** Saves pending edits and snapshots every project file; may outlive the page. */
  prepare: (signal: AbortSignal) => Promise<BrowserLatexFile[]>
}

export type StartCompileResult =
  | { ok: true; done: Promise<void> }
  | { ok: false; reason: 'busy'; busyProjectId: string }

/** One browser compile at a time: two engines on a student laptop is an OOM risk. */
let running: { projectId: string; controller: AbortController } | null = null

function patch(projectId: string, value: Partial<ProjectCompileState>): void {
  useCompileStore.getState().patch(projectId, value)
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
}

function waitForAbortable<T>(operation: Promise<T>, signal: AbortSignal): Promise<T> {
  if (signal.aborted) return Promise.reject(new DOMException('编译已取消', 'AbortError'))
  return new Promise<T>((resolve, reject) => {
    const cleanup = () => signal.removeEventListener('abort', onAbort)
    const onAbort = () => {
      cleanup()
      reject(new DOMException('编译已取消', 'AbortError'))
    }
    signal.addEventListener('abort', onAbort, { once: true })
    operation.then(
      (value) => { cleanup(); resolve(value) },
      (error: unknown) => { cleanup(); reject(error) },
    )
  })
}

/** Cross-page notice; the writing page of this project shows the inline state instead. */
function notifyAway(projectId: string, kind: 'success' | 'error', title: string, description?: string): void {
  const state = useCompileStore.getState()
  if (state.activeWritingProjectId === projectId) return
  const name = state.projects[projectId]?.projectName ?? '论文'
  const text = `《${name}》${title}`
  if (kind === 'success') toast.success(text, description ? { description } : undefined)
  else toast.error(text, description ? { description } : undefined)
}

async function runCompile(options: StartCompileOptions, controller: AbortController): Promise<void> {
  const { projectId, rootFile, compiler, isGuest } = options
  let jobId: string | null = null
  let compiled: PendingBrowserUpload | null = null
  let heartbeat: number | null = null
  try {
    const files = await waitForAbortable(options.prepare(controller.signal), controller.signal)
    if (controller.signal.aborted) throw new DOMException('编译已取消', 'AbortError')

    if (!isGuest) {
      const request = createBrowserCompileJob(projectId, rootFile, compiler)
      let created: { job_id: string }
      try {
        created = await waitForAbortable(request, controller.signal)
      } catch (error) {
        if (controller.signal.aborted) {
          void request
            .then((job) => cancelCompile(projectId, job.job_id))
            .catch((cancelError: unknown) => {
              toast.error('浏览器编译已停止，但服务端任务取消失败', {
                description: errorMessage(cancelError),
              })
            })
        }
        throw error
      }
      jobId = created.job_id
      patch(projectId, { jobId })
      // Tell the server the tab is still alive so a long compile is not
      // mistaken for an abandoned job.
      heartbeat = window.setInterval(() => {
        void sendBrowserCompileHeartbeat(projectId, jobId!).catch(() => {
          // Liveness only: a missed beat must not interrupt a running compile.
        })
      }, BROWSER_COMPILE_HEARTBEAT_MS)
    }

    patch(projectId, { phase: 'compiling' })

    const result = await compileLatexInBrowser(files, rootFile, compiler, {
      signal: controller.signal,
      onProgress: (progress: BrowserLatexProgress) => {
        patch(projectId, { progressMessage: progress.message })
        if (progress.stage === 'convert' && progress.license) patch(projectId, { license: progress.license })
        if (progress.stage === 'resource') {
          const resources = useCompileStore.getState().projects[projectId]?.resources ?? {}
          patch(projectId, {
            resources: { ...resources, [`${progress.resource.kind}:${progress.resource.name}`]: progress.resource },
          })
        }
      },
    })

    patch(projectId, {
      pdf: result.pdf,
      logs: result.log,
      warning: result.log.includes('did not stabilize')
        ? '编译已达到既有多遍上限，参考文献或交叉引用可能尚未稳定；请检查 PDF 中的引用编号。'
        : null,
    })

    if (!jobId) {
      patch(projectId, {
        phase: 'success',
        progressMessage: 'PDF 已生成（游客模式，仅保留在本次会话）',
      })
      void notifyWhenBackground('编译完成', 'PDF 已生成')
      notifyAway(projectId, 'success', 'PDF 编译完成', '打开撰写页即可查看和下载。')
      return
    }

    compiled = { jobId, pdf: result.pdf, logs: result.log }
    patch(projectId, {
      phase: 'uploading',
      pendingUpload: compiled,
      progressMessage: 'PDF 已生成，正在上传到对象存储并保存编译记录',
    })
    if (controller.signal.aborted) throw new DOMException('编译已取消', 'AbortError')
    const uploaded = await uploadBrowserCompileResult(projectId, jobId, result.pdf, result.log)
    patch(projectId, {
      phase: 'success',
      pendingUpload: null,
      buildId: uploaded.build_id,
      progressMessage: 'PDF 已保存',
    })
    void notifyWhenBackground('编译完成', 'PDF 已生成')
    notifyAway(projectId, 'success', 'PDF 编译完成', '打开撰写页即可查看和下载。')
  } catch (error) {
    if (controller.signal.aborted || isAbort(error)) {
      patch(projectId, {
        phase: 'cancelled',
        progressMessage: '编译已取消',
        error: null,
        logs: null,
        pendingUpload: null,
      })
      return
    }
    const message = errorMessage(error) || '浏览器编译失败'
    patch(projectId, {
      phase: 'failed',
      error: compiled
        ? `PDF 已在浏览器生成，但上传或保存编译记录失败：${message}`
        : message,
      logs: compiled?.logs ?? message,
    })
    if (jobId && !compiled) {
      try {
        await reportBrowserCompileFailure(projectId, jobId, message, message)
      } catch (reportError) {
        patch(projectId, {
          error: `${message}\n\n保存失败记录时也发生错误：${errorMessage(reportError)}`,
        })
      }
    }
    void notifyWhenBackground('编译失败', message)
    notifyAway(projectId, 'error', '编译失败', message)
  } finally {
    if (heartbeat !== null) window.clearInterval(heartbeat)
    if (running?.controller === controller) running = null
  }
}

export function startCompile(options: StartCompileOptions): StartCompileResult {
  if (running) return { ok: false, reason: 'busy', busyProjectId: running.projectId }
  const controller = new AbortController()
  running = { projectId: options.projectId, controller }
  useCompileStore.getState().begin(options.projectId, options.projectName)
  return { ok: true, done: runCompile(options, controller) }
}

export async function cancelProjectCompile(projectId: string): Promise<void> {
  if (!running || running.projectId !== projectId) return
  const state = useCompileStore.getState().projects[projectId]
  if (!state || state.phase === 'uploading') return
  running.controller.abort()
  patch(projectId, { progressMessage: '正在停止浏览器引擎并取消编译任务' })
  if (!state.jobId) return
  try {
    await cancelCompile(projectId, state.jobId)
  } catch (error) {
    patch(projectId, { error: `本地编译已停止，但未能同步取消服务端任务：${errorMessage(error)}` })
  }
}

export async function retryBrowserUpload(projectId: string): Promise<void> {
  const state = useCompileStore.getState().projects[projectId]
  const pending = state?.pendingUpload
  if (!pending || state.retryingUpload) return
  patch(projectId, { retryingUpload: true, error: null, progressMessage: '正在重试上传已生成的 PDF' })
  try {
    const uploaded = await uploadBrowserCompileResult(projectId, pending.jobId, pending.pdf, pending.logs)
    patch(projectId, {
      phase: 'success',
      pendingUpload: null,
      buildId: uploaded.build_id,
      progressMessage: 'PDF 已保存',
      retryingUpload: false,
    })
    void notifyWhenBackground('编译完成', 'PDF 已生成')
    notifyAway(projectId, 'success', 'PDF 编译完成', '打开撰写页即可查看和下载。')
  } catch (error) {
    patch(projectId, {
      retryingUpload: false,
      error: `PDF 已生成，但上传仍未成功；可以重试上传，无需重新编译：${errorMessage(error)}`,
      logs: pending.logs,
    })
  }
}

// Background tabs throttle timers; catch up on the heartbeat as soon as the tab
// is visible again so the server never judges a live compile abandoned.
function heartbeatNow(): void {
  if (!running) return
  const { projectId } = running
  const jobId = useCompileStore.getState().projects[projectId]?.jobId
  if (jobId) void sendBrowserCompileHeartbeat(projectId, jobId).catch(() => undefined)
}

if (typeof document !== 'undefined') {
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') heartbeatNow()
  })
}

// Warn before closing the tab while a compile runs or a PDF has not been saved yet.
let unloadGuardBound = false
function onBeforeUnload(event: BeforeUnloadEvent): void {
  event.preventDefault()
  event.returnValue = '编译仍在进行中，离开页面会丢失进度。'
}
useCompileStore.subscribe((state) => {
  const needed = hasUnfinishedCompile(state)
  if (needed === unloadGuardBound) return
  unloadGuardBound = needed
  if (needed) window.addEventListener('beforeunload', onBeforeUnload)
  else window.removeEventListener('beforeunload', onBeforeUnload)
})
