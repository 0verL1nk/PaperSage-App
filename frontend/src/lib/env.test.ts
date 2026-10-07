import { beforeEach, describe, expect, it } from 'vitest'

import {
  appEnv,
  getApiBaseUrlFor,
  getApiTarget,
  getOptionalApiBaseUrl,
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
})
