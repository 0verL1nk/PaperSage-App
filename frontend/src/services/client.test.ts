import { afterEach, describe, expect, it, vi } from 'vitest'
import { AxiosError } from 'axios'
import { toast } from 'sonner'
import { apiClient, getJson } from '@/services/client'

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
