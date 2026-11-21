import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createElement, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  save: vi.fn(),
  call: vi.fn(),
}))

vi.mock('@/services/latex-api', () => ({ saveLatexFile: mocks.save }))
vi.mock('@/lib/desktop', () => ({ isDesktop: () => true }))
vi.mock('@wailsio/runtime', () => ({
  Call: { ByName: mocks.call },
  Events: { On: vi.fn() },
}))

import { useAutoSave } from '../use-auto-save'
import { useSaveLatexFile } from '../use-latex'
import { prepareForUpdate, PREPARE_UPDATE_EVENT } from '@/lib/update-prepare'
import { useFolderSyncStore } from '@/store/folder-sync-store'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

function renderEditorSave() {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children)
  const hook = renderHook(() => useAutoSave(useSaveLatexFile('project-1')), { wrapper })
  act(() => {
    hook.result.current.setActiveFile('main.tex')
    hook.result.current.onChange('edited content')
  })
  const listener = (event: WindowEventMap[typeof PREPARE_UPDATE_EVENT]) => {
    event.detail.waitUntil(hook.result.current.saveNow())
  }
  window.addEventListener(PREPARE_UPDATE_EVENT, listener)
  return () => {
    window.removeEventListener(PREPARE_UPDATE_EVENT, listener)
    hook.unmount()
  }
}

describe('editor save and folder sync before update', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    window.localStorage.removeItem('papersage_sync_queue')
    useFolderSyncStore.setState({ projectId: 'project-1', conflicts: {}, pending: {} })
    mocks.save.mockResolvedValue(undefined)
  })

  it('waits for both folder discovery and the disk write after the API save', async () => {
    const info = deferred<{ bound: boolean; paused: boolean; files: number }>()
    const write = deferred<unknown>()
    mocks.call.mockImplementation((method: string) => method.endsWith('.Info') ? info.promise : write.promise)
    const dispose = renderEditorSave()
    try {
      const finished = vi.fn()
      const preparation = prepareForUpdate().then(finished)
      await waitFor(() => expect(mocks.call).toHaveBeenCalledWith(expect.stringContaining('.Info'), 'project-1'))
      await act(async () => { await Promise.resolve(); await Promise.resolve() })
      expect(mocks.save).toHaveBeenCalledWith('project-1', 'main.tex', 'edited content', { skipGlobalLoading: true })
      expect(finished).not.toHaveBeenCalled()

      info.resolve({ bound: true, paused: false, files: 1 })
      await waitFor(() => expect(mocks.call).toHaveBeenCalledWith(
        expect.stringContaining('.Write'), 'project-1', 'main.tex', btoa('edited content'),
      ))
      expect(finished).not.toHaveBeenCalled()
      write.resolve({})
      await preparation
      expect(finished).toHaveBeenCalledTimes(1)
    } finally {
      info.resolve({ bound: true, paused: false, files: 1 })
      write.resolve({})
      dispose()
    }
  })

  it('waits until a failed disk write is persisted in the retry queue', async () => {
    const write = deferred<unknown>()
    mocks.call.mockImplementation((method: string) => method.endsWith('.Info')
      ? Promise.resolve({ bound: true, paused: false, files: 1 })
      : write.promise)
    const dispose = renderEditorSave()
    try {
      const finished = vi.fn()
      const preparation = prepareForUpdate().then(finished)
      await waitFor(() => expect(mocks.call).toHaveBeenCalledWith(
        expect.stringContaining('.Write'), 'project-1', 'main.tex', btoa('edited content'),
      ))
      await act(async () => { await Promise.resolve(); await Promise.resolve() })
      expect(finished).not.toHaveBeenCalled()
      write.reject(new Error('disk unavailable'))
      await preparation
      expect(useFolderSyncStore.getState().pending['project-1']).toEqual(['main.tex'])
      const persisted = JSON.parse(window.localStorage.getItem('papersage_sync_queue') ?? '{}') as {
        state?: { pending?: Record<string, string[]> }
      }
      expect(persisted.state?.pending?.['project-1']).toEqual(['main.tex'])
    } finally {
      write.resolve({})
      dispose()
    }
  })

  it('preserves a conflict without overwriting the local file', async () => {
    const write = deferred<unknown>()
    mocks.call.mockImplementation((method: string) => method.endsWith('.Info')
      ? Promise.resolve({ bound: true, paused: false, files: 1 })
      : write.promise)
    const dispose = renderEditorSave()
    try {
      const preparation = prepareForUpdate()
      await waitFor(() => expect(mocks.call).toHaveBeenCalledWith(
        expect.stringContaining('.Write'), 'project-1', 'main.tex', btoa('edited content'),
      ))
      write.resolve({ conflict: { path: 'main.tex', localHash: 'local', remoteHash: 'remote' } })
      await preparation
      expect(useFolderSyncStore.getState().conflicts['main.tex']).toBe(true)
      expect(mocks.call).not.toHaveBeenCalledWith(expect.stringContaining('.MarkSynced'), expect.anything(), expect.anything())
    } finally {
      write.resolve({})
      dispose()
    }
  })

  it('rejects update preparation when the editor API save fails', async () => {
    mocks.save.mockRejectedValueOnce(new Error('API save failed'))
    const dispose = renderEditorSave()
    try {
      await expect(prepareForUpdate()).rejects.toThrow('API save failed')
      expect(mocks.call).not.toHaveBeenCalled()
    } finally {
      dispose()
    }
  })
})
