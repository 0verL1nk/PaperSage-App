const MAX_EPS_HEADER_BYTES = 1024 * 1024
const MAX_EPS_PAGE_POINTS = 14_400

interface WorkerRequest {
  type: 'init' | 'convert'
  id?: number
  wasm?: ArrayBuffer
  path?: string
  bytes?: ArrayBuffer
}

interface BoundingBox {
  values: [number, number, number, number]
  atEnd: boolean
}

interface WorkerScope {
  onmessage: ((event: MessageEvent<WorkerRequest>) => void) | null
  postMessage(message: Record<string, unknown>, transfer?: Transferable[]): void
  __papersageStetPanic?: (message: string) => void
}

interface StetEngine {
  convert_eps_to_pdf(input: Uint8Array, creationDate: string): Uint8Array
}

let enginePromise: Promise<StetEngine> | null = null
const workerScope = globalThis as unknown as WorkerScope
const post = (message: Record<string, unknown>, transfer: Transferable[] = []) => workerScope.postMessage(message, transfer)

function bytesToAscii(bytes: Uint8Array): string {
  let result = ''
  const chunkSize = 0x8000
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    result += String.fromCharCode(...bytes.subarray(offset, Math.min(offset + chunkSize, bytes.length)))
  }
  return result
}

function parseBoundingBoxLine(line: string, directive: 'BoundingBox' | 'HiResBoundingBox'): BoundingBox | null {
  const match = new RegExp(`^%%${directive}:\\s*(.*?)\\s*$`).exec(line)
  if (!match) return null
  const value = match[1]
  if (value === '(atend)') return { values: [0, 0, 0, 0], atEnd: true }
  const parts = value.split(/\s+/)
  if (parts.length !== 4 || parts.some((part) => !/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)$/.test(part))) return null
  const values = parts.map(Number) as [number, number, number, number]
  if (!values.every(Number.isFinite)) return null
  return { values, atEnd: false }
}

function findBoxes(section: string, directive: 'BoundingBox' | 'HiResBoundingBox'): BoundingBox[] {
  const boxes: BoundingBox[] = []
  for (const line of section.split(/\r?\n/)) {
    const box = parseBoundingBoxLine(line, directive)
    if (box) boxes.push(box)
  }
  return boxes
}

function readEpsBoundingBox(bytes: Uint8Array, path: string): [number, number, number, number] {
  const headerBytes = bytes.subarray(0, Math.min(bytes.byteLength, MAX_EPS_HEADER_BYTES))
  const header = bytesToAscii(headerBytes)
  const endCommentsIndex = header.indexOf('%%EndComments')
  const dscHeader = endCommentsIndex >= 0 ? header.slice(0, endCommentsIndex) : header
  const hiRes = findBoxes(dscHeader, 'HiResBoundingBox')
  const regular = findBoxes(dscHeader, 'BoundingBox')
  const endOffset = Math.max(0, bytes.byteLength - MAX_EPS_HEADER_BYTES)
  const trailer = bytesToAscii(bytes.subarray(endOffset))

  let box = hiRes.find((item) => !item.atEnd)?.values
  if (!box && hiRes.some((item) => item.atEnd)) {
    const trailerHiRes = findBoxes(trailer, 'HiResBoundingBox').filter((item) => !item.atEnd)
    box = trailerHiRes.at(-1)?.values
  }
  if (!box) box = regular.find((item) => !item.atEnd)?.values
  if (!box && regular.some((item) => item.atEnd)) {
    const trailerRegular = findBoxes(trailer, 'BoundingBox').filter((item) => !item.atEnd)
    box = trailerRegular.at(-1)?.values
  }
  if (!box) throw new Error(`EPS 文件 ${path} 缺少有效的 %%BoundingBox 或 %%HiResBoundingBox 注释`)

  const [llx, lly, urx, ury] = box
  const width = urx - llx
  const height = ury - lly
  if (width <= 0 || height <= 0 || width > MAX_EPS_PAGE_POINTS || height > MAX_EPS_PAGE_POINTS) {
    throw new Error(`EPS 文件 ${path} 的 BoundingBox 尺寸无效或超出 ${MAX_EPS_PAGE_POINTS} pt：${box.join(' ')}`)
  }
  return box
}

function prepareEpsProgram(bytes: Uint8Array, path: string): Uint8Array {
  readEpsBoundingBox(bytes, path)
  return bytes
}

workerScope.onmessage = async (event: MessageEvent<WorkerRequest>) => {
  const request = event.data
  try {
    if (request.type === 'init') {
      if (!request.wasm) throw new Error('没有收到 Stet WebAssembly 文件')
      workerScope.__papersageStetPanic = (message) => post({ type: 'error', error: `Stet WASM panic：${message}` })
      const moduleUrl = new URL('/latex-engines/stet/papersage_eps_pdf_wasm.js', globalThis.location.origin).href
      const module = await import(/* @vite-ignore */ moduleUrl) as {
        default(source: { module_or_path: ArrayBuffer }): Promise<unknown>
        convert_eps_to_pdf(input: Uint8Array, creationDate: string): Uint8Array
      }
      await module.default({ module_or_path: request.wasm })
      enginePromise = Promise.resolve(module)
      await enginePromise
      post({ type: 'ready' })
      return
    }
    if (request.type !== 'convert' || request.id === undefined || !request.path || !request.bytes || !enginePromise) {
      throw new Error('PostScript 转换器尚未初始化或收到的文件信息不完整')
    }
    const engine = await enginePromise
    const source = new Uint8Array(request.bytes)
    const program = prepareEpsProgram(source, request.path)
    const creationDate = `D:${new Date().toISOString().slice(0, 19).replace(/\D/g, '')}Z`
    const pdf = engine.convert_eps_to_pdf(program, creationDate)
    if (new TextDecoder().decode(pdf.subarray(0, 5)) !== '%PDF-') {
      throw new Error(`EPS 文件 ${request.path} 没有生成有效 PDF 页面`)
    }
    post({ type: 'result', id: request.id, pdf: pdf.buffer }, [pdf.buffer])
  } catch (error) {
    post({ type: 'error', id: request.id, error: error instanceof Error ? error.message : String(error) })
  }
}
