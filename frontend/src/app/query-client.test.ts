import { onlineManager } from '@tanstack/react-query'
import { toast } from 'sonner'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { createQueryClient } from './query-client'

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

afterEach(() => {
  onlineManager.setOnline(true)
  vi.mocked(toast.error).mockClear()
})

describe('createQueryClient', () => {
  it('runs page queries while offline instead of pausing them before the request starts', async () => {
    onlineManager.setOnline(false)
    const client = createQueryClient()
    const queryFn = vi.fn().mockResolvedValue('ok')

    const result = await Promise.race([
      client.fetchQuery({ queryKey: ['offline-page-query'], queryFn }),
      new Promise((_, reject) => setTimeout(() => reject(new Error('query paused while offline')), 1000)),
    ])

    expect(result).toBe('ok')
    expect(queryFn).toHaveBeenCalledTimes(1)
  })

  it('dedupes repeated query failures and shows Chinese text', async () => {
    const client = createQueryClient()
    const fail = () => Promise.reject(new Error('Network request failed.'))

    await expect(client.fetchQuery({ queryKey: ['dedupe-a'], queryFn: fail })).rejects.toBeTruthy()
    await expect(client.fetchQuery({ queryKey: ['dedupe-b'], queryFn: fail })).rejects.toBeTruthy()

    const calls = vi.mocked(toast.error).mock.calls
    expect(calls).toHaveLength(2)
    expect(calls[0][1]).toMatchObject({
      id: 'query-error:网络连接失败，请检查网络后重试。',
      description: '网络连接失败，请检查网络后重试。',
    })
    expect(calls[1][1]?.id).toBe(calls[0][1]?.id)
  })
})
