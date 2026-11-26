import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AxiosAdapter } from 'axios'
import { toast } from 'sonner'
import {
  getDesktopService,
  isDesktop,
  isSafeExternalUrl,
  notifyWhenBackground,
  onDesktopUpdateProgress,
  openExternalSafe,
} from './desktop'

const { callByName, onEvent } = vi.hoisted(() => ({ callByName: vi.fn(), onEvent: vi.fn() }))

vi.mock('@wailsio/runtime', () => ({ Call: { ByName: callByName }, Events: { On: onEvent } }))
vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

const FQN = 'github.com/0verL1nk/GraduateProject/desktop/internal/services.DesktopService'

type TestWindow = Window & { _wails?: unknown; wails?: unknown }

function installWailsRuntime() {
  ;(window as TestWindow)._wails = { environment: { OS: 'windows' }, clientId: 'test-client' }
}

describe('desktop bridge', () => {
  afterEach(() => {
    delete (window as TestWindow)._wails
    delete (window as TestWindow).wails
    callByName.mockReset()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('stays inert in a plain browser', () => {
    expect(isDesktop()).toBe(false)
    expect(getDesktopService()).toBeNull()
  })

  it('detects the Wails runtime marker', () => {
    installWailsRuntime()
    expect(isDesktop()).toBe(true)
    expect(getDesktopService()).not.toBeNull()
  })

  it('calls DesktopService by its fully qualified method name', async () => {
    installWailsRuntime()
    callByName.mockResolvedValue({ version: '1.2.3', platform: 'windows', isDesktop: true })

    await expect(getDesktopService()!.appInfo()).resolves.toEqual({
      version: '1.2.3',
      platform: 'windows',
      isDesktop: true,
    })

    expect(callByName).toHaveBeenCalledWith(`${FQN}.AppInfo`)
  })

  it('returns update information without losing its fields', async () => {
    installWailsRuntime()
    const update = { version: '2.0.0', name: 'Release', notes: 'Changes' }
    callByName.mockResolvedValue(update)

    await expect(getDesktopService()!.checkForUpdates()).resolves.toEqual(update)
    expect(callByName).toHaveBeenCalledWith(`${FQN}.CheckForUpdates`)
  })

  it('propagates native update-check failures', async () => {
    installWailsRuntime()
    callByName.mockRejectedValue(new Error('native failure'))

    await expect(getDesktopService()!.checkForUpdates()).rejects.toThrow('native failure')
    expect(callByName).toHaveBeenCalledWith(`${FQN}.CheckForUpdates`)
  })

  it('calls the native download, install, and discard bindings without swallowing failures', async () => {
    installWailsRuntime()
    const service = getDesktopService()!
    callByName.mockResolvedValue(undefined)

    await service.downloadUpdate()
    await service.installUpdate()
    await service.discardUpdate()
    expect(callByName.mock.calls).toEqual([
      [`${FQN}.DownloadUpdate`],
      [`${FQN}.InstallUpdate`],
      [`${FQN}.DiscardUpdate`],
    ])

    callByName.mockRejectedValue(new Error('native failure'))
    await expect(service.downloadUpdate()).rejects.toThrow('native failure')
    await expect(service.installUpdate()).rejects.toThrow('native failure')
    await expect(service.discardUpdate()).rejects.toThrow('native failure')
  })

  it('forwards valid progress events and unsubscribes', () => {
    installWailsRuntime()
    let listener!: (event: { data: unknown }) => void
    const unsubscribe = vi.fn()
    onEvent.mockImplementation((name: string, handler: typeof listener) => {
      expect(name).toBe('desktop:update-progress')
      listener = handler
      return unsubscribe
    })
    const handler = vi.fn()
    const off = onDesktopUpdateProgress(handler)

    listener({ data: { received: 25, total: 100 } })
    listener({ data: { received: 25, total: 0 } })
    for (const data of [null, {}, [], { received: '1', total: 2 }, { received: 1, total: '2' },
      { received: -1, total: 1 }, { received: 1, total: -1 },
      { received: Infinity, total: 1 }, { received: 1, total: NaN }]) {
      listener({ data })
    }
    expect(handler.mock.calls).toEqual([
      [{ received: 25, total: 100 }],
      [{ received: 25, total: 0 }],
    ])
    off()
    expect(unsubscribe).toHaveBeenCalledOnce()
  })

  it('does not subscribe to progress in a plain browser', () => {
    onEvent.mockClear()
    const off = onDesktopUpdateProgress(vi.fn())
    expect(onEvent).not.toHaveBeenCalled()
    expect(() => off()).not.toThrow()
  })

  it('allows only http, https and mailto links', () => {
    expect(isSafeExternalUrl('https://example.com')).toBe(true)
    expect(isSafeExternalUrl('http://example.com')).toBe(true)
    expect(isSafeExternalUrl('mailto:help@example.com')).toBe(true)
    expect(isSafeExternalUrl('javascript:alert(1)')).toBe(false)
    expect(isSafeExternalUrl('file:///etc/passwd')).toBe(false)
    expect(isSafeExternalUrl('not a url')).toBe(false)
  })

  it('refuses unsafe protocols without opening anything', async () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    await expect(openExternalSafe('javascript:alert(1)')).resolves.toBe(false)
    expect(open).not.toHaveBeenCalled()
  })

  it('does not notify while the window is focused', async () => {
    installWailsRuntime()
    vi.spyOn(document, 'hasFocus').mockReturnValue(true)
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'visible' })

    await notifyWhenBackground('编译完成', 'PDF 已生成')
    expect(callByName).not.toHaveBeenCalled()
  })
})

describe('desktop local backend startup', () => {
  const backendError = '本地后端版本不匹配'

  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    callByName.mockReset()
    onEvent.mockReset()
    vi.mocked(toast.error).mockClear()
  })

  afterEach(() => {
    delete (window as TestWindow)._wails
    localStorage.clear()
    callByName.mockReset()
    onEvent.mockReset()
    vi.mocked(toast.error).mockClear()
  })

  async function bootDesktop(error: unknown, localApiBaseUrl?: string) {
    installWailsRuntime()
    callByName.mockImplementation(async (method: string) => {
      if (method === `${FQN}.AppInfo`) return { isDesktop: true, localApiBaseUrl }
      if (method === `${FQN}.LocalBackendError`) {
        if (error instanceof Error) throw error
        return error
      }
      throw new Error(`Unexpected native call: ${method}`)
    })
    const env = await import('./env')
    const { initDesktop } = await import('./desktop')
    await initDesktop()
    return env
  }

  it('marks a rejected native backend unavailable for a persisted local target', async () => {
    localStorage.setItem('papersage_api_target', 'local')
    const env = await bootDesktop(backendError)

    expect(env.getApiTarget()).toBe('local')
    expect(env.getApiBaseUrlFor('local')).toBe('')
    expect(env.getOptionalApiBaseUrl()).toBe('')
    expect(() => env.getRequiredApiBaseUrl()).toThrow(backendError)
    expect(toast.error).toHaveBeenCalledWith('本地后端不可用', {
      description: backendError,
      duration: 12_000,
    })
  })

  it('rejects an axios request before invoking its adapter', async () => {
    localStorage.setItem('papersage_api_target', 'local')
    const { apiClient } = await import('@/services/client')
    expect(apiClient.defaults.baseURL).toContain('localhost:5478')
    await bootDesktop(backendError)
    const adapter = vi.fn<AxiosAdapter>(async (config) => ({
      config, data: {}, headers: {}, status: 200, statusText: 'OK',
    }))

    await expect(apiClient.get('/health', { adapter })).rejects.toThrow(backendError)
    expect(adapter).not.toHaveBeenCalled()
  })

  it('keeps local as the default target when startup fails', async () => {
    const env = await bootDesktop(backendError)

    expect(env.getApiTarget()).toBe('local')
    expect(env.getOptionalApiBaseUrl()).toBe('')
  })

  it.each([undefined, '', 'not a url'])('fails closed when the native error cannot be retrieved and the URL is %s', async (url) => {
    const env = await bootDesktop(new Error('bridge disconnected'), url)

    expect(env.getApiTarget()).toBe('local')
    expect(env.getApiBaseUrlFor('local')).toBe('')
    expect(() => env.getRequiredApiBaseUrl()).toThrow(/本地后端/)
    expect(toast.error).toHaveBeenCalledWith('本地后端不可用', expect.objectContaining({
      description: expect.stringMatching(/本地后端/),
    }))
  })

  it.each([undefined, null, ''])('fails closed when neither a supervised URL nor a usable native error is returned (%s)', async (error) => {
    const env = await bootDesktop(error)

    expect(env.getApiTarget()).toBe('local')
    expect(env.getOptionalApiBaseUrl()).toBe('')
    expect(() => env.getRequiredApiBaseUrl()).toThrow(/本地后端/)
  })

  it('keeps explicitly selected online and custom targets usable after a local failure', async () => {
    const env = await bootDesktop(backendError)
    const { apiClient } = await import('@/services/client')
    const adapter = vi.fn<AxiosAdapter>(async (config) => ({
      config, data: {}, headers: {}, status: 200, statusText: 'OK',
    }))

    env.setApiTarget('online')
    await expect(apiClient.get('/health', { adapter })).resolves.toBeTruthy()
    expect(adapter.mock.calls[0][0].baseURL).toBe(env.appEnv.apiBaseUrl || undefined)

    env.setCustomApiBaseUrl('https://custom.example.test/')
    env.setApiTarget('custom')
    await expect(apiClient.get('/health', { adapter })).resolves.toBeTruthy()
    expect(adapter.mock.calls[1][0].baseURL).toBe('https://custom.example.test')
    expect(env.getApiBaseUrlFor('local')).toBe('')
  })

  it('does not call AppInfo when only the npm runtime client id is present', async () => {
    ;(window as TestWindow)._wails = { clientId: 'browser-client' }
    const { initDesktop } = await import('./desktop')

    await initDesktop()

    expect(callByName).not.toHaveBeenCalled()
  })

  it('retains the browser local-development fallback', async () => {
    const env = await import('./env')
    const { initDesktop } = await import('./desktop')
    await initDesktop()
    env.setApiTarget('local')

    expect(env.getRequiredApiBaseUrl()).toBe(env.appEnv.localApiBaseUrl)
    expect(env.getRequiredApiBaseUrl()).toContain('localhost:5478')
  })

  it('does not install a duplicate global update-error listener at startup', async () => {
    await bootDesktop('', 'http://127.0.0.1:6000/')

    expect(onEvent.mock.calls.some(([event]) => event === 'desktop:update-error')).toBe(false)
  })

  it('uses the supervised URL during a normal desktop startup', async () => {
    const env = await bootDesktop('', 'http://127.0.0.1:6000/')

    expect(env.getApiTarget()).toBe('local')
    expect(env.getRequiredApiBaseUrl()).toBe('http://127.0.0.1:6000')
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('uses a valid supervised URL and clears an earlier startup failure', async () => {
    const env = await bootDesktop(backendError)
    expect(() => env.getRequiredApiBaseUrl()).toThrow(backendError)

    env.setDesktopLocalApiBaseUrl('not a url')
    expect(() => env.getRequiredApiBaseUrl()).toThrow(backendError)
    env.setDesktopLocalApiBaseUrl(' http://127.0.0.1:6001/ ')
    expect(env.getApiTarget()).toBe('local')
    expect(env.getRequiredApiBaseUrl()).toBe('http://127.0.0.1:6001')
  })

  it('keeps a valid supervised URL when querying the native error fails', async () => {
    const env = await bootDesktop(new Error('bridge disconnected'), 'http://127.0.0.1:6002/')

    expect(env.getApiTarget()).toBe('local')
    expect(env.getRequiredApiBaseUrl()).toBe('http://127.0.0.1:6002')
  })

  it('treats an explicit native error as unavailable even if AppInfo also returns a URL', async () => {
    const env = await bootDesktop(backendError, 'http://127.0.0.1:6003')

    expect(env.getApiTarget()).toBe('local')
    expect(env.getApiBaseUrlFor('local')).toBe('')
    expect(() => env.getRequiredApiBaseUrl()).toThrow(backendError)
  })
})
