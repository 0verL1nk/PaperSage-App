import { describe, expect, it, vi, beforeEach } from 'vitest'
import { FrontendApiError, postMultipart } from '@/shared/api'
import { loginWithPassword, registerWithPassword } from '@/features/auth/api'

vi.mock('@/shared/api', async () => {
  const actual = await vi.importActual<typeof import('@/shared/api')>('@/shared/api')

  return {
    ...actual,
    postMultipart: vi.fn(),
  }
})

/**
 * Calculate SHA-256 hash (same as the production code)
 */
async function sha256(message: string): Promise<string> {
  const msgBuffer = new TextEncoder().encode(message)
  const hashBuffer = await crypto.subtle.digest('SHA-256', msgBuffer)
  const hashArray = Array.from(new Uint8Array(hashBuffer))
  return hashArray.map((b) => b.toString(16).padStart(2, '0')).join('')
}

describe('auth api', () => {
  beforeEach(() => {
    vi.mocked(postMultipart).mockReset()
  })

  it('hashes the password and submits login as multipart data', async () => {
    vi.mocked(postMultipart).mockResolvedValue({
      accessToken: 'access-token',
      refreshToken: 'refresh-token',
    })

    await expect(
      loginWithPassword({
        username: 'alice',
        password: 'Pass1234',
      }),
    ).resolves.toEqual({
      accessToken: 'access-token',
      refreshToken: 'refresh-token',
    })

    expect(postMultipart).toHaveBeenCalledTimes(1)
    expect(postMultipart).toHaveBeenCalledWith(
      '/api/account/login',
      expect.any(FormData),
      expect.objectContaining({
        skipGlobalLoading: true,
      }),
    )

    const formData = vi.mocked(postMultipart).mock.calls[0]?.[1]

    expect(formData?.get('account')).toBe('alice')
    expect(formData?.get('password')).toBe(await sha256('Pass1234'))
  })

  it('rejects login responses that do not provide both long and short tokens', async () => {
    vi.mocked(postMultipart).mockResolvedValue({
      accessToken: 'access-only',
    })

    await expect(
      loginWithPassword({
        username: 'alice',
        password: 'Pass1234',
      }),
    ).rejects.toEqual(
      expect.objectContaining<Partial<FrontendApiError>>({
        message: '登录响应缺少 accessToken 或 refreshToken。',
        kind: 'backend',
      }),
    )
  })

  it('hashes the password and submits register as multipart data', async () => {
    vi.mocked(postMultipart).mockResolvedValue(null)

    await expect(
      registerWithPassword({
        username: 'alice',
        password: 'Pass1234',
      }),
    ).resolves.toBeUndefined()

    expect(postMultipart).toHaveBeenCalledTimes(1)
    expect(postMultipart).toHaveBeenCalledWith(
      '/api/account/register',
      expect.any(FormData),
      expect.objectContaining({
        skipGlobalLoading: true,
      }),
    )

    const formData = vi.mocked(postMultipart).mock.calls[0]?.[1]

    expect(formData?.get('account')).toBe('alice')
    expect(formData?.get('password')).toBe(await sha256('Pass1234'))
  })
})
