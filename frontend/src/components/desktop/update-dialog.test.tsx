import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '@/i18n'
import { UpdateDialog } from './update-dialog'

const mocks = vi.hoisted(() => ({
  getDesktopService: vi.fn(),
  onDesktopOpenUpdate: vi.fn(),
  onDesktopUpdateProgress: vi.fn(),
  onDesktopUpdateError: vi.fn(),
  openExternalSafe: vi.fn(),
  service: {
    checkForUpdates: vi.fn(),
    downloadUpdate: vi.fn(),
    installUpdate: vi.fn(),
    discardUpdate: vi.fn(),
  },
}))
vi.mock('@/lib/desktop', () => mocks)

const RELEASE_URL = 'https://github.com/0verL1nk/PaperSage-App/releases/latest'
const update = { version: '2.0.0', name: 'New release', notes: '<script>not HTML</script>' }
let nativeOpen: (() => void) | undefined
let nativeProgress: ((progress: { received: number; total: number }) => void) | undefined
let nativeError: ((message: string) => void) | undefined

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail })
  return { promise, resolve, reject }
}

function openUpdate() {
  act(() => { window.dispatchEvent(new Event('papersage:open-update')) })
}

function emitProgress(received: number, total: number) {
  act(() => { nativeProgress?.({ received, total }) })
}

function emitError(message: string) {
  act(() => { nativeError?.(message) })
}

async function openAvailable() {
  openUpdate()
  expect(await screen.findByText('版本 2.0.0')).toBeInTheDocument()
}

async function startDownload() {
  await openAvailable()
  fireEvent.click(screen.getByRole('button', { name: '下载并安装' }))
}

describe('desktop update dialog', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-CN')
    nativeOpen = undefined
    nativeProgress = undefined
    nativeError = undefined
    mocks.getDesktopService.mockReturnValue(mocks.service)
    mocks.service.checkForUpdates.mockResolvedValue(update)
    mocks.service.downloadUpdate.mockResolvedValue(undefined)
    mocks.service.installUpdate.mockResolvedValue(undefined)
    mocks.service.discardUpdate.mockResolvedValue(undefined)
    mocks.openExternalSafe.mockResolvedValue(true)
    mocks.onDesktopOpenUpdate.mockImplementation((handler: () => void) => {
      nativeOpen = handler
      return () => { nativeOpen = undefined }
    })
    mocks.onDesktopUpdateProgress.mockImplementation((handler: typeof nativeProgress) => {
      nativeProgress = handler
      return () => { nativeProgress = undefined }
    })
    mocks.onDesktopUpdateError.mockImplementation((handler: typeof nativeError) => {
      nativeError = handler
      return () => { nativeError = undefined }
    })
  })

  afterEach(() => {
    cleanup()
    vi.resetAllMocks()
  })

  it('downloads with progress, then starts installation without treating a disconnected binding as failure', async () => {
    const download = deferred<void>()
    const install = deferred<void>()
    mocks.service.downloadUpdate.mockReturnValue(download.promise)
    mocks.service.installUpdate.mockReturnValue(install.promise)
    render(<UpdateDialog />)
    expect(mocks.service.checkForUpdates).not.toHaveBeenCalled()

    await openAvailable()
    expect(screen.getByText('New release')).toBeInTheDocument()
    expect(screen.getByText('<script>not HTML</script>')).toBeInTheDocument()
    expect(document.querySelector('script')).toBeNull()
    expect(screen.getByText('安装过程中 Windows 会请求一次管理员权限。')).toBeInTheDocument()
    expect(mocks.openExternalSafe).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: '下载并安装' }))
    const progress = screen.getByRole('progressbar', { name: '正在下载更新…' })
    expect(progress).toHaveAttribute('max', '1')
    expect(progress).not.toHaveAttribute('value')
    expect(screen.getByRole('status')).toHaveTextContent('正在下载更新…')
    expect(screen.getByText('已下载 0.00 MB')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '关闭' })).not.toBeInTheDocument()
    fireEvent.keyDown(document, { key: 'Escape' })
    fireEvent.pointerDown(document.body, { button: 0 })
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    emitProgress(1048576, 4194304)
    expect(progress).toHaveAttribute('max', '4194304')
    expect(progress).toHaveAttribute('value', '1048576')
    expect(screen.getByText('已下载 1.00 MB（25%）')).toBeInTheDocument()
    await act(async () => { download.resolve(); await download.promise })
    expect(screen.getByRole('status')).toHaveTextContent('下载完成，重启后安装。')
    fireEvent.click(screen.getByRole('button', { name: '立即重启并安装' }))
    await waitFor(() => expect(mocks.service.installUpdate).toHaveBeenCalledOnce())
    expect(screen.getByRole('status')).toHaveTextContent('正在应用更新，应用即将重启…')
    expect(screen.queryByRole('button', { name: '关闭' })).not.toBeInTheDocument()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    await act(async () => { install.reject(new Error('binding closed')); await install.promise.catch(() => undefined) })
    expect(screen.getByRole('status')).toHaveTextContent('正在应用更新，应用即将重启…')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('ignores repeated download and install clicks before the dialog rerenders', async () => {
    const download = deferred<void>()
    const install = deferred<void>()
    mocks.service.downloadUpdate.mockReturnValue(download.promise)
    mocks.service.installUpdate.mockReturnValue(install.promise)
    render(<UpdateDialog />)
    await openAvailable()
    const downloadButton = screen.getByRole('button', { name: '下载并安装' })
    act(() => { fireEvent.click(downloadButton); fireEvent.click(downloadButton) })
    expect(mocks.service.downloadUpdate).toHaveBeenCalledOnce()
    await act(async () => { download.resolve(); await download.promise })
    const installButton = screen.getByRole('button', { name: '立即重启并安装' })
    act(() => { fireEvent.click(installButton); fireEvent.click(installButton) })
    await waitFor(() => expect(mocks.service.installUpdate).toHaveBeenCalledOnce())
    await act(async () => { install.resolve(); await install.promise })
  })

  it('keeps installing when the native install call resolves', async () => {
    render(<UpdateDialog />)
    await startDownload()
    fireEvent.click(await screen.findByRole('button', { name: '立即重启并安装' }))
    await waitFor(() => expect(mocks.service.installUpdate).toHaveBeenCalledOnce())
    expect(screen.getByRole('status')).toHaveTextContent('正在应用更新，应用即将重启…')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('reports a download failure, retries the download, and offers the official release page', async () => {
    mocks.service.downloadUpdate.mockRejectedValueOnce(new Error('network error')).mockResolvedValueOnce(undefined)
    render(<UpdateDialog />)
    await startDownload()
    expect(await screen.findByRole('alert')).toHaveTextContent('下载更新失败')
    expect(screen.getByText('network error')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '打开发布页' }))
    expect(mocks.openExternalSafe).toHaveBeenCalledWith(RELEASE_URL)
    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByText('下载完成，重启后安装。')).toBeInTheDocument()
    expect(mocks.service.downloadUpdate).toHaveBeenCalledTimes(2)
    expect(mocks.service.checkForUpdates).toHaveBeenCalledOnce()
  })

  it('reports a native installation failure and retries the installation', async () => {
    const install = deferred<void>()
    mocks.service.installUpdate.mockReturnValueOnce(install.promise).mockResolvedValueOnce(undefined)
    render(<UpdateDialog />)
    await startDownload()
    fireEvent.click(await screen.findByRole('button', { name: '立即重启并安装' }))
    emitError('could not launch installer')
    expect(screen.getByRole('alert')).toHaveTextContent('安装更新失败')
    expect(screen.getByText('could not launch installer')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '打开发布页' })).toBeInTheDocument()
    await act(async () => { install.reject(new Error('launch failed')); await install.promise.catch(() => undefined) })
    expect(screen.getByRole('alert')).toHaveTextContent('安装更新失败')
    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(screen.getByRole('status')).toHaveTextContent('正在应用更新，应用即将重启…')
    await waitFor(() => expect(mocks.service.installUpdate).toHaveBeenCalledTimes(2))
    expect(mocks.service.downloadUpdate).toHaveBeenCalledOnce()
  })

  it('flushes unsaved editor changes before starting the installer', async () => {
    const save = deferred<void>()
    let flushed = false
    const listener = (event: Event) => {
      const detail = (event as CustomEvent<{ waitUntil: (promise: Promise<unknown>) => void }>).detail
      detail.waitUntil(save.promise.then(() => { flushed = true }))
    }
    window.addEventListener('papersage:prepare-update', listener)
    try {
      render(<UpdateDialog />)
      await startDownload()
      fireEvent.click(await screen.findByRole('button', { name: '立即重启并安装' }))
      expect(screen.getByRole('status')).toHaveTextContent('正在应用更新，应用即将重启…')
      expect(mocks.service.installUpdate).not.toHaveBeenCalled()
      await act(async () => { save.resolve(); await save.promise })
      await waitFor(() => expect(mocks.service.installUpdate).toHaveBeenCalledOnce())
      expect(flushed).toBe(true)
    } finally {
      window.removeEventListener('papersage:prepare-update', listener)
    }
  })

  it('keeps the application open when unsaved changes cannot be saved', async () => {
    const listener = (event: Event) => {
      const detail = (event as CustomEvent<{ waitUntil: (promise: Promise<unknown>) => void }>).detail
      detail.waitUntil(Promise.reject(new Error('disk full')))
    }
    window.addEventListener('papersage:prepare-update', listener)
    try {
      render(<UpdateDialog />)
      await startDownload()
      fireEvent.click(await screen.findByRole('button', { name: '立即重启并安装' }))
      expect(await screen.findByRole('alert')).toHaveTextContent('还有未能保存的修改')
      expect(screen.getByText('disk full')).toBeInTheDocument()
      expect(mocks.service.installUpdate).not.toHaveBeenCalled()
      fireEvent.click(screen.getByRole('button', { name: '关闭' }))
      expect(mocks.service.discardUpdate).toHaveBeenCalledOnce()
    } finally {
      window.removeEventListener('papersage:prepare-update', listener)
    }
  })

  it('discards a staged download when an installation failure is closed', async () => {
    mocks.service.installUpdate.mockRejectedValue(new Error('installer failed'))
    render(<UpdateDialog />)
    await startDownload()
    fireEvent.click(await screen.findByRole('button', { name: '立即重启并安装' }))
    emitError('installer failed')
    expect(screen.getByRole('alert')).toHaveTextContent('安装更新失败')
    fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(mocks.service.discardUpdate).toHaveBeenCalledOnce()
  })

  it('discards a completed download only when ready is closed', async () => {
    mocks.service.discardUpdate.mockRejectedValue(new Error('cleanup failed'))
    render(<UpdateDialog />)
    await openAvailable()
    fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    expect(mocks.service.discardUpdate).not.toHaveBeenCalled()

    await startDownload()
    expect(await screen.findByText('下载完成，重启后安装。')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(mocks.service.discardUpdate).toHaveBeenCalledOnce()
  })

  it('waits for an earlier discard before checking for updates again', async () => {
    const discard = deferred<void>()
    mocks.service.discardUpdate.mockReturnValueOnce(discard.promise)
    render(<UpdateDialog />)
    await startDownload()
    fireEvent.click(await screen.findByRole('button', { name: '关闭' }))
    openUpdate()
    expect(screen.getByRole('status')).toHaveTextContent('正在检查更新…')
    expect(mocks.service.checkForUpdates).toHaveBeenCalledOnce()
    await act(async () => { discard.resolve(); await discard.promise })
    expect(await screen.findByText('版本 2.0.0')).toBeInTheDocument()
    expect(mocks.service.checkForUpdates).toHaveBeenCalledTimes(2)
  })

  it('keeps discard failures silent and can retry a subsequent busy update check', async () => {
    mocks.service.discardUpdate.mockRejectedValueOnce(new Error('cleanup failed'))
    mocks.service.checkForUpdates.mockResolvedValueOnce(update)
      .mockRejectedValueOnce(new Error('updater busy')).mockResolvedValueOnce(update)
    render(<UpdateDialog />)
    await startDownload()
    fireEvent.click(await screen.findByRole('button', { name: '关闭' }))
    openUpdate()
    expect(await screen.findByRole('alert')).toHaveTextContent('updater busy')
    expect(screen.queryByText('cleanup failed')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByText('版本 2.0.0')).toBeInTheDocument()
    expect(mocks.service.discardUpdate).toHaveBeenCalledOnce()
    expect(mocks.service.checkForUpdates).toHaveBeenCalledTimes(3)
  })

  it('opens the official release page only when asked', async () => {
    render(<UpdateDialog />)
    await openAvailable()
    expect(mocks.openExternalSafe).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: '打开发布页' }))
    expect(mocks.openExternalSafe).toHaveBeenCalledWith(RELEASE_URL)
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('checks only once per opening, including native menu requests, and reports current', async () => {
    mocks.service.checkForUpdates.mockResolvedValue(null)
    render(<UpdateDialog />)
    openUpdate()
    act(() => { nativeOpen?.() })
    expect(await screen.findByText('当前已是最新版本。')).toHaveAttribute('role', 'status')
    expect(mocks.service.checkForUpdates).toHaveBeenCalledOnce()
    fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(mocks.service.discardUpdate).not.toHaveBeenCalled()
    act(() => { nativeOpen?.() })
    expect(await screen.findByText('当前已是最新版本。')).toHaveAttribute('role', 'status')
    expect(mocks.service.checkForUpdates).toHaveBeenCalledTimes(2)
  })

  it('shows a check error and retries the check without opening a browser', async () => {
    mocks.service.checkForUpdates.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(null)
    render(<UpdateDialog />)
    openUpdate()
    expect(await screen.findByRole('alert')).toHaveTextContent('检查更新失败')
    expect(screen.getByText('offline')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '打开发布页' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByText('当前已是最新版本。')).toHaveAttribute('role', 'status')
    expect(mocks.service.checkForUpdates).toHaveBeenCalledTimes(2)
    expect(mocks.openExternalSafe).not.toHaveBeenCalled()
  })

  it('treats a missing native result as an error rather than an up-to-date response', async () => {
    mocks.service.checkForUpdates.mockResolvedValue(undefined)
    render(<UpdateDialog />)
    openUpdate()
    expect(await screen.findByRole('alert')).toHaveTextContent('检查更新失败')
    expect(screen.getByText('更新服务返回了无效的更新信息。')).toBeInTheDocument()
  })

  it('prevents duplicate checks or dismissal while checking', async () => {
    const check = deferred<typeof update | null>()
    mocks.service.checkForUpdates.mockReturnValue(check.promise)
    render(<UpdateDialog />)
    openUpdate()
    act(() => { nativeOpen?.() })
    expect(screen.getByRole('status')).toHaveTextContent('正在检查更新…')
    expect(screen.queryByRole('button', { name: '关闭' })).not.toBeInTheDocument()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(mocks.service.checkForUpdates).toHaveBeenCalledOnce()
    await act(async () => { check.resolve(null); await check.promise })
    expect(screen.getByRole('status')).toHaveTextContent('当前已是最新版本。')
  })

  it('does not open or subscribe in a plain browser', () => {
    mocks.getDesktopService.mockReturnValue(null)
    render(<UpdateDialog />)
    openUpdate()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(mocks.onDesktopOpenUpdate).not.toHaveBeenCalled()
    expect(mocks.onDesktopUpdateProgress).not.toHaveBeenCalled()
    expect(mocks.onDesktopUpdateError).not.toHaveBeenCalled()
  })

  it('unsubscribes native and DOM listeners on unmount', () => {
    const { unmount } = render(<UpdateDialog />)
    expect(nativeOpen).toBeDefined()
    expect(nativeProgress).toBeDefined()
    expect(nativeError).toBeDefined()
    unmount()
    expect(nativeOpen).toBeUndefined()
    expect(nativeProgress).toBeUndefined()
    expect(nativeError).toBeUndefined()
    openUpdate()
    expect(mocks.service.checkForUpdates).not.toHaveBeenCalled()
  })

  it('continues the active check when the language changes', async () => {
    const check = deferred<typeof update | null>()
    mocks.service.checkForUpdates.mockReturnValue(check.promise)
    render(<UpdateDialog />)
    openUpdate()
    await act(async () => { await i18n.changeLanguage('en-US') })
    await act(async () => { check.resolve(null); await check.promise })
    expect(screen.getByRole('status')).toHaveTextContent('You are up to date.')
    expect(mocks.service.checkForUpdates).toHaveBeenCalledOnce()
  })

  it('renders natural update actions in English', async () => {
    await i18n.changeLanguage('en-US')
    render(<UpdateDialog />)
    openUpdate()
    expect(await screen.findByRole('button', { name: 'Download and install' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Open release page' })).toBeInTheDocument()
    expect(screen.getByText('Windows will ask for administrator permission during installation.')).toBeInTheDocument()
  })
})
