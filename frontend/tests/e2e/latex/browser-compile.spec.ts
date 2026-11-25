import { expect, test } from '@playwright/test'
import { scopeVercelAccessToOrigin } from './helpers/vercel-origin-auth'

const ENABLED = process.env.RUN_BROWSER_LATEX_E2E === '1'

test.skip(!ENABLED, '设置 RUN_BROWSER_LATEX_E2E=1，并指定本地或 Vercel TeX 资源 API 后运行')

async function prepareCompilerPage(page: import('@playwright/test').Page) {
  const workerLogs: string[] = []
  const resourceNetworkLogs: string[] = []
  page.on('worker', (worker) => {
    worker.on('console', (message) => workerLogs.push(message.text()))
  })
  const apiBaseUrl = process.env.BROWSER_LATEX_RESOURCE_API
  await scopeVercelAccessToOrigin(page, apiBaseUrl)
  page.on('request', (request) => {
    if (request.url().includes('/texlive/')) {
      const requestHeaders = request.headers()
      const hasProtection = Boolean(requestHeaders['x-vercel-trusted-oidc-idp-token'] || requestHeaders['x-vercel-protection-bypass'])
      resourceNetworkLogs.push(`${request.method()} ${request.url()} (Vercel access header: ${hasProtection ? 'yes' : 'no'})`)
    }
  })
  page.on('requestfailed', (request) => {
    if (request.url().includes('/texlive/')) {
      resourceNetworkLogs.push(`FAILED ${request.failure()?.errorText ?? 'unknown'} ${request.url()}`)
    }
  })
  page.on('response', (response) => {
    if (response.url().includes('/texlive/')) {
      resourceNetworkLogs.push(`HTTP ${response.status()} ${response.url()}`)
    }
  })
  await page.goto('/')
  if (apiBaseUrl) {
    await page.evaluate((baseUrl) => {
      localStorage.setItem('papersage_custom_api_base_url', baseUrl)
      localStorage.setItem('papersage_api_target', 'custom')
    }, apiBaseUrl)
  }
  return { workerLogs, resourceNetworkLogs }
}

test('pdfLaTeX compiles in the browser and reuses IndexedDB resources', async ({ page }) => {
  test.setTimeout(10 * 60_000)
  const { workerLogs, resourceNetworkLogs } = await prepareCompilerPage(page)

  const results = await page.evaluate(async () => {
    const modulePath = '/src/lib/latex/swiftlatex-browser-compiler.ts'
    const { compileLatexInBrowser } = await import(modulePath)
    const source = String.raw`\documentclass{article}
\begin{document}
PaperSage browser LaTeX smoke test.
\end{document}
`

    const run = async () => {
      const events: Array<{
        state: string
        cached?: boolean
        name: string
        cacheSource?: 'browser' | 'shared' | 'network'
        lookup?: { kind: string; format: string; name: string }
      }> = []
      const result = await compileLatexInBrowser(
        [{ path: 'main.tex', content: source }],
        'main.tex',
        'pdflatex',
        { onProgress: (event) => {
          if (event.stage === 'resource') events.push(event.resource)
        } },
      )
      return {
        bytes: result.pdf.byteLength,
        prefix: new TextDecoder().decode(result.pdf.subarray(0, 5)),
        events,
      }
    }

    const first = await run()
    const second = await run()
    const target = second.events.find((event) => event.state === 'loaded' && event.cacheSource === 'browser' && event.lookup)
    if (!target?.lookup) throw new Error('第二轮编译没有从浏览器本地缓存加载可校验的 TeX 资源')
    const corruptedLookup = await new Promise<{ kind: string; format: string; name: string }>((resolve, reject) => {
      const request = indexedDB.open('papersage-swiftlatex-worker-cache-v2', 1)
      request.onerror = () => reject(request.error)
      request.onsuccess = () => {
        const db = request.result
        const tx = db.transaction('resources', 'readwrite')
        const store = tx.objectStore('resources')
        const read = store.getAll()
        let result: { kind: string; format: string; name: string } | undefined
        tx.oncomplete = () => {
          db.close()
          if (result) resolve(result)
          else reject(new Error('缓存库中没有找到第二轮实际命中的资源'))
        }
        tx.onerror = () => { db.close(); reject(tx.error) }
        tx.onabort = () => { db.close(); reject(tx.error ?? new Error('缓存污染验收事务已中止')) }
        read.onsuccess = () => {
          const entry = read.result.find((candidate: { url: string; bytes: ArrayBuffer }) => {
            const url = new URL(candidate.url)
            return url.pathname.endsWith(`/resolve/${target.lookup?.kind}`)
              && url.searchParams.get('format') === target.lookup?.format
              && url.searchParams.get('name') === target.lookup?.name
              && candidate.bytes.byteLength > 0
          })
          if (!entry) return
          const url = new URL(entry.url)
          const bytes = new Uint8Array(entry.bytes)
          bytes[0] ^= 0xff
          entry.bytes = bytes.buffer
          store.put(entry)
          result = {
            kind: url.pathname.split('/').at(-1) ?? '',
            format: url.searchParams.get('format') ?? '',
            name: url.searchParams.get('name') ?? '',
          }
        }
      }
    })
    const third = await run()
    const recoveredFromNetwork = third.events.some((event) =>
      event.state === 'loaded'
      && event.lookup?.kind === corruptedLookup.kind
      && event.lookup.format === corruptedLookup.format
      && event.lookup.name === corruptedLookup.name
      && event.cacheSource !== 'browser',
    )
    const fourth = await run()
    return { first, second, third, fourth, corruptedLookup, recoveredFromNetwork }
  }).catch((error: unknown) => {
    console.error(`Browser resource network:\n${resourceNetworkLogs.slice(-30).join('\n')}\nSwiftLaTeX worker logs:\n${workerLogs.slice(-30).join('\n')}`)
    throw error
  })

  expect(results.first.prefix).toBe('%PDF-')
  expect(results.first.bytes).toBeGreaterThan(100)
  expect(results.first.events.some((event) => event.state === 'loaded')).toBe(true)
  expect(results.second.prefix).toBe('%PDF-')
  expect(results.second.events.some((event) => event.state === 'loaded' && event.cacheSource === 'browser')).toBe(true)
  expect(results.second.events.every((event) => event.state !== 'loaded' || event.cacheSource === 'browser')).toBe(true)
  expect(
    results.recoveredFromNetwork,
    `损坏资源未通过网络恢复：${JSON.stringify({
      target: results.corruptedLookup,
      observed: results.third.events.filter((event) => event.state === 'loaded').map(({ name, lookup, cacheSource }) => ({ name, lookup, cacheSource })),
    })}`,
  ).toBe(true)
  expect(results.third.prefix).toBe('%PDF-')
  expect(results.fourth.prefix).toBe('%PDF-')
  expect(results.fourth.events.some((event) => event.state === 'loaded' && event.cacheSource === 'browser')).toBe(true)
  expect(results.fourth.events.every((event) => event.state !== 'loaded' || event.cacheSource === 'browser')).toBe(true)

  const apiOrigin = process.env.BROWSER_LATEX_RESOURCE_API
    ? new URL(process.env.BROWSER_LATEX_RESOURCE_API).origin
    : null
  expect(apiOrigin, '离线复用验收需要显式配置资源 API 源站').toBeTruthy()
  await page.reload()
  await page.route(`${apiOrigin}/texlive/**`, (route) => route.abort('internetdisconnected'))
  const offlineResult = await page.evaluate(async () => {
    const { compileLatexInBrowser } = await import('/src/lib/latex/swiftlatex-browser-compiler.ts')
    const events: Array<{ state: string; cacheSource?: string }> = []
    const result = await compileLatexInBrowser(
      [{ path: 'main.tex', content: String.raw`\documentclass{article}\begin{document}Offline cache smoke test.\end{document}` }],
      'main.tex',
      'pdflatex',
      { onProgress: (event) => { if (event.stage === 'resource') events.push(event.resource) } },
    )
    return { prefix: new TextDecoder().decode(result.pdf.subarray(0, 5)), events }
  })
  expect(offlineResult.prefix).toBe('%PDF-')
  expect(offlineResult.events.every((event) => event.state !== 'loaded' || event.cacheSource === 'browser')).toBe(true)

  const quotaRecovery = await page.evaluate(async () => {
    const db = await new Promise<IDBDatabase>((resolve, reject) => {
      const request = indexedDB.open('papersage-latex-resources', 3)
      request.onsuccess = () => resolve(request.result)
      request.onerror = () => reject(request.error)
    })
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction(['resources', 'formats'], 'readwrite')
      tx.objectStore('resources').put({
        id: 'quota-recovery-old-resource', version: 'test', url: '/old-resource', size: 1,
        sha256: '0'.repeat(64), etag: null, lastModified: null, downloaded: 1,
        blob: new Blob([new Uint8Array([0])]), verifiedAt: 1,
      })
      tx.objectStore('formats').clear()
      tx.oncomplete = () => resolve()
      tx.onerror = () => reject(tx.error)
      tx.onabort = () => reject(tx.error)
    })
    db.close()

    const nativePut = IDBObjectStore.prototype.put
    let quotaInjected = false
    IDBObjectStore.prototype.put = function (value, key) {
      if (!quotaInjected && this.name === 'formats' && (value as { id?: string }).id === 'quota-recovery-test') {
        quotaInjected = true
        throw new DOMException('Simulated browser storage quota exhaustion', 'QuotaExceededError')
      }
      return nativePut.call(this, value, key)
    }
    try {
      const { putCachedLatexFormat, getCachedLatexFormat } = await import('/src/lib/latex-resource-cache.ts')
      const bytes = new Uint8Array([1, 4, 9, 16, 25])
      await putCachedLatexFormat('quota-recovery-test', 'test-engine', bytes)
      const restored = await getCachedLatexFormat('quota-recovery-test', 'test-engine')
      const verifyDb = await new Promise<IDBDatabase>((resolve, reject) => {
        const request = indexedDB.open('papersage-latex-resources', 3)
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })
      const oldEntry = await new Promise<unknown>((resolve, reject) => {
        const tx = verifyDb.transaction('resources', 'readonly')
        const request = tx.objectStore('resources').get('quota-recovery-old-resource')
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })
      verifyDb.close()
      return {
        quotaInjected,
        restored: restored ? Array.from(restored) : null,
        oldEntryEvicted: oldEntry === undefined,
      }
    } finally {
      IDBObjectStore.prototype.put = nativePut
    }
  })
  expect(quotaRecovery).toEqual({ quotaInjected: true, restored: [1, 4, 9, 16, 25], oldEntryEvicted: true })

  const clearedCache = await page.evaluate(async () => {
    const { clearLatexResourceCache } = await import('/src/lib/latex-resource-cache.ts')
    await clearLatexResourceCache()
    const readCounts = async (name: string, version: number, stores: string[]) => {
      const db = await new Promise<IDBDatabase>((resolve, reject) => {
        const request = indexedDB.open(name, version)
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })
      try {
        return await Promise.all(stores.map((storeName) => new Promise<number>((resolve, reject) => {
          const tx = db.transaction(storeName, 'readonly')
          const request = tx.objectStore(storeName).count()
          request.onsuccess = () => resolve(request.result)
          request.onerror = () => reject(request.error)
        })))
      } finally {
        db.close()
      }
    }
    return {
      app: await readCounts('papersage-latex-resources', 3, ['downloads', 'chunks', 'resources', 'formats']),
      worker: await readCounts('papersage-swiftlatex-worker-cache-v2', 1, ['resources']),
    }
  })
  expect(clearedCache).toEqual({ app: [0, 0, 0, 0], worker: [0] })
})

test('XeLaTeX compiles in the browser', async ({ page }) => {
  test.setTimeout(10 * 60_000)
  const { workerLogs, resourceNetworkLogs } = await prepareCompilerPage(page)

  const result = await page.evaluate(async () => {
    const { compileLatexInBrowser } = await import('/src/lib/latex/swiftlatex-browser-compiler.ts')
    const resources: Array<{ state: string; name: string; status?: number; error?: string }> = []
    const source = String.raw`\documentclass{article}
\begin{document}
PaperSage XeLaTeX browser smoke test.
\end{document}
`
    try {
      const compiled = await compileLatexInBrowser(
        [{ path: 'main.tex', content: source }],
        'main.tex',
        'xelatex',
        { onProgress: (event) => {
          if (event.stage === 'resource') resources.push(event.resource)
        } },
      )
      return {
        bytes: compiled.pdf.byteLength,
        prefix: new TextDecoder().decode(compiled.pdf.subarray(0, 5)),
        resources,
      }
    } catch (error) {
      return {
        error: String(error),
        resources: resources.filter((resource) => resource.kind === 'font' || resource.name.toLowerCase().includes('lmroman')),
      }
    }
  }).catch((error: unknown) => {
    console.error(`Browser resource network:\n${resourceNetworkLogs.slice(-30).join('\n')}\nSwiftLaTeX worker logs:\n${workerLogs.slice(-30).join('\n')}`)
    throw error
  })

  expect(result).toMatchObject({ prefix: '%PDF-' })
  expect(result.bytes).toBeGreaterThan(100)
})

test('XeLaTeX 中文二次编译零回源，并清理旧 profile 缓存', async ({ page }) => {
  test.setTimeout(25 * 60_000)
  const { workerLogs, resourceNetworkLogs } = await prepareCompilerPage(page)
  // Optional fontconfig probes (.fontspec) legitimately 404 and are not cacheable;
  // only HTTP 200 resource bytes must be served from IndexedDB on the second run.
  const resolveResponses: Array<{ url: string; status: number }> = []
  page.on('response', (response) => {
    if (response.url().includes('/texlive/') && response.url().includes('/resolve/')) {
      resolveResponses.push({ url: response.url(), status: response.status() })
    }
  })

  const compileChinese = () => page.evaluate(async () => {
    const { compileLatexInBrowser } = await import('/src/lib/latex/swiftlatex-browser-compiler.ts')
    const events: Array<{ name: string; kind: string; state: string; cacheSource?: string; lookup?: { kind: string } }> = []
    const source = String.raw`\documentclass[UTF8,a4paper]{ctexart}
\begin{document}
PaperSage 中文缓存验收：宏包、字体与 PDF 转换资源应命中本机缓存。
\end{document}
`
    const compiled = await compileLatexInBrowser(
      [{ path: 'main.tex', content: source }],
      'main.tex',
      'xelatex',
      { onProgress: (event) => { if (event.stage === 'resource') events.push(event.resource) } },
    )
    return { prefix: new TextDecoder().decode(compiled.pdf.subarray(0, 5)), bytes: compiled.pdf.byteLength, events }
  }).catch((error: unknown) => {
    console.error(`Browser resource network:\n${resourceNetworkLogs.slice(-30).join('\n')}\nSwiftLaTeX worker logs:\n${workerLogs.slice(-30).join('\n')}`)
    throw error
  })

  const readWorkerCache = () => page.evaluate(async () => {
    const db = await new Promise<IDBDatabase>((resolve, reject) => {
      const request = indexedDB.open('papersage-swiftlatex-worker-cache-v2', 1)
      request.onsuccess = () => resolve(request.result)
      request.onerror = () => reject(request.error)
    })
    try {
      return await new Promise<Array<{ url: string; revision?: string; bytes: number; isFont: boolean }>>((resolve, reject) => {
        const tx = db.transaction('resources', 'readonly')
        const request = tx.objectStore('resources').getAll()
        request.onsuccess = () => resolve(request.result.map((entry: {
          url: string; revision?: string; fontid?: string; bytes?: ArrayBuffer
        }) => ({
          url: entry.url,
          revision: entry.revision,
          bytes: entry.bytes?.byteLength ?? 0,
          isFont: Boolean(entry.fontid) || /\.(?:otf|ttf|ttc)(?:$|&)/i.test(entry.url),
        })))
        request.onerror = () => reject(request.error)
      })
    } finally {
      db.close()
    }
  })

  const firstStartedAt = Date.now()
  const first = await compileChinese()
  const firstMs = Date.now() - firstStartedAt
  const firstOkCount = resolveResponses.filter((response) => response.status === 200).length
  const secondStartedAt = Date.now()
  const secondStartIndex = resolveResponses.length
  const second = await compileChinese()
  const secondMs = Date.now() - secondStartedAt
  const secondSlice = resolveResponses.slice(secondStartIndex)
  const secondOkCount = secondSlice.filter((response) => response.status === 200).length
  const cacheAfterSecond = await readWorkerCache()

  const runWarmup = () => page.evaluate(async () => {
    const { warmupBrowserLatex, WARMUP_SAMPLES } = await import('/src/lib/latex/warmup.ts')
    const stages = new Set<string>()
    await warmupBrowserLatex(WARMUP_SAMPLES[0], { onProgress: (event) => stages.add(event.stage) })
    return [...stages]
  })
  const warmupStartedAt = resolveResponses.length
  const warmupStages = await runWarmup()
  const warmupFirstOkCount = resolveResponses.slice(warmupStartedAt).filter((response) => response.status === 200).length
  const secondWarmupStartedAt = resolveResponses.length
  await runWarmup()
  const warmupSecondOkCount = resolveResponses.slice(secondWarmupStartedAt).filter((response) => response.status === 200).length

  await page.evaluate(async () => {
    const db = await new Promise<IDBDatabase>((resolve, reject) => {
      const request = indexedDB.open('papersage-swiftlatex-worker-cache-v2', 1)
      request.onsuccess = () => resolve(request.result)
      request.onerror = () => reject(request.error)
    })
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction('resources', 'readwrite')
      tx.objectStore('resources').put({
        url: 'https://example.invalid/texlive/tl-old-profile/resolve/xetex?format=26&name=stale.sty',
        bytes: new Uint8Array([1, 2, 3]).buffer,
        fileid: '0'.repeat(64),
        revision: 'tl-old-profile',
        lastUsed: 1,
      })
      tx.oncomplete = () => resolve()
      tx.onerror = () => reject(tx.error)
      tx.onabort = () => reject(tx.error ?? new Error('旧 profile 缓存预置事务已中止'))
    })
    db.close()
    localStorage.setItem('papersage_texlive_profile', 'tl-old-profile')
  })
  const third = await compileChinese()
  const cacheAfterThird = await readWorkerCache()

  const cacheBytes = cacheAfterSecond.reduce((total, entry) => total + entry.bytes, 0)
  console.log(`[hele39] firstMs=${firstMs} secondMs=${secondMs} firstResolve200=${firstOkCount} secondResolve200=${secondOkCount} secondResolveNon200=${secondSlice.filter((response) => response.status !== 200).length} cacheEntries=${cacheAfterSecond.length} cacheBytes=${cacheBytes} fontEntries=${cacheAfterSecond.filter((entry) => entry.isFont).length} warmupFirst200=${warmupFirstOkCount} warmupSecond200=${warmupSecondOkCount}`)

  expect(first.prefix).toBe('%PDF-')
  expect(first.events.some((event) => event.state === 'loaded')).toBe(true)
  expect(firstOkCount).toBeGreaterThan(0)
  expect(second.prefix).toBe('%PDF-')
  expect(second.events.every((event) => event.state !== 'loaded' || event.cacheSource === 'browser')).toBe(true)
  expect(second.events.some((event) => event.state === 'loaded' && event.cacheSource === 'browser' && /\.(?:otf|ttf|ttc)$/i.test(event.name))).toBe(true)
  expect(
    secondOkCount,
    `第二次编译仍回源 200 资源：\n${secondSlice.filter((response) => response.status === 200).slice(0, 10).map((response) => response.url).join('\n')}`,
  ).toBe(0)
  expect(warmupStages).toContain('compile')
  expect(warmupStages).toContain('convert')
  expect(warmupSecondOkCount, '第二次预热仍回源').toBe(0)
  expect(cacheAfterSecond.some((entry) => entry.isFont)).toBe(true)
  expect(cacheAfterSecond.every((entry) => !entry.url.includes('.fontspec'))).toBe(true)
  if (first.events.some((event) => event.lookup?.kind === 'dvipdfmx')) {
    expect(cacheAfterSecond.some((entry) => entry.url.includes('/resolve/dvipdfmx'))).toBe(true)
  }
  expect(third.prefix).toBe('%PDF-')
  expect(cacheAfterThird.some((entry) => entry.revision === 'tl-old-profile')).toBe(false)
})
