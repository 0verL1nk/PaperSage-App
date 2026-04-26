/** Constants for the uploading module. */

export const ACCEPTED_FILE_TYPES: Record<string, string[]> = {
  'application/pdf': ['.pdf'],
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document': ['.docx'],
  'text/plain': ['.txt'],
}

export const ACCEPTED_EXTENSIONS = Object.values(ACCEPTED_FILE_TYPES).flat()

export const MAX_FILE_SIZE_BYTES = 50 * 1024 * 1024 // 50 MB

export const MOBILE_BREAKPOINT = 1024

export const PANEL_SIZES = {
  /** Left panel (table + filters) default size percentage */
  left: 62,
  /** Right panel (preview) default size percentage */
  right: 38,
  /** Minimum panel sizes */
  minLeft: 40,
  minRight: 20,
} as const

export const FILE_TYPE_LABELS = [
  { value: 'all', label: '全部' },
  { value: 'application/pdf', label: 'PDF' },
  { value: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', label: 'DOCX' },
  { value: 'text/plain', label: 'TXT' },
] as const

export function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export function getShortType(mimeType: string): string {
  if (mimeType === 'application/pdf') return 'PDF'
  if (mimeType.includes('wordprocessingml')) return 'DOCX'
  if (mimeType === 'text/plain') return 'TXT'
  return mimeType.split('/').pop()?.toUpperCase() ?? 'Unknown'
}
