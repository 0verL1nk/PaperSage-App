import { FrontendApiError, getJson, postJson, postMultipart } from '@/services'
import { clearSessionTokens, getRefreshToken } from '@/store/session-store'
import type { SessionTokens } from '@/store/session-store'
import { AUTH } from './api-routes'

type AuthPayload = {
  username: string
  password: string
}

type LoginResponse = Partial<SessionTokens>

export type CurrentUser = {
  username: string
  email: string | null
  emailVerified: boolean
  hasPassword: boolean
  isGuest: boolean
  guestExpiresAt?: string
  isSuperuser: boolean
}

import { sha256 as jsSha256 } from 'js-sha256'

async function sha256(message: string): Promise<string> {
  const msgBuffer = new TextEncoder().encode(message)
  if (crypto.subtle) {
    const hashBuffer = await crypto.subtle.digest('SHA-256', msgBuffer)
    const hashArray = Array.from(new Uint8Array(hashBuffer))
    return hashArray.map((b) => b.toString(16).padStart(2, '0')).join('')
  }
  // Fallback for non-secure contexts (e.g. HTTP accessed via IP)
  return jsSha256(message)
}

async function buildAuthFormData(payload: AuthPayload) {
  const formData = new FormData()
  formData.append('account', payload.username)
  formData.append('password', await sha256(payload.password))
  return formData
}

async function buildRegisterFormData(email: string, rawPassword: string) {
  const formData = new FormData()
  formData.append('email', email)
  formData.append('password', await sha256(rawPassword))
  return formData
}

export async function loginWithPassword(payload: AuthPayload) {
  const formData = await buildAuthFormData(payload)
  const data = await postMultipart<LoginResponse>(
    AUTH.login,
    formData,
    { skipGlobalLoading: true },
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

export async function getCurrentUser(): Promise<CurrentUser> {
  return getJson<CurrentUser>(AUTH.me, { skipGlobalLoading: true })
}

/** 发送邮箱登录验证码（5 分钟有效，60s 冷却）。 */
export async function sendEmailCode(email: string) {
  return postJson<{ message?: string; cooldownSeconds?: number }>(
    AUTH.emailCodeSend,
    { email },
    // 弱网/断网时按钮不能无限转圈：与页面查询同样的 JS 级超时兜底。
    { skipGlobalLoading: true, timeout: 15_000 },
  )
}

/** 核销验证码登录；新邮箱会由后端自动注册并返回 isNewUser。 */
export async function verifyEmailCode(email: string, code: string) {
  const data = await postJson<Partial<SessionTokens> & { isNewUser?: boolean }>(
    AUTH.emailCodeVerify,
    { email, code },
    { skipGlobalLoading: true, timeout: 15_000 },
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
    isNewUser: data.isNewUser === true,
  }
}

/** 设置/修改密码；与登录一致，口令在前端 SHA-256 后再传输。 */
export async function setPassword(payload: { currentPassword?: string; newPassword: string }) {
  return postJson<{ message?: string; hasPassword?: boolean }>(
    AUTH.password,
    {
      currentPassword: payload.currentPassword ? await sha256(payload.currentPassword) : undefined,
      newPassword: await sha256(payload.newPassword),
    },
    { skipGlobalLoading: true },
  )
}

export async function registerWithEmail(email: string, rawPassword: string) {
  const formData = await buildRegisterFormData(email, rawPassword)
  const data = await postMultipart<{ message?: string }>(AUTH.register, formData, {
    skipGlobalLoading: true,
  })
  return data
}

export async function getEmailDomains(): Promise<string[]> {
  const data = await getJson<{ domains?: string[] }>('/api/v1/auth/email-domains')
  return data?.domains ?? []
}

export async function resendVerification(email: string) {
  return postJson<{ message?: string }>('/api/v1/auth/resend-verification', { email })
}

export async function verifyEmail(token: string) {
  return getJson<{ verified: boolean }>(`/api/v1/auth/verify-email?token=${encodeURIComponent(token)}`)
}

export async function requestPasswordReset(email: string) {
  return postJson<{ message?: string }>(AUTH.passwordResetRequest, { email }, { skipGlobalLoading: true })
}

export async function confirmPasswordReset(token: string, rawPassword: string) {
  return postJson<{ message?: string }>(AUTH.passwordResetConfirm, {
    token,
    password: await sha256(rawPassword),
  }, { skipGlobalLoading: true })
}

export async function logout() {
  const refreshToken = getRefreshToken()
  try {
    // 后端只接受 JSON {refresh_token}；无 token（本地访客）时无需请求。
    if (refreshToken) {
      await postJson<null>(AUTH.logout, { refresh_token: refreshToken }, {
        skipGlobalLoading: true,
        skipAuthRefresh: true,
      })
    }
  } finally {
    clearSessionTokens()
    if (typeof window !== 'undefined') {
      window.location.href = '/auth'
    }
  }
}
