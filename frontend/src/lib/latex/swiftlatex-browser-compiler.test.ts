import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as desktop from '@/lib/desktop'
import {
  appEnv,
  setApiTarget,
  setCustomApiBaseUrl,
  setDesktopLocalApiBaseUrl,
  setDesktopLocalBackendError,
} from '@/lib/env'
import { compileLatexInBrowser } from './swiftlatex-browser-compiler'

const files = [{ path: 'main.tex', content: '\\documentclass{article}' }]
const backendError = '本地后端版本不匹配'
const startWorker = vi.fn()
const messages: Array<Record<string, unknown>> = []

const resourceCache = vi.hoisted(() => ({
  getCachedLatexFormat: vi.fn(),
  putCachedLatexFormat: vi.fn(),
  pruneSupersededProfileCache: vi.fn(),
  downloadLatexResource: vi.fn(),
  getLatexResourceStatus: vi.fn(),
}))
vi.mock('@/lib/latex-resource-cache', () => resourceCache)

class TestWorker {
  onmessage: ((event: { data: { result: string } }) => void) | null = null
  onerror = null

  constructor() {
    startWorker()
    queueMicrotask(() => this.onmessage?.({ data: { result: 'ok' } }))
  }

  postMessage(message: Record<string, unknown>) {
    messages.push(message)
    if (message.cmd === 'settexliveurl') throw new Error('worker stopped after URL inspection')
  }

  terminate() {}
}

describe('browser compiler resource origin', () => {
  beforeEach(() => {
    localStorage.clear()
    messages.length = 0
    startWorker.mockClear()
    resourceCache.getCachedLatexFormat.mockResolvedValue(null)
    resourceCache.putCachedLatexFormat.mockResolvedValue(undefined)
    resourceCache.pruneSupersededProfileCache.mockResolvedValue(undefined)
    resourceCache.downloadLatexResource.mockResolvedValue(undefined)
    resourceCache.getLatexResourceStatus.mockResolvedValue({ cached: false, downloaded: 0 })
    vi.stubGlobal('Worker', TestWorker)
    // Compilation starts by discovering the served profile from the backend.
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ profile: 'tl-test-profile' }),
    }))
    vi.stubGlobal('navigator', {
      serviceWorker: { register: vi.fn().mockResolvedValue(undefined), ready: Promise.resolve() },
    })
  })

  afterEach(() => {
    vi.useRealTimers()
    setDesktopLocalApiBaseUrl(appEnv.localApiBaseUrl)
    setDesktopLocalApiBaseUrl('')
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('does not start the engine for a failed desktop local backend', async () => {
    vi.spyOn(desktop, 'isDesktop').mockReturnValue(true)
    setApiTarget('local')
    setDesktopLocalBackendError(backendError)

    await expect(compileLatexInBrowser(files, 'main.tex', 'pdflatex')).rejects.toThrow(backendError)
    expect(startWorker).not.toHaveBeenCalled()
    expect(messages).toEqual([])
  })

  it('keeps the same-origin TeX proxy for a working supervised desktop backend', async () => {
    vi.spyOn(desktop, 'isDesktop').mockReturnValue(true)
    setDesktopLocalApiBaseUrl('http://127.0.0.1:6001')
    setApiTarget('local')

    await expect(compileLatexInBrowser(files, 'main.tex', 'pdflatex')).rejects.toThrow('worker stopped after URL inspection')
    expect(messages[0]).toMatchObject({ cmd: 'settexliveurl' })
    expect((messages[0].url as string).startsWith(`${window.location.origin}/texlive/`)).toBe(true)
  })

  it.each(['online', 'custom'] as const)('keeps the %s resource origin after a local failure', async (target) => {
    vi.spyOn(desktop, 'isDesktop').mockReturnValue(true)
    setDesktopLocalBackendError(backendError)
    setCustomApiBaseUrl('https://custom.example.test')
    setApiTarget(target)

    await expect(compileLatexInBrowser(files, 'main.tex', 'pdflatex')).rejects.toThrow('worker stopped after URL inspection')
    const origin = target === 'custom' ? 'https://custom.example.test' : appEnv.apiBaseUrl || window.location.origin
    expect(messages[0]).toMatchObject({ cmd: 'settexliveurl' })
    expect((messages[0].url as string).startsWith(`${origin}/texlive/`)).toBe(true)
  })

  it('keeps the browser local-development resource origin', async () => {
    vi.spyOn(desktop, 'isDesktop').mockReturnValue(false)
    setApiTarget('local')

    await expect(compileLatexInBrowser(files, 'main.tex', 'pdflatex')).rejects.toThrow('worker stopped after URL inspection')
    expect(messages[0]).toMatchObject({ cmd: 'settexliveurl' })
    expect((messages[0].url as string).startsWith(`${appEnv.localApiBaseUrl}/texlive/`)).toBe(true)
  })

  it('stops the compile when a single resource download never answers', async () => {
    vi.useFakeTimers()
    class StalledWorker {
      onmessage: ((event: { data: Record<string, unknown> }) => void) | null = null
      onerror = null
      constructor() {
        startWorker()
        queueMicrotask(() => this.onmessage?.({ data: { result: 'ok' } }))
      }
      postMessage(message: Record<string, unknown>) {
        messages.push(message)
        if (message.cmd === 'compileformat') {
          queueMicrotask(() => this.onmessage?.({
            data: { cmd: 'resource', kind: 'package', name: 'FandolSong-Regular.otf', state: 'loading' },
          }))
        }
      }
      terminate() {}
    }
    vi.stubGlobal('Worker', StalledWorker)

    const compilation = compileLatexInBrowser(files, 'main.tex', 'xelatex')
    const rejection = expect(compilation).rejects.toThrow('下载 FandolSong-Regular.otf 超过 2 分钟没有响应')
    await vi.advanceTimersByTimeAsync(120_000)
    await rejection
  })

  it('times out an engine that never reports ready instead of hanging the compile', async () => {
    vi.useFakeTimers()
    class SilentWorker {
      onmessage: ((event: { data: unknown }) => void) | null = null
      onerror = null
      constructor() { startWorker() }
      postMessage() {}
      terminate() {}
    }
    vi.stubGlobal('Worker', SilentWorker)

    const compilation = compileLatexInBrowser(files, 'main.tex', 'pdflatex')
    const rejection = expect(compilation).rejects.toThrow('浏览器引擎初始化超时：超过 2 分钟仍未就绪')
    await vi.advanceTimersByTimeAsync(120_000)
    await rejection
    expect(startWorker).toHaveBeenCalledTimes(1)
  })
})
