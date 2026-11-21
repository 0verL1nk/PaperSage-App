import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '@/i18n'
import { UpdateDialog } from './update-dialog'

const { callByName, onEvent } = vi.hoisted(() => ({ callByName: vi.fn(), onEvent: vi.fn() }))
vi.mock('@wailsio/runtime', () => ({ Call: { ByName: callByName }, Events: { On: onEvent } }))

const FQN = 'github.com/0verL1nk/GraduateProject/desktop/internal/services.DesktopService'
const RELEASE_URL = 'https://github.com/0verL1nk/PaperSage-App/releases/latest'
type TestWindow = Window & { _wails?: unknown }
type NativeHandler = (event: { data: unknown }) => void
const listeners = new Map<string, NativeHandler>()
const removers = new Map<string, ReturnType<typeof vi.fn>>()
const update = {
  version: '2.0.0',
  name: 'New release',
  notes: '<script>not HTML</script>',
  releaseUrl: 'https://untrusted.example.test/installer',
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((accept) => { resolve = accept })
  return { promise, resolve }
}

function openUpdate() {
  act(() => { window.dispatchEvent(new Event('papersage:open-update')) })
}

function nativeEvent(name: string, data?: unknown) {
  act(() => { listeners.get(name)?.({ data }) })
}

function calls(method: string) {
  return callByName.mock.calls.filter(([name]) => name === `${FQN}.${method}`).length
}

describe('manual desktop update dialog', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-CN')
    ;(window as TestWindow)._wails = { environment: { OS: 'windows' } }
    onEvent.mockImplementation((name: string, handler: NativeHandler) => {
      listeners.set(name, handler)
      const off = vi.fn(() => { listeners.delete(name) })
      removers.set(name, off)
      return off
    })
    callByName.mockImplementation(async (method: string) => {
      if (method === `${FQN}.CheckForUpdates`) return update
      if (method === `${FQN}.OpenExternal`) return undefined
      throw new Error(`Unexpected call: ${method}`)
    })
  })

  afterEach(() => {
    cleanup()
    delete (window as TestWindow)._wails
    listeners.clear()
    removers.clear()
    onEvent.mockReset()
    callByName.mockReset()
    vi.restoreAllMocks()
  })

  it('checks only on explicit open, once per opening, including native menu events', async () => {
    callByName.mockResolvedValue(null)
    render(<UpdateDialog />)
    expect(callByName).not.toHaveBeenCalled()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    openUpdate()
    nativeEvent('desktop:open-update')
    expect(await screen.findByText('当前已是最新版本。')).toBeInTheDocument()
    expect(calls('CheckForUpdates')).toBe(1)

    fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(calls('DiscardUpdate')).toBe(0)
    nativeEvent('desktop:open-update')
    expect(await screen.findByText('当前已是最新版本。')).toBeInTheDocument()
    expect(calls('CheckForUpdates')).toBe(2)
  })

  it('shows release details as text and opens only the official page on request', async () => {
    const dispatch = vi.spyOn(window, 'dispatchEvent')
    render(<UpdateDialog />)
    openUpdate()

    expect(await screen.findByText('版本 2.0.0')).toBeInTheDocument()
    expect(screen.getByText('<script>not HTML</script>')).toBeInTheDocument()
    expect(screen.getByText(/手动安装.*原有安装目录.*数据将保留/)).toBeInTheDocument()
    expect(document.querySelector('script')).toBeNull()
    expect(screen.queryByRole('button', { name: '下载更新' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '关闭并安装' })).not.toBeInTheDocument()
    expect(callByName).toHaveBeenCalledTimes(1)

    fireEvent.click(screen.getByRole('button', { name: '打开下载页' }))
    await waitFor(() => expect(callByName).toHaveBeenCalledWith(`${FQN}.OpenExternal`, RELEASE_URL))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(callByName.mock.calls.map(([name]) => name)).toEqual([
      `${FQN}.CheckForUpdates`, `${FQN}.OpenExternal`,
    ])
    expect(dispatch.mock.calls.some(([event]) => event.type === 'papersage:prepare-update')).toBe(false)
  })

  it('reports a check failure and retries only on request', async () => {
    callByName.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(null)
    render(<UpdateDialog />)
    openUpdate()
    expect(await screen.findByText(/检查更新失败/)).toBeInTheDocument()
    expect(screen.getByText('offline')).toBeInTheDocument()
    expect(calls('CheckForUpdates')).toBe(1)
    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByText('当前已是最新版本。')).toBeInTheDocument()
    expect(calls('CheckForUpdates')).toBe(2)
    expect(calls('OpenExternal')).toBe(0)
  })

  it('treats a missing native result as an error rather than an up-to-date response', async () => {
    callByName.mockResolvedValue(undefined)
    render(<UpdateDialog />)
    openUpdate()
    expect(await screen.findByText(/检查更新失败/)).toBeInTheDocument()
    expect(screen.getByText('更新服务返回了无效的更新信息。')).toBeInTheDocument()
    expect(screen.queryByText('当前已是最新版本。')).not.toBeInTheDocument()
  })

  it('prevents duplicate checks or dismissal while a check is in flight', async () => {
    const check = deferred<typeof update | null>()
    callByName.mockReturnValue(check.promise)
    render(<UpdateDialog />)
    openUpdate()
    nativeEvent('desktop:open-update')
    expect(screen.getByText('正在检查更新…')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '关闭' })).not.toBeInTheDocument()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(calls('CheckForUpdates')).toBe(1)
    await act(async () => { check.resolve(null); await check.promise })
    expect(await screen.findByText('当前已是最新版本。')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(calls('DiscardUpdate')).toBe(0)
  })

  it('does not open or subscribe in a plain browser', () => {
    delete (window as TestWindow)._wails
    render(<UpdateDialog />)
    openUpdate()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(onEvent).not.toHaveBeenCalled()
    expect(callByName).not.toHaveBeenCalled()
  })

  it('unsubscribes native and DOM open events without subscribing to download progress', () => {
    const remove = vi.spyOn(window, 'removeEventListener')
    const { unmount } = render(<UpdateDialog />)
    const removeOpen = removers.get('desktop:open-update')
    expect(removeOpen).toBeDefined()
    expect(listeners.has('desktop:update-progress')).toBe(false)

    unmount()
    expect(removeOpen).toHaveBeenCalledOnce()
    expect(remove).toHaveBeenCalledWith('papersage:open-update', expect.any(Function))
    openUpdate()
    nativeEvent('desktop:open-update')
    expect(callByName).not.toHaveBeenCalled()
    remove.mockRestore()
  })

  it('continues the active check when the language changes', async () => {
    const check = deferred<typeof update | null>()
    callByName.mockReturnValue(check.promise)
    render(<UpdateDialog />)
    openUpdate()
    await act(async () => { await i18n.changeLanguage('en-US') })
    await act(async () => { check.resolve(null); await check.promise })
    expect(await screen.findByText('You are up to date.')).toBeInTheDocument()
    expect(calls('CheckForUpdates')).toBe(1)
    await i18n.changeLanguage('zh-CN')
  })

  it('renders manual installation guidance in English', async () => {
    await i18n.changeLanguage('en-US')
    render(<UpdateDialog />)
    openUpdate()
    expect(await screen.findByRole('button', { name: 'Open download page' })).toBeInTheDocument()
    expect(screen.getByText(/install it manually.*existing installation directory.*data will be preserved/i)).toBeInTheDocument()
    await i18n.changeLanguage('zh-CN')
  })
})
