import { create } from 'zustand'

const ACCESS_TOKEN_KEY = 'papersage_access_token'
const REFRESH_TOKEN_KEY = 'papersage_refresh_token'
const LOCAL_GUEST_MODE_KEY = 'papersage_local_guest_mode'
const GUEST_IMPORT_PENDING_KEY = 'papersage_guest_import_pending'
const TOKEN_TYPE_KEY = 'papersage_token_type'
// Remove data from the retired server-side guest session model on restore.
const LEGACY_GUEST_KEYS = ['papersage_is_guest', 'papersage_session_expires_at', 'papersage_guest_limits'] as const

export type SessionTokens = {
  accessToken: string
  refreshToken: string
  tokenType?: string
}

export type GuestSessionState = {
  isGuest: boolean
  importPending: boolean
}

type SessionState = {
  accessToken: string
  refreshToken: string
  isGuest: boolean
  guestImportPending: boolean
  tokenType: string
  restored: boolean
  setTokens: (tokens: SessionTokens) => void
  clearTokens: () => void
  activateLocalGuest: () => void
  setGuestImportPending: (pending: boolean) => void
  restoreSession: () => SessionTokens
}

function canUseBrowserStorage() {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined'
}

function readStoredSession() {
  if (!canUseBrowserStorage()) {
    return { accessToken: '', refreshToken: '', isGuest: false, guestImportPending: false, tokenType: 'Bearer' }
  }
  const hadRetiredServerGuest = window.localStorage.getItem(LEGACY_GUEST_KEYS[0]) === 'true'
  for (const key of LEGACY_GUEST_KEYS) window.localStorage.removeItem(key)
  return {
    accessToken: hadRetiredServerGuest ? '' : window.localStorage.getItem(ACCESS_TOKEN_KEY) || '',
    refreshToken: hadRetiredServerGuest ? '' : window.localStorage.getItem(REFRESH_TOKEN_KEY) || '',
    isGuest: window.localStorage.getItem(LOCAL_GUEST_MODE_KEY) === 'true',
    guestImportPending: window.localStorage.getItem(GUEST_IMPORT_PENDING_KEY) === 'true',
    tokenType: window.localStorage.getItem(TOKEN_TYPE_KEY) || 'Bearer',
  }
}

function persistSession(session: Pick<SessionState, 'accessToken' | 'refreshToken' | 'isGuest' | 'guestImportPending' | 'tokenType'>) {
  if (!canUseBrowserStorage()) return
  if (session.accessToken) window.localStorage.setItem(ACCESS_TOKEN_KEY, session.accessToken)
  else window.localStorage.removeItem(ACCESS_TOKEN_KEY)
  if (session.refreshToken) window.localStorage.setItem(REFRESH_TOKEN_KEY, session.refreshToken)
  else window.localStorage.removeItem(REFRESH_TOKEN_KEY)
  if (session.isGuest) window.localStorage.setItem(LOCAL_GUEST_MODE_KEY, 'true')
  else window.localStorage.removeItem(LOCAL_GUEST_MODE_KEY)
  if (session.guestImportPending) window.localStorage.setItem(GUEST_IMPORT_PENDING_KEY, 'true')
  else window.localStorage.removeItem(GUEST_IMPORT_PENDING_KEY)
  if (session.tokenType) window.localStorage.setItem(TOKEN_TYPE_KEY, session.tokenType)
  else window.localStorage.removeItem(TOKEN_TYPE_KEY)
}

export const useSessionStore = create<SessionState>((set, get) => ({
  accessToken: '',
  refreshToken: '',
  isGuest: false,
  guestImportPending: false,
  tokenType: 'Bearer',
  restored: false,
  setTokens: (tokens) => {
    const next = {
      accessToken: tokens.accessToken,
      refreshToken: tokens.refreshToken,
      isGuest: false,
      // Preserve the pending import flag across account login and page reload.
      guestImportPending: get().guestImportPending || (canUseBrowserStorage() && window.localStorage.getItem(GUEST_IMPORT_PENDING_KEY) === 'true'),
      tokenType: tokens.tokenType || 'Bearer',
    }
    persistSession(next)
    set({ ...next, restored: true })
  },
  clearTokens: () => {
    const next = { accessToken: '', refreshToken: '', isGuest: false, guestImportPending: get().guestImportPending, tokenType: 'Bearer' }
    persistSession(next)
    set({ ...next, restored: true })
  },
  activateLocalGuest: () => {
    const next = { accessToken: '', refreshToken: '', isGuest: true, guestImportPending: true, tokenType: 'Bearer' }
    persistSession(next)
    set({ ...next, restored: true })
  },
  setGuestImportPending: (pending) => {
    const current = get()
    const next = { ...current, guestImportPending: pending }
    persistSession(next)
    set({ guestImportPending: pending })
  },
  restoreSession: () => {
    const saved = readStoredSession()
    // An account token always takes precedence over local guest mode after login.
    const normalized = saved.accessToken
      ? { ...saved, isGuest: false }
      : { ...saved, accessToken: '', refreshToken: '', tokenType: 'Bearer' }
    persistSession(normalized)
    set({ ...normalized, restored: true })
    return { accessToken: normalized.accessToken, refreshToken: normalized.refreshToken, tokenType: normalized.tokenType }
  },
}))

export function getAccessToken() {
  return useSessionStore.getState().accessToken
}

export function getRefreshToken() {
  return useSessionStore.getState().refreshToken
}

export function selectGuestSession(state: SessionState): GuestSessionState {
  return { isGuest: state.isGuest, importPending: state.guestImportPending }
}

export function mergeRefreshedTokens(tokens: Pick<SessionTokens, 'accessToken' | 'refreshToken'>, _legacySession?: unknown): SessionTokens {
  void _legacySession
  return { ...tokens, tokenType: 'Bearer' }
}

export function hasSessionToken() {
  return Boolean(getAccessToken())
}

export function hasWorkspaceSession() {
  const state = useSessionStore.getState()
  return Boolean(state.accessToken) || state.isGuest
}

export function restoreSessionState() {
  return useSessionStore.getState().restoreSession()
}

export function setSessionTokens(tokens: SessionTokens) {
  return useSessionStore.getState().setTokens(tokens)
}

export function activateLocalGuestSession() {
  return useSessionStore.getState().activateLocalGuest()
}

export function setGuestImportPending(pending: boolean) {
  return useSessionStore.getState().setGuestImportPending(pending)
}

export function clearSessionTokens() {
  return useSessionStore.getState().clearTokens()
}

export {
  ACCESS_TOKEN_KEY as ACCESS_TOKEN_STORAGE_KEY,
  REFRESH_TOKEN_KEY as REFRESH_TOKEN_STORAGE_KEY,
}
