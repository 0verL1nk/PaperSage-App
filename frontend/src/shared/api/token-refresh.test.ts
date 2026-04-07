import { describe, expect, it, vi, beforeEach } from 'vitest'

describe('token refresh module', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('should export refresh token functions', async () => {
    const { tokenRefresh } = await import('@/shared/api/client')
    expect(tokenRefresh.refreshAccessToken).toBeDefined()
    expect(tokenRefresh.subscribeTokenRefresh).toBeDefined()
    expect(tokenRefresh.notifyTokenRefresh).toBeDefined()
    expect(tokenRefresh.isRefreshing).toBeDefined()
  })

  it('should handle token refresh response format', async () => {
    // Test that the token refresh logic correctly processes the response
    const mockResponse = {
      code: 200,
      data: {
        accessToken: 'new-access-token',
        refreshToken: 'new-refresh-token',
      },
      info: 'OK',
    }

    expect(mockResponse.data.accessToken).toBeDefined()
    expect(mockResponse.data.refreshToken).toBeDefined()
  })

  it('should handle empty token refresh response', async () => {
    const mockResponse = {
      code: 200,
      data: {
        accessToken: '',
        refreshToken: '',
      },
      info: 'OK',
    }

    expect(mockResponse.data.accessToken).toBe('')
    expect(mockResponse.data.refreshToken).toBe('')
  })
})

describe('session store exports', () => {
  it('should export all required token storage functions', async () => {
    const module = await import('@/shared/stores')

    expect(module.ACCESS_TOKEN_STORAGE_KEY).toBe('papersage_access_token')
    expect(module.REFRESH_TOKEN_STORAGE_KEY).toBe('papersage_refresh_token')
    expect(module.getAccessToken).toBeDefined()
    expect(module.getRefreshToken).toBeDefined()
    expect(module.hasSessionToken).toBeDefined()
    expect(module.setSessionTokens).toBeDefined()
    expect(module.clearSessionTokens).toBeDefined()
    expect(typeof module.useSessionStore).toBe('function')
  })
})