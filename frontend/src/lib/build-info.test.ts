import { afterEach, describe, expect, it } from 'vitest'
import type { AxiosAdapter } from 'axios'

import { appEnv, setApiTarget, setCustomApiBaseUrl } from '@/lib/env'
import { apiClient } from '@/services/client'
import { fetchBackendBuildInfo } from './build-info'

const originalAdapter = apiClient.defaults.adapter

afterEach(() => {
  apiClient.defaults.adapter = originalAdapter
})

describe('fetchBackendBuildInfo', () => {
  it('uses the currently selected API target for each health request', async () => {
    const baseURLs: (string | undefined)[] = []
    apiClient.defaults.adapter = (async (config) => {
      baseURLs.push(config.baseURL)
      return {
        config,
        status: 200,
        statusText: 'OK',
        headers: {},
        data: { status: 'ok', version: 'v1', commit: 'sha', buildTime: 'date', goVersion: 'go' },
      }
    }) satisfies AxiosAdapter

    setApiTarget('online')
    await fetchBackendBuildInfo()
    setApiTarget('local')
    await fetchBackendBuildInfo()
    setCustomApiBaseUrl('https://preview.example.test')
    setApiTarget('custom')
    await fetchBackendBuildInfo()

    expect(baseURLs).toEqual([appEnv.apiBaseUrl || undefined, appEnv.localApiBaseUrl, 'https://preview.example.test'])
  })

  it('treats non-success health responses as unavailable without auth handling', async () => {
    apiClient.defaults.adapter = (async (config) => {
      expect(config.validateStatus?.(401)).toBe(true)
      return { config, status: 401, statusText: 'Unauthorized', headers: {}, data: {} }
    }) satisfies AxiosAdapter
    await expect(fetchBackendBuildInfo()).rejects.toThrow('Backend version unavailable')
  })

  it('rejects a partial health response', async () => {
    apiClient.defaults.adapter = (async (config) => ({
      config, status: 200, statusText: 'OK', headers: {}, data: { version: 'v1' },
    })) satisfies AxiosAdapter
    await expect(fetchBackendBuildInfo()).rejects.toThrow('Backend version unavailable')
  })

  it('rejects a health response without version metadata', async () => {
    apiClient.defaults.adapter = (async (config) => ({
      config, status: 200, statusText: 'OK', headers: {}, data: '<html>not an API</html>',
    })) satisfies AxiosAdapter
    await expect(fetchBackendBuildInfo()).rejects.toThrow('Backend version unavailable')
  })
})
