import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useAutoSave } from '../use-auto-save'

function createMockMutation() {
  return {
    mutate: vi.fn(),
    mutateAsync: vi.fn().mockResolvedValue(undefined),
    isPending: false,
  }
}

function deferred() {
  let resolve!: () => void
  let reject!: (reason: Error) => void
  const promise = new Promise<void>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

describe('useAutoSave', () => {
  beforeEach(() => { vi.useFakeTimers() })
  afterEach(() => { vi.useRealTimers() })

  it('saves after debounce delay', () => {
    const mutation = createMockMutation()
    const { result } = renderHook(() => useAutoSave(mutation))

    result.current.setActiveFile('main.tex')
    result.current.onChange('new content')

    expect(mutation.mutateAsync).not.toHaveBeenCalled()

    act(() => { vi.advanceTimersByTime(1500) })

    expect(mutation.mutateAsync).toHaveBeenCalledWith({
      filePath: 'main.tex',
      content: 'new content',
    })
  })

  it('does not save identical content', () => {
    const mutation = createMockMutation()
    const { result } = renderHook(() => useAutoSave(mutation))

    result.current.setActiveFile('main.tex')
    result.current.onChange('same')
    act(() => { vi.advanceTimersByTime(1500) })
    expect(mutation.mutateAsync).toHaveBeenCalledTimes(1)

    // Server responds with same content
    result.current.applyServerContent('same')

    // Change to same content again
    result.current.onChange('same')
    act(() => { vi.advanceTimersByTime(1500) })
    expect(mutation.mutateAsync).toHaveBeenCalledTimes(1) // not called again
  })

  // --- Bug 1: File switch race condition ---

  it('does NOT save old content to new file after switching', () => {
    const mutation = createMockMutation()
    const { result } = renderHook(() => useAutoSave(mutation))

    result.current.setActiveFile('A.tex')
    result.current.onChange('content of A')

    // Switch to B before auto-save fires
    act(() => { result.current.switchFile('B.tex') })

    // Advance past auto-save delay — stale timer should NOT fire for A
    act(() => { vi.advanceTimersByTime(2000) })

    // Should have saved A's content to A.tex (on switch), not to B.tex
    expect(mutation.mutateAsync).toHaveBeenCalledWith({
      filePath: 'A.tex',
      content: 'content of A',
    })
    expect(mutation.mutateAsync).toHaveBeenCalledTimes(1)
    // No save to B.tex with A's content
    expect(mutation.mutateAsync).not.toHaveBeenCalledWith(
      expect.objectContaining({ filePath: 'B.tex', content: 'content of A' }),
    )
  })

  // --- Bug 2: Auto-save overwrites later edits ---

  it('saves LATEST content, not the content at schedule time', () => {
    const mutation = createMockMutation()
    const { result } = renderHook(() => useAutoSave(mutation))

    result.current.setActiveFile('main.tex')

    // First edit triggers debounced save
    result.current.onChange('edit 1')

    // User continues editing before timer fires
    result.current.onChange('edit 2')
    result.current.onChange('edit 3 — final')

    // Auto-save fires — should save the latest content
    act(() => { vi.advanceTimersByTime(1500) })

    expect(mutation.mutateAsync).toHaveBeenCalledWith({
      filePath: 'main.tex',
      content: 'edit 3 — final',
    })
    expect(mutation.mutateAsync).toHaveBeenCalledTimes(1)
  })

  it('saves correctly when edits happen between two auto-saves', async () => {
    const mutation = createMockMutation()
    const { result } = renderHook(() => useAutoSave(mutation))

    result.current.setActiveFile('main.tex')
    result.current.onChange('first batch')

    // First auto-save fires
    await act(async () => { vi.advanceTimersByTime(1500) })
    expect(mutation.mutateAsync).toHaveBeenCalledWith({
      filePath: 'main.tex',
      content: 'first batch',
    })

    // User edits again
    result.current.onChange('second batch')

    // Second auto-save should save the new content
    await act(async () => { vi.advanceTimersByTime(1500) })
    expect(mutation.mutateAsync).toHaveBeenCalledWith({
      filePath: 'main.tex',
      content: 'second batch',
    })
  })

  it('applyServerContent returns true and sets content when no local edits', () => {
    const mutation = createMockMutation()
    const { result } = renderHook(() => useAutoSave(mutation))

    const applied = result.current.applyServerContent('server content')

    expect(applied).toBe(true)
    expect(result.current.getContent()).toBe('server content')
  })

  it('applyServerContent returns false when user has local edits', () => {
    const mutation = createMockMutation()
    const { result } = renderHook(() => useAutoSave(mutation))

    result.current.onChange('my local edit')
    const applied = result.current.applyServerContent('server content')

    expect(applied).toBe(false)
    // Content stays as local edit, not overwritten by server
    expect(result.current.getContent()).toBe('my local edit')
  })

  it('saveNow immediately saves and clears timer', async () => {
    const mutation = createMockMutation()
    const { result } = renderHook(() => useAutoSave(mutation))

    result.current.setActiveFile('main.tex')
    result.current.onChange('urgent save')

    // SaveNow bypasses debounce
    await act(async () => { await result.current.saveNow() })

    expect(mutation.mutateAsync).toHaveBeenCalledWith({
      filePath: 'main.tex',
      content: 'urgent save',
    })

    // Timer should be cleared — no double save
    act(() => { vi.advanceTimersByTime(2000) })
    expect(mutation.mutateAsync).toHaveBeenCalledTimes(1)
  })

  it('no-ops saveNow when content matches last saved', async () => {
    const mutation = createMockMutation()
    const { result } = renderHook(() => useAutoSave(mutation))

    result.current.setActiveFile('main.tex')
    result.current.applyServerContent('already saved')

    await act(async () => { await result.current.saveNow() })

    expect(mutation.mutateAsync).not.toHaveBeenCalled()
  })

  it('waits for a pending debounced save before resolving saveNow', async () => {
    const pending = deferred()
    const mutation = createMockMutation()
    mutation.mutateAsync.mockReturnValue(pending.promise)
    const { result } = renderHook(() => useAutoSave(mutation))
    result.current.setActiveFile('main.tex')
    result.current.onChange('edited')
    act(() => { vi.advanceTimersByTime(1500) })

    const finished = vi.fn()
    const saving = result.current.saveNow().then(finished)
    await act(async () => { await Promise.resolve() })
    const completedWhilePending = finished.mock.calls.length
    await act(async () => { pending.resolve(); await saving })

    expect(completedWhilePending).toBe(0)
    expect(mutation.mutateAsync).toHaveBeenCalledTimes(1)
  })

  it('persists final content only after an earlier save settles', async () => {
    const earlier = deferred()
    const final = deferred()
    let persisted = ''
    const mutation = createMockMutation()
    mutation.mutateAsync.mockImplementation(({ content }) =>
      (content === 'earlier' ? earlier.promise : final.promise).then(() => { persisted = content }),
    )
    const { result } = renderHook(() => useAutoSave(mutation))
    result.current.setActiveFile('main.tex')
    result.current.onChange('earlier')
    act(() => { vi.advanceTimersByTime(1500) })
    result.current.onChange('final')
    const saving = result.current.saveNow()
    await act(async () => { await Promise.resolve() })
    const callsBeforeEarlierSettled = mutation.mutateAsync.mock.calls.map(([args]) => args.content)
    await act(async () => { earlier.resolve(); await Promise.resolve() })
    await act(async () => { final.resolve(); await saving })

    expect(callsBeforeEarlierSettled).toEqual(['earlier'])
    expect(mutation.mutateAsync.mock.calls.map(([args]) => args.content)).toEqual(['earlier', 'final'])
    expect(persisted).toBe('final')
  })

  it('saves a reverted edit after the older write settles', async () => {
    const earlier = deferred()
    const mutation = createMockMutation()
    mutation.mutateAsync.mockImplementation(({ content }) => content === 'intermediate' ? earlier.promise : Promise.resolve())
    const { result } = renderHook(() => useAutoSave(mutation))
    result.current.setActiveFile('main.tex')
    result.current.applyServerContent('original')
    result.current.onChange('intermediate')
    act(() => { vi.advanceTimersByTime(1500) })
    result.current.onChange('original')
    const saving = result.current.saveNow()
    await act(async () => { earlier.resolve(); await saving })

    expect(mutation.mutateAsync.mock.calls.map(([args]) => args.content)).toEqual(['intermediate', 'original'])
  })

  it('rejects saveNow when an earlier debounced save failed', async () => {
    const pending = deferred()
    const mutation = createMockMutation()
    mutation.mutateAsync.mockReturnValue(pending.promise)
    const { result } = renderHook(() => useAutoSave(mutation))
    result.current.setActiveFile('main.tex')
    result.current.onChange('unsaved')
    act(() => { vi.advanceTimersByTime(1500) })
    await act(async () => { pending.reject(new Error('disk full')); await Promise.resolve() })

    await expect(result.current.saveNow()).rejects.toThrow('disk full')
  })

  it('reports a pending failure even if a newer queued save succeeds', async () => {
    const earlier = deferred()
    const mutation = createMockMutation()
    mutation.mutateAsync.mockImplementation(({ content }) => content === 'earlier' ? earlier.promise : Promise.resolve())
    const { result } = renderHook(() => useAutoSave(mutation))
    result.current.setActiveFile('main.tex')
    result.current.onChange('earlier')
    act(() => { vi.advanceTimersByTime(1500) })
    result.current.onChange('newer')
    act(() => { vi.advanceTimersByTime(1500) })
    const saving = result.current.saveNow()
    const resultPromise = saving.then(() => null, (reason: unknown) => reason)
    await act(async () => { earlier.reject(new Error('temporary failure')); await resultPromise })

    await expect(resultPromise).resolves.toEqual(expect.objectContaining({ message: expect.stringContaining('temporary failure') }))
    await expect(result.current.saveNow()).resolves.toBeUndefined()
  })

  it('rejects saveNow when its immediate save fails', async () => {
    const mutation = createMockMutation()
    mutation.mutateAsync.mockRejectedValue(new Error('network error'))
    const { result } = renderHook(() => useAutoSave(mutation))
    result.current.setActiveFile('main.tex')
    result.current.onChange('unsaved')

    await expect(result.current.saveNow()).rejects.toThrow('network error')
  })

  it('waits for a previous file save after switching files', async () => {
    const pending = deferred()
    const mutation = createMockMutation()
    mutation.mutateAsync.mockReturnValue(pending.promise)
    const { result } = renderHook(() => useAutoSave(mutation))
    result.current.setActiveFile('A.tex')
    result.current.onChange('A edit')
    act(() => { result.current.switchFile('B.tex') })
    result.current.applyServerContent('B content')

    const finished = vi.fn()
    const saving = result.current.saveNow().then(finished)
    await act(async () => { await Promise.resolve() })
    const completedWhilePending = finished.mock.calls.length
    await act(async () => { pending.resolve(); await saving })

    expect(completedWhilePending).toBe(0)
    expect(mutation.mutateAsync).toHaveBeenCalledWith({ filePath: 'A.tex', content: 'A edit' })
  })

  it('reports a failed previous-file save after switching files', async () => {
    const pending = deferred()
    const mutation = createMockMutation()
    mutation.mutateAsync.mockReturnValue(pending.promise)
    const { result } = renderHook(() => useAutoSave(mutation))
    result.current.setActiveFile('A.tex')
    result.current.onChange('A edit')
    act(() => { result.current.switchFile('B.tex') })
    result.current.applyServerContent('B content')
    await act(async () => { pending.reject(new Error('disk full')); await Promise.resolve() })

    await expect(result.current.saveNow()).rejects.toThrow(/A\.tex.*disk full/)
  })

  it('also saves edits made while saveNow is waiting', async () => {
    const pending = deferred()
    const mutation = createMockMutation()
    mutation.mutateAsync.mockImplementation(({ content }) => content === 'earlier' ? pending.promise : Promise.resolve())
    const { result } = renderHook(() => useAutoSave(mutation))
    result.current.setActiveFile('main.tex')
    result.current.onChange('earlier')
    act(() => { vi.advanceTimersByTime(1500) })
    const saving = result.current.saveNow()
    result.current.onChange('during wait')
    await act(async () => { pending.resolve(); await saving })

    expect(mutation.mutateAsync.mock.calls.map(([args]) => args.content)).toEqual(['earlier', 'during wait'])
  })

  it('saves newer edits after a pending failure but still reports the failure', async () => {
    const pending = deferred()
    const mutation = createMockMutation()
    mutation.mutateAsync.mockReturnValueOnce(pending.promise).mockResolvedValue(undefined)
    const { result } = renderHook(() => useAutoSave(mutation))
    result.current.setActiveFile('main.tex')
    result.current.onChange('earlier')
    act(() => { vi.advanceTimersByTime(1500) })
    const saving = result.current.saveNow().then(() => null, (reason: unknown) => reason)
    result.current.onChange('newer')
    await act(async () => { pending.reject(new Error('temporary failure')); await saving })

    await expect(saving).resolves.toEqual(expect.objectContaining({ message: expect.stringContaining('temporary failure') }))
    expect(mutation.mutateAsync.mock.calls.map(([args]) => args.content)).toEqual(['earlier', 'newer'])
    await expect(result.current.saveNow()).resolves.toBeUndefined()
  })

  it('keeps a reverted draft when its last queued save fails', async () => {
    const earlier = deferred()
    const mutation = createMockMutation()
    mutation.mutateAsync
      .mockReturnValueOnce(earlier.promise)
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new Error('disk full'))
      .mockResolvedValue(undefined)
    const { result } = renderHook(() => useAutoSave(mutation))
    result.current.setActiveFile('A.tex')
    result.current.onChange('one')
    act(() => { vi.advanceTimersByTime(1500) })
    result.current.onChange('two')
    act(() => { vi.advanceTimersByTime(1500) })
    result.current.onChange('one')
    act(() => { vi.advanceTimersByTime(1500) })
    const saving = result.current.saveNow().then(() => null, (reason: unknown) => reason)
    await act(async () => { earlier.resolve(); await saving })
    await expect(saving).resolves.toEqual(expect.objectContaining({ message: expect.stringContaining('disk full') }))

    act(() => { result.current.switchFile('B.tex') })
    result.current.applyServerContent('B content')
    act(() => { result.current.switchFile('A.tex') })
    expect(result.current.getContent()).toBe('one')
    await expect(result.current.saveNow()).resolves.toBeUndefined()
  })

  it('allows a failed active-file save to be retried', async () => {
    const mutation = createMockMutation()
    mutation.mutateAsync.mockRejectedValueOnce(new Error('disk full')).mockResolvedValue(undefined)
    const { result } = renderHook(() => useAutoSave(mutation))
    result.current.setActiveFile('main.tex')
    result.current.onChange('unsaved')

    await expect(result.current.saveNow()).rejects.toThrow('disk full')
    await expect(result.current.saveNow()).resolves.toBeUndefined()
    expect(mutation.mutateAsync).toHaveBeenCalledTimes(2)
    expect(mutation.mutateAsync).toHaveBeenLastCalledWith({ filePath: 'main.tex', content: 'unsaved' })
  })

  it('restores a failed previous-file draft when returning to it', async () => {
    const pending = deferred()
    const mutation = createMockMutation()
    mutation.mutateAsync.mockReturnValueOnce(pending.promise).mockResolvedValue(undefined)
    const { result } = renderHook(() => useAutoSave(mutation))
    result.current.setActiveFile('A.tex')
    result.current.onChange('A unsaved draft')
    act(() => { result.current.switchFile('B.tex') })
    result.current.applyServerContent('B content')
    await act(async () => { pending.reject(new Error('disk full')); await Promise.resolve() })
    await expect(result.current.saveNow()).rejects.toThrow(/A\.tex.*disk full/)

    act(() => { result.current.switchFile('A.tex') })
    expect(result.current.getContent()).toBe('A unsaved draft')
    expect(result.current.applyServerContent('A stale server content')).toBe(false)
    await expect(result.current.saveNow()).resolves.toBeUndefined()
    expect(mutation.mutateAsync).toHaveBeenLastCalledWith({ filePath: 'A.tex', content: 'A unsaved draft' })
  })

  it('no-ops saveNow when no active file', async () => {
    const mutation = createMockMutation()
    const { result } = renderHook(() => useAutoSave(mutation))

    result.current.onChange('orphan content')

    await act(async () => { await result.current.saveNow() })

    expect(mutation.mutateAsync).not.toHaveBeenCalled()
  })
})
