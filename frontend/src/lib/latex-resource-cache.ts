import { sha256 } from 'js-sha256'

export interface LatexResource {
  id: string
  name: string
  url: string
  version: string
  size: number
  sha256: string
}

export interface LatexDownloadProgress {
  resource: LatexResource
  downloaded: number
  total: number
  resumed: boolean
}

interface DownloadRecord {
  id: string
  version: string
  url: string
  size: number
  sha256: string
  etag: string | null
  lastModified: string | null
  downloaded: number
}

interface ChunkRecord {
  key: string
  resourceId: string
  start: number
  bytes: ArrayBuffer
}

interface CachedResource extends DownloadRecord {
  blob: Blob
  verifiedAt: number
}

const DB_NAME = 'papersage-latex-resources'
const DB_VERSION = 3
const CHUNK_SIZE = 1024 * 1024
const liveDownloads = new Set<string>()

function openDb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION)
    request.onupgradeneeded = () => {
      const db = request.result
      if (!db.objectStoreNames.contains('downloads')) db.createObjectStore('downloads', { keyPath: 'id' })
      if (!db.objectStoreNames.contains('chunks')) db.createObjectStore('chunks', { keyPath: 'key' })
      if (!db.objectStoreNames.contains('resources')) db.createObjectStore('resources', { keyPath: 'id' })
      if (!db.objectStoreNames.contains('formats')) db.createObjectStore('formats', { keyPath: 'id' })
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error ?? new Error('无法打开浏览器资源缓存'))
  })
}

async function requestValue<T>(storeName: string, mode: IDBTransactionMode, operation: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  const db = await openDb()
  try {
    return await new Promise<T>((resolve, reject) => {
      const tx = db.transaction(storeName, mode)
      const request = operation(tx.objectStore(storeName))
      let result: T
      request.onsuccess = () => { result = request.result }
      request.onerror = () => reject(request.error ?? new Error('浏览器资源缓存读写失败'))
      tx.oncomplete = () => resolve(result)
      tx.onerror = () => reject(tx.error ?? request.error ?? new Error('浏览器资源缓存事务失败'))
      tx.onabort = () => reject(tx.error ?? new Error('浏览器资源缓存事务已中止'))
    })
  } finally {
    db.close()
  }
}

async function putDownloadAndChunk(record: DownloadRecord, chunk: ChunkRecord): Promise<void> {
  for (;;) {
    const db = await openDb()
    try {
      await new Promise<void>((resolve, reject) => {
        const tx = db.transaction(['downloads', 'chunks'], 'readwrite')
        tx.objectStore('downloads').put(record)
        tx.objectStore('chunks').put(chunk)
        tx.oncomplete = () => resolve()
        tx.onerror = () => reject(tx.error ?? new Error('下载分块未能保存到浏览器缓存'))
        tx.onabort = () => reject(tx.error ?? new Error('下载分块保存已中止'))
      })
      return
    } catch (error) {
      if (!isQuotaExceeded(error)) throw error
      if (!await evictOldestEntry('resources', chunk.resourceId)) {
        throw new Error('浏览器存储空间不足，无法保存已下载分块。已保留之前的续传进度，请清理浏览器空间后重试。')
      }
    } finally {
      db.close()
    }
  }
}

async function clearPartial(id: string): Promise<void> {
  const db = await openDb()
  try {
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction(['downloads', 'chunks'], 'readwrite')
      tx.objectStore('downloads').delete(id)
      const request = tx.objectStore('chunks').indexNames.contains('resourceId')
        ? tx.objectStore('chunks').index('resourceId').openKeyCursor(IDBKeyRange.only(id))
        : tx.objectStore('chunks').openCursor()
      request.onsuccess = () => {
        const cursor = request.result
        if (!cursor) return
        const value = (cursor as IDBCursorWithValue).value as ChunkRecord
        if (value.resourceId === id) cursor.delete()
        cursor.continue()
      }
      tx.oncomplete = () => resolve()
      tx.onerror = () => reject(tx.error ?? new Error('无法清理旧下载分块'))
    })
  } finally {
    db.close()
  }
}

async function getPartialData(id: string): Promise<{ record: DownloadRecord | undefined; chunks: ChunkRecord[] }> {
  const db = await openDb()
  try {
    return await new Promise((resolve, reject) => {
      const tx = db.transaction(['downloads', 'chunks'], 'readonly')
      const recordRequest = tx.objectStore('downloads').get(id)
      const chunksRequest = tx.objectStore('chunks').openCursor()
      const chunks: ChunkRecord[] = []
      chunksRequest.onsuccess = () => {
        const cursor = chunksRequest.result
        if (!cursor) return
        const value = (cursor as IDBCursorWithValue).value as ChunkRecord
        if (value.resourceId === id) chunks.push(value)
        cursor.continue()
      }
      tx.oncomplete = () => resolve({ record: recordRequest.result as DownloadRecord | undefined, chunks })
      tx.onerror = () => reject(tx.error ?? new Error('无法读取下载续传状态'))
    })
  } finally {
    db.close()
  }
}

function getValidator(record: Pick<DownloadRecord, 'etag' | 'lastModified'>): string | null {
  // HTTP If-Range requires a strong ETag; static CDNs commonly emit W/ ETags.
  return record.etag && !record.etag.startsWith('W/') ? record.etag : record.lastModified
}

function parseRange(value: string | null): { start: number; end: number; total: number } | null {
  const match = /^bytes\s+(\d+)-(\d+)\/(\d+)$/i.exec(value ?? '')
  return match ? { start: Number(match[1]), end: Number(match[2]), total: Number(match[3]) } : null
}

export async function hasCachedLatexResource(resource: LatexResource): Promise<boolean> {
  return Boolean(await readVerifiedResource(resource.id, resource))
}

export async function getCachedLatexResource(resourceId: string): Promise<Blob | null> {
  const cached = await readVerifiedResource(resourceId)
  return cached?.blob ?? null
}

interface CachedFormat {
  id: string
  engineVersion: string
  bytes: ArrayBuffer
  sha256: string
  savedAt: number
}

function isQuotaExceeded(error: unknown): boolean {
  return typeof error === 'object' && error !== null && 'name' in error
    && (error.name === 'QuotaExceededError' || error.name === 'NS_ERROR_DOM_QUOTA_REACHED')
}

async function evictOldestEntry(storeName: 'resources' | 'formats', protectedId: string): Promise<boolean> {
  const entries = await requestValue<Array<CachedResource | CachedFormat>>(
    storeName,
    'readonly',
    (store) => store.getAll(),
  )
  const oldest = entries
    .filter((entry) => entry.id !== protectedId)
    .sort((left, right) => {
      const leftTime = 'verifiedAt' in left ? left.verifiedAt : left.savedAt
      const rightTime = 'verifiedAt' in right ? right.verifiedAt : right.savedAt
      return leftTime - rightTime
    })[0]
  if (!oldest) return false
  await requestValue<undefined>(storeName, 'readwrite', (store) => store.delete(oldest.id))
  return true
}

async function putCacheEntry<T>(storeName: 'resources' | 'formats', entry: T & { id: string }): Promise<void> {
  try {
    await requestValue<IDBValidKey>(storeName, 'readwrite', (store) => store.put(entry))
  } catch (error) {
    if (!isQuotaExceeded(error)) throw error
    const stores: Array<'resources' | 'formats'> = storeName === 'formats' ? ['formats', 'resources'] : ['resources']
    for (const evictStore of stores) {
      while (await evictOldestEntry(evictStore, evictStore === storeName ? entry.id : '')) {
        try {
          await requestValue<IDBValidKey>(storeName, 'readwrite', (store) => store.put(entry))
          return
        } catch (retryError) {
          if (!isQuotaExceeded(retryError)) throw retryError
        }
      }
    }
    throw new Error('浏览器存储空间不足，且没有可安全回收的旧缓存。请在编译环境设置中清理缓存后重试。')
  }
}

async function readVerifiedResource(resourceId: string, expected?: LatexResource): Promise<CachedResource | null> {
  const cached = await requestValue<CachedResource | undefined>('resources', 'readonly', (store) => store.get(resourceId))
  if (!cached) return null
  const metadataMatches = !expected
    || (cached.version === expected.version && cached.sha256 === expected.sha256 && cached.size === expected.size)
  const byteLengthMatches = cached.blob instanceof Blob && cached.blob.size === cached.size
  const actualHash = byteLengthMatches ? sha256(new Uint8Array(await cached.blob.arrayBuffer())) : ''
  if (!metadataMatches || !byteLengthMatches || typeof cached.sha256 !== 'string'
    || actualHash.toLowerCase() !== cached.sha256.toLowerCase()) {
    await requestValue<undefined>('resources', 'readwrite', (store) => store.delete(resourceId))
    return null
  }
  return cached
}

export async function getCachedLatexFormat(id: string, engineVersion: string): Promise<Uint8Array | null> {
  const cached = await requestValue<CachedFormat | undefined>('formats', 'readonly', (store) => store.get(id))
  if (!cached || cached.engineVersion !== engineVersion) return null
  if (!(cached.bytes instanceof ArrayBuffer) || typeof cached.sha256 !== 'string') {
    await requestValue<undefined>('formats', 'readwrite', (store) => store.delete(id))
    return null
  }
  const bytes = new Uint8Array(cached.bytes)
  if (sha256(bytes).toLowerCase() !== cached.sha256.toLowerCase()) {
    await requestValue<undefined>('formats', 'readwrite', (store) => store.delete(id))
    return null
  }
  return bytes
}

export async function putCachedLatexFormat(id: string, engineVersion: string, bytes: Uint8Array): Promise<void> {
  const copy = bytes.slice()
  await putCacheEntry('formats', {
    id, engineVersion, bytes: copy.buffer, sha256: sha256(copy), savedAt: Date.now(),
  } satisfies CachedFormat)
}

async function clearDatabaseStores(db: IDBDatabase): Promise<void> {
  const stores = [...db.objectStoreNames]
  if (stores.length === 0) return
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(stores, 'readwrite')
    for (const storeName of stores) tx.objectStore(storeName).clear()
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error ?? new Error('清理浏览器 LaTeX 缓存失败'))
    tx.onabort = () => reject(tx.error ?? new Error('清理浏览器 LaTeX 缓存已中止'))
  })
}

export async function clearLatexResourceCache(): Promise<void> {
  if (liveDownloads.size > 0) throw new Error('资源正在下载；请先暂停下载，再清理缓存')
  const appDb = await openDb()
  try {
    await clearDatabaseStores(appDb)
  } finally {
    appDb.close()
  }

  const workerDb = await new Promise<IDBDatabase>((resolve, reject) => {
    const request = indexedDB.open('papersage-swiftlatex-worker-cache-v2', 1)
    request.onupgradeneeded = () => {
      if (!request.result.objectStoreNames.contains('resources')) {
        request.result.createObjectStore('resources', { keyPath: 'url' })
      }
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error ?? new Error('无法打开 TeX 宏包缓存'))
  })
  try {
    await clearDatabaseStores(workerDb)
  } finally {
    workerDb.close()
  }
}

export async function getLatexResourceStatus(resource: LatexResource): Promise<{ cached: boolean; downloaded: number }> {
  const [cached, partial] = await Promise.all([
    readVerifiedResource(resource.id, resource),
    getPartialData(resource.id),
  ])
  if (cached) {
    return { cached: true, downloaded: resource.size }
  }
  return { cached: false, downloaded: partial.record?.version === resource.version ? partial.record.downloaded : 0 }
}

export async function downloadLatexResource(
  resource: LatexResource,
  options: { signal?: AbortSignal; onProgress?: (progress: LatexDownloadProgress) => void } = {},
): Promise<Blob> {
  if (liveDownloads.has(resource.id)) throw new Error(`${resource.name} 已在另一个标签页下载中`)
  liveDownloads.add(resource.id)
  try {
    const cached = await readVerifiedResource(resource.id, resource)
    if (cached) {
      options.onProgress?.({ resource, downloaded: resource.size, total: resource.size, resumed: true })
      return cached.blob
    }

    let { record, chunks } = await getPartialData(resource.id)
    if (record && (record.version !== resource.version || record.url !== resource.url || record.size !== resource.size || record.sha256 !== resource.sha256)) {
      await clearPartial(resource.id)
      record = undefined
      chunks = []
    }
    const chunkMap = new Map(chunks.map((part) => [part.start, part]))
    let offset = 0
    while (offset < resource.size && chunkMap.has(offset)) {
      const part = chunkMap.get(offset)!
      offset += part.bytes.byteLength
    }
    if (chunks.some((part) => part.start >= offset)) {
      await clearPartial(resource.id)
      record = undefined
      chunks = []
      chunkMap.clear()
      offset = 0
    }
    const resumed = offset > 0
    options.onProgress?.({ resource, downloaded: offset, total: resource.size, resumed })

    while (offset < resource.size) {
      if (options.signal?.aborted) throw new DOMException('下载已暂停；再次初始化会从已保存的位置继续', 'AbortError')
      const end = Math.min(offset + CHUNK_SIZE, resource.size) - 1
      const headers = new Headers({ Range: `bytes=${offset}-${end}` })
      const validator = record && getValidator(record)
      if (validator) headers.set('If-Range', validator)
      const response = await fetch(resource.url, { headers, signal: options.signal, cache: 'no-store' })
      if (response.status !== 206) {
        if (response.status === 200 && offset > 0) {
          await clearPartial(resource.id)
          throw new Error(`${resource.name} 的下载源不支持断点续传，已清除旧分块；请改用支持 HTTP Range 的资源源`)
        }
        throw new Error(`${resource.name} 下载源未返回 206 Partial Content（HTTP ${response.status}）`)
      }
      const range = parseRange(response.headers.get('Content-Range'))
      if (!range || range.start !== offset || range.end !== end || range.total !== resource.size) {
        await clearPartial(resource.id)
        throw new Error(`${resource.name} 的 Content-Range 与资源清单不一致，已清除不可信的续传数据`)
      }
      const etag = response.headers.get('ETag')
      const lastModified = response.headers.get('Last-Modified')
      if (record && ((record.etag && etag !== record.etag) || (record.lastModified && lastModified !== record.lastModified))) {
        await clearPartial(resource.id)
        throw new Error(`${resource.name} 下载期间源文件版本发生变化，旧分块已清除；请重试`)
      }
      const bytes = await response.arrayBuffer()
      if (bytes.byteLength !== end - offset + 1) throw new Error(`${resource.name} 当前分块长度不完整；已保存分块会保留，重试可续传`)
      record = {
        id: resource.id, version: resource.version, url: resource.url, size: resource.size,
        sha256: resource.sha256, etag: etag ?? record?.etag ?? null,
        lastModified: lastModified ?? record?.lastModified ?? null,
        downloaded: offset + bytes.byteLength,
      }
      await putDownloadAndChunk(record, { key: `${resource.id}:${offset}`, resourceId: resource.id, start: offset, bytes })
      chunkMap.set(offset, { key: `${resource.id}:${offset}`, resourceId: resource.id, start: offset, bytes })
      offset += bytes.byteLength
      options.onProgress?.({ resource, downloaded: offset, total: resource.size, resumed })
    }

    const ordered = [...chunkMap.values()].sort((a, b) => a.start - b.start)
    const blob = new Blob(ordered.map((part) => part.bytes), { type: 'application/wasm' })
    const actualHash = sha256(new Uint8Array(await blob.arrayBuffer()))
    if (actualHash.toLowerCase() !== resource.sha256.toLowerCase()) {
      await clearPartial(resource.id)
      throw new Error(`${resource.name} 完整性校验失败（SHA-256 不匹配）；已删除损坏缓存`)
    }
    const complete: CachedResource = { ...record!, blob, verifiedAt: Date.now() }
    await putCacheEntry('resources', complete)
    await clearPartial(resource.id)
    return blob
  } finally {
    liveDownloads.delete(resource.id)
  }
}
