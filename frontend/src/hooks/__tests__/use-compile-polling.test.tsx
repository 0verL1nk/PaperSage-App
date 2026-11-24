import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ getCompileStatus: vi.fn() }))

vi.mock('@/services/latex-api', () => ({ getCompileStatus: mocks.getCompileStatus }))

import { COMPILE_STATUS_STALL_MS, useCompileStatusPolling } from '../use-compile-polling'

function status(updatedAt: string) {
  return {
    job_id: 'job-1',
    status: 'running',
    build_id: null,
    error: null,
    logs: null,
    root_file: 'main.tex',
    compiler: 'pdflatex',
    elapsed_seconds: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: updatedAt,
  }
}

function renderPolling() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  return renderHook(() => useCompileStatusPolling('project-1', 'job-1'), { wrapper })
}

describe('useCompileStatusPolling', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mocks.getCompileStatus.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('stops polling and reports a stall when updated_at never moves', async () => {
    mocks.getCompileStatus.mockResolvedValue(status('2026-01-01T00:00:00Z'))
    const { result, unmount } = renderPolling()

    await act(async () => { await vi.advanceTimersByTimeAsync(100) })
    expect(result.current.data?.status).toBe('running')
    expect(result.current.isStalled).toBe(false)

    await act(async () => { await vi.advanceTimersByTimeAsync(COMPILE_STATUS_STALL_MS) })
    expect(result.current.isStalled).toBe(true)

    // Give the interval one last tick to observe the stall, then no more requests.
    await act(async () => { await vi.advanceTimersByTimeAsync(5_000) })
    const callsAtStall = mocks.getCompileStatus.mock.calls.length
    await act(async () => { await vi.advanceTimersByTimeAsync(30_000) })
    expect(mocks.getCompileStatus).toHaveBeenCalledTimes(callsAtStall)
    unmount()
  })

  it('keeps waiting while heartbeats move updated_at', async () => {
    let beat = 0
    mocks.getCompileStatus.mockImplementation(async () => {
      beat += 1
      return status(`2026-01-01T00:${String(beat).padStart(2, '0')}:00Z`)
    })
    const { result, unmount } = renderPolling()

    // More wall-clock time than the stall window passes, but every response
    // carries a fresh updated_at, so the job is still alive.
    await act(async () => { await vi.advanceTimersByTimeAsync(COMPILE_STATUS_STALL_MS + 60_000) })
    expect(result.current.isStalled).toBe(false)
    unmount()
  })
})
