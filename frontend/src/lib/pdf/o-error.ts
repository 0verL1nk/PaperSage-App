/**
 * OError replacement — simplified version of @overleaf/o-error.
 * Supports tag(), getFullInfo(), getFullStack() for error context chaining.
 */
class OError extends Error {
  info?: Record<string, unknown>
  cause?: unknown
  _oErrorTags?: Array<{ name: string; message?: string; info?: Record<string, unknown>; stack?: string }>

  constructor(message: string, info?: Record<string, unknown>, cause?: unknown) {
    super(message)
    this.name = this.constructor.name
    if (info) this.info = info
    if (cause) this.cause = cause
  }

  static tag<T>(error: T, message?: string, info?: Record<string, unknown>): T {
    const oError = error as { _oErrorTags?: OError['_oErrorTags'] }
    if (!oError._oErrorTags) oError._oErrorTags = []
    oError._oErrorTags.push({ name: 'TaggedError', message, info })
    return error
  }

  static getFullInfo(error: unknown): Record<string, unknown> {
    const info: Record<string, unknown> = {}
    if (!error) return info
    const oError = error as OError
    if (oError.cause) Object.assign(info, OError.getFullInfo(oError.cause))
    if (typeof oError.info === 'object') Object.assign(info, oError.info)
    if (oError._oErrorTags) {
      for (const tag of oError._oErrorTags) {
        if (tag.info) Object.assign(info, tag.info)
      }
    }
    return info
  }

  static getFullStack(error: unknown): string {
    if (!error) return ''
    const oError = error as OError
    let stack = oError.stack || oError.message || '(no stack)'
    if (Array.isArray(oError._oErrorTags) && oError._oErrorTags.length) {
      stack += '\n' + oError._oErrorTags.map(tag => tag.stack || `${tag.name}: ${tag.message ?? ''}`).join('\n')
    }
    const causeStack = OError.getFullStack((error as OError).cause)
    if (causeStack) stack += '\ncaused by:\n    ' + causeStack.replace(/\n/g, '\n    ')
    return stack
  }
}

export default OError
