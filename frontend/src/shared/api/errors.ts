import axios from 'axios'

export type ApiErrorKind = 'backend' | 'http' | 'network' | 'unknown'

export type FrontendApiErrorOptions = {
  kind: ApiErrorKind
  code?: number
  status?: number
  info?: string
  details?: unknown
  cause?: unknown
}

export class FrontendApiError extends Error {
  readonly kind: ApiErrorKind
  readonly code?: number
  readonly status?: number
  readonly info?: string
  readonly details?: unknown

  constructor(message: string, options: FrontendApiErrorOptions) {
    super(message)
    this.name = 'FrontendApiError'
    this.kind = options.kind
    this.code = options.code
    this.status = options.status
    this.info = options.info
    this.details = options.details
    this.cause = options.cause
  }
}

export function createBackendEnvelopeError(payload: {
  code: number
  info?: string
  details?: unknown
}) {
  return new FrontendApiError(payload.info || 'Backend request failed.', {
    kind: 'backend',
    code: payload.code,
    info: payload.info,
    details: payload.details,
  })
}

export function isFrontendApiError(error: unknown): error is FrontendApiError {
  return error instanceof FrontendApiError
}

export function normalizeApiError(error: unknown) {
  if (isFrontendApiError(error)) {
    return error
  }

  if (axios.isAxiosError(error)) {
    if (error.response) {
      const info =
        typeof error.response.data === 'object' &&
        error.response.data !== null &&
        'info' in error.response.data
          ? String(error.response.data.info)
          : error.message

      return new FrontendApiError(info || 'HTTP request failed.', {
        kind: 'http',
        status: error.response.status,
        info,
        details: error.response.data,
        cause: error,
      })
    }

    if (error.request) {
      return new FrontendApiError('Network request failed.', {
        kind: 'network',
        cause: error,
      })
    }
  }

  if (error instanceof Error) {
    return new FrontendApiError(error.message, {
      kind: 'unknown',
      cause: error,
    })
  }

  return new FrontendApiError('Unexpected API error.', {
    kind: 'unknown',
    details: error,
  })
}
