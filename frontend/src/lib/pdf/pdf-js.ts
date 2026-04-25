/**
 * PDF.js setup — re-export from react-pdf's pdfjs instance.
 *
 * react-pdf bundles its own pdfjs-dist; using the same instance ensures
 * `instanceof PDFDataRangeTransport` checks work correctly across modules.
 */
import { pdfjs } from 'react-pdf'

export const PDFJS = pdfjs

export const imageResourcesPath = '/images/pdfjs-dist/'

export const loadPdfDocumentFromUrl = (
  url: string,
  options: Record<string, unknown> = {},
) =>
  pdfjs.getDocument({
    url,
    disableAutoFetch: true,
    disableStream: false,
    isEvalSupported: false,
    enableXfa: false,
    ...options,
  } as Parameters<typeof pdfjs.getDocument>[0])
