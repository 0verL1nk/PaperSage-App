/** Replacement for @/infrastructure/error-reporter */
import OError from './o-error'
import { debugConsole } from './debug'

export function captureException(err: Error, options?: { tags?: Record<string, unknown>; extra?: Record<string, unknown> }) {
  const extra = Object.assign(OError.getFullInfo(err), options?.extra || {})
  debugConsole.error('[pdf-error]', err.message, { ...options, extra })
}
