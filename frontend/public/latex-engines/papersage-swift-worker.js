/*
 * Exact-resource adapter for the SwiftLaTeX workers and PaperSage's immutable
 * Kpathsea catalog. The adapter preserves the original lookup tuple.
 */
const params = new URL(self.location.href).searchParams
const engine = params.get('engine')
if (engine !== 'pdftex' && engine !== 'xetex' && engine !== 'dvipdfmx' && engine !== 'bibtex') {
  throw new Error(`Unsupported SwiftLaTeX engine: ${engine}`)
}

const runtimeByEngine = {
  pdftex: 'swiftlatexpdftex.js?v=81a2809fbf3bf08ecba62116b691540784719a37806823797f67f7544a1172d3',
  xetex: 'swiftlatexxetex.js?v=58464629cc774fbe3e7d3cde1df3aca17f16c415ba8b1298cdfc06e7a0c44e50',
  dvipdfmx: 'wasmtex-dvipdfmx-2026/wasmtex-dvipdfm.worker.js?v=5110f9f3e8917072be1281ea44b94fd4966ad1e816b081646015bfc2420a0606',
}
const BIBTEX_WORKER_URL = '/latex-engines/wasmtex-bibtex-2026-9147f5703951082b/wasmtex-bibtex.worker.js?v=faa856e34c03a920767f906791ea71654bbb52b54c24116c31a08faaa45ee9ba'
const RESOURCE_DB = 'papersage-swiftlatex-worker-cache-v2'
const RESOURCE_DB_VERSION = 1
const RESOURCE_STORE = 'resources'
const RESOURCE_CACHE_LIMIT = 128 * 1024 * 1024
const RESOURCE_MAX_ENTRY = 32 * 1024 * 1024
const BUILTIN_RESOURCES = new Map([
  ['pdftex/11/pdftex.map', {
    url: '/latex-engines/pdftex.map?sha256=8299e9bd08ec7f02462a6cced838de5c16c6797a4b481a921bc3f0f10e043b59',
    fileid: '8299e9bd08ec7f02462a6cced838de5c16c6797a4b481a921bc3f0f10e043b59',
    revision: 'tl2026-swift-ef78b612-r80534-v7',
  }],
])
const resourceCache = new Map()
const pendingCacheWrites = new Set()
const SHA256_K = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
])

function sha256Hex(bytes) {
  const input = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes)
  const bitLength = input.byteLength * 8
  const paddedLength = Math.ceil((input.byteLength + 9) / 64) * 64
  const padded = new Uint8Array(paddedLength)
  padded.set(input)
  padded[input.byteLength] = 0x80
  const view = new DataView(padded.buffer)
  view.setUint32(paddedLength - 8, Math.floor(bitLength / 0x100000000), false)
  view.setUint32(paddedLength - 4, bitLength >>> 0, false)
  const hash = new Uint32Array([
    0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a,
    0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
  ])
  const words = new Uint32Array(64)
  const rotateRight = (value, bits) => (value >>> bits) | (value << (32 - bits))
  for (let offset = 0; offset < paddedLength; offset += 64) {
    for (let index = 0; index < 16; index++) words[index] = view.getUint32(offset + index * 4, false)
    for (let index = 16; index < 64; index++) {
      const left = words[index - 15]
      const right = words[index - 2]
      const sigma0 = rotateRight(left, 7) ^ rotateRight(left, 18) ^ (left >>> 3)
      const sigma1 = rotateRight(right, 17) ^ rotateRight(right, 19) ^ (right >>> 10)
      words[index] = (words[index - 16] + sigma0 + words[index - 7] + sigma1) >>> 0
    }
    let a = hash[0], b = hash[1], c = hash[2], d = hash[3]
    let e = hash[4], f = hash[5], g = hash[6], h = hash[7]
    for (let index = 0; index < 64; index++) {
      const sigma1 = rotateRight(e, 6) ^ rotateRight(e, 11) ^ rotateRight(e, 25)
      const choose = (e & f) ^ (~e & g)
      const temp1 = (h + sigma1 + choose + SHA256_K[index] + words[index]) >>> 0
      const sigma0 = rotateRight(a, 2) ^ rotateRight(a, 13) ^ rotateRight(a, 22)
      const majority = (a & b) ^ (a & c) ^ (b & c)
      const temp2 = (sigma0 + majority) >>> 0
      h = g; g = f; f = e; e = (d + temp1) >>> 0
      d = c; c = b; b = a; a = (temp1 + temp2) >>> 0
    }
    hash[0] = (hash[0] + a) >>> 0; hash[1] = (hash[1] + b) >>> 0
    hash[2] = (hash[2] + c) >>> 0; hash[3] = (hash[3] + d) >>> 0
    hash[4] = (hash[4] + e) >>> 0; hash[5] = (hash[5] + f) >>> 0
    hash[6] = (hash[6] + g) >>> 0; hash[7] = (hash[7] + h) >>> 0
  }
  return Array.from(hash, (word) => word.toString(16).padStart(8, '0')).join('')
}

function resourceChecksum(bytes, headers) {
  const expected = headers.fileid || headers.fontid
  return /^[a-f0-9]{64}$/i.test(expected || '') && sha256Hex(bytes).toLowerCase() === expected.toLowerCase()
}

function mapStaticResourceUrl(value) {
  const url = new URL(value, self.location.href)
  const base = new URL(self.texlive_endpoint, self.location.href)
  if (url.origin !== base.origin || !url.pathname.startsWith(base.pathname)) return url.href

  const request = url.pathname.slice(base.pathname.length).split('/')
  if (request.length < 3 || request.some((part) => !part)) {
    throw new Error(`Unsupported TeX Live resource lookup: ${url.pathname}`)
  }
  const source = request[0]
  const format = request.slice(1, -1).join('/')
  const encodedName = request.at(-1)
  const requestedName = decodeURIComponent(encodedName)
  const builtin = BUILTIN_RESOURCES.get(`${source}/${format}/${requestedName}`)
    || (engine === 'dvipdfmx' && format === '11' && requestedName === 'pdftex.map'
      ? BUILTIN_RESOURCES.get('pdftex/11/pdftex.map')
      : undefined)
  if (builtin) return new URL(builtin.url, self.location.origin).href
  const kind = source === 'fontconfig' ? 'font' : source
  if (!['xetex', 'pdftex', 'dvipdfmx', 'font'].includes(kind)) {
    throw new Error(`Unsupported TeX Live resource kind: ${source}`)
  }
  const name = decodeURIComponent(encodedName)
  if (name.includes('/') || name.includes('\\') || name === '.' || name === '..') {
    throw new Error('Invalid TeX Live resource name')
  }
  const lookupName = kind === 'font' ? name.toLowerCase() : name
  url.pathname = `${base.pathname}resolve/${kind}`
  url.search = ''
  url.searchParams.set('format', format)
  url.searchParams.set('name', lookupName)
  return url.href
}

function openResourceDb() {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(RESOURCE_DB, RESOURCE_DB_VERSION)
    request.onupgradeneeded = () => {
      if (!request.result.objectStoreNames.contains(RESOURCE_STORE)) {
        request.result.createObjectStore(RESOURCE_STORE, { keyPath: 'url' })
      }
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

function deletePersistedResource(url) {
  const operation = openResourceDb().then((db) => new Promise((resolve, reject) => {
    const tx = db.transaction(RESOURCE_STORE, 'readwrite')
    tx.objectStore(RESOURCE_STORE).delete(url)
    tx.oncomplete = () => { db.close(); resolve() }
    tx.onerror = () => { db.close(); reject(tx.error) }
    tx.onabort = () => { db.close(); reject(tx.error) }
  }))
  pendingCacheWrites.add(operation)
  operation.catch((error) => console.warn('Corrupt LaTeX cache entry could not be removed', error))
    .finally(() => pendingCacheWrites.delete(operation))
}

async function loadResourceCache() {
  const db = await openResourceDb()
  try {
    await new Promise((resolve, reject) => {
      const tx = db.transaction(RESOURCE_STORE, 'readonly')
      const request = tx.objectStore(RESOURCE_STORE).getAll()
      request.onsuccess = () => {
        const retained = request.result
          .filter((item) => item.bytes?.byteLength <= RESOURCE_MAX_ENTRY && (item.fileid || item.fontid) && item.revision)
          .sort((left, right) => right.lastUsed - left.lastUsed)
        let retainedBytes = 0
        const keep = []
        for (const item of retained) {
          const size = item.bytes.byteLength
          if (keep.length >= 512 || retainedBytes + size > RESOURCE_CACHE_LIMIT) continue
          keep.push(item)
          retainedBytes += size
          resourceCache.set(item.url, item)
        }
      }
      tx.oncomplete = () => resolve()
      tx.onerror = () => reject(tx.error)
    })
  } finally {
    db.close()
  }
}

function quotaExceeded(error) {
  return error && (error.name === 'QuotaExceededError' || error.name === 'NS_ERROR_DOM_QUOTA_REACHED')
}

async function evictOldestPersistedResource() {
  const db = await openResourceDb()
  try {
    return await new Promise((resolve, reject) => {
      const tx = db.transaction(RESOURCE_STORE, 'readwrite')
      const store = tx.objectStore(RESOURCE_STORE)
      const request = store.getAll()
      let evictedUrl = null
      request.onsuccess = () => {
        const oldest = request.result
          .filter((item) => typeof item.url === 'string')
          .sort((left, right) => (left.lastUsed || 0) - (right.lastUsed || 0))[0]
        if (!oldest) return
        evictedUrl = oldest.url
        store.delete(oldest.url)
      }
      tx.oncomplete = () => resolve(evictedUrl)
      tx.onerror = () => reject(tx.error)
      tx.onabort = () => reject(tx.error)
    })
  } finally {
    db.close()
  }
}

async function writePersistedResource(item) {
  for (;;) {
    const db = await openResourceDb()
    try {
      await new Promise((resolve, reject) => {
        const tx = db.transaction(RESOURCE_STORE, 'readwrite')
        const store = tx.objectStore(RESOURCE_STORE)
        store.put(item)
        const request = store.getAll()
        request.onsuccess = () => {
          const retained = request.result.sort((left, right) => right.lastUsed - left.lastUsed)
          let retainedBytes = 0
          let retainedCount = 0
          for (const cached of retained) {
            const size = cached.bytes?.byteLength || 0
            if (size > RESOURCE_MAX_ENTRY || retainedCount >= 512 || retainedBytes + size > RESOURCE_CACHE_LIMIT) {
              store.delete(cached.url)
              continue
            }
            retainedBytes += size
            retainedCount++
          }
        }
        tx.oncomplete = resolve
        tx.onerror = () => reject(tx.error)
        tx.onabort = () => reject(tx.error)
      })
      return
    } catch (error) {
      if (!quotaExceeded(error)) throw error
      const evictedUrl = await evictOldestPersistedResource()
      if (!evictedUrl) {
        throw new Error('浏览器存储空间不足，TeX 资源仅保留在当前编译 Worker 中，无法跨编译缓存')
      }
      resourceCache.delete(evictedUrl)
    } finally {
      db.close()
    }
  }
}

function persistResource(url, bytes, headers, onCacheError) {
  if (bytes.byteLength === 0 || bytes.byteLength > RESOURCE_MAX_ENTRY) return
  const copy = bytes.slice(0)
  const item = { url, bytes: copy, fileid: headers.fileid, fontid: headers.fontid, sha256: headers.fileid || headers.fontid, package: headers.package, revision: headers.revision, lastUsed: Date.now() }
  if (!resourceChecksum(copy, headers) || !item.revision) return
  resourceCache.set(url, item)
  let memoryBytes = 0
  for (const cached of resourceCache.values()) memoryBytes += cached.bytes.byteLength
  while (resourceCache.size > 512 || memoryBytes > RESOURCE_CACHE_LIMIT) {
    const oldestKey = resourceCache.keys().next().value
    if (!oldestKey) break
    const oldest = resourceCache.get(oldestKey)
    memoryBytes -= oldest?.bytes.byteLength || 0
    resourceCache.delete(oldestKey)
  }
  const write = writePersistedResource(item)
  pendingCacheWrites.add(write)
  write.catch((error) => {
    console.warn('LaTeX package could not be persisted in IndexedDB', error)
    onCacheError?.(error)
  })
    .finally(() => pendingCacheWrites.delete(write))
}

self.addEventListener('message', (event) => {
  const data = event.data
  if (data?.cmd === 'readfile' || data?.cmd === 'listaux') {
    event.stopImmediatePropagation()
    const virtualPath = typeof data.url === 'string' ? data.url : ''
    const parts = virtualPath.replaceAll('\\', '/').split('/').filter(Boolean)
    const normalized = []
    for (const part of parts) {
      if (part === '.') continue
      if (part === '..') {
        if (normalized.length === 0) {
          self.postMessage({ cmd: data.cmd, result: 'error', error: 'Virtual path escapes the project filesystem' })
          return
        }
        normalized.pop()
        continue
      }
      normalized.push(part)
    }
    if (data.cmd === 'readfile') {
      if (!normalized.length || virtualPath.startsWith('/') || /^[A-Za-z]:/.test(virtualPath)) {
        self.postMessage({ cmd: 'readfile', result: 'error', error: 'Invalid project filesystem path' })
        return
      }
      const path = `/work/${normalized.join('/')}`
      try {
        const contents = FS.readFile(path, { encoding: 'utf8' })
        if (new TextEncoder().encode(contents).byteLength > 16 * 1024 * 1024) throw new Error('Generated auxiliary file exceeds 16 MiB')
        self.postMessage({ cmd: 'readfile', result: 'ok', data: contents })
      } catch (error) {
        self.postMessage({ cmd: 'readfile', result: 'error', error: String(error) })
      }
      return
    }

    try {
      const files = []
      let visited = 0
      const visit = (directory, depth = 0) => {
        if (depth > 64) throw new Error('Generated project filesystem exceeds 64 directory levels')
        for (const name of FS.readdir(directory)) {
          if (name === '.' || name === '..') continue
          if (++visited > 10000) throw new Error('Generated project filesystem exceeds 10000 entries')
          const candidate = `${directory}/${name}`
          const stat = FS.stat(candidate)
          if (FS.isDir(stat.mode)) visit(candidate, depth + 1)
          else if (/\.(?:aux|bcf)$/i.test(name)) {
            if (files.length >= 1024) throw new Error('Project generated more than 1024 auxiliary files')
            files.push(candidate.slice('/work/'.length))
          }
        }
      }
      visit('/work')
      self.postMessage({ cmd: 'listaux', result: 'ok', files: files.sort() })
    } catch (error) {
      self.postMessage({ cmd: 'listaux', result: 'error', error: String(error) })
    }
    return
  }
  if (data?.cmd !== 'seed-package-font-map' || !(data.bytes instanceof ArrayBuffer)) return
  event.stopImmediatePropagation()
  const builtin = [...BUILTIN_RESOURCES.values()].find((item) => new URL(item.url, self.location.origin).pathname.endsWith('/pdftex.map'))
  if (!builtin || !data.fileid || !data.revision) return
  persistResource(new URL(builtin.url, self.location.origin).href, data.bytes, {
    fileid: data.fileid,
    revision: data.revision,
  })
})

const NativeXMLHttpRequest = self.XMLHttpRequest
self.XMLHttpRequest = class PaperSageXMLHttpRequest extends NativeXMLHttpRequest {
  open(method, url, async = true, username, password) {
    this.__papersageOriginalUrl = String(url)
    this.__papersageMappedUrl = mapStaticResourceUrl(String(url))
    this.__papersageBuiltin = [...BUILTIN_RESOURCES.values()].find((item) => new URL(item.url, self.location.origin).href === this.__papersageMappedUrl)
    return super.open(method, this.__papersageMappedUrl, async, username, password)
  }

  send(body = null) {
    const originalUrl = this.__papersageOriginalUrl || ''
    const mappedUrl = this.__papersageMappedUrl || originalUrl
    const mappedResource = new URL(mappedUrl, self.location.href)
    const name = mappedResource.searchParams.get('name') || decodeURIComponent(mappedResource.pathname.split('/').pop() || mappedUrl)
    const source = mappedResource.pathname.split('/').at(-1)
    const lookup = source && ['xetex', 'pdftex', 'dvipdfmx'].includes(source)
      ? { kind: source, format: mappedResource.searchParams.get('format') || '', name }
      : undefined
    const kind = originalUrl.includes('/fontconfig/') ? 'font' : 'package'
    const cached = resourceCache.get(mappedUrl)
    if (cached) {
      if (resourceChecksum(cached.bytes, cached)) {
        cached.lastUsed = Date.now()
        resourceCache.delete(mappedUrl)
        resourceCache.set(mappedUrl, cached)
        this.__papersageCached = cached
        self.postMessage({ cmd: 'resource', kind, name, state: 'loading', cached: true, cacheSource: 'browser', lookup })
        self.postMessage({ cmd: 'resource', kind, name, state: 'loaded', bytes: cached.bytes.byteLength, cached: true, cacheSource: 'browser', package: cached.package, lookup })
        return
      }
      resourceCache.delete(mappedUrl)
      deletePersistedResource(mappedUrl)
      self.postMessage({ cmd: 'resource', kind, name, state: 'loading', error: '本机缓存校验失败，已清除并重新下载', cacheSource: 'network', lookup })
    }
    else self.postMessage({ cmd: 'resource', kind, name, state: 'loading', lookup })
    try {
      super.send(body)
    } catch (error) {
      self.postMessage({ cmd: 'resource', kind, name, state: 'failed', error: String(error) })
      throw error
    }
    if (this.status === 200) {
      if (this.response instanceof ArrayBuffer) {
        const headers = {
          fileid: this.getResponseHeader('fileid'),
          fontid: this.getResponseHeader('fontid'),
          package: this.getResponseHeader('X-PaperSage-Package'),
          revision: this.getResponseHeader('X-PaperSage-Resource-Revision'),
        }
        if (!resourceChecksum(this.response, headers)) {
          this.__papersageIntegrityError = 'TeX 资源 SHA-256 与服务端 fileid/fontid 不匹配'
          self.postMessage({ cmd: 'resource', kind, name, state: 'failed', error: this.__papersageIntegrityError, lookup })
          return
        }
        persistResource(mappedUrl, this.response, headers, (error) => {
          self.postMessage({
            cmd: 'resource', kind, name, state: 'cacheWarning', lookup,
            error: `本次编译可继续，但浏览器未能保存此资源：${error instanceof Error ? error.message : String(error)}`,
          })
        })
      }
      self.postMessage({
        cmd: 'resource', kind, name, state: 'loaded',
        bytes: this.response?.byteLength || 0,
          cached: false,
          cacheSource: this.getResponseHeader('X-PaperSage-Cache') === 'HIT' ? 'shared' : 'network',
        package: this.getResponseHeader('X-PaperSage-Package'),
        lookup,
      })
    } else if (this.status === 404 || this.status === 301) {
      self.postMessage({ cmd: 'resource', kind, name, state: 'missing', status: this.status, error: this.readResourceError(), lookup })
    } else if (this.status !== 200) {
      self.postMessage({ cmd: 'resource', kind, name, state: 'failed', status: this.status, error: this.readResourceError(), lookup })
    }
  }

  readResourceError() {
    try {
      const body = super.response
      if (body instanceof ArrayBuffer) return new TextDecoder().decode(new Uint8Array(body))
      if (typeof body === 'string') return body
    } catch (_) {
      try {
        return super.responseText || ''
      } catch (_) {
        return ''
      }
    }
    return ''
  }

  get status() {
    return this.__papersageCached ? 200 : this.__papersageIntegrityError ? 0 : super.status
  }

  get readyState() {
    return this.__papersageCached ? NativeXMLHttpRequest.DONE : super.readyState
  }

  get response() {
    if (this.__papersageCached) return this.__papersageCached.bytes.slice(0)
    if (this.__papersageIntegrityError) return new ArrayBuffer(0)
    return super.response
  }

  get responseURL() {
    return this.__papersageCached ? this.__papersageMappedUrl : super.responseURL
  }

  getResponseHeader(name) {
    if (this.__papersageIntegrityError) return null
    if (this.__papersageCached) {
      if (name.toLowerCase() === 'content-length') return String(this.__papersageCached.bytes.byteLength)
      if (name.toLowerCase() === 'x-papersage-cache') return 'HIT'
      if (name.toLowerCase() === 'fileid') return this.__papersageCached.fileid || null
      if (name.toLowerCase() === 'fontid') return this.__papersageCached.fontid || null
      if (name.toLowerCase() === 'x-papersage-package') return this.__papersageCached.package || null
      if (name.toLowerCase() === 'x-papersage-resource-revision') return this.__papersageCached.revision
    }
    if (this.__papersageBuiltin) {
      if (name.toLowerCase() === 'fileid') return this.__papersageBuiltin.fileid
      if (name.toLowerCase() === 'x-papersage-resource-revision') return this.__papersageBuiltin.revision
    }
    return super.getResponseHeader(name)
  }
}

self.postMessage({ cmd: 'cache', state: 'loading' })
loadResourceCache().catch((error) => {
  console.warn('LaTeX package cache is unavailable; falling back to network', error)
}).finally(() => {
  self.postMessage({ cmd: 'cache', state: 'ready', count: resourceCache.size })
  const nativePostMessage = self.postMessage.bind(self)
  self.postMessage = (message, transfer) => {
    if (message?.cmd === 'compile' || message?.cmd === 'compileformat') {
      Promise.allSettled([...pendingCacheWrites]).then(() => nativePostMessage(message, transfer))
      return
    }
    nativePostMessage(message, transfer)
  }
  importScripts(engine === 'bibtex' ? BIBTEX_WORKER_URL : `/latex-engines/${runtimeByEngine[engine]}`)
})
