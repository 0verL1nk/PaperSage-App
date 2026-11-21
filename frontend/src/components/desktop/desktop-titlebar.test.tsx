import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import i18n from '@/i18n'
import { DesktopAppMenu } from './desktop-titlebar'

const { callByName, onEvent } = vi.hoisted(() => ({ callByName: vi.fn(), onEvent: vi.fn() }))
vi.mock('@wailsio/runtime', () => ({ Call: { ByName: callByName }, Events: { On: onEvent } }))

type TestWindow = Window & { _wails?: unknown }

describe('desktop update menu', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-CN')
    ;(window as TestWindow)._wails = { environment: { OS: 'windows' } }
  })

  afterEach(() => {
    delete (window as TestWindow)._wails
    callByName.mockReset()
  })

  it('opens the update dialog instead of calling the native check directly', () => {
    const open = vi.fn()
    window.addEventListener('papersage:open-update', open)
    render(<DesktopAppMenu />)

    fireEvent.click(screen.getByRole('button', { name: '应用菜单' }))
    fireEvent.click(screen.getByRole('menuitem', { name: '检查更新' }))

    expect(open).toHaveBeenCalledTimes(1)
    expect(callByName).not.toHaveBeenCalled()
    window.removeEventListener('papersage:open-update', open)
  })

  it('does not show the desktop menu in a browser', () => {
    delete (window as TestWindow)._wails
    render(<DesktopAppMenu />)
    expect(screen.queryByRole('button', { name: '应用菜单' })).not.toBeInTheDocument()
  })

  it('localizes the update command', async () => {
    await i18n.changeLanguage('en-US')
    render(<DesktopAppMenu />)
    fireEvent.click(screen.getByRole('button', { name: '应用菜单' }))
    expect(screen.getByRole('menuitem', { name: 'Check for updates' })).toBeInTheDocument()
    await i18n.changeLanguage('zh-CN')
  })
})
