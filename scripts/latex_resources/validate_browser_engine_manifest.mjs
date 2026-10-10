import { createHash } from 'node:crypto'
import { readFile, stat } from 'node:fs/promises'
import { dirname, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const frontendRoot = resolve(repositoryRoot, 'frontend')
const manifestPath = resolve(frontendRoot, 'src/lib/latex/browser-engine-manifest.json')
const workerPath = resolve(frontendRoot, 'public/latex-engines/papersage-swift-worker.js')
const manifest = JSON.parse(await readFile(manifestPath, 'utf8'))
const workerSource = await readFile(workerPath, 'utf8')

function readWasmMemoryLimits(bytes) {
  let offset = 8
  const readVarUint32 = () => {
    let value = 0
    let shift = 0
    for (let count = 0; count < 5; count += 1) {
      if (offset >= bytes.length) throw new Error('truncated WebAssembly section')
      const byte = bytes[offset++]
      value |= (byte & 0x7f) << shift
      if ((byte & 0x80) === 0) return value >>> 0
      shift += 7
    }
    throw new Error('invalid WebAssembly unsigned LEB128 value')
  }

  if (bytes.length < 8 || bytes.toString('hex', 0, 8) !== '0061736d01000000') {
    throw new Error('invalid WebAssembly module header')
  }
  while (offset < bytes.length) {
    const sectionId = bytes[offset++]
    const sectionSize = readVarUint32()
    const sectionEnd = offset + sectionSize
    if (sectionEnd > bytes.length) throw new Error('truncated WebAssembly section payload')
    if (sectionId === 5) {
      const memoryCount = readVarUint32()
      const limits = []
      for (let index = 0; index < memoryCount; index += 1) {
        const flags = readVarUint32()
        const minimumPages = readVarUint32()
        const maximumPages = (flags & 0x01) !== 0 ? readVarUint32() : undefined
        if ((flags & 0x02) !== 0 || (flags & 0x04) !== 0) {
          throw new Error('shared or 64-bit WebAssembly memory is outside the manifest policy')
        }
        limits.push({ minimumPages, maximumPages })
      }
      if (offset !== sectionEnd) throw new Error('unexpected WebAssembly memory section payload')
      return limits
    }
    offset = sectionEnd
  }
  throw new Error('WebAssembly memory section is missing')
}

function fail(message) {
  console.error(`Browser engine manifest validation failed: ${message}`)
  process.exitCode = 1
}

if (manifest.schemaVersion !== 1) fail(`unsupported schema version: ${manifest.schemaVersion}`)
if (!/^[a-f0-9]{64}$/.test(manifest.resourceCatalog?.localCatalogSha256 ?? '')) {
  fail('resource catalog SHA-256 is missing or invalid')
}
const executionLimits = manifest.executionLimits
if (!Number.isSafeInteger(executionLimits?.swiftLatexWasmMaximumMemoryBytes)
  || executionLimits.swiftLatexWasmMaximumMemoryBytes <= 0
  || executionLimits.swiftLatexWasmMaximumMemoryBytes % 65536 !== 0) {
  fail('SwiftLaTeX WASM maximum memory must be a positive whole number of 64 KiB pages')
}
if (!Number.isSafeInteger(executionLimits?.compileCommandTimeoutMs) || executionLimits.compileCommandTimeoutMs <= 0) {
  fail('compile command timeout must be a positive integer')
}
if (!Number.isSafeInteger(executionLimits?.maximumTexPasses) || executionLimits.maximumTexPasses <= 0) {
  fail('maximum TeX pass count must be a positive integer')
}
if (!Number.isSafeInteger(executionLimits?.maximumAuxiliaryFiles) || executionLimits.maximumAuxiliaryFiles <= 0
  || !Number.isSafeInteger(executionLimits?.maximumAuxiliaryBytes) || executionLimits.maximumAuxiliaryBytes <= 0) {
  fail('auxiliary file limits must be positive integers')
}

const buildReceiptUrl = new URL(manifest.sources?.swiftlatex?.buildReceipt ?? '', 'https://papersage.invalid')
if (buildReceiptUrl.origin !== 'https://papersage.invalid' || !buildReceiptUrl.pathname.startsWith('/latex-engines/')) {
  fail('SwiftLaTeX build receipt must be a same-origin static asset under /latex-engines/')
} else {
  const buildReceiptPath = resolve(frontendRoot, 'public', `.${buildReceiptUrl.pathname}`)
  try {
    const buildReceiptBytes = await readFile(buildReceiptPath)
    const buildReceiptSha256 = createHash('sha256').update(buildReceiptBytes).digest('hex')
    if (buildReceiptSha256 !== manifest.sources.swiftlatex.buildReceiptSha256) {
      fail(`SwiftLaTeX build receipt SHA-256 mismatch: got ${buildReceiptSha256}`)
    }
    const receipt = JSON.parse(buildReceiptBytes.toString('utf8'))
    if (receipt.sourceRevision !== manifest.sources.swiftlatex.sourceRevision) {
      fail('SwiftLaTeX source revision does not match the pinned build receipt')
    }
    if (receipt.toolchain?.emscriptenVersion !== manifest.sources.swiftlatex.emscriptenVersion) {
      fail('SwiftLaTeX Emscripten version does not match the pinned build receipt')
    }
    const receiptResources = new Map([
      ['pdftex:pdftex.wasm/swiftlatexpdftex.js', 'swiftlatex-pdftex-js'],
      ['pdftex:pdftex.wasm/swiftlatexpdftex.wasm', 'swiftlatex-pdftex-wasm'],
      ['xetex:xetex.wasm/swiftlatexxetex.js', 'swiftlatex-xetex-js'],
      ['xetex:xetex.wasm/swiftlatexxetex.wasm', 'swiftlatex-xetex-wasm'],
    ])
    for (const file of receipt.files ?? []) {
      const resourceId = receiptResources.get(`${file.engine}:${file.path}`)
      const resource = manifest.resources.find((entry) => entry.id === resourceId)
      if (!resource || resource.size !== file.bytes || resource.sha256 !== file.sha256) {
        fail(`SwiftLaTeX build receipt does not match manifest resource ${resourceId ?? file.path}`)
      }
    }
    if (receipt.files?.length !== receiptResources.size) {
      fail(`SwiftLaTeX build receipt must contain ${receiptResources.size} engine outputs`)
    }
  } catch (error) {
    fail(`cannot verify pinned SwiftLaTeX build receipt: ${error.message}`)
  }
}

for (const [sourceId, expectedFamily, wasmResourceId] of [
  ['dvipdfmx', 'xetex', 'texlive2026-dvipdfmx-wasm'],
  ['bibtex', 'bibtex', 'wasmtex-bibtex-wasm'],
]) {
  const source = manifest.sources?.[sourceId]
  const receiptUrl = new URL(source?.buildReceipt ?? '', 'https://papersage.invalid')
  if (receiptUrl.origin !== 'https://papersage.invalid' || !receiptUrl.pathname.startsWith('/latex-engines/')) {
    fail(`${sourceId} build receipt must be a same-origin static asset under /latex-engines/`)
    continue
  }
  try {
    const receiptBytes = await readFile(resolve(frontendRoot, 'public', `.${receiptUrl.pathname}`))
    const receiptSha256 = createHash('sha256').update(receiptBytes).digest('hex')
    if (receiptSha256 !== source.buildReceiptSha256) fail(`${sourceId} build receipt SHA-256 mismatch: got ${receiptSha256}`)
    const receipt = JSON.parse(receiptBytes.toString('utf8'))
    if (receipt.family !== expectedFamily || !receipt.sourceRevision || !receipt.toolchain?.emscriptenVersion) {
      fail(`${sourceId} build receipt is missing exact family, source revision, or toolchain provenance`)
    }
    const wasmFilename = sourceId === 'dvipdfmx' ? 'wasmtex-dvipdfm.wasm' : 'wasmtex-bibtex.wasm'
    const receiptWasm = receipt.files?.find((file) => file.name === wasmFilename)
    const wasmResource = manifest.resources.find((entry) => entry.id === wasmResourceId)
    if (!receiptWasm || !wasmResource || receiptWasm.bytes !== wasmResource.size || receiptWasm.sha256 !== wasmResource.sha256) {
      fail(`${sourceId} distributed WASM does not match its pinned source build receipt`)
    }
  } catch (error) {
    fail(`cannot verify pinned ${sourceId} build receipt: ${error.message}`)
  }
}

const resourceIds = new Set()
for (const resource of manifest.resources ?? []) {
  if (resourceIds.has(resource.id)) fail(`duplicate resource id: ${resource.id}`)
  resourceIds.add(resource.id)
  if (!/^[a-f0-9]{64}$/.test(resource.sha256 ?? '')) fail(`${resource.id}: invalid SHA-256`)
  if (!Number.isSafeInteger(resource.size) || resource.size <= 0) fail(`${resource.id}: invalid byte size`)
  if (!resource.license || !resource.notice) fail(`${resource.id}: license notice is missing`)

  const url = new URL(resource.url, 'https://papersage.invalid')
  if (url.origin !== 'https://papersage.invalid' || !url.pathname.startsWith('/latex-engines/')) {
    fail(`${resource.id}: asset URL must be same-origin under /latex-engines/`)
    continue
  }
  if (url.searchParams.get('v') !== resource.sha256) fail(`${resource.id}: URL version does not pin its SHA-256`)

  const filePath = resolve(frontendRoot, 'public', `.${url.pathname}`)
  if (!filePath.startsWith(`${resolve(frontendRoot, 'public')}${sep}`)) {
    fail(`${resource.id}: asset path escapes the public directory`)
    continue
  }
  const [bytes, fileInfo] = await Promise.all([readFile(filePath), stat(filePath)]).catch((error) => {
    fail(`${resource.id}: cannot read ${url.pathname}: ${error.message}`)
    return [null, null]
  })
  if (!bytes || !fileInfo) continue
  const hash = createHash('sha256').update(bytes).digest('hex')
  if (fileInfo.size !== resource.size || hash !== resource.sha256) {
    fail(`${resource.id}: expected ${resource.size}/${resource.sha256}, got ${fileInfo.size}/${hash}`)
  }
  if (resource.id === 'swiftlatex-pdftex-wasm' || resource.id === 'swiftlatex-xetex-wasm') {
    try {
      const limits = readWasmMemoryLimits(bytes)
      const expectedPages = executionLimits.swiftLatexWasmMaximumMemoryBytes / 65536
      if (limits.length !== 1 || limits[0].maximumPages !== expectedPages) {
        fail(`${resource.id}: expected one WASM memory with maximum ${expectedPages} pages, got ${JSON.stringify(limits)}`)
      }
    } catch (error) {
      fail(`${resource.id}: cannot verify declared WASM memory limit: ${error.message}`)
    }
  }
}

for (const profile of manifest.profiles ?? []) {
  for (const resourceId of profile.resourceIds ?? []) {
    if (!resourceIds.has(resourceId)) fail(`${profile.id}: references unknown resource ${resourceId}`)
  }
}

const runtimePins = [
  'swiftlatex-pdftex-js', 'swiftlatex-xetex-js',
  'texlive2026-dvipdfmx-worker', 'wasmtex-bibtex-worker',
]
for (const id of runtimePins) {
  const resource = manifest.resources.find((entry) => entry.id === id)
  if (resource && !workerSource.includes(resource.sha256)) {
    fail(`worker runtime URL for ${id} is not pinned to manifest SHA-256 ${resource.sha256}`)
  }
}

if (process.exitCode) process.exit(process.exitCode)
console.log(`Validated ${manifest.resources.length} engine assets, profile references, and worker URL pins.`)
