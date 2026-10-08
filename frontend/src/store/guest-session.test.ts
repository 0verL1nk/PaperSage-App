import { beforeEach, describe, expect, it } from 'vitest'
import { activateLocalGuestSession, mergeRefreshedTokens, setGuestImportPending, setSessionTokens, useSessionStore } from '@/store'

describe('browser-local guest session', () => {
  beforeEach(() => {
    window.localStorage.clear()
    useSessionStore.setState({ accessToken: '', refreshToken: '', isGuest: false, guestImportPending: false, tokenType: 'Bearer', restored: true })
  })

  it('persists tokenless guest mode and pending import marker across reloads', () => {
    activateLocalGuestSession()
    expect(useSessionStore.getState()).toMatchObject({ accessToken: '', refreshToken: '', isGuest: true, guestImportPending: true })
    expect(window.localStorage.getItem('papersage_local_guest_mode')).toBe('true')
    expect(window.localStorage.getItem('papersage_access_token')).toBeNull()

    useSessionStore.setState({ accessToken: '', refreshToken: '', isGuest: false, guestImportPending: false, tokenType: 'Bearer', restored: false })
    useSessionStore.getState().restoreSession()
    expect(useSessionStore.getState()).toMatchObject({ accessToken: '', refreshToken: '', isGuest: true, guestImportPending: true })
  })

  it('keeps guest import pending after account login and clears only the local guest mode', () => {
    activateLocalGuestSession()
    setSessionTokens({ accessToken: 'account-access', refreshToken: 'account-refresh' })
    expect(useSessionStore.getState()).toMatchObject({ accessToken: 'account-access', isGuest: false, guestImportPending: true })
    expect(window.localStorage.getItem('papersage_local_guest_mode')).toBeNull()
    expect(window.localStorage.getItem('papersage_guest_import_pending')).toBe('true')
  })

  it('keeps account tokens when stale retired guest metadata remains', () => {
    setGuestImportPending(true)
    window.localStorage.setItem('papersage_access_token', 'account-access')
    window.localStorage.setItem('papersage_refresh_token', 'account-refresh')
    window.localStorage.setItem('papersage_is_guest', 'true')
    window.localStorage.setItem('papersage_session_expires_at', new Date(Date.now() + 60_000).toISOString())
    window.localStorage.setItem('papersage_local_guest_mode', 'true')

    const restored = useSessionStore.getState().restoreSession()
    expect(restored).toMatchObject({ accessToken: 'account-access', refreshToken: 'account-refresh' })
    expect(useSessionStore.getState()).toMatchObject({ accessToken: 'account-access', isGuest: false, guestImportPending: true })
    expect(window.localStorage.getItem('papersage_access_token')).toBe('account-access')
    expect(window.localStorage.getItem('papersage_is_guest')).toBeNull()
  })

  it('refreshes account tokens without recreating server guest metadata', () => {
    const refreshed = mergeRefreshedTokens({ accessToken: 'new-access', refreshToken: 'new-refresh' })
    setSessionTokens(refreshed)
    expect(useSessionStore.getState()).toMatchObject({ accessToken: 'new-access', refreshToken: 'new-refresh', isGuest: false })
  })
})
