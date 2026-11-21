import { describe, expect, it, vi } from 'vitest'
import { prepareForUpdate } from './update-prepare'

function deferred() {
  let resolve!: () => void
  let reject!: (reason: Error) => void
  const promise = new Promise<void>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

type PrepareEvent = CustomEvent<{ waitUntil: (promise: Promise<void>) => void }>

describe('prepareForUpdate', () => {
  it('resolves when there are no listeners', async () => {
    await expect(prepareForUpdate()).resolves.toBeUndefined()
  })

  it('waits for every listener to finish', async () => {
    const first = deferred()
    const second = deferred()
    const listener1 = (event: Event) => (event as PrepareEvent).detail.waitUntil(first.promise)
    const listener2 = (event: Event) => (event as PrepareEvent).detail.waitUntil(second.promise)
    window.addEventListener('papersage:prepare-update', listener1)
    window.addEventListener('papersage:prepare-update', listener2)
    try {
      const finished = vi.fn()
      const preparation = prepareForUpdate().then(finished)
      first.resolve()
      await Promise.resolve()
      await Promise.resolve()
      expect(finished).not.toHaveBeenCalled()
      second.resolve()
      await preparation
      expect(finished).toHaveBeenCalledTimes(1)
    } finally {
      window.removeEventListener('papersage:prepare-update', listener1)
      window.removeEventListener('papersage:prepare-update', listener2)
    }
  })

  it('rejects on failure only after every listener settles', async () => {
    const failed = deferred()
    const pending = deferred()
    const listener1 = (event: Event) => (event as PrepareEvent).detail.waitUntil(failed.promise)
    const listener2 = (event: Event) => (event as PrepareEvent).detail.waitUntil(pending.promise)
    window.addEventListener('papersage:prepare-update', listener1)
    window.addEventListener('papersage:prepare-update', listener2)
    try {
      const error = new Error('save failed')
      const finished = vi.fn()
      const preparation = prepareForUpdate().then(() => undefined, (reason: unknown) => { finished(); return reason })
      failed.reject(error)
      await Promise.resolve()
      await Promise.resolve()
      expect(finished).not.toHaveBeenCalled()
      pending.resolve()
      expect(await preparation).toBe(error)
    } finally {
      window.removeEventListener('papersage:prepare-update', listener1)
      window.removeEventListener('papersage:prepare-update', listener2)
    }
  })
})
