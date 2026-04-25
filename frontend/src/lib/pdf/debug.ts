/** Replacement for @/utils/debugging */
type DebugConsole = {
  debug(...data: unknown[]): void
  log(...data: unknown[]): void
  warn(...data: unknown[]): void
  error(...data: unknown[]): void
}

const debugging =
  typeof window !== 'undefined' &&
  new URLSearchParams(window.location.search).get('debug') === 'true'

export const debugConsole: DebugConsole = debugging
  ? console
  : { debug() {}, log() {}, warn: console.warn.bind(console), error: console.error.bind(console) }
