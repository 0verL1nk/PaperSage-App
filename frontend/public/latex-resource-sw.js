const DB_NAME = 'papersage-latex-resources'
const DB_VERSION = 3

self.addEventListener('install', (event) => {
  event.waitUntil(self.skipWaiting())
})

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim())
})

self.addEventListener('fetch', (event) => {
  const requestUrl = new URL(event.request.url)
  if (requestUrl.origin !== self.location.origin) return
  if (requestUrl.pathname.startsWith('/latex-engines/')) {
    event.respondWith(serveCachedOrNetwork(event.request))
  }
})

function openDb() {
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
    request.onerror = () => reject(request.error)
  })
}

async function sha256(blob) {
  const digest = await crypto.subtle.digest('SHA-256', await blob.arrayBuffer())
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('')
}

async function deleteCachedResource(id) {
  const db = await openDb()
  try {
    await new Promise((resolve, reject) => {
      const tx = db.transaction('resources', 'readwrite')
      tx.objectStore('resources').delete(id)
      tx.oncomplete = resolve
      tx.onerror = () => reject(tx.error)
      tx.onabort = () => reject(tx.error)
    })
  } finally {
    db.close()
  }
}

async function serveCachedOrNetwork(request) {
  const requestedUrl = new URL(request.url)
  const requestedHash = requestedUrl.searchParams.get('sha256') || requestedUrl.searchParams.get('v')
  try {
    const db = await openDb()
    const cached = await new Promise((resolve, reject) => {
      const tx = db.transaction('resources', 'readonly')
      const cursorRequest = tx.objectStore('resources').openCursor()
      let found = null
      cursorRequest.onsuccess = () => {
        const cursor = cursorRequest.result
        if (!cursor) return
        const resource = cursor.value
        const resourceUrl = new URL(resource.url, self.location.origin)
        const hashMatches = !/^[a-f0-9]{64}$/i.test(requestedHash || '')
          || requestedHash.toLowerCase() === String(resource.sha256).toLowerCase()
        if (resourceUrl.pathname === requestedUrl.pathname && hashMatches) {
          found = resource
          return
        }
        cursor.continue()
      }
      tx.oncomplete = () => resolve(found)
      tx.onerror = () => reject(tx.error)
    })
    db.close()
    if (cached && cached.blob) {
      const actualHash = await sha256(cached.blob)
      if (cached.blob.size !== cached.size || actualHash.toLowerCase() !== String(cached.sha256).toLowerCase()) {
        await deleteCachedResource(cached.id)
        return fetch(request)
      }
      const contentType = new URL(request.url).pathname.endsWith('.wasm') ? 'application/wasm' : 'text/javascript; charset=utf-8'
      return new Response(cached.blob, { headers: { 'Content-Type': contentType, 'Content-Length': String(cached.size), 'Accept-Ranges': 'bytes' } })
    }
  } catch (error) {
    console.warn('LaTeX resource cache unavailable; using network asset', error)
  }
  return fetch(request)
}
