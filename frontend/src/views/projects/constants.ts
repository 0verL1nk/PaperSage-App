/** Compile queue polling and display constants. */

/** Poll interval while job is in pending state (ms). */
export const POLL_INTERVAL_PENDING_MS = 1000

/** Poll interval while job is in running state (ms). */
export const POLL_INTERVAL_RUNNING_MS = 2000

/** Max seconds to display before showing timeout message. */
export const COMPILE_TIMEOUT_DISPLAY_S = 120

/** Number of error log lines to show in summary. */
export const ERROR_SUMMARY_LINES = 5

/** Number of history items per page. */
export const HISTORY_PAGE_SIZE = 20

/** Terminal statuses — polling stops when job reaches one of these. */
export const TERMINAL_STATUSES = new Set([
  'success',
  'failed',
  'timeout',
  'cancelled',
])
