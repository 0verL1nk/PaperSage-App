import JSZip from 'jszip'

/**
 * Import planning for multi-file LaTeX projects: ZIP archives, dropped folders
 * and picked files all become one path-preserving import plan.
 */

export interface ImportLimits {
  maxFiles: number
  maxFileBytes: number
  maxTotalBytes: number
}

export const DEFAULT_IMPORT_LIMITS: ImportLimits = {
  maxFiles: 500,
  maxFileBytes: 40 * 1024 * 1024,
  maxTotalBytes: 200 * 1024 * 1024,
}

/** Text files the editor and browser compiler can treat as strings. */
export const TEXT_FILE_EXTENSIONS = new Set([
  'tex', 'bib', 'cls', 'sty', 'bst', 'def', 'ist', 'fd', 'cfg', 'clo',
  'bbx', 'cbx', 'lbx', 'lof', 'lot', 'toc', 'txt', 'md', 'mp',
])

const BINARY_FILE_EXTENSIONS = new Set([
  'png', 'jpg', 'jpeg', 'gif', 'bmp', 'svg', 'webp', 'pdf', 'eps',
  'zip', 'gz', 'tar', 'bz2', 'ttf', 'otf', 'woff', 'woff2',
  'xlsx', 'docx', 'pptx',
])

const JUNK_SEGMENTS = new Set(['__MACOSX', 'Thumbs.db', 'desktop.ini'])
const INVALID_PATH_REASON = '路径不安全或文件名无效（含 .. 、绝对路径或非法字符）'

export interface DroppedImportFile {
  file: File
  path: string
}

export interface ImportEntry {
  path: string
  file: File
  isText: boolean
  /** Decoded text for ZIP entries, so callers need not re-read them. */
  content?: string
}

export interface SkippedImportFile {
  path: string
  reason: string
}

export interface ImportPlan {
  entries: ImportEntry[]
  skipped: SkippedImportFile[]
}

export interface ImportFailure {
  path: string
  message: string
}

export interface ImportOutcome {
  imported: ImportEntry[]
  failed: ImportFailure[]
}

function extensionOf(path: string): string {
  const name = path.split('/').pop() ?? ''
  const dot = name.lastIndexOf('.')
  return dot >= 0 ? name.slice(dot + 1).toLowerCase() : ''
}

export function isTextImportPath(path: string): boolean {
  return TEXT_FILE_EXTENSIONS.has(extensionOf(path))
}

function isBinaryImportPath(path: string): boolean {
  return BINARY_FILE_EXTENSIONS.has(extensionOf(path))
}

/** Normalizes to a project-relative POSIX path, or null when unsafe/invalid. */
export function normalizeImportPath(raw: string): string | null {
  const unified = raw.replace(/\\/g, '/').trim()
  if (!unified || unified.startsWith('/') || unified.includes('..')) return null
  const segments = unified.split('/').filter((segment) => segment !== '' && segment !== '.')
  if (segments.length === 0) return null
  const path = segments.join('/')
  // eslint-disable-next-line no-control-regex
  if (/[<>:"|?*\u0000-\u001f]/.test(path)) return null
  return path
}

function isJunkImportPath(path: string): boolean {
  return path.split('/').some((segment) => segment.startsWith('.') || JUNK_SEGMENTS.has(segment))
}

function isZipFile(file: File): boolean {
  return /\.zip$/i.test(file.name)
}

function looksBinary(bytes: Uint8Array): boolean {
  return bytes.subarray(0, 1024).includes(0)
}

function declaredEntrySize(entry: JSZip.JSZipObject): number | null {
  // JSZip exposes the central-directory size only on its internal payload.
  const size = (entry as unknown as { _data?: { uncompressedSize?: number } })._data?.uncompressedSize
  return typeof size === 'number' ? size : null
}

function tooManyFilesMessage(limits: ImportLimits): string {
  return `导入的文件过多（最多 ${limits.maxFiles} 个）。请只保留论文工程需要的文件后重试。`
}

function tooLargeMessage(limits: ImportLimits): string {
  return `导入内容超过 ${Math.round(limits.maxTotalBytes / 1024 / 1024)} MB 上限。请只保留论文工程需要的文件后重试。`
}

async function readZipEntries(file: File, limits: ImportLimits): Promise<{ entries: ImportEntry[]; skipped: SkippedImportFile[] }> {
  let zip: JSZip
  try {
    zip = await JSZip.loadAsync(await file.arrayBuffer())
  } catch {
    throw new Error(`无法读取 ZIP「${file.name}」。文件可能已损坏或不是 ZIP 格式，请重新打包后再试。`)
  }

  const skipped: SkippedImportFile[] = []
  const candidates: Array<{ entry: JSZip.JSZipObject; path: string; declaredSize: number | null }> = []
  for (const entry of Object.values(zip.files)) {
    if (entry.dir) continue
    const path = normalizeImportPath(entry.name)
    if (!path) {
      skipped.push({ path: entry.name, reason: INVALID_PATH_REASON })
      continue
    }
    if (isJunkImportPath(path)) {
      skipped.push({ path, reason: '系统或隐藏文件' })
      continue
    }
    const declaredSize = declaredEntrySize(entry)
    if (declaredSize !== null && declaredSize > limits.maxFileBytes) {
      skipped.push({ path, reason: '文件超过大小上限' })
      continue
    }
    candidates.push({ entry, path, declaredSize })
  }

  // 用中央目录声明的数量/体积先拒绝超限归档，避免整包解压后才报错、把内容全读进内存。
  if (candidates.length > limits.maxFiles) throw new Error(tooManyFilesMessage(limits))
  const declaredTotalBytes = candidates.reduce((sum, item) => sum + (item.declaredSize ?? 0), 0)
  if (declaredTotalBytes > limits.maxTotalBytes) throw new Error(tooLargeMessage(limits))

  const entries: ImportEntry[] = []
  let totalBytes = 0
  for (const { entry, path } of candidates) {
    let buffer: ArrayBuffer
    try {
      buffer = await entry.async('arraybuffer')
    } catch {
      skipped.push({ path, reason: '无法读取压缩包内的文件' })
      continue
    }
    if (buffer.byteLength > limits.maxFileBytes) {
      skipped.push({ path, reason: '文件超过大小上限' })
      continue
    }
    totalBytes += buffer.byteLength
    if (totalBytes > limits.maxTotalBytes) throw new Error(tooLargeMessage(limits))
    const bytes = new Uint8Array(buffer)
    const isText = isTextImportPath(path) || (!isBinaryImportPath(path) && !looksBinary(bytes))
    entries.push({
      path,
      file: new File([buffer], path.split('/').pop() || path),
      isText,
      content: isText ? new TextDecoder().decode(bytes) : undefined,
    })
  }
  return { entries, skipped }
}

export async function buildImportPlan(
  sources: DroppedImportFile[],
  existingPaths: ReadonlySet<string>,
  limits: ImportLimits = DEFAULT_IMPORT_LIMITS,
): Promise<ImportPlan> {
  const plan: ImportPlan = { entries: [], skipped: [] }
  const seen = new Set<string>()
  let totalBytes = 0

  const addEntry = (entry: ImportEntry) => {
    if (seen.has(entry.path)) {
      plan.skipped.push({ path: entry.path, reason: '导入内容中路径重复' })
      return
    }
    if (existingPaths.has(entry.path)) {
      plan.skipped.push({ path: entry.path, reason: '项目内已存在同名文件' })
      return
    }
    if (plan.entries.length >= limits.maxFiles) {
      throw new Error(tooManyFilesMessage(limits))
    }
    if (totalBytes + entry.file.size > limits.maxTotalBytes) {
      throw new Error(tooLargeMessage(limits))
    }
    seen.add(entry.path)
    totalBytes += entry.file.size
    plan.entries.push(entry)
  }

  for (const source of sources) {
    if (isZipFile(source.file)) {
      const extracted = await readZipEntries(source.file, limits)
      plan.skipped.push(...extracted.skipped)
      extracted.entries.forEach(addEntry)
      continue
    }
    const rawPath = source.path || source.file.name
    const path = normalizeImportPath(rawPath)
    if (!path) {
      plan.skipped.push({ path: rawPath, reason: INVALID_PATH_REASON })
      continue
    }
    if (isJunkImportPath(path)) {
      plan.skipped.push({ path, reason: '系统或隐藏文件' })
      continue
    }
    addEntry({
      path,
      file: source.file,
      isText: isTextImportPath(path) || source.file.type.startsWith('text/'),
    })
  }

  return plan
}

export function filesFromInput(files: FileList | File[]): DroppedImportFile[] {
  return Array.from(files).map((file) => ({ file, path: file.webkitRelativePath || file.name }))
}

function readFileEntry(entry: FileSystemFileEntry): Promise<File | null> {
  return new Promise((resolve) => entry.file((file) => resolve(file), () => resolve(null)))
}

function readDirectoryEntries(reader: FileSystemDirectoryReader): Promise<FileSystemEntry[]> {
  return new Promise((resolve) => {
    const all: FileSystemEntry[] = []
    const readBatch = () => reader.readEntries(
      (batch) => {
        if (batch.length === 0) resolve(all)
        else {
          all.push(...batch)
          readBatch()
        }
      },
      () => resolve(all),
    )
    readBatch()
  })
}

async function walkDroppedEntry(entry: FileSystemEntry, prefix: string, out: DroppedImportFile[]): Promise<void> {
  const path = prefix ? `${prefix}/${entry.name}` : entry.name
  if (entry.isFile) {
    const file = await readFileEntry(entry as FileSystemFileEntry)
    if (file) out.push({ file, path })
    return
  }
  if (!entry.isDirectory) return
  const children = await readDirectoryEntries((entry as FileSystemDirectoryEntry).createReader())
  for (const child of children) await walkDroppedEntry(child, path, out)
}

/** Collects dropped files and folders; `webkitGetAsEntry` must run before any await. */
export async function collectDroppedFiles(dataTransfer: DataTransfer): Promise<DroppedImportFile[]> {
  const items = dataTransfer.items ? Array.from(dataTransfer.items) : []
  const roots = items
    .filter((item) => item.kind === 'file')
    .map((item) => item.webkitGetAsEntry())
    .filter((entry): entry is FileSystemEntry => entry !== null)
  if (roots.length === 0) return filesFromInput(dataTransfer.files)

  const files: DroppedImportFile[] = []
  for (const root of roots) await walkDroppedEntry(root, '', files)
  return files
}

/** Maps each absolute path to its path relative to the deepest shared folder. */
export function commonRelativePaths(paths: string[]): Map<string, string> {
  const segments = paths.map((path) => path.replace(/\\/g, '/').replace(/\/+$/, '').split('/').filter(Boolean))
  const directories = segments.map((parts) => parts.slice(0, -1))
  let common = directories[0] ?? []
  for (const parts of directories) {
    let index = 0
    while (index < common.length && index < parts.length && common[index] === parts[index]) index += 1
    common = common.slice(0, index)
  }

  const relative = new Map<string, string>()
  paths.forEach((path, index) => {
    const parts = segments[index]
    const rest = parts.slice(common.length).join('/')
    relative.set(path, rest || parts[parts.length - 1] || path)
  })
  return relative
}

export async function importEntries(
  entries: ImportEntry[],
  write: (entry: ImportEntry) => Promise<void>,
  onProgress?: (done: number, total: number) => void,
  concurrency = 4,
): Promise<ImportOutcome> {
  const succeeded = new Array<boolean>(entries.length).fill(false)
  const failed: ImportFailure[] = []
  let cursor = 0
  let done = 0

  const worker = async () => {
    while (cursor < entries.length) {
      const index = cursor
      cursor += 1
      const entry = entries[index]
      try {
        await write(entry)
        succeeded[index] = true
      } catch (error) {
        failed.push({ path: entry.path, message: error instanceof Error ? error.message : String(error) })
      }
      done += 1
      onProgress?.(done, entries.length)
    }
  }

  const workers = Array.from({ length: Math.min(Math.max(concurrency, 1), entries.length) }, worker)
  await Promise.all(workers)
  return { imported: entries.filter((_, index) => succeeded[index]), failed }
}
