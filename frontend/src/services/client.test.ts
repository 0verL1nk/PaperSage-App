import { afterEach, describe, expect, it, vi } from 'vitest'
import axios, { AxiosError, type AxiosAdapter, type AxiosInstance, type InternalAxiosRequestConfig } from 'axios'
import { waitFor } from '@testing-library/react'
import { toast } from 'sonner'
import { appEnv, setApiTarget, setCustomApiBaseUrl, setDesktopLocalApiBaseUrl, setDesktopLocalBackendError } from '@/lib/env'
import { getAccessToken, getRefreshToken, setSessionTokens } from '@/store'
import { apiClient, getJson, tokenRefresh } from '@/services/client'

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))
// client.ts → lib/desktop.ts imports the Wails runtime; keep it inert in jsdom.
vi.mock('@wailsio/runtime', () => ({ Call: { ByName: vi.fn() }, Events: { On: vi.fn() } }))

const originalAdapter = apiClient.defaults.adapter

function failWithNetworkError() {
  apiClient.defaults.adapter = async (config) => {
    throw new AxiosError('Network Error', 'ERR_NETWORK', config, {})
  }
}

type ErrorToastArgs = [string, { id?: string; description?: string }?]

function errorToastCalls() {
  return vi.mocked(toast.error).mock.calls as unknown as ErrorToastArgs[]
}

afterEach(() => {
  apiClient.defaults.adapter = originalAdapter
  vi.mocked(toast.error).mockClear()
})

describe('unreachable API errors', () => {
  it('shows one actionable message instead of the generic request-failed toast', async () => {
    failWithNetworkError()

    await expect(getJson('/health')).rejects.toMatchObject({ kind: 'network' })

    const calls = errorToastCalls()
    expect(calls).toHaveLength(1)
    expect(calls[0][0]).toBe('无法连接到服务器')
    expect(calls[0][1]?.id).toBe('api-unreachable')
    expect(calls[0][1]?.description).toContain('无法访问')
    expect(calls[0][1]?.description).toContain('请检查网络')
  })

  it('reuses the same toast id on repeated failures so outage retries do not stack', async () => {
    failWithNetworkError()

    await expect(getJson('/health')).rejects.toBeTruthy()
    await expect(getJson('/health')).rejects.toBeTruthy()

    const calls = errorToastCalls()
    expect(calls).toHaveLength(2)
    expect(new Set(calls.map(([, options]) => options?.id))).toEqual(new Set(['api-unreachable']))
    expect(calls.every(([title]) => title === '无法连接到服务器')).toBe(true)
  })
})

describe('hung transport', () => {
  it('rejects a page query at its own timeout even when the transport never settles', async () => {
    vi.useFakeTimers()
    try {
      // 真实断网时 XHR 可能既不成功也不失败；这里模拟一个永不 settle 的传输层。
      apiClient.defaults.adapter = () => new Promise(() => {})

      const pending = getJson('/writing-state', { timeout: 15_000, skipGlobalLoading: true })
      const assertion = expect(pending).rejects.toMatchObject({
        kind: 'network',
        message: '请求超时，请检查网络后重试。',
      })

      await vi.advanceTimersByTimeAsync(15_000)
      await assertion
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('credential auth endpoints skip token refresh', () => {
  afterEach(() => {
    setSessionTokens({ accessToken: '', refreshToken: '' })
    vi.restoreAllMocks()
  })

  // B1 回归：验证码错误是 401，绝不能走 refresh → 清 token → 整页跳 /auth，
  // 否则用户输入被清空且看不到任何错误提示。
  it('surfaces wrong email-code errors without refreshing or redirecting', async () => {
    setSessionTokens({ accessToken: 'old-access', refreshToken: 'old-refresh' })
    const create = vi.spyOn(axios, 'create')
    const adapter: AxiosAdapter = async (config) => {
      throw new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, {}, {
        config,
        data: { code: 401, info: 'Invalid or expired verification code' },
        headers: {},
        status: 401,
        statusText: 'Unauthorized',
      })
    }

    await expect(
      apiClient.post('/api/v1/auth/email-code/verify', { email: 'a@hit.edu.cn', code: '000000' }, { adapter }),
    ).rejects.toMatchObject({ status: 401, message: '验证码错误或已过期，请重新获取。' })

    expect(create).not.toHaveBeenCalled()
    expect(getAccessToken()).toBe('old-access')
    expect(getRefreshToken()).toBe('old-refresh')
  })
})

describe('refresh with an unavailable local backend', () => {
  const backendError = '本地后端版本不匹配'
  const tokens = { accessToken: 'old-access', refreshToken: 'old-refresh' }

  afterEach(() => {
    setDesktopLocalApiBaseUrl(appEnv.localApiBaseUrl)
    setDesktopLocalApiBaseUrl('')
    vi.restoreAllMocks()
  })

  it('does not send refresh traffic or clear tokens after local startup fails', async () => {
    setSessionTokens(tokens)
    setApiTarget('local')
    setDesktopLocalBackendError(backendError)
    const post = vi.fn().mockResolvedValue({ data: { code: 200, data: { accessToken: 'new', refreshToken: 'new' } } })
    const create = vi.spyOn(axios, 'create').mockReturnValue({ post } as unknown as AxiosInstance)

    await expect(tokenRefresh.refreshAccessToken()).rejects.toThrow(backendError)
    expect(create).not.toHaveBeenCalled()
    expect(post).not.toHaveBeenCalled()
    expect(getAccessToken()).toBe(tokens.accessToken)
    expect(getRefreshToken()).toBe(tokens.refreshToken)
  })

  it('prioritizes the local failure even when only an access token remains', async () => {
    setSessionTokens({ accessToken: tokens.accessToken, refreshToken: '' })
    setApiTarget('local')
    setDesktopLocalBackendError(backendError)

    await expect(tokenRefresh.refreshAccessToken()).rejects.toThrow(backendError)
    expect(getAccessToken()).toBe(tokens.accessToken)
  })

  it.each([true, false])('preserves tokens after an in-flight 401 when refresh token present: %s', async (hasRefreshToken) => {
    const refreshToken = hasRefreshToken ? tokens.refreshToken : ''
    setSessionTokens({ ...tokens, refreshToken })
    setApiTarget('local')
    const post = vi.fn().mockResolvedValue({ data: { code: 200, data: { accessToken: 'new', refreshToken: 'new' } } })
    vi.spyOn(axios, 'create').mockReturnValue({ post } as unknown as AxiosInstance)
    let resume!: () => void
    let started!: () => void
    const waitFor401 = new Promise<void>((resolve) => { resume = resolve })
    const requestStarted = new Promise<void>((resolve) => { started = resolve })
    const adapter: AxiosAdapter = async (config) => {
      started()
      await waitFor401
      throw new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, {}, {
        config, data: {}, headers: {}, status: 401, statusText: 'Unauthorized',
      })
    }

    const request = apiClient.get('/protected', { adapter })
    await requestStarted
    setDesktopLocalBackendError(backendError)
    resume()

    await expect(request).rejects.toThrow(backendError)
    expect(post).not.toHaveBeenCalled()
    expect(getAccessToken()).toBe(tokens.accessToken)
    expect(getRefreshToken()).toBe(refreshToken)
  })

  it.each(['online', 'custom', 'browser-local', 'desktop-local'] as const)('keeps refreshing on the %s target', async (target) => {
    setSessionTokens(tokens)
    if (target === 'online' || target === 'custom') {
      setDesktopLocalBackendError(backendError)
      setCustomApiBaseUrl('https://custom.example.test')
      setApiTarget(target)
    } else {
      if (target === 'desktop-local') setDesktopLocalApiBaseUrl('http://127.0.0.1:6001')
      setApiTarget('local')
    }
    const post = vi.fn().mockResolvedValue({ data: { code: 200, data: { accessToken: 'new-access', refreshToken: 'new-refresh' } } })
    const create = vi.spyOn(axios, 'create').mockReturnValue({ post } as unknown as AxiosInstance)

    await expect(tokenRefresh.refreshAccessToken()).resolves.toMatchObject({ accessToken: 'new-access', refreshToken: 'new-refresh' })
    const baseURL = target === 'online' ? appEnv.apiBaseUrl || undefined
      : target === 'custom' ? 'https://custom.example.test'
        : target === 'desktop-local' ? 'http://127.0.0.1:6001' : appEnv.localApiBaseUrl
    expect(create).toHaveBeenCalledWith(expect.objectContaining({ baseURL }))
    expect(post).toHaveBeenCalledOnce()
  })
})

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail })
  return { promise, resolve, reject }
}

function unauthorized(config: InternalAxiosRequestConfig) {
  return new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, {}, {
    config, data: {}, headers: {}, status: 401, statusText: 'Unauthorized',
  })
}

// Run one event-loop turn so the mocked adapter's response reaches the interceptor.
const nextTurn = () => new Promise<void>((resolve) => setTimeout(resolve, 0))

describe('concurrent refresh subscribers', () => {
  const tokens = { accessToken: 'old-access', refreshToken: 'old-refresh' }
  const renewed = { accessToken: 'new-access', refreshToken: 'new-refresh' }

  afterEach(() => {
    setDesktopLocalApiBaseUrl(appEnv.localApiBaseUrl)
    setDesktopLocalApiBaseUrl('')
    window.history.replaceState(null, '', '/')
    vi.restoreAllMocks()
  })

  function adapterWithOneUnauthorized() {
    const attempts = new Map<string, number>()
    return vi.fn<AxiosAdapter>(async (config) => {
      const url = config.url ?? ''
      const count = (attempts.get(url) ?? 0) + 1
      attempts.set(url, count)
      if (count === 1) throw unauthorized(config)
      return { config, data: {}, headers: {}, status: 200, statusText: 'OK' }
    })
  }

  it('rejects every waiting request and clears subscribers when refresh fails', async () => {
    window.history.replaceState(null, '', '/auth')
    setApiTarget('online')
    setSessionTokens(tokens)
    const refresh = deferred<unknown>()
    const post = vi.fn(() => refresh.promise)
    vi.spyOn(axios, 'create').mockReturnValue({ post } as unknown as AxiosInstance)
    const adapter = adapterWithOneUnauthorized()

    const first = apiClient.get('/first', { adapter }).catch((error: unknown) => error)
    await waitFor(() => expect(post).toHaveBeenCalledOnce())
    const secondSettled = vi.fn()
    const thirdSettled = vi.fn()
    void apiClient.put('/second', { content: 'draft' }, { adapter }).then(
      () => secondSettled('resolved'),
      (error: unknown) => secondSettled('rejected', error),
    )
    void apiClient.post('/third', { content: 'draft' }, { adapter }).then(
      () => thirdSettled('resolved'),
      (error: unknown) => thirdSettled('rejected', error),
    )
    await waitFor(() => expect(adapter).toHaveBeenCalledTimes(3))
    await nextTurn()

    refresh.reject(new Error('refresh failed'))
    expect(await first).toMatchObject({ message: 'refresh failed' })
    await nextTurn()
    try {
      expect(secondSettled).toHaveBeenCalledWith('rejected', expect.objectContaining({ message: 'refresh failed' }))
      expect(thirdSettled).toHaveBeenCalledWith('rejected', expect.objectContaining({ message: 'refresh failed' }))
      expect(getAccessToken()).toBe('')
      expect(getRefreshToken()).toBe('')
      expect(tokenRefresh.isRefreshing()).toBe(false)
      tokenRefresh.notifyTokenRefresh(renewed)
      await nextTurn()
      expect(adapter).toHaveBeenCalledTimes(3)
    } finally {
      tokenRefresh.notifyTokenRefresh(renewed)
    }
  })

  it('retries both concurrent requests when the shared refresh succeeds', async () => {
    setApiTarget('online')
    setSessionTokens(tokens)
    const refresh = deferred<unknown>()
    const post = vi.fn(() => refresh.promise)
    vi.spyOn(axios, 'create').mockReturnValue({ post } as unknown as AxiosInstance)
    const adapter = adapterWithOneUnauthorized()

    const first = apiClient.get('/first', { adapter })
    await waitFor(() => expect(post).toHaveBeenCalledOnce())
    const second = apiClient.put('/second', { content: 'draft' }, { adapter })
    await waitFor(() => expect(adapter).toHaveBeenCalledTimes(2))
    await nextTurn()

    refresh.resolve({ data: { code: 200, data: renewed } })
    await expect(Promise.all([first, second])).resolves.toHaveLength(2)
    expect(post).toHaveBeenCalledOnce()
    expect(adapter).toHaveBeenCalledTimes(4)
    expect(adapter.mock.calls.slice(2).map(([config]) => config.headers.Authorization)).toEqual([
      'Bearer new-access', 'Bearer new-access',
    ])
    expect(getAccessToken()).toBe(renewed.accessToken)
  })

  it('rejects concurrent in-flight requests without clearing tokens or redirecting on local failure', async () => {
    window.history.replaceState(null, '', '/workspace')
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    setApiTarget('local')
    setSessionTokens(tokens)
    const release = deferred<void>()
    const bothStarted = deferred<void>()
    const attempts = new Map<string, number>()
    const adapter = vi.fn<AxiosAdapter>(async (config) => {
      const url = config.url ?? ''
      const count = (attempts.get(url) ?? 0) + 1
      attempts.set(url, count)
      if (count > 1) return { config, data: {}, headers: {}, status: 200, statusText: 'OK' }
      if (attempts.size === 2) bothStarted.resolve()
      await release.promise
      throw unauthorized(config)
    })
    const post = vi.fn()
    vi.spyOn(axios, 'create').mockReturnValue({ post } as unknown as AxiosInstance)
    const first = apiClient.get('/first', { adapter }).catch((error: unknown) => error)
    const secondSettled = vi.fn()
    void apiClient.put('/second', { content: 'draft' }, { adapter }).then(
      () => secondSettled('resolved'),
      (error: unknown) => secondSettled('rejected', error),
    )
    await bothStarted.promise
    setDesktopLocalBackendError('本地后端版本不匹配')
    release.resolve()

    expect(await first).toMatchObject({ message: '本地后端版本不匹配' })
    await nextTurn()
    try {
      expect(secondSettled).toHaveBeenCalledWith('rejected', expect.objectContaining({ message: '本地后端版本不匹配' }))
      expect(post).not.toHaveBeenCalled()
      expect(getAccessToken()).toBe(tokens.accessToken)
      expect(getRefreshToken()).toBe(tokens.refreshToken)
      expect(window.location.pathname).toBe('/workspace')
      expect(consoleError).not.toHaveBeenCalled()
      setDesktopLocalApiBaseUrl(appEnv.localApiBaseUrl)
      setDesktopLocalApiBaseUrl('')
      tokenRefresh.notifyTokenRefresh(renewed)
      await nextTurn()
      expect(adapter).toHaveBeenCalledTimes(2)
    } finally {
      setDesktopLocalApiBaseUrl(appEnv.localApiBaseUrl)
      setDesktopLocalApiBaseUrl('')
      tokenRefresh.notifyTokenRefresh(renewed)
    }
  })
})
