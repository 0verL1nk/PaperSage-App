import { useEffect } from 'react'
import { toast } from 'sonner'

import { flushPendingWrites, folderSync } from '@/lib/folder-sync'
import { isDesktop } from '@/lib/desktop'
import { fetchLatexFileContent } from '@/services/latex-api'
import { useFolderSyncStore } from '@/store/folder-sync-store'

/**
 * Keeps the per-file sync state for one project up to date: replays any queued
 * app-to-disk writes, takes one scan, then follows the filesystem watcher.
 */
export function useFolderSyncStatus(projectId: string): void {
  const setSnapshot = useFolderSyncStore((state) => state.setSnapshot)
  const setChanges = useFolderSyncStore((state) => state.setChanges)

  useEffect(() => {
    if (!isDesktop() || !projectId) return
    let alive = true
    void (async () => {
      try {
        const info = await folderSync.info(projectId)
        if (!info.bound) {
          if (alive) setSnapshot(projectId, false, false, [])
          return
        }
        if (!info.paused) {
          const written = await flushPendingWrites(projectId, async (path) => {
            const content = await fetchLatexFileContent(projectId, path, { skipGlobalLoading: true })
            return content.content
          })
          if (alive && written > 0) toast.success(`已把 ${written} 个项目改动写入本机`)
        }
        const changes = info.paused ? [] : (await folderSync.scan(projectId)) ?? []
        if (alive) setSnapshot(projectId, true, info.paused, changes)
      } catch {
        // A missing binding or backend is not an error for the editor.
      }
    })()
    const off = folderSync.onChanges((changedProject, changes) => {
      if (alive && changedProject === projectId) setChanges(projectId, changes)
    })
    return () => {
      alive = false
      off()
    }
  }, [projectId, setSnapshot, setChanges])
}
