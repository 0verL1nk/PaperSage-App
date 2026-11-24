import { onlineManager } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { createQueryClient } from './query-client'

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

afterEach(() => {
  onlineManager.setOnline(true)
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
})
