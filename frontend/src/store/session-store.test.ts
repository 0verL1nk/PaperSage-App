import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ACCESS_TOKEN_STORAGE_KEY,
  REFRESH_TOKEN_STORAGE_KEY,
  clearSessionTokens,
  getAccessToken,
  getRefreshToken,
  setSessionTokens,
  syncSessionTokensFromStorage,
  useSessionStore,
} from '@/store'

// localStorage 是唯一真相，zustand 只是缓存：其他标签页轮换/登出后，
// 本标签页必须能吸收新状态，否则会拿旧 refresh token 触发重放检测。
describe('session storage sync', () => {
  beforeEach(() => {
    window.localStorage.clear()
    useSessionStore.setState({
      accessToken: '',
      refreshToken: '',
      isGuest: false,
      guestImportPending: false,
      tokenType: 'Bearer',
      restored: true,
    })
  })

  it('absorbs tokens another tab wrote to localStorage', () => {
    setSessionTokens({ accessToken: 'old-access', refreshToken: 'old-refresh' })
    window.localStorage.setItem(ACCESS_TOKEN_STORAGE_KEY, 'tab2-access')
    window.localStorage.setItem(REFRESH_TOKEN_STORAGE_KEY, 'tab2-refresh')

    const synced = syncSessionTokensFromStorage()

    expect(synced.accessToken).toBe('tab2-access')
    expect(synced.refreshToken).toBe('tab2-refresh')
    expect(getAccessToken()).toBe('tab2-access')
    expect(getRefreshToken()).toBe('tab2-refresh')
  })

  it('syncs on a storage event fired by another tab', () => {
    setSessionTokens({ accessToken: 'old-access', refreshToken: 'old-refresh' })
    window.localStorage.setItem(ACCESS_TOKEN_STORAGE_KEY, 'tab2-access')
    window.localStorage.setItem(REFRESH_TOKEN_STORAGE_KEY, 'tab2-refresh')

    window.dispatchEvent(new StorageEvent('storage', { key: REFRESH_TOKEN_STORAGE_KEY }))

    expect(getAccessToken()).toBe('tab2-access')
    expect(getRefreshToken()).toBe('tab2-refresh')
  })

  it('follows another tab logging out without writing back', () => {
    setSessionTokens({ accessToken: 'access', refreshToken: 'refresh' })
    window.localStorage.removeItem(ACCESS_TOKEN_STORAGE_KEY)
    window.localStorage.removeItem(REFRESH_TOKEN_STORAGE_KEY)
    const setItem = vi.spyOn(Storage.prototype, 'setItem')

    window.dispatchEvent(new StorageEvent('storage', { key: null }))

    expect(getAccessToken()).toBe('')
    expect(getRefreshToken()).toBe('')
    expect(setItem).not.toHaveBeenCalled()
    setItem.mockRestore()
  })

  it('leaves the store untouched when storage already matches', () => {
    setSessionTokens({ accessToken: 'same-access', refreshToken: 'same-refresh' })
    const before = useSessionStore.getState()
    const setItem = vi.spyOn(Storage.prototype, 'setItem')

    syncSessionTokensFromStorage()

    expect(useSessionStore.getState()).toBe(before)
    expect(setItem).not.toHaveBeenCalled()
    setItem.mockRestore()
  })

  it('clearSessionTokens clears both keys (used by logout path)', () => {
    setSessionTokens({ accessToken: 'a', refreshToken: 'r' })
    clearSessionTokens()
    expect(window.localStorage.getItem(ACCESS_TOKEN_STORAGE_KEY)).toBeNull()
    expect(window.localStorage.getItem(REFRESH_TOKEN_STORAGE_KEY)).toBeNull()
  })
})
