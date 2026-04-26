/**
 * Integration layer: connects Overleaf's PDF caching transport with react-pdf.
 *
 * Creates a custom PDFDataRangeTransport that makes HTTP Range requests,
 * then passes it to react-pdf's <Document file={{ range: transport }}>.
 */

import { generatePdfCachingTransportFactory } from './pdf-caching-transport'
import type { PDFFile } from './types'

export type { PDFFile }

export interface RangeTransportResult {
  range: unknown
}

/**
 * Create a PDF range transport for react-pdf.
 *
 * react-pdf's Document accepts `file={range: PDFDataRangeTransport}`,
 * which bypasses the default full-download and uses our custom Range requests.
 */
export function createPdfRangeTransport(
  url: string,
  pdfFile: PDFFile,
  authHeaders?: Record<string, string>,
): RangeTransportResult {
  const abortController = new AbortController()

  const factory = generatePdfCachingTransportFactory()

  const transport = factory({
    url,
    pdfFile,
    abortController,
    authHeaders,
    handleFetchError: (error: unknown) => {
      console.error('[pdf-range-transport] fetch error:', error)
    },
  })

  if (!transport) {
    throw new Error('PDF caching transport not available')
  }

  return { range: transport }
}

/**
 * Build a PDFFile object suitable for the range transport.
 */
export function buildPdfFile(size: number, buildId: string): PDFFile {
  return {
    clsiCacheShard: 'cache',
    contentId: buildId,
    editorId: crypto.randomUUID(),
    pdfDownloadUrl: '',
    pdfUrl: '',
    size,
    build: buildId,
    ranges: [],
    prefetched: [],
  }
}

/**
 * Fetch PDF file size via HEAD request, then create transport.
 */
export async function createPdfSource(
  url: string,
  headers: Record<string, string>,
  buildId: string,
): Promise<RangeTransportResult> {
  const resp = await fetch(url, {
    method: 'HEAD',
    headers,
  })
  if (!resp.ok) {
    throw new Error(`Failed to get PDF info: ${resp.status}`)
  }
  const contentLength = parseInt(resp.headers.get('Content-Length') ?? '0', 10)
  if (!contentLength) {
    throw new Error('Missing Content-Length header')
  }

  const pdfFile = buildPdfFile(contentLength, buildId)
  return createPdfRangeTransport(url, pdfFile, headers)
}
