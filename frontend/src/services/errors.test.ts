import { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { describe, expect, it } from 'vitest'

import {
  createBackendEnvelopeError,
  friendlyErrorMessage,
  NETWORK_ERROR_MESSAGE,
  normalizeApiError,
  translateBackendInfo,
} from './errors'

function httpError(status: number, data: unknown) {
  const config = {} as InternalAxiosRequestConfig
  return new AxiosError('Request failed', 'ERR_BAD_RESPONSE', config, {}, {
    config, data, headers: {}, status, statusText: '',
  })
}

describe('translateBackendInfo', () => {
  it.each([
    ['Project not found', '项目不存在或已被删除'],
    ['Source version not found', '来源版本不存在或已被删除'],
    ['Revision not found', '修订版本不存在，请刷新后重试'],
    ['Artifact not found', '研究产物不存在或已被删除'],
    ['Norm pack not found', '规范包不存在或已被删除'],
    ['Norm entry not found', '规范条目不存在或已被删除'],
    ['Adopted norm not found', '该规范尚未被采用或已移除'],
    ['Resource not found', '请求的内容不存在或已被删除'],
    ['Username already exists', '该账号已被注册，请换一个账号名'],
  ] as const)('translates %s', (raw, zh) => {
    expect(translateBackendInfo(raw)).toBe(zh)
  })

  it('passes through Chinese messages unchanged', () => {
    expect(translateBackendInfo('请求参数验证失败，请检查输入')).toBe('请求参数验证失败，请检查输入')
  })
})

describe('normalizeApiError', () => {
  it('uses a Chinese message for transport failures', () => {
    const config = {} as InternalAxiosRequestConfig
    const error = new AxiosError('Network Error', 'ERR_NETWORK', config, {})
    expect(normalizeApiError(error)).toMatchObject({ kind: 'network', message: NETWORK_ERROR_MESSAGE })
  })

  it('translates backend envelope info', () => {
    expect(normalizeApiError(httpError(404, { code: 404, info: 'Source version not found' }))).toMatchObject({
      kind: 'http', status: 404, message: '来源版本不存在或已被删除',
    })
  })

  it('translates FastAPI-style route-level detail', () => {
    expect(normalizeApiError(httpError(404, { detail: 'Not Found' }))).toMatchObject({
      message: '请求的接口不存在，请刷新页面或升级客户端后重试',
    })
  })

  it('falls back to a Chinese status message when the body has no info', () => {
    expect(normalizeApiError(httpError(500, {}))).toMatchObject({
      message: '服务开小差了，请稍后重试。',
    })
    expect(normalizeApiError(httpError(429, undefined))).toMatchObject({
      message: '操作过于频繁，请稍后再试。',
    })
  })
})

describe('createBackendEnvelopeError', () => {
  it('never falls back to English', () => {
    expect(createBackendEnvelopeError({ code: 500 }).message).toBe('请求失败，请稍后重试。')
  })
})

describe('friendlyErrorMessage', () => {
  it('turns English transport messages into actionable Chinese', () => {
    expect(friendlyErrorMessage(new Error('Network request failed.'))).toBe('网络连接失败，请检查网络后重试。')
  })
})
