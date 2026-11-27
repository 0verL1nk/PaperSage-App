import { beforeEach, describe, expect, it } from 'vitest'

import { hasUnfinishedCompile, isCompileActive, useCompileStore } from './compile-store'

const emptyPdf = new Uint8Array([1, 2, 3])

describe('compile store', () => {
  beforeEach(() => {
    useCompileStore.getState().reset()
  })

  it('keeps compile state isolated per project', () => {
    const store = useCompileStore.getState()
    store.begin('p1', '论文一')
    store.begin('p2', '论文二')
    store.patch('p1', { phase: 'compiling', progressMessage: '第二遍编译' })

    expect(useCompileStore.getState().projects.p1).toMatchObject({
      phase: 'compiling',
      projectName: '论文一',
      progressMessage: '第二遍编译',
    })
    expect(useCompileStore.getState().projects.p2).toMatchObject({
      phase: 'preparing',
      projectName: '论文二',
    })
  })

  it('only treats preparing/compiling/uploading as active', () => {
    expect(isCompileActive('preparing')).toBe(true)
    expect(isCompileActive('compiling')).toBe(true)
    expect(isCompileActive('uploading')).toBe(true)
    expect(isCompileActive('success')).toBe(false)
    expect(isCompileActive('failed')).toBe(false)
    expect(isCompileActive('cancelled')).toBe(false)
    expect(isCompileActive(undefined)).toBe(false)
  })

  it('starts a new compile from a clean slate', () => {
    const store = useCompileStore.getState()
    const before = Date.now()
    store.begin('p1', '论文一')
    store.patch('p1', {
      phase: 'failed',
      error: '旧错误',
      pendingUpload: { jobId: 'job-old', pdf: emptyPdf, logs: 'old' },
      resources: { 'font:x': { name: 'x', kind: 'font', state: 'missing' } },
      stage: 'compile',
      formatCacheHit: false,
    })
    store.begin('p1', '论文一')

    const fresh = useCompileStore.getState().projects.p1
    expect(fresh).toMatchObject({
      phase: 'preparing',
      error: null,
      logs: null,
      pendingUpload: null,
      pdf: null,
      resources: {},
      stage: 'prepare',
      formatCacheHit: null,
    })
    // 进度视图按开始时间实时算耗时，新的编译必须重置计时起点。
    expect(fresh.startedAt).toBeGreaterThanOrEqual(before)
    expect(fresh.startedAt).toBeLessThanOrEqual(Date.now())
  })

  it('ignores patches for projects that never started', () => {
    useCompileStore.getState().patch('missing', { phase: 'success' })
    expect(useCompileStore.getState().projects.missing).toBeUndefined()
  })

  it('flags unfinished work while compiling or holding an unsaved PDF', () => {
    const state = () => useCompileStore.getState()

    expect(hasUnfinishedCompile(state())).toBe(false)

    state().begin('p1', '论文一')
    expect(hasUnfinishedCompile(state())).toBe(true)

    state().patch('p1', { phase: 'failed', pendingUpload: { jobId: 'job-1', pdf: emptyPdf, logs: '' } })
    expect(hasUnfinishedCompile(state())).toBe(true)

    state().patch('p1', { phase: 'failed', pendingUpload: null })
    expect(hasUnfinishedCompile(state())).toBe(false)
  })
})
