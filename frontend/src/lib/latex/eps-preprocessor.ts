import {
  downloadLatexResource,
  getLatexResourceStatus,
  type LatexResource,
} from '@/lib/latex-resource-cache'

const STET_VERSION = '0.8.4'
const STET_RESOURCE: LatexResource = {
  // Keep the logical resource key stable so a successful update replaces the
  // previous converter cache entry instead of retaining a second 11 MiB copy.
  id: 'efterscript-postscript-pdf-wasm',
  name: `Stet ${STET_VERSION}（PostScript 转 PDF）`,
  url: '/latex-engines/stet/papersage_eps_pdf_wasm_bg.wasm?v=8fe6e27674ec87a89df27d9eb5fa18eb467ac7e06ce99fcbd807d900cd6301d7',
  version: `stet-${STET_VERSION}-8fe6e276`,
  size: 9276146,
  sha256: '8fe6e27674ec87a89df27d9eb5fa18eb467ac7e06ce99fcbd807d900cd6301d7',
}

const CONVERSION_LICENSE = {
  name: `Stet ${STET_VERSION} + PaperSage WASM wrapper`,
  spdx: 'Apache-2.0 OR MIT; embedded fonts: OFL-1.1',
  licenseUrl: '/latex-engines/stet/LICENSE-MIT',
  sourceUrl: 'https://github.com/AndyCappDev/stet/tree/v0.8.4',
  thirdPartyNoticesUrl: '/latex-engines/stet/THIRD-PARTY-NOTICES.md',
} as const

const EPS_CONVERSION_TIMEOUT_MS = 10 * 60 * 1000

export interface EpsPreprocessProgress {
  message: string
  downloaded?: number
  total?: number
  license?: {
    name: string
    spdx: string
    licenseUrl: string
    sourceUrl: string
    thirdPartyNoticesUrl: string
  }
}

export interface EpsPreprocessResult {
  files: Array<{ path: string; sourcePath: string; content: Uint8Array }>
  graphicsRule: string
}

interface EpsWorkerMessage {
  type: 'ready' | 'result' | 'error'
  id?: number
  pdf?: ArrayBuffer
  error?: string
}

function toBytes(content: string | Uint8Array): Uint8Array {
  return typeof content === 'string' ? new TextEncoder().encode(content) : content
}

function getEpsExtension(filePath: string): string | null {
  const extension = /\.[^./]+$/.exec(filePath)?.[0]
  return extension && /^\.eps$/i.test(extension) ? extension : null
}

function getGeneratedPdfPath(epsPath: string): string {
  return epsPath.replace(/\.[^./]+$/, '.pdf')
}

function getEpstopdfConvertedPath(epsPath: string): string {
  return epsPath.replace(/\.[^./]+$/, '-eps-converted-to.pdf')
}

function planGeneratedPaths(files: Array<{ path: string }>, epsFiles: Array<{ path: string }>): Map<string, string[]> {
  const existingPaths = new Set(files.map((file) => file.path.toLowerCase()))
  const generatedPathOwners = new Map<string, string>()
  const outputsBySource = new Map<string, string[]>()
  for (const { path } of epsFiles) {
    const outputs: string[] = []
    for (const outputPath of [getGeneratedPdfPath(path), getEpstopdfConvertedPath(path)]) {
      const key = outputPath.toLowerCase()
      // Project resources take precedence; generated files only fill missing paths.
      if (existingPaths.has(key)) continue
      const owner = generatedPathOwners.get(key)
      if (owner && owner !== path) {
        throw new Error(`EPS 转换输出路径冲突：${outputPath}；请先重命名冲突文件`)
      }
      generatedPathOwners.set(key, path)
      outputs.push(outputPath)
    }
    outputsBySource.set(path, outputs)
  }
  return outputsBySource
}

async function ensureStetResource(
  signal: AbortSignal | undefined,
  onProgress: (progress: EpsPreprocessProgress) => void,
): Promise<ArrayBuffer> {
  if (signal?.aborted) throw new DOMException('编译已取消', 'AbortError')
  const status = await getLatexResourceStatus(STET_RESOURCE)
  if (status.cached) {
    onProgress({
      message: `从浏览器缓存载入：${STET_RESOURCE.name}`,
      downloaded: STET_RESOURCE.size,
      total: STET_RESOURCE.size,
      license: CONVERSION_LICENSE,
    })
  } else {
    onProgress({
      message: `准备下载：${STET_RESOURCE.name}`,
      downloaded: status.downloaded,
      total: STET_RESOURCE.size,
      license: CONVERSION_LICENSE,
    })
  }
  const blob = await downloadLatexResource(STET_RESOURCE, {
    signal,
    onProgress: ({ downloaded, total, resumed }) => onProgress({
      message: status.cached
        ? `从浏览器缓存载入：${STET_RESOURCE.name}`
        : `${resumed ? '断点续传' : '正在下载'}：${STET_RESOURCE.name}`,
      downloaded,
      total,
      license: CONVERSION_LICENSE,
    }),
  })
  if (signal?.aborted) throw new DOMException('编译已取消', 'AbortError')
  return blob.arrayBuffer()
}

function createEpsWorker(wasm: ArrayBuffer, signal?: AbortSignal): {
  worker: Worker
  ready: Promise<void>
  convert(id: number, path: string, bytes: Uint8Array): Promise<Uint8Array>
  close(): void
} {
  if (signal?.aborted) throw new DOMException('编译已取消', 'AbortError')
  const worker = new Worker(new URL('./eps-converter.worker.ts', import.meta.url), { type: 'module' })
  let readyResolve: (() => void) | undefined
  let readyReject: ((error: Error) => void) | undefined
  let readySettled = false
  let active: { id: number; resolve: (pdf: Uint8Array) => void; reject: (error: Error) => void } | null = null

  const ready = new Promise<void>((resolve, reject) => {
    readyResolve = resolve
    readyReject = reject
  })

  const onAbort = () => {
    worker.terminate()
    const error = new DOMException('编译已取消', 'AbortError')
    active?.reject(error)
    if (!readySettled) readyReject?.(error)
  }
  signal?.addEventListener('abort', onAbort, { once: true })

  worker.onmessage = (event: MessageEvent<EpsWorkerMessage>) => {
    const message = event.data
    if (message.type === 'ready' && !readySettled) {
      readySettled = true
      readyResolve?.()
      return
    }
    if (message.type === 'error') {
      const error = new Error(message.error || 'PostScript 转换 Worker 出错')
      if (!readySettled) {
        readySettled = true
        readyReject?.(error)
      } else if (active && (message.id === undefined || message.id === active.id)) {
        active.reject(error)
        active = null
      }
      return
    }
    if (message.type === 'result' && active && message.id === active.id) {
      if (!message.pdf) {
        active.reject(new Error('PostScript 转换器没有返回 PDF 数据'))
      } else {
        active.resolve(new Uint8Array(message.pdf))
      }
      active = null
    }
  }

  worker.onerror = (event) => {
    const error = new Error(event.message || 'PostScript 转换 Worker 加载失败')
    if (!readySettled) {
      readySettled = true
      readyReject?.(error)
    }
    active?.reject(error)
    active = null
  }

  worker.postMessage({ type: 'init', wasm }, [wasm])

  return {
    worker,
    ready,
    async convert(id, path, bytes) {
      await ready
      if (signal?.aborted) throw new DOMException('编译已取消', 'AbortError')
      if (active) throw new Error('PostScript 转换 Worker 当前已有图形在处理')
      return new Promise<Uint8Array>((resolve, reject) => {
        const timeout = window.setTimeout(() => {
          active = null
          reject(new Error(`EPS 图形 ${path} 转换超过 10 分钟未完成`))
        }, EPS_CONVERSION_TIMEOUT_MS)
        const finish = (callback: () => void) => {
          window.clearTimeout(timeout)
          callback()
        }
        active = {
          id,
          resolve: (pdf) => finish(() => resolve(pdf)),
          reject: (error) => finish(() => reject(error)),
        }
        const copy = bytes.slice()
        worker.postMessage({ type: 'convert', id, path, bytes: copy.buffer }, [copy.buffer])
      })
    },
    close() {
      signal?.removeEventListener('abort', onAbort)
      worker.terminate()
    },
  }
}

export async function preprocessEpsFiles(
  files: Array<{ path: string; content: string | Uint8Array }>,
  options: {
    signal?: AbortSignal
    onProgress?: (progress: EpsPreprocessProgress) => void
  } = {},
): Promise<EpsPreprocessResult> {
  const onProgress = options.onProgress ?? (() => undefined)
  const epsFiles = files.filter((file) => getEpsExtension(file.path) !== null)
  if (epsFiles.length === 0) return { files: [], graphicsRule: '' }
  const outputsBySource = planGeneratedPaths(files, epsFiles)
  const extensions = new Set(epsFiles.map((file) => getEpsExtension(file.path)).filter((extension): extension is string => extension !== null))
  const rules = [...extensions].sort().map((extension) => {
    const escaped = extension.replace(/[{}]/g, '')
    return `\\DeclareGraphicsRule{${escaped}}{pdf}{.pdf}{}`
  }).join('\n')
  const graphicsRule = `\\makeatletter\n\\AddToHook{package/graphics/after}{${rules}}\n\\makeatother\n`
  const filesToConvert = epsFiles.filter((file) => (outputsBySource.get(file.path)?.length ?? 0) > 0)
  if (filesToConvert.length === 0) return { files: [], graphicsRule }

  const wasm = await ensureStetResource(options.signal, onProgress)
  if (options.signal?.aborted) throw new DOMException('编译已取消', 'AbortError')
  onProgress({
    message: `正在启动 Stet，并转换 ${filesToConvert.length} 个 EPS 图形`,
    license: CONVERSION_LICENSE,
  })
  const runtime = createEpsWorker(wasm, options.signal)
  try {
    await runtime.ready
    const converted: EpsPreprocessResult['files'] = []
    for (const [index, file] of filesToConvert.entries()) {
      if (options.signal?.aborted) throw new DOMException('编译已取消', 'AbortError')
      onProgress({ message: `正在转换 EPS 图形（${index + 1}/${filesToConvert.length}）：${file.path}`, license: CONVERSION_LICENSE })
      const pdf = await runtime.convert(index, file.path, toBytes(file.content))
      if (new TextDecoder().decode(pdf.subarray(0, 5)) !== '%PDF-') {
        throw new Error(`EPS 转换失败：${file.path}；PostScript 引擎返回的数据不是 PDF`)
      }
      for (const path of outputsBySource.get(file.path) ?? []) {
        converted.push({ path, sourcePath: file.path, content: pdf })
      }
      onProgress({ message: `EPS 图形已转换（${index + 1}/${filesToConvert.length}）：${file.path}`, license: CONVERSION_LICENSE })
    }
    return { files: converted, graphicsRule }
  } finally {
    runtime.close()
  }
}

export { STET_RESOURCE }
