import axios, { type AxiosRequestConfig, type AxiosResponse } from 'axios'
import { getOptionalApiBaseUrl, getRequiredApiBaseUrl } from '@/lib/env'
import { AUTH } from '@/services/api-routes'
import { clearSessionTokens, getAccessToken, getRefreshToken, setSessionTokens, type SessionTokens } from '@/store'
import { hideGlobalLoading, showGlobalLoading } from '@/store'
import { API_SUCCESS_CODE, type ApiEnvelope } from '@/services/contracts'
import {
  createBackendEnvelopeError,
  normalizeApiError,
  type FrontendApiError,
} from '@/services/errors'
import { toast } from 'sonner'

export type ApiRequestConfig<TData = unknown> = AxiosRequestConfig<TData> & {
  skipGlobalLoading?: boolean
  loadingText?: string
  skipAuthRefresh?: boolean
}

function withRequiredRuntimeConfig() {
  getRequiredApiBaseUrl()
}

export const apiClient = axios.create({
  baseURL: getOptionalApiBaseUrl() || undefined,
  timeout: 60_000,
})

// Token refresh state to prevent multiple concurrent refresh requests
let isRefreshing = false
let refreshSubscribers: ((tokens: SessionTokens) => void)[] = []

function subscribeTokenRefresh(callback: (tokens: SessionTokens) => void) {
  refreshSubscribers.push(callback)
}

function notifyTokenRefresh(tokens: SessionTokens) {
  refreshSubscribers.forEach((callback) => callback(tokens))
  refreshSubscribers = []
}

// Refresh token endpoint
async function refreshAccessToken(): Promise<SessionTokens> {
  const refreshToken = getRefreshToken()

  if (!refreshToken) {
    throw new Error('No refresh token available')
  }

  // Use a separate axios instance to avoid interceptor loops
  const refreshClient = axios.create({
    baseURL: getOptionalApiBaseUrl() || undefined,
    timeout: 30_000,
  })

  try {
    const response = await refreshClient.post<ApiEnvelope<SessionTokens>>(
      AUTH.refresh,
      { refresh_token: refreshToken },
      { headers: { 'Content-Type': 'application/json' } }
    )

    if (response.data.code !== API_SUCCESS_CODE) {
      throw createBackendEnvelopeError({
        code: response.data.code,
        info: response.data.info,
        details: response.data,
      })
    }

    const newTokens: SessionTokens = {
      accessToken: response.data.data.accessToken,
      refreshToken: response.data.data.refreshToken,
    }

    // Save new tokens
    setSessionTokens(newTokens)

    return newTokens
  } catch (error) {
    // Refresh failed, clear tokens
    clearSessionTokens()
    throw error
  }
}

// Request interceptor: add auth header
apiClient.interceptors.request.use(
  (config) => {
    // 环境切换在这里生效：baseURL 按当前选中的目标（online/local）逐请求解析，
    // 不在模块加载时固化——否则部署出去的静态前端切不回本地后端。
    config.baseURL = getOptionalApiBaseUrl() || undefined

    withRequiredRuntimeConfig()

    const accessToken = getAccessToken()
    if (accessToken) {
      config.headers = config.headers ?? {}
      config.headers.Authorization = `Bearer ${accessToken}`
    }

    const requestConfig = config as ApiRequestConfig
    // 顶层全局指示只跟随读请求（GET）；写操作由调用处的按钮
    // spinner/行内状态提供就地反馈，不再弹全局条（业界通行分层）
    const isRead = !requestConfig.method || requestConfig.method.toLowerCase() === 'get'
    if (!requestConfig.skipGlobalLoading && isRead) {
      showGlobalLoading(requestConfig.loadingText)
    }

    return config
  },
  (error) => Promise.reject(normalizeApiError(error)),
)

// Response interceptor: handle 401 and token refresh
apiClient.interceptors.response.use(
  (response) => {
    const requestConfig = response.config as ApiRequestConfig
    // 与请求侧对称：只有 show 过的（读请求）才 hide，避免偷走并发 GET 的计数
    const isRead = !requestConfig.method || requestConfig.method.toLowerCase() === 'get'
    if (!requestConfig.skipGlobalLoading && isRead) {
      hideGlobalLoading()
    }

    return response
  },
  async (error) => {
    const requestConfig = error.config as ApiRequestConfig | undefined
    const isRead = !requestConfig?.method || requestConfig?.method.toLowerCase() === 'get'
    if (!requestConfig?.skipGlobalLoading && isRead) {
      hideGlobalLoading()
    }

    // Only handle 401 errors that aren't already from the refresh endpoint
    // and aren't marked to skip auth refresh
    if (
      error.response?.status === 401 &&
      !requestConfig?.url?.includes('/auth/login') &&
      !requestConfig?.url?.includes('/auth/register') &&
      !requestConfig?.url?.includes('/auth/refresh') &&
      !requestConfig?.skipAuthRefresh
    ) {
      // login/register 的 401 是凭据错误，应交给页面展示错误提示；
      // 走 refresh 重定向会整页跳转 /auth 并清空表单状态
      // If already refreshing, wait for the refresh to complete
      if (isRefreshing) {
        return new Promise((resolve) => {
          subscribeTokenRefresh((tokens) => {
            // Retry the original request with new token
            const retryConfig = {
              ...requestConfig,
              headers: {
                ...requestConfig?.headers,
                Authorization: `Bearer ${tokens.accessToken}`,
              },
            }
            resolve(apiClient.request(retryConfig))
          })
        })
      }

      isRefreshing = true

      try {
        const newTokens = await refreshAccessToken()
        notifyTokenRefresh(newTokens)

        // Retry the original request with new token
        const retryConfig = {
          ...requestConfig,
          headers: {
            ...requestConfig?.headers,
            Authorization: `Bearer ${newTokens.accessToken}`,
          },
        }
        return apiClient.request(retryConfig)
      } catch (refreshError) {
        // Refresh failed, redirect to auth
        clearSessionTokens()
        // Only redirect if we're in a browser environment
        if (typeof window !== 'undefined') {
          window.location.href = '/auth'
        }
        return Promise.reject(normalizeApiError(refreshError))
      } finally {
        isRefreshing = false
      }
    }

    const normalized = normalizeApiError(error)
    // auth 端点的 401/403 是凭据问题，交给页面渲染错误提示；
    // 这里若再整页跳转 /auth 会清空表单与错误状态（login wrong-password
    // 用例曾因此永远看不到 alert）
    const isAuthEndpoint =
      requestConfig?.url?.includes('/auth/login') ||
      requestConfig?.url?.includes('/auth/register') ||
      requestConfig?.url?.includes('/auth/refresh')
    if (!isAuthEndpoint && (normalized.status === 401 || normalized.status === 403)) {
      clearSessionTokens()
      // Redirect to auth page
      if (typeof window !== 'undefined') {
        const currentPath = window.location.pathname
        window.location.href = `/auth?redirect=${encodeURIComponent(currentPath)}`
      }
    }

    return Promise.reject(normalized)
  },
)

export function unwrapApiEnvelope<TData>(response: AxiosResponse<ApiEnvelope<TData>>) {
  if (response.data.code !== API_SUCCESS_CODE) {
    throw createBackendEnvelopeError({
      code: response.data.code,
      info: response.data.info,
      details: response.data,
    })
  }

  return response.data.data
}

export async function requestApi<TResponse, TBody = unknown>(
  config: ApiRequestConfig<TBody>,
) {
  try {
    const response = await apiClient.request<ApiEnvelope<TResponse>, AxiosResponse<ApiEnvelope<TResponse>>, TBody>(
      config,
    )
    return unwrapApiEnvelope(response)
  } catch (error) {
    const normalized = normalizeApiError(error)
    // Auto-show error toast unless suppressed
    if (!(config as ApiRequestConfig).skipGlobalLoading) {
      const msg = normalized.info || normalized.message
      toast.error('请求失败', { description: msg, duration: 4000 })
    }
    throw normalized
  }
}

export async function requestBlob<TBody = unknown>(config: ApiRequestConfig<TBody>) {
  try {
    const response = await apiClient.request<Blob, AxiosResponse<Blob>, TBody>({
      ...config,
      responseType: 'blob',
    })
    return response.data
  } catch (error) {
    throw normalizeApiError(error)
  }
}

export function getJson<TResponse>(url: string, config?: ApiRequestConfig) {
  return requestApi<TResponse>({
    ...config,
    method: 'get',
    url,
  })
}

export function postJson<TResponse, TBody = unknown>(
  url: string,
  data?: TBody,
  config?: ApiRequestConfig<TBody>,
) {
  return requestApi<TResponse, TBody>({
    ...config,
    method: 'post',
    url,
    data,
  })
}

export function postMultipart<TResponse>(
  url: string,
  formData: FormData,
  config?: ApiRequestConfig<FormData>,
) {
  return requestApi<TResponse, FormData>({
    ...config,
    method: 'post',
    url,
    data: formData,
    headers: {
      ...config?.headers,
      'Content-Type': 'multipart/form-data',
    },
  })
}

export function downloadBlob(url: string, config?: ApiRequestConfig) {
  return requestBlob({
    ...config,
    method: 'get',
    url,
  })
}

export function putJson<TResponse, TBody = unknown>(
  url: string,
  data?: TBody,
  config?: ApiRequestConfig<TBody>,
) {
  return requestApi<TResponse, TBody>({
    ...config,
    method: 'put',
    url,
    data,
  })
}

export function deleteFetch<TResponse = void, TBody = unknown>(
  url: string,
  config?: ApiRequestConfig<TBody>,
) {
  return requestApi<TResponse, TBody>({
    ...config,
    method: 'delete',
    url,
  })
}

// Export for testing
export const tokenRefresh = {
  refreshAccessToken,
  subscribeTokenRefresh,
  notifyTokenRefresh,
  isRefreshing: () => isRefreshing,
}

export type { FrontendApiError }