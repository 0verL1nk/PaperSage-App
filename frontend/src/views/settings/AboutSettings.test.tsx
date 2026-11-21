import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import i18n from '@/i18n'
import { appEnv, setApiTarget, setCustomApiBaseUrl, setDesktopLocalApiBaseUrl, setDesktopLocalBackendError } from '@/lib/env'
import { AboutSettings } from './AboutSettings'

const backendError = '本地后端版本不匹配'

describe('About API target', () => {
  beforeEach(async () => {
    localStorage.clear()
    await i18n.changeLanguage('zh-CN')
  })

  afterEach(async () => {
    setDesktopLocalApiBaseUrl(appEnv.localApiBaseUrl)
    setDesktopLocalApiBaseUrl('')
    await i18n.changeLanguage('zh-CN')
  })

  it.each([
    ['zh-CN', '本地后端不可用', '同源（当前站点）'],
    ['en-US', 'Local backend unavailable', 'Same origin (this site)'],
  ])('shows a localized unavailable status instead of same origin in %s', async (language, unavailable, sameOrigin) => {
    await i18n.changeLanguage(language)
    setApiTarget('local')
    setDesktopLocalBackendError(backendError)

    render(<AboutSettings />)

    expect(screen.getByText(new RegExp(unavailable))).toBeInTheDocument()
    expect(screen.getByText(new RegExp(backendError))).toBeInTheDocument()
    expect(screen.queryByText(sameOrigin)).not.toBeInTheDocument()
  })

  it('still shows the explicit online address after a local failure', () => {
    setDesktopLocalBackendError(backendError)
    setApiTarget('online')

    render(<AboutSettings />)

    expect(screen.getByText(appEnv.apiBaseUrl || '同源（当前站点）')).toBeInTheDocument()
  })

  it('still shows the custom address after a local failure', () => {
    setDesktopLocalBackendError(backendError)
    setCustomApiBaseUrl('https://custom.example.test/')
    setApiTarget('custom')

    render(<AboutSettings />)

    expect(screen.getByText('https://custom.example.test')).toBeInTheDocument()
  })

  it('still shows the browser local-development address', () => {
    setApiTarget('local')

    render(<AboutSettings />)

    expect(screen.getByText(appEnv.localApiBaseUrl)).toBeInTheDocument()
  })
})
