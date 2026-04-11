import { FrontendApiError, postMultipart } from '@/shared/api'
import type { SessionTokens } from '@/shared/stores/session-store'

type AuthPayload = {
  username: string
  password: string
}

type LoginResponse = Partial<SessionTokens>

/**
 * SHA-256 hash using Web Crypto API
 */
async function sha256(message: string): Promise<string> {
  const msgBuffer = new TextEncoder().encode(message)
  const hashBuffer = await crypto.subtle.digest('SHA-256', msgBuffer)
  const hashArray = Array.from(new Uint8Array(hashBuffer))
  return hashArray.map((b) => b.toString(16).padStart(2, '0')).join('')
}

async function buildAuthFormData(payload: AuthPayload) {
  const formData = new FormData()

  formData.append('account', payload.username)
  formData.append('password', await sha256(payload.password))

  return formData
}

export async function loginWithPassword(payload: AuthPayload) {
  const formData = await buildAuthFormData(payload)
  const data = await postMultipart<LoginResponse>(
    '/api/account/login',
    formData,
    {
      skipGlobalLoading: true,
    },
  )

  if (!data?.accessToken || !data.refreshToken) {
    throw new FrontendApiError('登录响应缺少 accessToken 或 refreshToken。', {
      kind: 'backend',
      info: 'Missing accessToken or refreshToken in login response.',
      details: data,
    })
  }

  return {
    accessToken: data.accessToken,
    refreshToken: data.refreshToken,
  }
}

export async function registerWithPassword(payload: AuthPayload) {
  const formData = await buildAuthFormData(payload)
  await postMultipart<null>('/api/account/register', formData, {
    skipGlobalLoading: true,
  })
}
