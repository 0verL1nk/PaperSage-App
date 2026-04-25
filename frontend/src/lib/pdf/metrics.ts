/** Metrics — replacement for Overleaf's pdf-preview/util/metrics.ts */
import type { PdfCachingMetrics } from './types'

let _sessionId = 'session'

try {
  _sessionId = crypto.randomUUID()
} catch {
  // fallback for environments without crypto.randomUUID
}

export const EDITOR_SESSION_ID = _sessionId

const pdfCachingMetrics: PdfCachingMetrics = {
  viewerId: EDITOR_SESSION_ID,
}

export function getPdfCachingMetrics() {
  return pdfCachingMetrics
}
