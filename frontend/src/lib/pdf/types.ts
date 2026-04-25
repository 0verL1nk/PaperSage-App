/**
 * PDF types — replacement for Overleaf's @ol-types/compile types.
 * Only includes types used by the PDF caching/transport system.
 */

// --- Core chunk types ---

export type Chunk = {
  start: number
  end: number
}

export type PDFRange<ObjectIdType = Uint8Array | string> = Chunk & {
  objectId: ObjectIdType
  hash: string
  size: number
  totalUsage: number
}

export type PrefetchedChunk<ChunkType extends Chunk> = ChunkType & {
  buffer: Uint8Array
}

// --- PDF file types ---

type PDFFileBase = {
  clsiCacheShard: string
  contentId: string
  editorId: string
  pdfDownloadUrl: string
  pdfUrl: string
  size: number
  build: string
  startXRefTable?: number
}

export type PDFFile = PDFFileBase & {
  createdAt?: string
  ranges: PDFRange<string>[]
  prefetched?: PrefetchedChunk<PDFRange<Uint8Array>>[]
}

export type ProcessedPDFFile = PDFFileBase & {
  preprocessed: true
  createdAt: Date
  prefetched: PrefetchedChunk<PDFRange<Uint8Array>>[]
  ranges: PDFRange<Uint8Array>[]
}

export type PartiallyProcessedPDFFile = PDFFileBase & {
  preprocessed?: boolean
  createdAt?: Date | string
  prefetched?: PrefetchedChunk<PDFRange<Uint8Array>>[]
  ranges: PDFRange<Uint8Array>[] | PDFRange<string>[]
}

// --- Metrics types ---

export type PdfCachingMetrics = {
  viewerId: string
}

export type PdfCachingMetricsFull = PdfCachingMetrics & {
  failedCount: number
  failedOnce: boolean
  tooMuchBandwidthCount: number
  tooManyRequestsCount: number
  cachedCount: number
  cachedBytes: number
  fetchedCount: number
  fetchedBytes: number
  latencyComputeMax: number
  latencyComputeTotal: number
  requestedCount: number
  requestedBytes: number
  oldUrlHitCount: number
  oldUrlMissCount: number
  enablePdfCaching: boolean
  prefetchingEnabled: boolean
  prefetchLargeEnabled: boolean
  cachedUrlLookupEnabled: boolean
  chunkVerifySizeDiffers?: number
  chunkVerifyMismatch?: number
  chunkVerifySuccess?: number
  headerVerifyFailure?: number
}
