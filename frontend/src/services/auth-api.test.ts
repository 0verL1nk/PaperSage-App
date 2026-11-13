import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { FrontendApiError, getJson, postJson, postMultipart } from '@/services'
import { confirmPasswordReset, getCurrentUser, loginWithPassword, registerWithEmail, requestPasswordReset } from '@/services/auth-api'
import { sha256 as jsSha256 } from 'js-sha256'

vi.mock('@/services', async () => {
  const actual = await vi.importActual<typeof import('@/services')>('@/services')

  return {
    ...actual,
    getJson: vi.fn(),
    postMultipart: vi.fn(),
    postJson: vi.fn(),
  }
})

/**
 * Reference SHA-256 implementation using Web Crypto. The production code
 * mirrors this when crypto.subtle is available, and falls back to js-sha256
 * otherwise. The two paths MUST produce identical hex output.
 */
async function sha256WebCrypto(message: string): Promise<string> {
  const msgBuffer = new TextEncoder().encode(message)
  const hashBuffer = await crypto.subtle.digest('SHA-256', msgBuffer)
  const hashArray = Array.from(new Uint8Array(hashBuffer))
  return hashArray.map((b) => b.toString(16).padStart(2, '0')).join('')
}
describe('auth api', () => {
  beforeEach(() => {
    vi.mocked(postMultipart).mockReset()
    vi.mocked(getJson).mockReset()
    vi.mocked(postJson).mockReset()
  })

  it('reads the active account identity from the normal auth endpoint', async () => {
    vi.mocked(getJson).mockResolvedValue({ isGuest: false, emailVerified: true })
    await expect(getCurrentUser()).resolves.toMatchObject({ isGuest: false, emailVerified: true })
    expect(getJson).toHaveBeenCalledWith('/api/v1/auth/me', { skipGlobalLoading: true })
  })

  it('requests a reset link without exposing whether the email exists', async () => {
    vi.mocked(postJson).mockResolvedValue({ message: '如果该邮箱已验证并关联 PaperSage 账号，我们会发送密码重置链接。' })
    await requestPasswordReset('alice@example.com')
    expect(postJson).toHaveBeenCalledWith(
      '/api/v1/auth/password-reset/request',
      { email: 'alice@example.com' },
      { skipGlobalLoading: true },
    )
  })

  it('hashes a new password before submitting the reset token', async () => {
    vi.mocked(postJson).mockResolvedValue({ message: '密码已重置' })
    await confirmPasswordReset('opaque-reset-token', 'NewSecret123')
    const [, payload] = vi.mocked(postJson).mock.calls[0] ?? []
    expect(postJson).toHaveBeenCalledWith(
      '/api/v1/auth/password-reset/confirm',
      { token: 'opaque-reset-token', password: await sha256WebCrypto('NewSecret123') },
      { skipGlobalLoading: true },
    )
    expect(payload).not.toHaveProperty('rawPassword')
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
      '/api/v1/auth/login',
      expect.any(FormData),
      expect.objectContaining({
        skipGlobalLoading: true,
      }),
    )

    const formData = vi.mocked(postMultipart).mock.calls[0]?.[1]

    expect(formData?.get('account')).toBe('alice')
    expect(formData?.get('password')).toBe(await sha256WebCrypto('Pass1234'))
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
      registerWithEmail('alice@hit.edu.cn', 'Pass1234'),
    ).resolves.toBeNull()

    expect(postMultipart).toHaveBeenCalledTimes(1)
    expect(postMultipart).toHaveBeenCalledWith(
      '/api/v1/auth/register',
      expect.any(FormData),
      expect.objectContaining({
        skipGlobalLoading: true,
      }),
    )

    const formData = vi.mocked(postMultipart).mock.calls[0]?.[1]

    expect(formData?.get('email')).toBe('alice@hit.edu.cn')
    expect(formData?.get('password')).toBe(await sha256WebCrypto('Pass1234'))
  })
})

describe('auth api — Web Crypto fallback', () => {
  // crypto.subtle is only available in "secure contexts" (HTTPS or localhost).
  // When the user accesses the app via IP over plain HTTP, crypto.subtle is
  // undefined and our code must fall back to js-sha256. These tests stub the
  // global to simulate that branch.

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  /** Capture the password hash sent to the server for a given input. */
  async function hashSubmitted(password: string): Promise<string> {
    vi.mocked(postMultipart).mockReset()
    vi.mocked(postMultipart).mockResolvedValue({
      accessToken: 'a',
      refreshToken: 'r',
    })
    await loginWithPassword({ username: 'u', password })
    const formData = vi.mocked(postMultipart).mock.calls[0]?.[1] as FormData
    return formData.get('password') as string
  }

  it('submits a valid 64-char hex hash when crypto.subtle is unavailable', async () => {
    vi.stubGlobal('crypto', { subtle: undefined } as unknown as Crypto)
    const hashed = await hashSubmitted('Secret123')
    expect(hashed).toMatch(/^[0-9a-f]{64}$/)
  })

  it('fallback path produces the same hash as the Web Crypto path', async () => {
    // Path 1: Web Crypto (default in jsdom secure-context simulation)
    const fromWebCrypto = await hashSubmitted('Secret123')

    // Path 2: js-sha256 fallback (stub crypto.subtle to undefined)
    vi.stubGlobal('crypto', { subtle: undefined } as unknown as Crypto)
    const fromFallback = await hashSubmitted('Secret123')

    // Both paths MUST produce identical output, otherwise password hashes
    // sent from non-secure contexts would not match those from secure ones.
    expect(fromFallback).toBe(fromWebCrypto)
  })

  it('js-sha256 reference matches Web Crypto for varied inputs', async () => {
    const inputs = ['', 'a', 'Pass1234', 'a fox jumps over', 'a'.repeat(1000)]
    for (const input of inputs) {
      const fromWeb = await sha256WebCrypto(input)
      const fromLib = jsSha256(input)
      expect(fromLib, `mismatch for ${JSON.stringify(input)}`).toBe(fromWeb)
    }
  })

  it('register path also falls back without throwing', async () => {
    vi.stubGlobal('crypto', { subtle: undefined } as unknown as Crypto)
    vi.mocked(postMultipart).mockReset()
    vi.mocked(postMultipart).mockResolvedValue(null)
    await expect(
      registerWithEmail('eve@hit.edu.cn', 'pw'),
    ).resolves.toBeNull()
  })
})
