import { Call, Events } from '@wailsio/runtime'
import { toast } from 'sonner'
import { useFolderSyncStore } from '@/store/folder-sync-store'
import { isDesktop } from './desktop'

const SYNC_SERVICE_FQN =
  'github.com/0verL1nk/GraduateProject/desktop/internal/services.SyncService'

export type SyncChangeKind = 'created' | 'modified' | 'deleted' | 'renamed'

export interface SyncChange {
  kind: SyncChangeKind
  path: string
  oldPath?: string
  hash?: string
  size?: number
}

export interface SyncBindingInfo {
  bound: boolean
  root?: string
  paused: boolean
  files: number
}

export interface SyncConflict {
  path: string
  baseHash?: string
  localHash: string
  remoteHash: string
}

export interface SyncWriteResult {
  conflict?: SyncConflict
}

async function callSync<T>(method: string, ...args: unknown[]): Promise<T> {
  if (!isDesktop()) throw new Error('仅桌面客户端支持本地目录同步')
  return (await Call.ByName(`${SYNC_SERVICE_FQN}.${method}`, ...args)) as T
}

export function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  const chunk = 0x8000
  for (let offset = 0; offset < bytes.length; offset += chunk) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + chunk))
  }
  return btoa(binary)
}

export function textToBase64(text: string): string {
  return bytesToBase64(new TextEncoder().encode(text))
}

export function base64ToText(data: string): string {
  const binary = atob(data)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i)
  return new TextDecoder().decode(bytes)
}

export const folderSync = {
  info: (projectId: string) => callSync<SyncBindingInfo>('Info', projectId),
  bind: (projectId: string, root: string) => callSync<void>('Bind', projectId, root),
  unbind: (projectId: string) => callSync<void>('Unbind', projectId),
  setPaused: (projectId: string, paused: boolean) => callSync<void>('SetPaused', projectId, paused),
  scan: (projectId: string) => callSync<SyncChange[] | null>('Scan', projectId),
  read: (projectId: string, rel: string) => callSync<string>('Read', projectId, rel),
  write: (projectId: string, rel: string, dataBase64: string) =>
    callSync<SyncWriteResult>('Write', projectId, rel, dataBase64),
  markSynced: (projectId: string, rel: string) => callSync<void>('MarkSynced', projectId, rel),
  watch: (projectId: string) => callSync<void>('Watch', projectId),
  unwatch: (projectId: string) => callSync<void>('Unwatch', projectId),
  onChanges: (handler: (projectId: string, changes: SyncChange[]) => void): (() => void) => {
    if (!isDesktop()) return () => undefined
    const off = Events.On('desktop:sync-change', (event) => {
      const data = event.data as { projectId?: string; changes?: SyncChange[] } | null
      if (data?.projectId && Array.isArray(data.changes)) handler(data.projectId, data.changes)
    })
    return typeof off === 'function' ? off : () => undefined
  },
}

/**
 * Best-effort push of an editor save into the bound folder. Conflicts are
 * surfaced instead of overwriting the newer local file.
 */
export async function pushFileToDisk(projectId: string, filePath: string, content: string): Promise<void> {
  if (!isDesktop()) return
  try {
    const info = await folderSync.info(projectId)
    if (!info.bound) return
    if (info.paused) {
      // Paused bindings keep the save in the app; the dialog retries it.
      useFolderSyncStore.getState().markPending(projectId, filePath)
      return
    }
    const result = await folderSync.write(projectId, filePath, textToBase64(content))
    if (result?.conflict) {
      useFolderSyncStore.getState().markConflict(projectId, filePath)
      toast.warning(`本机文件已被修改：${filePath}`, {
        description: '云端已保存。可直接用云端版本覆盖本机。',
        action: {
          label: '覆盖本机',
          onClick: () => {
            void folderSync.markSynced(projectId, filePath)
              .then(() => folderSync.write(projectId, filePath, textToBase64(content)))
              .then(() => {
                useFolderSyncStore.getState().clearConflict(projectId, filePath)
                useFolderSyncStore.getState().clearPending(projectId, filePath)
              })
              .catch(() => toast.error('覆盖本机失败'))
          },
        },
      })
      return
    }
    useFolderSyncStore.getState().clearConflict(projectId, filePath)
    useFolderSyncStore.getState().clearPending(projectId, filePath)
  } catch {
    // Sync is best-effort; the editor save already succeeded.
    useFolderSyncStore.getState().markPending(projectId, filePath)
  }
}

/**
 * Replays queued app-to-disk writes in order. Entries that conflict or fail
 * stay in the queue for the next resume or manual sync.
 */
export async function flushPendingWrites(
  projectId: string,
  readContent: (path: string) => Promise<string>,
): Promise<number> {
  if (!isDesktop()) return 0
  const store = useFolderSyncStore.getState()
  const queue = [...(store.pending[projectId] ?? [])]
  let written = 0
  for (const path of queue) {
    try {
      const content = await readContent(path)
      const result = await folderSync.write(projectId, path, textToBase64(content))
      if (result?.conflict) {
        store.markConflict(projectId, path)
        continue
      }
      store.clearPending(projectId, path)
      written += 1
    } catch {
      // Keep the entry queued; the next resume or manual sync retries it.
    }
  }
  return written
}
