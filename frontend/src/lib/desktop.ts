/**
 * Wails v3 desktop bridge.
 *
 * Wails injects `window._wails` / `window.wails` and the `/wails/runtime`
 * endpoint before the app loads. Generated bindings are not checked into this
 * frontend yet, so native calls use the same wire format as the generated
 * `Call.ByName` helper (object 0 / method 0 / `<pkg>.<Type>.<Method>`).
 *
 * Every entry point degrades to null/no-op in a plain browser.
 */

import { getDesktopLocalApiBaseUrl, setDesktopLocalApiBaseUrl, setDesktopLocalBackendError } from './env'

import { Call, Events } from '@wailsio/runtime'
import { toast } from 'sonner'

const DESKTOP_SERVICE_FQN =
  'github.com/0verL1nk/GraduateProject/desktop/internal/services.DesktopService'

const SAFE_EXTERNAL_PROTOCOLS = new Set(['http:', 'https:', 'mailto:'])

type WailsWindow = Window & {
  _wails?: {
    environment?: { OS?: string; Arch?: string; Debug?: boolean }
    flags?: Record<string, unknown>
    clientId?: string
  }
  wails?: unknown
}

export interface DesktopAppInfo {
  version?: string
  platform?: string
  isDesktop?: boolean
  localApiBaseUrl?: string
}

export interface DroppedFile {
  name: string
  path: string
  data: string
}

export interface DesktopUpdate {
  version: string
  name?: string
  notes?: string
}

export interface DesktopService {
  appInfo(): Promise<DesktopAppInfo | null>
  notify(title: string, body: string): Promise<void>
  openExternal(url: string): Promise<void>
  pickFolder(title: string): Promise<string | null>
  pickFiles(title: string, allowMultiple?: boolean): Promise<string[] | null>
  /** Picks a destination path only; it does not write file contents. */
  saveFile(defaultName: string): Promise<string | null>
  readFiles(paths: string[]): Promise<DroppedFile[] | null>
  readFolder(root: string): Promise<DroppedFile[] | null>
  saveFileBytes(defaultName: string, dataBase64: string): Promise<string | null>
  consumeDeepLink(): Promise<string | null>
  revealInFolder(path: string): Promise<void>
  windowMinimise(): Promise<void>
  windowToggleMaximise(): Promise<void>
  windowClose(): Promise<void>
  windowIsMaximised(): Promise<boolean>
  windowToggleFullscreen(): Promise<void>
  checkForUpdates(): Promise<DesktopUpdate | null>
  downloadUpdate(): Promise<void>
  installUpdate(): Promise<void>
  discardUpdate(): Promise<void>
  localBackendError(): Promise<string | null>
  quitApp(): Promise<void>
}

function wailsWindow(): WailsWindow | null {
  return typeof window === 'undefined' ? null : (window as WailsWindow)
}

let desktopConfirmed = false

/** DEV-only preview: `?desktop=1` renders the Wails shell in a plain browser. */
function desktopPreviewEnabled(): boolean {
  if (!import.meta.env.DEV || typeof window === 'undefined') return false
  return new URLSearchParams(window.location.search).get('desktop') === '1'
}

/** True only inside the Wails webview. The npm runtime also creates
 * `window._wails.clientId` in a browser, so a successful native call is the
 * reliable signal; the injected environment is a synchronous fallback. */
export function isDesktop(): boolean {
  if (desktopPreviewEnabled()) return true
  const w = wailsWindow()
  if (!w) return false
  return desktopConfirmed || Boolean(w._wails?.environment)
}

export function getDesktopPlatform(): string | null {
  return wailsWindow()?._wails?.environment?.OS ?? null
}

/** Windows uses a frameless window owned by the React titlebar. */
export function hasCustomTitlebar(): boolean {
  if (!isDesktop()) return false
  const platform = getDesktopPlatform()
  if (platform) return platform.toLowerCase() === 'windows'
  if (desktopPreviewEnabled()) return true
  // The environment object can arrive after the first render; the WebView2 UA
  // is stable at startup.
  return typeof navigator !== 'undefined' && /Windows/i.test(navigator.userAgent)
}

/**
 * Calls a bound Go service method through the official Wails runtime.
 * Returns null in a browser or when the bridge rejects the call.
 */
async function callDesktop<T>(method: string, args: unknown[] = []): Promise<T | null> {
  if (!isDesktop()) return null
  // The ?desktop=1 preview has no Go bridge; keep the UI inert instead of
  // letting the runtime package attempt (and log) failing calls.
  if (desktopPreviewEnabled() && !wailsWindow()?._wails) return null
  try {
    return (await Call.ByName(`${DESKTOP_SERVICE_FQN}.${method}`, ...args)) as T
  } catch {
    return null
  }
}

let cachedService: DesktopService | null = null

/** Native bridge, or null in a browser. */
export function getDesktopService(): DesktopService | null {  if (!isDesktop()) return null
  cachedService ??= {
    appInfo: () => callDesktop<DesktopAppInfo>('AppInfo'),
    notify: async (title, body) => {
      await callDesktop('Notify', [title, body])
    },
    openExternal: async (url) => {
      await callDesktop('OpenExternal', [url])
    },
    pickFolder: async (title) => {
      const result = await callDesktop<string>('PickFolder', [title])
      return result || null
    },
    pickFiles: async (title, allowMultiple = false) => {
      const result = await callDesktop<string[]>('PickFiles', [title, allowMultiple])
      return Array.isArray(result) ? result : null
    },
    saveFile: async (defaultName) => {
      const result = await callDesktop<string>('SaveFile', [defaultName])
      return result || null
    },
    readFiles: async (paths) => {
      const result = await callDesktop<DroppedFile[]>('ReadFiles', [paths])
      return Array.isArray(result) ? result : null
    },
    readFolder: async (root) => {
      const result = await callDesktop<DroppedFile[]>('ReadFolder', [root])
      return Array.isArray(result) ? result : null
    },
    saveFileBytes: async (defaultName, dataBase64) => {
      const result = await callDesktop<string>('SaveFileBytes', [defaultName, dataBase64])
      return result || null
    },
    consumeDeepLink: async () => {
      const result = await callDesktop<string>('ConsumeDeepLink')
      return result || null
    },
    revealInFolder: async (path) => {
      await callDesktop('RevealInFolder', [path])
    },
    windowMinimise: async () => {
      await callDesktop('WindowMinimise')
    },
    windowToggleMaximise: async () => {
      await callDesktop('WindowToggleMaximise')
    },
    windowClose: async () => {
      await callDesktop('WindowClose')
    },
    windowIsMaximised: async () => Boolean(await callDesktop<boolean>('WindowIsMaximised')),
    windowToggleFullscreen: async () => {
      await callDesktop('WindowToggleFullscreen')
    },
    checkForUpdates: async () => (await Call.ByName(`${DESKTOP_SERVICE_FQN}.CheckForUpdates`)) as DesktopUpdate | null,
    downloadUpdate: async () => { await Call.ByName(`${DESKTOP_SERVICE_FQN}.DownloadUpdate`) },
    installUpdate: async () => { await Call.ByName(`${DESKTOP_SERVICE_FQN}.InstallUpdate`) },
    discardUpdate: async () => { await Call.ByName(`${DESKTOP_SERVICE_FQN}.DiscardUpdate`) },
    localBackendError: async () => callDesktop<string>('LocalBackendError'),
    quitApp: async () => {
      await callDesktop('QuitApp')
    },
  }
  return cachedService
}

let initialized: Promise<void> | null = null

/**
 * Resolve the supervised backend address before the first API request. Safe to
 * call multiple times; a browser resolves immediately.
 */
export function initDesktop(): Promise<void> {
  if (initialized) return initialized
  initialized = (async () => {
    if (!wailsWindow()?._wails) return
    try {
      const info = (await Call.ByName(`${DESKTOP_SERVICE_FQN}.AppInfo`)) as DesktopAppInfo
      if (!info?.isDesktop) return
      desktopConfirmed = true
      if (typeof info.localApiBaseUrl === 'string') setDesktopLocalApiBaseUrl(info.localApiBaseUrl)
      let backendError: unknown
      try {
        backendError = await Call.ByName(`${DESKTOP_SERVICE_FQN}.LocalBackendError`)
      } catch {
        backendError = null
      }
      const message = (typeof backendError === 'string' && backendError.trim()) ||
        (!getDesktopLocalApiBaseUrl() ? '无法确认本地后端状态，请重启桌面应用后重试。' : '')
      if (message) {
        setDesktopLocalBackendError(message)
        toast.error('本地后端不可用', { description: message, duration: 12_000 })
      }
    } catch {
      // Running in a browser: the runtime package exists but the Go bridge does not.
    }
  })()
  return initialized
}

export function isSafeExternalUrl(url: string): boolean {  try {
    return SAFE_EXTERNAL_PROTOCOLS.has(new URL(url).protocol)
  } catch {
    return false
  }
}

/** Opens http/https/mailto links only; other protocols are refused. */
export async function openExternalSafe(url: string): Promise<boolean> {
  if (!isSafeExternalUrl(url)) return false

  const service = getDesktopService()
  if (service) {
    await service.openExternal(url)
    return true
  }

  window.open(url, '_blank', 'noopener,noreferrer')
  return true
}

/** Visibility is the reliable signal in both a browser tab and the desktop webview. */
export function isWindowInactive(): boolean {
  if (typeof document === 'undefined') return false
  const hidden = document.visibilityState !== 'visible'
  const unfocused = typeof document.hasFocus === 'function' && !document.hasFocus()
  return hidden || unfocused
}

/** Desktop notification only when the window is in the background; no-op otherwise. */
export async function notifyWhenBackground(title: string, body: string): Promise<void> {
  if (!isWindowInactive()) return
  await getDesktopService()?.notify(title, body)
}

/** Subscribes to native file drops. Returns an unsubscribe function. */
export function onDesktopFilesDropped(handler: (files: string[], target: string) => void): () => void {
  if (!isDesktop()) return () => undefined
  const off = Events.On('desktop:files-dropped', (event) => {
    const data = event.data as { files?: string[]; target?: string } | null
    handler(Array.isArray(data?.files) ? data.files : [], data?.target ?? '')
  })
  return typeof off === 'function' ? off : () => undefined
}

/** Native menu request to open the update dialog. */
export function onDesktopOpenUpdate(handler: () => void): () => void {
  if (!isDesktop()) return () => undefined
  const off = Events.On('desktop:open-update', handler)
  return typeof off === 'function' ? off : () => undefined
}

/** Subscribes to valid native updater progress events. */
export function onDesktopUpdateProgress(handler: (progress: { received: number; total: number }) => void): () => void {
  if (!isDesktop()) return () => undefined
  const off = Events.On('desktop:update-progress', (event) => {
    const data = event.data as { received?: unknown; total?: unknown } | null
    if (!data || typeof data.received !== 'number' || !Number.isFinite(data.received) || data.received < 0 ||
        typeof data.total !== 'number' || !Number.isFinite(data.total) || data.total < 0) return
    handler({ received: data.received, total: data.total })
  })
  return typeof off === 'function' ? off : () => undefined
}

/** Subscribes to updater failures surfaced by the native side. */
export function onDesktopUpdateError(handler: (message: string) => void): () => void {
  if (!isDesktop()) return () => undefined
  const off = Events.On('desktop:update-error', (event) => {
    const data = event.data as { message?: string } | null
    if (data?.message) handler(data.message)
  })
  return typeof off === 'function' ? off : () => undefined
}

/** Subscribes to papersage:// routes opened while the app is already running. */
export function onDesktopDeepLink(handler: (path: string) => void): () => void {
  if (!isDesktop()) return () => undefined
  const off = Events.On('desktop:deeplink', (event) => {
    if (typeof event.data === 'string' && event.data) handler(event.data)
  })
  return typeof off === 'function' ? off : () => undefined
}

/** Converts a base64 payload from the native bridge into a File for upload APIs. */
export function base64File(name: string, dataBase64: string, type = 'application/octet-stream'): File {
  const binary = atob(dataBase64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i)
  return new File([bytes], name || 'file', { type })
}
