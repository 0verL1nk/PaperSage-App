import { create } from 'zustand'

const ACCESS_TOKEN_KEY = 'papersage_access_token'
const REFRESH_TOKEN_KEY = 'papersage_refresh_token'

export type SessionTokens = {
  accessToken: string
  refreshToken: string
}

type SessionState = {
  accessToken: string
  refreshToken: string
  restored: boolean
  setTokens: (tokens: SessionTokens) => void
  clearTokens: () => void
  restoreSession: () => SessionTokens
}

function canUseBrowserStorage() {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined'
}

function readStoredToken() {
  if (!canUseBrowserStorage()) {
    return {
      accessToken: '',
      refreshToken: '',
    }
  }

  return {
    accessToken: window.localStorage.getItem(ACCESS_TOKEN_KEY) || '',
    refreshToken: window.localStorage.getItem(REFRESH_TOKEN_KEY) || '',
  }
}

function writeStoredToken(tokens: SessionTokens) {
  if (!canUseBrowserStorage()) {
    return
  }

  if (tokens.accessToken) {
    window.localStorage.setItem(ACCESS_TOKEN_KEY, tokens.accessToken)
  } else {
    window.localStorage.removeItem(ACCESS_TOKEN_KEY)
  }

  if (tokens.refreshToken) {
    window.localStorage.setItem(REFRESH_TOKEN_KEY, tokens.refreshToken)
  } else {
    window.localStorage.removeItem(REFRESH_TOKEN_KEY)
  }
}

export const useSessionStore = create<SessionState>((set) => ({
  accessToken: '',
  refreshToken: '',
  restored: false,
  setTokens: (tokens) => {
    writeStoredToken(tokens)
    set({ ...tokens, restored: true })
  },
  clearTokens: () => {
    writeStoredToken({ accessToken: '', refreshToken: '' })
    set({ accessToken: '', refreshToken: '', restored: true })
  },
  restoreSession: () => {
    const tokens = readStoredToken()
    set({ ...tokens, restored: true })
    return tokens
  },
}))

export function getAccessToken() {
  return useSessionStore.getState().accessToken
}

export function getRefreshToken() {
  return useSessionStore.getState().refreshToken
}

export function hasSessionToken() {
  return Boolean(getAccessToken())
}

export function restoreSessionState() {
  return useSessionStore.getState().restoreSession()
}

export function setSessionTokens(tokens: SessionTokens) {
  return useSessionStore.getState().setTokens(tokens)
}

export function clearSessionTokens() {
  return useSessionStore.getState().clearTokens()
}

export {
  ACCESS_TOKEN_KEY as ACCESS_TOKEN_STORAGE_KEY,
  REFRESH_TOKEN_KEY as REFRESH_TOKEN_STORAGE_KEY,
}
