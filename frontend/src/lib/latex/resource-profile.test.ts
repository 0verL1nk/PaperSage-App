import { afterEach, describe, expect, it, vi } from 'vitest'
import { discoverResourceProfile } from './resource-profile'

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as Response
}

describe('discoverResourceProfile', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('返回后端下发的 profile', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ profile: 'tl2026-test' }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(discoverResourceProfile('https://api.example.test')).resolves.toBe('tl2026-test')
    expect(fetchMock).toHaveBeenCalledWith(
      'https://api.example.test/texlive/profile',
      expect.objectContaining({ headers: { Accept: 'application/json' } }),
    )
  })

  it('服务错误时给出可行动的中文提示', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({}, 503)))

    await expect(discoverResourceProfile('https://api.example.test')).rejects.toThrow('服务返回 503')
  })

  it('响应缺少 profile 时明确报错', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({})))

    await expect(discoverResourceProfile('https://api.example.test')).rejects.toThrow('缺少 profile')
  })

  it('网络不可用时提示检查连接', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))

    await expect(discoverResourceProfile('https://api.example.test')).rejects.toThrow('网络不可用')
  })
})
