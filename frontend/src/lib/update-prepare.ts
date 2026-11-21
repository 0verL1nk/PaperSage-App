export const PREPARE_UPDATE_EVENT = 'papersage:prepare-update'

export interface PrepareUpdateDetail {
  waitUntil: (promise: Promise<unknown>) => void
}

declare global {
  interface WindowEventMap {
    'papersage:prepare-update': CustomEvent<PrepareUpdateDetail>
  }
}

export async function prepareForUpdate(): Promise<void> {
  const pending: Promise<unknown>[] = []
  window.dispatchEvent(new CustomEvent<PrepareUpdateDetail>(PREPARE_UPDATE_EVENT, {
    detail: { waitUntil: (promise) => { pending.push(promise) } },
  }))
  const results = await Promise.allSettled(pending)
  const failed = results.find((result) => result.status === 'rejected')
  if (failed?.status === 'rejected') throw failed.reason
}
