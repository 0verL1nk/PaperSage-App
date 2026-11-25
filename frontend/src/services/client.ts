import axios, { type AxiosRequestConfig, type AxiosResponse } from 'axios'
import { getOptionalApiBaseUrl, getRequiredApiBaseUrl, LocalBackendUnavailableError } from '@/lib/env'
import { AUTH } from '@/services/api-routes'
import { clearSessionTokens, getAccessToken, getRefreshToken, mergeRefreshedTokens, setSessionTokens, useSessionStore, type SessionTokens } from '@/store'
import { hideGlobalLoading, showGlobalLoading } from '@/store'
import { API_SUCCESS_CODE, type ApiEnvelope } from '@/services/contracts'
import {
  createBackendEnvelopeError,
  FrontendApiError,
  normalizeApiError,
} from '@/services/errors'
import { isDesktop } from '@/lib/desktop'
import { toast } from 'sonner'

/** 无响应的传输层失败（离线 / 服务不可达 / 超时）。有响应的 HTTP 错误不在此列。 */
function isTransportFailure(error: unknown): boolean {
  if (!axios.isAxiosError(error) || error.response) return false
  if (error.code === 'ERR_NETWORK' || error.code === 'ECONNABORTED' || error.code === 'ETIMEDOUT') {
    return true
  }
  return typeof navigator !== 'undefined' && navigator.onLine === false
}

function unreachableDescription(): string {
  const baseUrl =
    getOptionalApiBaseUrl() ||
    (typeof window !== 'undefined' ? window.location.origin : '当前地址')
  const hint = isDesktop()
    ? '请检查网络连接；若持续失败，请在 设置 → 关于 中确认 API 地址。'
    : '请检查网络连接后重试。'
  return `无法访问 ${baseUrl}，${hint}`
}

export type ApiRequestConfig<TData = unknown> = AxiosRequestConfig<TData> & {
  skipGlobalLoading?: boolean
  loadingText?: string
  skipAuthRefresh?: boolean
}

function withRequiredRuntimeConfig() {
  getRequiredApiBaseUrl()
}

/**
 * 凭据类 auth 端点的 401 是「这次输入不对」，不是会话过期：
 * 既不能走 refresh（会把错误变成整页跳转），也不能在失败分支里清 token 跳 /auth，
 * 否则页面上的表单与错误提示会一起被清空。唯一名单，避免两处判断漂移。
 */
function isCredentialAuthEndpoint(url: string | undefined): boolean {
  if (!url) return false
  return ['/auth/login', '/auth/register', '/auth/guest', '/auth/refresh', '/auth/email-code/']
    .some((path) => url.includes(path))
}

export const apiClient = axios.create({
  baseURL: getOptionalApiBaseUrl() || undefined,
  timeout: 60_000,
})

/**
 * 页面首屏数据查询的超时：离线或服务不可达时浏览器可能长时间不返回错误，
 * 60s 的全局超时会让页面一直停在「正在加载」。页面级查询统一用更短的上限，
 * 让错误态和重试入口及时出现。
 */
export const PAGE_QUERY_TIMEOUT_MS = 15_000

// Token refresh state to prevent multiple concurrent refresh requests
let isRefreshing = false
let refreshSubscribers: Array<{
  resolve: (tokens: SessionTokens) => void
  reject: (error: unknown) => void
}> = []

function subscribeTokenRefresh(resolve: (tokens: SessionTokens) => void, reject: (error: unknown) => void) {
  refreshSubscribers.push({ resolve, reject })
}

function notifyTokenRefresh(tokens: SessionTokens) {
  const subscribers = refreshSubscribers
  refreshSubscribers = []
  subscribers.forEach(({ resolve }) => resolve(tokens))
}

function rejectTokenRefresh(error: unknown) {
  const subscribers = refreshSubscribers
  refreshSubscribers = []
  subscribers.forEach(({ reject }) => reject(error))
}

// Refresh token endpoint
async function refreshAccessToken(): Promise<SessionTokens> {
  const baseURL = getRequiredApiBaseUrl() || undefined
  const refreshToken = getRefreshToken()

  if (!refreshToken) {
    throw new Error('No refresh token available')
  }

  // Use a separate axios instance to avoid interceptor loops
  const refreshClient = axios.create({
    baseURL,
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

    const newTokens = mergeRefreshedTokens({
      accessToken: response.data.data.accessToken,
      refreshToken: response.data.data.refreshToken,
    }, useSessionStore.getState())

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
      !isCredentialAuthEndpoint(requestConfig?.url) &&
      !requestConfig?.skipAuthRefresh
    ) {
      // Guests have no tokens to refresh: a 401 is a capability limit, not an
      // expired session. Bouncing them to /auth would discard device-local work.
      if (useSessionStore.getState().isGuest) {
        return Promise.reject(normalizeApiError(error))
      }
      // login/register 的 401 是凭据错误，应交给页面展示错误提示；
      // 走 refresh 重定向会整页跳转 /auth 并清空表单状态
      // If already refreshing, wait for the refresh to complete
      if (isRefreshing) {
        return new Promise((resolve, reject) => {
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
          }, reject)
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
        const normalized = normalizeApiError(refreshError)
        rejectTokenRefresh(normalized)
        if (refreshError instanceof LocalBackendUnavailableError) {
          return Promise.reject(normalized)
        }
        // Refresh failed, redirect to auth
        clearSessionTokens()
        // Only redirect if we're in a browser environment
        if (typeof window !== 'undefined') {
          window.location.href = '/auth'
        }
        return Promise.reject(normalized)
      } finally {
        isRefreshing = false
      }
    }

    // 无响应 = 传输层失败：离线 / 服务不可达 / 超时。统一归为 network，
    // 供 requestApi 换成可操作提示，而不是 axios 的 "Network Error"。
    const normalized = isTransportFailure(error)
      ? new FrontendApiError('Network request failed.', { kind: 'network', cause: error })
      : normalizeApiError(error)
    // auth 端点的 401/403 是凭据问题，交给页面渲染错误提示；
    // 这里若再整页跳转 /auth 会清空表单与错误状态（login wrong-password
    // 用例曾因此永远看不到 alert）
    if (!isCredentialAuthEndpoint(requestConfig?.url) && !useSessionStore.getState().isGuest && (normalized.status === 401 || normalized.status === 403)) {
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

/**
 * 传输层可能永久挂起：真实断网/弱网时浏览器既不返回成功也不返回错误，
 * axios 的 timeout 依赖底层 XHR 计时，挂起时不一定触发。页面级查询再套一层
 * JS 计时，保证错误态一定会在上限内出现。
 */
function withRequestTimeout<T>(request: Promise<T>, timeoutMs?: number): Promise<T> {
  if (!timeoutMs) return request
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => {
      reject(new FrontendApiError('请求超时，请检查网络后重试。', { kind: 'network' }))
    }, timeoutMs)
    request.then(
      (value) => {
        clearTimeout(timer)
        resolve(value)
      },
      (error) => {
        clearTimeout(timer)
        reject(error)
      },
    )
  })
}

export async function requestApi<TResponse, TBody = unknown>(
  config: ApiRequestConfig<TBody>,
) {
  try {
    const response = await withRequestTimeout(
      apiClient.request<ApiEnvelope<TResponse>, AxiosResponse<ApiEnvelope<TResponse>>, TBody>(config),
      config.timeout,
    )
    return unwrapApiEnvelope(response)
  } catch (error) {
    const normalized = normalizeApiError(error)
    // Auto-show error toast unless suppressed
    if (!(config as ApiRequestConfig).skipGlobalLoading) {
      if (normalized.kind === 'network') {
        // 固定 id：离线期间的重试共用一条 toast，sonner 按 id 更新而不堆叠。
        toast.error('无法连接到服务器', {
          id: 'api-unreachable',
          description: unreachableDescription(),
          duration: 4000,
        })
      } else {
        const msg = normalized.info || normalized.message
        toast.error('请求失败', { description: msg, duration: 4000 })
      }
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

export function patchJson<TResponse, TBody = unknown>(
  url: string,
  data?: TBody,
  config?: ApiRequestConfig<TBody>,
) {
  return requestApi<TResponse, TBody>({
    ...config,
    method: 'patch',
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
