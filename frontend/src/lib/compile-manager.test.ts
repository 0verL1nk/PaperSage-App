import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  compileLatexInBrowser: vi.fn(),
  createBrowserCompileJob: vi.fn(),
  sendBrowserCompileHeartbeat: vi.fn(),
  uploadBrowserCompileResult: vi.fn(),
  reportBrowserCompileFailure: vi.fn(),
  cancelCompile: vi.fn(),
  notifyWhenBackground: vi.fn(),
  reportCompileDiagnostic: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}))

vi.mock('sonner', () => ({ toast: { success: mocks.toastSuccess, error: mocks.toastError } }))
vi.mock('@/lib/desktop', () => ({
  notifyWhenBackground: mocks.notifyWhenBackground,
  isDesktop: () => false,
}))
vi.mock('@/services/diagnostics-api', () => ({ reportCompileDiagnostic: mocks.reportCompileDiagnostic }))
vi.mock('@/lib/latex/swiftlatex-browser-compiler', () => ({ compileLatexInBrowser: mocks.compileLatexInBrowser }))
vi.mock('@/services/latex-api', () => ({
  BROWSER_COMPILE_HEARTBEAT_MS: 60_000,
  cancelCompile: mocks.cancelCompile,
  createBrowserCompileJob: mocks.createBrowserCompileJob,
  reportBrowserCompileFailure: mocks.reportBrowserCompileFailure,
  sendBrowserCompileHeartbeat: mocks.sendBrowserCompileHeartbeat,
  uploadBrowserCompileResult: mocks.uploadBrowserCompileResult,
}))

import { cancelProjectCompile, retryBrowserUpload, startCompile, type StartCompileOptions } from './compile-manager'
import { useCompileStore } from '@/store/compile-store'

const pdf = new Uint8Array([37, 80, 68, 70])

function options(overrides: Partial<StartCompileOptions> = {}): StartCompileOptions {
  return {
    projectId: 'p1',
    projectName: '论文一',
    rootFile: 'main.tex',
    compiler: 'xelatex',
    isGuest: false,
    prepare: vi.fn(async () => [{ path: 'main.tex', content: '\\documentclass{article}' }]),
    ...overrides,
  }
}

function projectState(projectId = 'p1') {
  return useCompileStore.getState().projects[projectId]
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => { resolve = res })
  return { promise, resolve }
}

async function started(overrides: Partial<StartCompileOptions> = {}) {
  const result = startCompile(options(overrides))
  if (!result.ok) throw new Error(`expected start, got busy on ${result.busyProjectId}`)
  return result.done
}

describe('compile manager', () => {
  beforeEach(() => {
    useCompileStore.getState().reset()
    for (const mock of Object.values(mocks)) mock.mockReset()
    mocks.createBrowserCompileJob.mockResolvedValue({ job_id: 'job-1', status: 'running' })
    mocks.sendBrowserCompileHeartbeat.mockResolvedValue({ job_id: 'job-1', status: 'running' })
    mocks.uploadBrowserCompileResult.mockResolvedValue({ build_id: 'build-1', status: 'success' })
    mocks.reportBrowserCompileFailure.mockResolvedValue({ job_id: 'job-1', status: 'failed' })
    mocks.reportCompileDiagnostic.mockResolvedValue({ id: 'diag-1' })
    mocks.cancelCompile.mockResolvedValue(undefined)
  })

  afterEach(() => {
    vi.useRealTimers()
    useCompileStore.getState().reset()
  })

  it('runs prepare → job → compile → upload and keeps the result', async () => {
    mocks.compileLatexInBrowser.mockImplementation(async (_files: unknown, _root: string, _compiler: string, opts: {
      onProgress: (progress: Record<string, unknown>) => void
    }) => {
      opts.onProgress({ stage: 'compile', message: '第二遍编译', pass: 2, totalPasses: 2 })
      return { pdf, log: 'Output written on main.pdf' }
    })

    await started()

    expect(projectState()).toMatchObject({
      phase: 'success',
      jobId: 'job-1',
      buildId: 'build-1',
      pdf,
      pendingUpload: null,
      retryingUpload: false,
    })
    expect(mocks.compileLatexInBrowser).toHaveBeenCalledWith(
      [{ path: 'main.tex', content: '\\documentclass{article}' }],
      'main.tex',
      'xelatex',
      expect.objectContaining({ signal: expect.anything() }),
    )
    expect(mocks.uploadBrowserCompileResult).toHaveBeenCalledWith('p1', 'job-1', pdf, 'Output written on main.pdf')
  })

  it('keeps guest compiles local: no server job and no upload', async () => {
    mocks.compileLatexInBrowser.mockResolvedValue({ pdf, log: '' })

    await started({ isGuest: true })

    expect(mocks.createBrowserCompileJob).not.toHaveBeenCalled()
    expect(mocks.uploadBrowserCompileResult).not.toHaveBeenCalled()
    expect(projectState()).toMatchObject({ phase: 'success', pdf, jobId: null, buildId: null })
  })

  it('rejects a second compile while one is running', async () => {
    const compile = deferred<{ pdf: Uint8Array; log: string }>()
    mocks.compileLatexInBrowser.mockReturnValue(compile.promise)

    const first = startCompile(options())
    expect(first.ok).toBe(true)
    expect(startCompile(options({ projectId: 'p2', projectName: '论文二' }))).toEqual({
      ok: false,
      reason: 'busy',
      busyProjectId: 'p1',
    })

    compile.resolve({ pdf, log: '' })
    if (first.ok) await first.done
  })

  it('heartbeats every 60s while compiling and stops at the terminal state', async () => {
    vi.useFakeTimers()
    const compile = deferred<{ pdf: Uint8Array; log: string }>()
    mocks.compileLatexInBrowser.mockReturnValue(compile.promise)

    const done = started()
    await vi.advanceTimersByTimeAsync(0)
    await vi.advanceTimersByTimeAsync(60_000)
    expect(mocks.sendBrowserCompileHeartbeat).toHaveBeenCalledWith('p1', 'job-1')

    compile.resolve({ pdf, log: '' })
    await done
    const callsAtTerminal = mocks.sendBrowserCompileHeartbeat.mock.calls.length
    await vi.advanceTimersByTimeAsync(180_000)
    expect(mocks.sendBrowserCompileHeartbeat).toHaveBeenCalledTimes(callsAtTerminal)
  })

  it('completes a long compile even when the server heartbeat never lands', async () => {
    vi.useFakeTimers()
    mocks.sendBrowserCompileHeartbeat.mockRejectedValue(new Error('offline'))
    const compile = deferred<{ pdf: Uint8Array; log: string }>()
    mocks.compileLatexInBrowser.mockReturnValue(compile.promise)

    const done = started()
    // Past the old 5-minute stall window: the local engine is still the truth.
    await vi.advanceTimersByTimeAsync(6 * 60_000)
    compile.resolve({ pdf, log: '' })
    await done

    expect(projectState()).toMatchObject({ phase: 'success', buildId: 'build-1' })
    expect(mocks.uploadBrowserCompileResult).toHaveBeenCalledTimes(1)
  })

  it('aborts only on explicit cancel and cancels the server job once', async () => {
    mocks.compileLatexInBrowser.mockImplementation((_files: unknown, _root: string, _compiler: string, opts: {
      signal: AbortSignal
    }) => new Promise((_resolve, reject) => {
      opts.signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })
    }))

    const done = started()
    await vi.waitFor(() => expect(projectState()?.jobId).toBe('job-1'))

    await cancelProjectCompile('p1')
    await done

    expect(mocks.cancelCompile).toHaveBeenCalledTimes(1)
    expect(mocks.cancelCompile).toHaveBeenCalledWith('p1', 'job-1')
    expect(mocks.reportBrowserCompileFailure).not.toHaveBeenCalled()
    expect(projectState()).toMatchObject({ phase: 'cancelled', error: null })

    await cancelProjectCompile('p1')
    expect(mocks.cancelCompile).toHaveBeenCalledTimes(1)
  })

  it('only toasts when the user is not on that project page', async () => {
    mocks.compileLatexInBrowser.mockResolvedValue({ pdf, log: '' })

    useCompileStore.getState().setActiveWritingProject('p1')
    await started()
    expect(mocks.toastSuccess).not.toHaveBeenCalled()

    useCompileStore.getState().setActiveWritingProject(null)
    await started()
    expect(mocks.toastSuccess).toHaveBeenCalledWith('《论文一》PDF 编译完成', expect.anything())
  })

  it('keeps the PDF when upload fails and retries without recompiling', async () => {
    mocks.compileLatexInBrowser.mockResolvedValue({ pdf, log: 'fixture log' })
    mocks.uploadBrowserCompileResult.mockRejectedValueOnce(new Error('对象存储不可用'))

    await started()

    expect(projectState()).toMatchObject({ phase: 'failed', pdf })
    expect(projectState()?.pendingUpload?.jobId).toBe('job-1')
    expect(projectState()?.error).toContain('上传或保存编译记录失败')

    mocks.uploadBrowserCompileResult.mockResolvedValueOnce({ build_id: 'build-2', status: 'success' })
    await retryBrowserUpload('p1')

    expect(projectState()).toMatchObject({
      phase: 'success',
      buildId: 'build-2',
      pendingUpload: null,
      retryingUpload: false,
    })
    expect(mocks.compileLatexInBrowser).toHaveBeenCalledTimes(1)
  })

  it('does not report a server failure when no job was created', async () => {
    mocks.createBrowserCompileJob.mockRejectedValueOnce(new Error('网络中断'))

    await started()

    expect(projectState()?.phase).toBe('failed')
    expect(projectState()?.error).toBe('网络中断')
    expect(mocks.reportBrowserCompileFailure).not.toHaveBeenCalled()
  })


  it('reports a sanitized diagnostic when the browser compile fails', async () => {
    mocks.compileLatexInBrowser.mockRejectedValue(
      new Error('! Undefined control sequence.\nl.42 \\secretSourceText\n(./chapters/intro.tex'),
    )

    await started()

    await vi.waitFor(() => expect(mocks.reportCompileDiagnostic).toHaveBeenCalledTimes(1))
    const payload = mocks.reportCompileDiagnostic.mock.calls[0][0]
    expect(payload).toMatchObject({
      project_id: 'p1',
      compile_job_id: 'job-1',
      engine: 'xelatex',
      failure_kind: 'latex',
      failure_stage: 'compile',
      client_kind: 'web',
    })
    expect(payload.log_excerpt).toContain('! Undefined control sequence.')
    expect(payload.log_excerpt).toContain('l.42')
    expect(payload.log_excerpt).not.toContain('secretSourceText')
    expect(payload.log_excerpt).not.toContain('chapters/intro.tex')
    expect(payload.error_message).toBe('! Undefined control sequence.')
    await vi.waitFor(() => expect(projectState()?.diagnosticStatus).toBe('sent'))
    expect(projectState()?.diagnosticId).toBe('diag-1')
  })

  it('does not report a diagnostic when the user cancels', async () => {
    mocks.compileLatexInBrowser.mockImplementation((_files: unknown, _root: string, _compiler: string, opts: {
      signal: AbortSignal
    }) => new Promise((_resolve, reject) => {
      opts.signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })
    }))

    const done = started()
    await vi.waitFor(() => expect(projectState()?.jobId).toBe('job-1'))
    await cancelProjectCompile('p1')
    await done

    expect(mocks.reportCompileDiagnostic).not.toHaveBeenCalled()
    expect(projectState()?.diagnosticStatus).toBeNull()
  })

  it('keeps the compile error visible when diagnostic reporting fails', async () => {
    mocks.compileLatexInBrowser.mockRejectedValue(new Error('LaTeX 引擎没有生成输出文件'))
    mocks.reportCompileDiagnostic.mockRejectedValue(new Error('诊断接口不可用'))

    await started()

    await vi.waitFor(() => expect(projectState()?.diagnosticStatus).toBe('failed'))
    expect(projectState()?.phase).toBe('failed')
    expect(projectState()?.error).toBe('LaTeX 引擎没有生成输出文件')
  })

  it('reports guest failures without project or job links', async () => {
    mocks.compileLatexInBrowser.mockRejectedValue(new Error('HTTP 404'))

    await started({ isGuest: true })

    await vi.waitFor(() => expect(mocks.reportCompileDiagnostic).toHaveBeenCalledTimes(1))
    const payload = mocks.reportCompileDiagnostic.mock.calls[0][0]
    expect(payload.project_id).toBeUndefined()
    expect(payload.compile_job_id).toBeUndefined()
    expect(payload.failure_kind).toBe('resource')
  })

  it('guards tab close only while work is unfinished', async () => {
    const compile = deferred<{ pdf: Uint8Array; log: string }>()
    mocks.compileLatexInBrowser.mockReturnValue(compile.promise)

    const done = started()
    const during = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(during)
    expect(during.defaultPrevented).toBe(true)

    compile.resolve({ pdf, log: '' })
    await done
    const after = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(after)
    expect(after.defaultPrevented).toBe(false)
  })
})
