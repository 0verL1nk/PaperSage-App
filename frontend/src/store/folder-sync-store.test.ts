import { beforeEach, describe, expect, it } from 'vitest'

import { fileSyncStatus, useFolderSyncStore } from '@/store/folder-sync-store'

const PROJECT = 'p1'
const state = () => useFolderSyncStore.getState()

describe('folder sync status', () => {
  beforeEach(() => {
    state().reset()
    useFolderSyncStore.setState({ pending: {} })
    window.localStorage.clear()
  })

  it('reports the most specific state first', () => {
    state().setSnapshot(PROJECT, true, false, [
      { kind: 'modified', path: 'main.tex' },
      { kind: 'deleted', path: 'old.tex' },
    ])
    expect(fileSyncStatus(state(), 'main.tex')).toBe('modified')
    expect(fileSyncStatus(state(), 'old.tex')).toBe('deleted')
    expect(fileSyncStatus(state(), 'untouched.tex')).toBe('synced')

    state().markPending(PROJECT, 'main.tex')
    expect(fileSyncStatus(state(), 'main.tex')).toBe('pending')

    state().markConflict(PROJECT, 'main.tex')
    expect(fileSyncStatus(state(), 'main.tex')).toBe('conflict')

    state().clearConflict(PROJECT, 'main.tex')
    expect(fileSyncStatus(state(), 'main.tex')).toBe('pending')
    state().clearPending(PROJECT, 'main.tex')
    expect(fileSyncStatus(state(), 'main.tex')).toBe('modified')
  })

  it('marks every file paused while the binding is paused', () => {
    state().setSnapshot(PROJECT, true, true, [])
    expect(fileSyncStatus(state(), 'main.tex')).toBe('paused')
  })

  it('queues writes in order without duplicates and survives project switches', () => {
    state().setSnapshot(PROJECT, true, true, [])
    state().markPending(PROJECT, 'b.tex')
    state().markPending(PROJECT, 'a.tex')
    state().markPending(PROJECT, 'b.tex')
    expect(state().pending[PROJECT]).toEqual(['b.tex', 'a.tex'])

    state().setSnapshot('p2', true, false, [])
    expect(state().pending[PROJECT]).toEqual(['b.tex', 'a.tex'])
    expect(fileSyncStatus({ ...state(), projectId: PROJECT }, 'b.tex')).toBe('pending')
  })

  it('ignores conflicts for a project that is not open but keeps its queue', () => {
    state().setSnapshot(PROJECT, true, false, [])
    state().markConflict('other', 'main.tex')
    state().markPending('other', 'main.tex')
    expect(state().conflicts).toEqual({})
    expect(state().pending.other).toEqual(['main.tex'])
  })

  it('persists the queue so a restart can replay it', () => {
    state().markPending(PROJECT, 'main.tex')
    const stored = JSON.parse(window.localStorage.getItem('papersage_sync_queue') ?? '{}')
    expect(stored.state.pending[PROJECT]).toEqual(['main.tex'])
  })
})
