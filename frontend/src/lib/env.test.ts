import { beforeEach, describe, expect, it } from 'vitest'

import {
  appEnv,
  getApiBaseUrlFor,
  getApiTarget,
  getOptionalApiBaseUrl,
  normalizeApiBaseUrl,
  setCustomApiBaseUrl,
  setApiTarget,
} from '@/lib/env'

const KEY = 'papersage_api_target'

describe('接口环境解析', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('默认是 online，且 online 的地址来自构建期注入', () => {
    expect(getApiTarget()).toBe('online')
    expect(getOptionalApiBaseUrl()).toBe(appEnv.apiBaseUrl)
  })

  it('切到 local 后解析出本地地址（默认 localhost:5478）', () => {
    setApiTarget('local')
    expect(getApiTarget()).toBe('local')
    expect(getOptionalApiBaseUrl()).toBe(appEnv.localApiBaseUrl)
    expect(getOptionalApiBaseUrl()).toContain('localhost:5478')
  })

  it('localStorage 里是非法值时回落 online，而不是抛出', () => {
    localStorage.setItem(KEY, 'production')
    expect(getApiTarget()).toBe('online')
    expect(getOptionalApiBaseUrl()).toBe(appEnv.apiBaseUrl)
  })

  it('两个目标的地址互不相同（否则切换没有意义）', () => {
    expect(getApiBaseUrlFor('online')).not.toBe(getApiBaseUrlFor('local'))
  })

  it('支持保存并切换到自定义后端地址', () => {
    setCustomApiBaseUrl(' https://preview-api.example.test/ ')
    setApiTarget('custom')
    expect(getApiTarget()).toBe('custom')
    expect(getOptionalApiBaseUrl()).toBe('https://preview-api.example.test')
  })

  it('只接受安全的 http(s) 基础地址', () => {
    expect(normalizeApiBaseUrl('http://localhost:5478/')).toBe('http://localhost:5478')
    expect(() => normalizeApiBaseUrl('localhost:5478')).toThrow(/完整地址/)
    expect(() => normalizeApiBaseUrl('https://user:pass@example.test')).toThrow(/账号、密码/)
  })
})
