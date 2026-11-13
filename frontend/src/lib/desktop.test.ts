import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  getDesktopService,
  isDesktop,
  isSafeExternalUrl,
  notifyWhenBackground,
  openExternalSafe,
} from './desktop'

const { callByName } = vi.hoisted(() => ({ callByName: vi.fn() }))

vi.mock('@wailsio/runtime', () => ({ Call: { ByName: callByName } }))

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
