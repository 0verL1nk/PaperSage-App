import { create } from 'zustand'
import { persist } from 'zustand/middleware'

import type { SyncChange, SyncChangeKind } from '@/lib/folder-sync'

/**
 * Per-file sync state for the project open in the writing page. `pending` is an
 * ordered queue per project: editor saves that could not be written to disk
 * (paused binding, conflict, or a failed write), retried in order.
 */
export type FileSyncStatus = 'synced' | 'paused' | 'conflict' | 'pending' | SyncChangeKind

interface FolderSyncState {
  projectId: string | null
  bound: boolean
  paused: boolean
  changes: Record<string, SyncChangeKind>
  conflicts: Record<string, true>
  pending: Record<string, string[]>
  setBinding: (projectId: string, bound: boolean, paused: boolean) => void
  setSnapshot: (projectId: string, bound: boolean, paused: boolean, changes: SyncChange[]) => void
  setChanges: (projectId: string, changes: SyncChange[]) => void
  markConflict: (projectId: string, path: string) => void
  clearConflict: (projectId: string, path: string) => void
  markPending: (projectId: string, path: string) => void
  clearPending: (projectId: string, path: string) => void
  reset: () => void
}

function freshProject() {
  return {
    bound: false,
    paused: false,
    changes: {} as Record<string, SyncChangeKind>,
    conflicts: {} as Record<string, true>,
  }
}

function changeMap(changes: SyncChange[]): Record<string, SyncChangeKind> {
  const out: Record<string, SyncChangeKind> = {}
  for (const change of changes) out[change.path] = change.kind
  return out
}

export const useFolderSyncStore = create<FolderSyncState>()(persist((set) => ({
  projectId: null,
  ...freshProject(),
  pending: {},
  setBinding: (projectId, bound, paused) =>
    set((state) => (state.projectId === projectId
      ? { bound, paused }
      : { projectId, ...freshProject(), bound, paused })),
  setSnapshot: (projectId, bound, paused, changes) =>
    set((state) => ({
      projectId,
      ...(state.projectId === projectId ? {} : freshProject()),
      bound,
      paused,
      changes: changeMap(changes),
    })),
  setChanges: (projectId, changes) =>
    set((state) => (state.projectId === projectId ? { changes: changeMap(changes) } : state)),
  markConflict: (projectId, path) =>
    set((state) => (state.projectId === projectId
      ? { conflicts: { ...state.conflicts, [path]: true } }
      : state)),
  clearConflict: (projectId, path) =>
    set((state) => {
      if (state.projectId !== projectId) return state
      const conflicts = { ...state.conflicts }
      delete conflicts[path]
      return { conflicts }
    }),
  markPending: (projectId, path) =>
    set((state) => {
      const queue = state.pending[projectId] ?? []
      if (queue.includes(path)) return state
      return { pending: { ...state.pending, [projectId]: [...queue, path] } }
    }),
  clearPending: (projectId, path) =>
    set((state) => {
      const queue = state.pending[projectId]
      if (!queue?.includes(path)) return state
      const next = queue.filter((entry) => entry !== path)
      const pending = { ...state.pending }
      if (next.length > 0) pending[projectId] = next
      else delete pending[projectId]
      return { pending }
    }),
  reset: () => set({ projectId: null, ...freshProject() }),
}), {
  name: 'papersage_sync_queue',
  partialize: (state) => ({ projectId: state.projectId, pending: state.pending }),
}))

export function fileSyncStatus(
  state: Pick<FolderSyncState, 'projectId' | 'paused' | 'conflicts' | 'pending' | 'changes'>,
  path: string,
): FileSyncStatus {
  if (state.conflicts[path]) return 'conflict'
  if (state.projectId && state.pending[state.projectId]?.includes(path)) return 'pending'
  if (state.paused) return 'paused'
  return state.changes[path] ?? 'synced'
}

export function useFileSyncStatus(path: string): FileSyncStatus {
  return useFolderSyncStore((state) => (state.projectId ? fileSyncStatus(state, path) : 'synced'))
}
