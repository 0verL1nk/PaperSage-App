import { useEffect } from 'react'
import { redirect, useNavigate } from '@tanstack/react-router'
import { getAccessToken, useSessionStore } from '@/store'

export const AUTH_ROUTE_PATH = '/auth'

export function hasAuthenticatedSession() {
  return Boolean(getAccessToken())
}

export function requireAuthenticatedSession() {
  const accessToken = getAccessToken()
  if (!accessToken) {
    throw redirect({ to: AUTH_ROUTE_PATH })
  }

  return accessToken
}

type UseAuthGuardOptions = {
  enabled?: boolean
  redirectTo?: string
  replace?: boolean
}

export function useAuthGuard(options: UseAuthGuardOptions = {}) {
  const accessToken = useSessionStore((state) => state.accessToken)
  const navigate = useNavigate()
  const {
    enabled = true,
    redirectTo = AUTH_ROUTE_PATH,
    replace = true,
  } = options

  useEffect(() => {
    if (enabled && !accessToken) {
      void navigate({ to: redirectTo, replace })
    }
  }, [accessToken, enabled, navigate, redirectTo, replace])

  return {
    accessToken,
    isAuthenticated: Boolean(accessToken),
  }
}
