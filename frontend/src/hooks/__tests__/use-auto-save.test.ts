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

describe('useAutoSave', () => {
  beforeEach(() => { vi.useFakeTimers() })
  afterEach(() => { vi.useRealTimers() })

  it('saves after debounce delay', () => {
    const mutation = createMockMutation()
    const { result } = renderHook(() => useAutoSave(mutation))

    result.current.setActiveFile('main.tex')
    result.current.onChange('new content')

    expect(mutation.mutate).not.toHaveBeenCalled()

    act(() => { vi.advanceTimersByTime(1500) })

    expect(mutation.mutate).toHaveBeenCalledWith({
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
    expect(mutation.mutate).toHaveBeenCalledTimes(1)

    // Server responds with same content
    result.current.applyServerContent('same')

    // Change to same content again
    result.current.onChange('same')
    act(() => { vi.advanceTimersByTime(1500) })
    expect(mutation.mutate).toHaveBeenCalledTimes(1) // not called again
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
    expect(mutation.mutate).toHaveBeenCalledWith({
      filePath: 'A.tex',
      content: 'content of A',
    })
    expect(mutation.mutate).toHaveBeenCalledTimes(1)
    // No save to B.tex with A's content
    expect(mutation.mutate).not.toHaveBeenCalledWith(
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

    expect(mutation.mutate).toHaveBeenCalledWith({
      filePath: 'main.tex',
      content: 'edit 3 — final',
    })
    expect(mutation.mutate).toHaveBeenCalledTimes(1)
  })

  it('saves correctly when edits happen between two auto-saves', () => {
    const mutation = createMockMutation()
    const { result } = renderHook(() => useAutoSave(mutation))

    result.current.setActiveFile('main.tex')
    result.current.onChange('first batch')

    // First auto-save fires
    act(() => { vi.advanceTimersByTime(1500) })
    expect(mutation.mutate).toHaveBeenCalledWith({
      filePath: 'main.tex',
      content: 'first batch',
    })

    // User edits again
    result.current.onChange('second batch')

    // Second auto-save should save the new content
    act(() => { vi.advanceTimersByTime(1500) })
    expect(mutation.mutate).toHaveBeenCalledWith({
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
    expect(mutation.mutate).not.toHaveBeenCalled()
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

  it('no-ops saveNow when no active file', async () => {
    const mutation = createMockMutation()
    const { result } = renderHook(() => useAutoSave(mutation))

    result.current.onChange('orphan content')

    await act(async () => { await result.current.saveNow() })

    expect(mutation.mutateAsync).not.toHaveBeenCalled()
  })
})
