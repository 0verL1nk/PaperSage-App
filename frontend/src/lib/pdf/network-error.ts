/** Replacement for @/utils/is-network-error */

const NETWORK_ERRORS = [
  'Failed to fetch',
  'NetworkError when attempting to fetch resource.',
  'Load failed',
  'A server with the specified hostname could not be found.',
  'The Internet connection appears to be offline.',
  'Could not connect to the server.',
  'The network connection was lost.',
  'The request timed out.',
]

export function isNetworkError(err?: Error): boolean {
  return !!err && NETWORK_ERRORS.includes(err.message)
}
