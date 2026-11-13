import { getCachedLatexFormat, putCachedLatexFormat } from '@/lib/latex-resource-cache'
import { isDesktop } from '@/lib/desktop'
import { getApiTarget, getRequiredApiBaseUrl } from '@/lib/env'
import { preprocessEpsFiles, type EpsPreprocessProgress } from '@/lib/latex/eps-preprocessor'
import browserEngineManifest from '@/lib/latex/browser-engine-manifest.json'

export type BrowserLatexCompiler = 'pdflatex' | 'xelatex'
export type BrowserLatexResourceLookup = { kind: 'xetex' | 'pdftex' | 'dvipdfmx'; format: string; name: string }
export type BrowserLatexResourceEvent = {
  name: string
  kind: 'package' | 'font'
  state: 'loading' | 'loaded' | 'missing' | 'failed' | 'cacheWarning'
  bytes?: number
  cached?: boolean
  cacheSource?: 'browser' | 'shared' | 'network'
  package?: string
  lookup?: BrowserLatexResourceLookup
  status?: number
  error?: string
}

export type BrowserLatexProgress =
  | { stage: 'engine'; message: string }
  | { stage: 'format'; message: string }
  | { stage: 'resource'; message: string; resource: BrowserLatexResourceEvent }
  | { stage: 'compile'; message: string; pass: number; totalPasses: number }
  | { stage: 'convert'; message: string; license?: EpsPreprocessProgress['license'] }

export interface BrowserLatexFile {
  path: string
  content: string | Uint8Array
}

export interface BrowserLatexResult {
  pdf: Uint8Array
  log: string
}

interface WorkerMessage {
  cmd?: string
  result?: string
  status?: number
  log?: string
  pdf?: ArrayBuffer
  fonts?: Array<{ id: string; bytes: ArrayBuffer }>
  format?: ArrayBuffer
  data?: string
  files?: string[]
  name?: string
  kind?: 'package' | 'font'
  state?: 'loading' | 'loaded' | 'missing' | 'failed' | 'cacheWarning'
  bytes?: number
  cached?: boolean
  cacheSource?: 'browser' | 'shared' | 'network'
  package?: string
  lookup?: BrowserLatexResourceLookup
  error?: string
}

type BrowserEnginePass =
  | { cmd: 'compilelatex'; responseCmd: 'compile' }
  | { cmd: 'compileformat'; responseCmd: 'compileformat' }
  | { cmd: 'listaux'; responseCmd: 'listaux' }
  | { cmd: 'readfile'; responseCmd: 'readfile'; path: string }
  | { cmd: 'compilebibtex'; responseCmd: 'compile'; jobName: string }
  | { cmd: 'compilepdf'; responseCmd: 'compile' }

/** Stable browser boundary shared by pdfTeX, XeTeX, BibTeX, and Dvipdfmx workers. */
export interface BrowserEngineAdapter {
  worker: Worker
  readonly progress: (event: BrowserLatexProgress) => void
  initialize(signal?: AbortSignal): Promise<void>
  close(): void
  cancel(): void
  diagnostics(): string
  send(data: Record<string, unknown>): void
  writeFile(path: string, content: string | Uint8Array): void
  readFile(path: string, signal?: AbortSignal): Promise<WorkerMessage>
  runPass<T extends WorkerMessage>(pass: BrowserEnginePass, signal?: AbortSignal): Promise<T>
}

interface EngineWorker extends BrowserEngineAdapter {
  command<T extends WorkerMessage>(cmd: string, responseCmd: string, signal?: AbortSignal, payload?: Record<string, unknown>): Promise<T>
}

const engineResourceUrl = (id: string): string => {
  const resource = browserEngineManifest.resources.find((entry) => entry.id === id)
  if (!resource) throw new Error(`LaTeX engine manifest is missing ${id}`)
  return resource.url
}
const ENGINE_VERSION = browserEngineManifest.engineVersion
const TEXLIVE_RESOURCE_PROFILE = import.meta.env.VITE_LATEX_RESOURCE_PROFILE || browserEngineManifest.resourceCatalog.profileId
const ENGINE_URL = engineResourceUrl('papersage-swift-worker')
const BIBTEX_WORKER_ENTRY_URL = engineResourceUrl('wasmtex-bibtex-entry')
const DVIPDFMX_WORKER_URL = engineResourceUrl('texlive2026-dvipdfmx-worker')
const DVIPDFMX_LICENSE: NonNullable<EpsPreprocessProgress['license']> = {
  name: 'TeX Live 2026 Dvipdfmx',
  spdx: 'GPL-2.0-or-later',
  licenseUrl: '/latex-engines/wasmtex-dvipdfmx-2026/LICENSE-GPL-2.0.txt',
  sourceUrl: 'https://github.com/corca-ai/wasmtex/releases/download/engine-2026-9147f5703951082b/wasmtex-2026-9147f5703951082b-source.tar.xz',
  thirdPartyNoticesUrl: '/latex-engines/wasmtex-dvipdfmx-2026/THIRD-PARTY-NOTICES.md',
}
const MAX_TEX_PASSES = browserEngineManifest.executionLimits.maximumTexPasses
const COMMAND_TIMEOUT_MS = browserEngineManifest.executionLimits.compileCommandTimeoutMs
const MAX_AUXILIARY_FILES = browserEngineManifest.executionLimits.maximumAuxiliaryFiles
const MAX_AUXILIARY_TOTAL_BYTES = browserEngineManifest.executionLimits.maximumAuxiliaryBytes
const PDFTEX_BANNER_COMPATIBILITY_SOURCE = '\\ifdefined\\pdftexbanner\\edef\\pdftexbanner{This is \\pdftexbanner}\\fi\n'

function abortError(): DOMException {
  return new DOMException('编译已取消', 'AbortError')
}

function createEngineWorker(
  engine: 'pdftex' | 'xetex' | 'dvipdfmx' | 'bibtex',
  onProgress: (progress: BrowserLatexProgress) => void,
  signal?: AbortSignal,
): EngineWorker {
  if (signal?.aborted) throw abortError()

  const url = new URL(
    engine === 'dvipdfmx' ? DVIPDFMX_WORKER_URL : engine === 'bibtex' ? BIBTEX_WORKER_ENTRY_URL : ENGINE_URL,
    window.location.origin,
  )
  if (engine !== 'dvipdfmx') url.searchParams.set('engine', engine)
  const worker = new Worker(url, { type: 'classic' })
  let readyResolve: (() => void) | undefined
  let readyReject: ((reason: Error) => void) | undefined
  let readySettled = false
  let latestDiagnostics = ''
  let activeCommand: { responseCmd: string; resolve: (data: WorkerMessage) => void; reject: (error: Error) => void } | null = null

  const ready = new Promise<void>((resolve, reject) => {
    readyResolve = resolve
    readyReject = reject
  })

  const abort = () => {
    worker.terminate()
    activeCommand?.reject(abortError())
    if (!readySettled) readyReject?.(abortError())
  }
  signal?.addEventListener('abort', abort, { once: true })

  worker.onmessage = (event: MessageEvent<WorkerMessage>) => {
    const data = event.data
    if (data.log) latestDiagnostics = data.log
    if (data.cmd === 'resource' && data.name && data.kind && data.state) {
      const resource: BrowserLatexResourceEvent = {
        name: data.name,
        kind: data.kind,
        state: data.state,
        bytes: data.bytes,
        cached: data.cached,
        cacheSource: data.cacheSource,
        package: data.package,
        lookup: data.lookup,
        status: data.status,
        error: data.error,
      }
      const label = resource.kind === 'font' ? '字体' : '宏包/引擎资源'
      const message = resource.state === 'loading'
        ? resource.error ?? `正在加载${label}：${resource.name}`
        : resource.state === 'loaded'
          ? resource.cacheSource === 'browser'
            ? `从本机缓存加载：${resource.name}`
            : resource.cacheSource === 'shared'
              ? `从资源共享缓存加载：${resource.name}`
              : `已从资源源下载：${resource.name}`
          : resource.state === 'missing'
            ? `资源不存在：${resource.name}${resource.error ? `（${resource.error}）` : ''}`
            : resource.state === 'cacheWarning'
              ? resource.error ?? `资源已用于本次编译，但未能保存到浏览器缓存：${resource.name}`
              : `资源加载失败：${resource.name}${resource.status ? `（HTTP ${resource.status}）` : ''}${resource.error ? `：${resource.error}` : ''}`
      onProgress({ stage: 'resource', message, resource })
      return
    }

    if (!data.cmd && data.result === 'ok' && !readySettled) {
      readySettled = true
      readyResolve?.()
      return
    }
    if (activeCommand && data.cmd === activeCommand.responseCmd) {
      const command = activeCommand
      activeCommand = null
      if (data.result === 'ok') command.resolve(data)
      else command.reject(new Error(data.log || `${engine} 返回错误状态 ${data.status ?? 'unknown'}`))
    }
  }

  worker.onerror = (event) => {
    const error = new Error(event.message || `${engine} Worker 加载失败`)
    if (!readySettled) {
      readySettled = true
      readyReject?.(error)
    }
    activeCommand?.reject(error)
    activeCommand = null
  }

  const adapter: EngineWorker = {
    worker,
    progress: onProgress,
    async initialize(initializeSignal) {
      if (initializeSignal?.aborted) throw abortError()
      await ready
      if (initializeSignal?.aborted) throw abortError()
    },
    close() {
      signal?.removeEventListener('abort', abort)
      adapter.cancel()
    },
    cancel() {
      worker.terminate()
      activeCommand?.reject(abortError())
      activeCommand = null
      if (!readySettled) {
        readySettled = true
        readyReject?.(abortError())
      }
    },
    diagnostics() {
      return latestDiagnostics
    },
    send(data) {
      worker.postMessage(data)
    },
    writeFile(path, content) {
      worker.postMessage({ cmd: 'writefile', url: path, src: content })
    },
    readFile(path, commandSignal) {
      return adapter.runPass<WorkerMessage>({ cmd: 'readfile', responseCmd: 'readfile', path }, commandSignal)
    },
    runPass<T extends WorkerMessage>(pass: BrowserEnginePass, commandSignal?: AbortSignal): Promise<T> {
      const payload = 'path' in pass ? { url: pass.path }
        : 'jobName' in pass ? { url: pass.jobName }
          : {}
      return adapter.command<T>(pass.cmd, pass.responseCmd, commandSignal, payload)
    },
    async command<T extends WorkerMessage>(cmd: string, responseCmd: string, commandSignal?: AbortSignal, payload: Record<string, unknown> = {}): Promise<T> {
      await adapter.initialize(commandSignal)
      if (commandSignal?.aborted) throw abortError()
      if (activeCommand) throw new Error(`${engine} Worker 当前已有命令执行`)
      return new Promise<T>((resolve, reject) => {
        const timeout = window.setTimeout(() => {
          activeCommand = null
          reject(new Error(`${engine} ${cmd} 超过 10 分钟未完成`))
        }, COMMAND_TIMEOUT_MS)
        const finish = (callback: () => void) => {
          window.clearTimeout(timeout)
          commandSignal?.removeEventListener('abort', onAbort)
          callback()
        }
        const onAbort = () => {
          finish(() => reject(abortError()))
          worker.terminate()
        }
        activeCommand = {
          responseCmd,
          resolve: (data) => finish(() => resolve(data as T)),
          reject: (error) => finish(() => reject(error)),
        }
        commandSignal?.addEventListener('abort', onAbort, { once: true })
        worker.postMessage({ cmd, ...payload })
      })
    },
  }
  return adapter
}

function writeProjectFiles(engine: BrowserEngineAdapter, files: BrowserLatexFile[]): void {
  const directories = new Set<string>()
  for (const file of files) {
    const parts = file.path.split('/').filter(Boolean)
    for (let index = 1; index < parts.length; index++) {
      directories.add(parts.slice(0, index).join('/'))
    }
  }
  for (const directory of [...directories].sort((a, b) => a.split('/').length - b.split('/').length)) {
    engine.send({ cmd: 'mkdir', url: directory })
  }
  for (const file of files) {
    engine.writeFile(file.path, file.content)
  }
}

interface AuxiliaryFile {
  path: string
  content: string
}

async function readAuxiliaryFiles(engine: BrowserEngineAdapter, signal?: AbortSignal): Promise<AuxiliaryFile[]> {
  const listed = await engine.runPass<WorkerMessage>({ cmd: 'listaux', responseCmd: 'listaux' }, signal)
  const paths = listed.files ?? []
  if (paths.length > MAX_AUXILIARY_FILES) {
    throw new Error(`TeX 生成了过多辅助文件（${paths.length} 个，上限 ${MAX_AUXILIARY_FILES}）`)
  }
  if (paths.some((path) => path.endsWith('.bcf'))) {
    throw new Error('检测到 BibLaTeX 的 .bcf 文件；浏览器目前只支持 BibTeX，不支持 Biber 后端。请将 biblatex backend 设置为 bibtex，或使用支持 Biber 的编译环境。')
  }
  const auxiliaries: AuxiliaryFile[] = []
  let totalBytes = 0
  for (const path of paths.filter((item) => item.toLowerCase().endsWith('.aux'))) {
    const result = await engine.readFile(path, signal)
    if (typeof result.data !== 'string') throw new Error(`无法读取 TeX 辅助文件：${path}`)
    totalBytes += new TextEncoder().encode(result.data).byteLength
    if (totalBytes > MAX_AUXILIARY_TOTAL_BYTES) {
      throw new Error(`TeX 辅助文件总量超过 ${Math.floor(MAX_AUXILIARY_TOTAL_BYTES / (1024 * 1024))} MiB 上限`)
    }
    auxiliaries.push({ path, content: result.data })
  }
  return auxiliaries.sort((left, right) => left.path.localeCompare(right.path))
}

function uncommentedAuxiliaryLines(content: string): string[] {
  return content.split(/\r?\n/).map((line) => {
    let escaped = false
    for (let index = 0; index < line.length; index++) {
      if (line[index] === '%' && !escaped) return line.slice(0, index).trim()
      if (line[index] === '\\') escaped = !escaped
      else escaped = false
    }
    return line.trim()
  }).filter(Boolean)
}

function bibliographyDirectiveSignature(auxiliaries: AuxiliaryFile[]): string {
  const directive = /^\\(?:citation|bibdata|bibstyle|@input)\b/
  return auxiliaries.map(({ path, content }) => {
    const lines = uncommentedAuxiliaryLines(content).filter((line) => directive.test(line))
    return `${path}\n${lines.join('\n')}`
  }).join('\n\0\n')
}

function bibtexJobs(auxiliaries: AuxiliaryFile[]): AuxiliaryFile[] {
  return auxiliaries.filter(({ content }) =>
    uncommentedAuxiliaryLines(content).some((line) => /^\\bibdata\s*\{/.test(line)),
  )
}

function bibliographyProjectFiles(files: BrowserLatexFile[]): BrowserLatexFile[] {
  const bibliographyFiles = files.filter((file) => /\.(?:bib|bst)$/i.test(file.path))
  const byRootName = new Map<string, BrowserLatexFile>()
  for (const file of files) {
    if (!file.path.includes('/')) byRootName.set(file.path.toLowerCase(), file)
  }

  const aliases: BrowserLatexFile[] = []
  for (const file of bibliographyFiles) {
    const rootName = file.path.split('/').at(-1)!
    if (rootName === file.path) continue
    const key = rootName.toLowerCase()
    const existing = byRootName.get(key) ?? aliases.find((alias) => alias.path.toLowerCase() === key)
    if (existing) {
      if (!sameFileContents(existing.content, file.content)) {
        throw new Error(`BibTeX 项目文件存在同名冲突：${existing.path} 与 ${file.path}`)
      }
      continue
    }
    aliases.push({ path: rootName, content: file.content })
  }
  return [...bibliographyFiles, ...aliases]
}

function sameFileContents(left: string | Uint8Array, right: string | Uint8Array): boolean {
  const leftBytes = typeof left === 'string' ? new TextEncoder().encode(left) : left
  const rightBytes = typeof right === 'string' ? new TextEncoder().encode(right) : right
  return leftBytes.byteLength === rightBytes.byteLength
    && leftBytes.every((byte, index) => byte === rightBytes[index])
}

function isPdftexFontExpansionError(error: unknown): boolean {
  return error instanceof Error
    && /font expansion[\s\S]{0,128}?auto expansion is only possible with scalable\s+fonts/i.test(error.message)
}

function withPdftexFontMap(source: string): string {
  return `\\ifdefined\\pdfmapfile\\pdfmapfile{+papersage-package-fonts.map}\\fi\n${source}`
}

function bibtexJobName(auxiliaryPath: string): string {
  if (!/\.aux$/i.test(auxiliaryPath)) throw new Error(`BibTeX 收到无效辅助文件路径：${auxiliaryPath}`)
  return auxiliaryPath.slice(0, -4)
}

async function ensureFormat(
  engine: BrowserEngineAdapter,
  engineName: 'pdftex' | 'xetex',
  onProgress: (progress: BrowserLatexProgress) => void,
  signal?: AbortSignal,
): Promise<void> {
  const formatId = `swiftlatex-${engineName}-format`
  const cached = await getCachedLatexFormat(formatId, ENGINE_VERSION)
  if (cached) {
    engine.writeFile(engineName === 'pdftex' ? 'swiftlatexpdftex.fmt' : 'swiftlatexxetex.fmt', cached)
    onProgress({ stage: 'format', message: '已从浏览器缓存载入 LaTeX 格式文件' })
    return
  }

  onProgress({ stage: 'format', message: '正在首次生成 LaTeX 格式文件；宏包会边下载边显示并缓存' })
  const result = await engine.runPass<WorkerMessage>({ cmd: 'compileformat', responseCmd: 'compileformat' }, signal)
  if (!result.format) throw new Error(result.log || '格式文件生成成功，但没有返回格式数据')
  const format = new Uint8Array(result.format)
  await putCachedLatexFormat(formatId, ENGINE_VERSION, format)
  onProgress({ stage: 'format', message: `LaTeX 格式文件已生成并缓存（${format.byteLength.toLocaleString()} bytes）` })
}

async function ensureResourceCache(): Promise<void> {
  // The desktop shell serves the SPA from an embedded custom scheme where
  // service worker registration never settles. Engine assets are bundled, so
  // the cache adds nothing there and awaiting it would block compiling.
  if (isDesktop() || !('serviceWorker' in navigator)) return
  const timeout = (promise: Promise<unknown>, ms: number) => Promise.race([
    promise,
    new Promise<never>((_, reject) => setTimeout(() => reject(new Error('service worker timeout')), ms)),
  ])
  try {
    await timeout(navigator.serviceWorker.register('/latex-resource-sw.js', { scope: '/' }), 10_000)
    await timeout(navigator.serviceWorker.ready, 10_000)
    await navigator.storage?.persist?.()
  } catch {
    // Caching is an optimization; compile with direct asset requests.
  }
}

export async function compileLatexInBrowser(
  files: BrowserLatexFile[],
  rootFile: string,
  compiler: string,
  options: {
    signal?: AbortSignal
    onProgress?: (progress: BrowserLatexProgress) => void
  } = {},
): Promise<BrowserLatexResult> {
  const onProgress = options.onProgress ?? (() => undefined)
  const signal = options.signal
  if (compiler === 'lualatex') {
    throw new Error('LuaLaTeX 浏览器 WASM 引擎尚未完成；当前支持 pdfLaTeX 与 XeLaTeX。')
  }
  if (compiler !== 'pdflatex' && compiler !== 'xelatex') {
    throw new Error(`不支持的浏览器编译器：${compiler}`)
  }
  if (!files.some((file) => file.path === rootFile)) {
    throw new Error(`主文件 ${rootFile} 不在当前项目文件列表中`)
  }
  if (!('serviceWorker' in navigator)) {
    throw new Error('此浏览器不支持 Service Worker，无法缓存 TeX Live 资源')
  }

  await ensureResourceCache()

  const epsPreprocessing = await preprocessEpsFiles(files, {
    signal,
    onProgress: ({ message, license }) => onProgress({ stage: 'convert', message, license }),
  })
  let compilationFiles = files
  if (epsPreprocessing.files.length > 0 || epsPreprocessing.graphicsRule.length > 0) {
    const root = files.find((file) => file.path === rootFile)
    if (!root) throw new Error(`主文件 ${rootFile} 不在当前项目文件列表中`)
    const convertedBySource = new Map(epsPreprocessing.files.map(({ sourcePath, content }) => [sourcePath, content]))
    const source = typeof root.content === 'string'
      ? root.content
      : new TextDecoder('utf-8').decode(root.content)
    compilationFiles = [
      ...files.map((file) => {
        const converted = convertedBySource.get(file.path)
        if (file.path === rootFile) {
          return {
            ...file,
            content: `${epsPreprocessing.graphicsRule}${source}`,
          }
        }
        return converted ? { ...file, content: converted } : file
      }),
      ...epsPreprocessing.files.map(({ path, content }) => ({ path, content })),
    ]
  }
  if (compiler === 'pdflatex') {
    compilationFiles = compilationFiles.map((file) => file.path === rootFile
      ? {
          ...file,
          content: `${PDFTEX_BANNER_COMPATIBILITY_SOURCE}${typeof file.content === 'string'
            ? file.content
            : new TextDecoder('utf-8').decode(file.content)}`,
        }
      : file)
  }

  const engineName = compiler === 'xelatex' ? 'xetex' : 'pdftex'
  // In desktop local mode the asset handler proxies /texlive on the app
  // origin; worker XHRs to 127.0.0.1 are blocked as a private-network request.
  const resourceOrigin = isDesktop() && getApiTarget() === 'local'
    ? window.location.origin
    : getRequiredApiBaseUrl() || window.location.origin
  const texLiveBaseUrl = new URL(`/texlive/${TEXLIVE_RESOURCE_PROFILE}/`, resourceOrigin).href
  const loadedPackages = new Set<string>()
  const loadedLookups = new Map<string, BrowserLatexResourceLookup>()
  const failedResources = new Map<string, BrowserLatexResourceEvent>()
  const reportEngineProgress = (progress: BrowserLatexProgress) => {
    if (progress.stage === 'resource' && progress.resource.state === 'loaded') {
      if (progress.resource.package) loadedPackages.add(progress.resource.package)
      const lookup = progress.resource.lookup
      if (lookup) loadedLookups.set(`${lookup.kind}\0${lookup.format}\0${lookup.name}`, lookup)
    } else if (progress.stage === 'resource' && progress.resource.state === 'failed') {
      const { kind, format, name } = progress.resource.lookup ?? { kind: '', format: '', name: progress.resource.name }
      failedResources.set(`${kind}\0${format}\0${name}`, progress.resource)
    }
    onProgress(progress)
  }
  const engine = createEngineWorker(engineName, reportEngineProgress, signal)
  let converter: BrowserEngineAdapter | null = null
  let bibtex: BrowserEngineAdapter | null = null
  try {
    onProgress({ stage: 'engine', message: `正在启动 ${compiler === 'xelatex' ? 'XeTeX' : 'pdfTeX'} WebAssembly 引擎` })
    await engine.initialize(signal)
    engine.send({ cmd: 'settexliveurl', url: texLiveBaseUrl })
    await ensureFormat(engine, engineName, onProgress, signal)
    writeProjectFiles(engine, compilationFiles)
    engine.send({ cmd: 'setmainfile', url: rootFile })

    let xdv: Uint8Array | null = null
    let pdf: Uint8Array | null = null
    const embeddedFonts = new Map<string, ArrayBuffer>()
    const handedOffFonts = new Map<string, number>()
    const logs: string[] = []
    let priorAuxiliaryState: string | null = null
    let priorBibliographyInput: string | null = null
    let bibliographyProjectFilesSeeded = false
    let auxiliaryFilesConverged = false
    let pdftexPackageFontMapLoaded = false
    for (let pass = 1; pass <= MAX_TEX_PASSES; pass++) {
      if (signal?.aborted) throw abortError()
      onProgress({ stage: 'compile', message: `正在编译（第 ${pass}/${MAX_TEX_PASSES} 遍）`, pass, totalPasses: MAX_TEX_PASSES })
      let result: WorkerMessage
      try {
        result = await engine.runPass<WorkerMessage>({ cmd: 'compilelatex', responseCmd: 'compile' }, signal)
      } catch (error) {
        if (engineName !== 'pdftex' || pdftexPackageFontMapLoaded || !isPdftexFontExpansionError(error)) throw error

        onProgress({ stage: 'engine', message: '正在补齐本次文档所需的 PDF 字体映射并重试' })
        const fontMap = await createPackageFontMap(
          texLiveBaseUrl,
          [...loadedPackages].sort(),
          [...loadedLookups.values()],
          signal,
          onProgress,
        )
        if (!fontMap) throw error

        engine.writeFile('papersage-package-fonts.map', new Uint8Array(fontMap.bytes))
        const root = compilationFiles.find((file) => file.path === rootFile)
        if (!root) throw new Error(`主文件 ${rootFile} 不在当前项目文件列表中`)
        const source = typeof root.content === 'string' ? root.content : new TextDecoder('utf-8').decode(root.content)
        engine.writeFile(rootFile, withPdftexFontMap(source))
        pdftexPackageFontMapLoaded = true
        result = await engine.runPass<WorkerMessage>({ cmd: 'compilelatex', responseCmd: 'compile' }, signal)
      }
      if (result.log) logs.push(`--- ${compiler} pass ${pass} ---\n${result.log}`)
      if (!result.pdf) throw new Error(result.log || 'LaTeX 引擎没有生成输出文件')
      const bytes = new Uint8Array(result.pdf)
      if (engineName === 'pdftex') pdf = bytes
      else {
        xdv = bytes
        for (const font of result.fonts ?? []) embeddedFonts.set(font.id, font.bytes)
      }

      const auxiliaries = await readAuxiliaryFiles(engine, signal)
      const jobs = bibtexJobs(auxiliaries)
      const bibliographySignature = bibliographyDirectiveSignature(auxiliaries)
      let bibliographyRan = false
      if (jobs.length > 0 && bibliographySignature !== priorBibliographyInput) {
        bibtex ??= createEngineWorker('bibtex', reportEngineProgress, signal)
        await bibtex.initialize(signal)
        bibtex.send({ cmd: 'settexliveurl', url: texLiveBaseUrl })
        if (!bibliographyProjectFilesSeeded) {
          writeProjectFiles(bibtex, bibliographyProjectFiles(compilationFiles))
          bibliographyProjectFilesSeeded = true
        }
        writeProjectFiles(bibtex, auxiliaries.map(({ path, content }) => ({ path, content })))

        for (const auxiliary of jobs) {
          const jobName = bibtexJobName(auxiliary.path)
          onProgress({ stage: 'engine', message: `正在生成参考文献：${jobName}` })
          const bibliography = await bibtex.runPass<WorkerMessage>(
            { cmd: 'compilebibtex', responseCmd: 'compile', jobName },
            signal,
          )
          if (bibliography.log) logs.push(`--- BibTeX ${jobName} ---\n${bibliography.log}`)
          const bbl = await bibtex.readFile(`${jobName}.bbl`, signal)
          if (typeof bbl.data !== 'string') {
            throw new Error(`BibTeX 没有生成 ${jobName}.bbl${bibliography.log ? `\n\nBibTeX 日志：\n${bibliography.log}` : ''}`)
          }
          engine.writeFile(`${jobName}.bbl`, bbl.data)
        }
        priorBibliographyInput = bibliographySignature
        bibliographyRan = true
      }

      const auxiliaryState = auxiliaries.map(({ path, content }) => `${path}\0${content}`).join('\n\0\n')
      if (pass >= 2 && auxiliaryState === priorAuxiliaryState && !bibliographyRan) {
        auxiliaryFilesConverged = true
        break
      }
      priorAuxiliaryState = auxiliaryState
    }
    if (!auxiliaryFilesConverged) {
      logs.push(`Warning: TeX auxiliary files did not stabilize within ${MAX_TEX_PASSES} passes; references may remain unresolved.`)
    }

    if (xdv) {
      if (embeddedFonts.size === 0) {
        throw new Error('XeTeX 生成了 XDV，但没有返回本次使用的字体文件；无法交给 Dvipdfmx 转换')
      }
      onProgress({
        stage: 'convert',
        message: `XeTeX 已生成 XDV，正在交接 ${embeddedFonts.size} 个字体文件并转换为 PDF`,
        license: DVIPDFMX_LICENSE,
      })
      converter = createEngineWorker('dvipdfmx', onProgress, signal)
      await converter.initialize(signal)
      converter.send({ cmd: 'settexliveurl', url: texLiveBaseUrl })
      if (loadedPackages.size > 0 || loadedLookups.size > 0) {
        onProgress({ stage: 'convert', message: `正在按本次加载的 ${loadedPackages.size} 个 TeX 包准备字体映射` })
        const mergedFontMap = await createPackageFontMap(
          texLiveBaseUrl,
          [...loadedPackages].sort(),
          [...loadedLookups.values()],
          signal,
          onProgress,
        )
        if (mergedFontMap && !compilationFiles.some((file) => file.path === 'pdftex.map')) {
          converter.writeFile('pdftex.map', new Uint8Array(mergedFontMap.bytes))
        }
      }
      for (const [id, bytes] of embeddedFonts) {
        const font = new Uint8Array(bytes)
        const signature = new TextDecoder('ascii').decode(font.subarray(0, 4))
        const extension = signature === 'OTTO' ? 'otf'
          : signature === 'ttcf' ? 'ttc'
            : signature === 'true' || (font[0] === 0 && font[1] === 1 && font[2] === 0 && font[3] === 0) ? 'ttf'
              : null
        if (!extension) throw new Error(`XeTeX 返回了无法识别的 OpenType 字体格式：${id}`)
        const formats = extension === 'otf' ? [36, 47] : [36]
        for (const format of formats) {
          for (const filename of [id, `${id}.${extension}`]) {
            converter.send({ cmd: 'preloadtexlive', filename, format, data: bytes })
          }
        }
        handedOffFonts.set(id, font.byteLength)
      }
      writeProjectFiles(converter, compilationFiles)
      const xdvPath = `${rootFile.replace(/\.tex$/i, '')}.xdv`
      converter.writeFile(xdvPath, xdv)
      converter.send({ cmd: 'setmainfile', url: xdvPath })
      let converted: WorkerMessage
      try {
        converted = await converter.runPass<WorkerMessage>({ cmd: 'compilepdf', responseCmd: 'compile' }, signal)
      } catch (error) {
        const handoff = [...handedOffFonts].map(([id, bytes]) => `${id} (${bytes} bytes)`).join(', ')
        const compilerLogs = logs.length > 0
          ? `\n\nXeTeX 编译日志：\n${logs.join('\n')}`
          : ''
        throw new Error(`${error instanceof Error ? error.message : String(error)}${compilerLogs}\n已交接字体文件 ID：${handoff}`)
      }
      if (converted.log) logs.push(`--- xdvipdfmx ---\n${converted.log}`)
      if (!converted.pdf) throw new Error(converted.log || 'xdvipdfmx 没有生成 PDF')
      pdf = new Uint8Array(converted.pdf)
    }

    if (!pdf || pdf.byteLength < 5 || new TextDecoder().decode(pdf.subarray(0, 5)) !== '%PDF-') {
      throw new Error('浏览器引擎输出不是有效的 PDF 文件')
    }
    return { pdf, log: logs.join('\n') }
  } catch (error) {
    if (failedResources.size === 0) throw error
    const details = [...failedResources.values()].map((resource) => {
      const lookup = resource.lookup
        ? ` (${resource.lookup.kind}, format ${resource.lookup.format}, ${resource.lookup.name})`
        : ''
      const status = resource.status ? ` HTTP ${resource.status}` : ''
      return `- ${resource.name}${lookup}${status}${resource.error ? `: ${resource.error}` : ''}`
    }).join('\n')
    throw new Error(`${error instanceof Error ? error.message : String(error)}\n\n失败的 TeX 资源请求：\n${details}`)
  } finally {
    bibtex?.close()
    converter?.close()
    engine.close()
  }
}

async function createPackageFontMap(
  texLiveBaseUrl: string,
  packages: string[],
  lookups: BrowserLatexResourceLookup[],
  signal: AbortSignal | undefined,
  onProgress: (progress: BrowserLatexProgress) => void,
): Promise<{ bytes: ArrayBuffer; fileid: string; revision: string } | null> {
  const baseResponse = await fetch('/latex-engines/pdftex.map?sha256=8299e9bd08ec7f02462a6cced838de5c16c6797a4b481a921bc3f0f10e043b59', { signal })
  if (!baseResponse.ok) throw new Error(`无法读取内置字体映射表（HTTP ${baseResponse.status}）`)
  const baseBytes = new Uint8Array(await baseResponse.arrayBuffer())
  const mapParts: Uint8Array[] = [baseBytes]
  onProgress({ stage: 'convert', message: `正在获取本次使用的字体映射（${packages.length} 个包、${lookups.length} 项资源）` })
  const response = await fetch(new URL('package-maps', texLiveBaseUrl), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ packages, lookups }),
    signal,
  })
  if (response.status === 204) return null
  if (!response.ok) {
    const detail = await response.text().catch(() => '')
    throw new Error(`TeX 字体映射加载失败（HTTP ${response.status}）${detail ? `：${detail}` : ''}`)
  }
  mapParts.push(new Uint8Array(await response.arrayBuffer()))
  const revision = response.headers.get('X-PaperSage-Resource-Revision') || ''
  const length = mapParts.reduce((total, part) => total + part.byteLength, 0)
  const merged = new Uint8Array(length)
  let offset = 0
  for (const part of mapParts) {
    merged.set(part, offset)
    offset += part.byteLength
  }
  const digest = await crypto.subtle.digest('SHA-256', merged)
  const fileid = [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, '0')).join('')
  onProgress({ stage: 'convert', message: `已合并字体映射，正在交给 Dvipdfmx` })
  return { bytes: merged.buffer, fileid, revision: revision || 'local-package-font-maps' }
}
