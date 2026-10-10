import { expect, test } from '@playwright/test'
import { scopeVercelAccessToOrigin } from './helpers/vercel-origin-auth'

const ENABLED = process.env.RUN_BROWSER_LATEX_E2E === '1'

test('pinned BibTeX worker starts and supports virtual filesystem read/write', async ({ page }) => {
  test.setTimeout(120_000)
  await page.goto('/')
  const result = await page.evaluate(async () => {
    const worker = new Worker('/latex-engines/wasmtex-bibtex-2026-9147f5703951082b/papersage-bibtex-worker.js?engine=bibtex')
    const waitFor = (predicate: (message: Record<string, unknown>) => boolean) => new Promise<Record<string, unknown>>((resolve, reject) => {
      const timeout = window.setTimeout(() => reject(new Error('BibTeX worker protocol timed out')), 60_000)
      const listener = (event: MessageEvent<Record<string, unknown>>) => {
        if (!predicate(event.data)) return
        window.clearTimeout(timeout)
        worker.removeEventListener('message', listener)
        resolve(event.data)
      }
      worker.addEventListener('message', listener)
      worker.addEventListener('error', (event) => {
        window.clearTimeout(timeout)
        reject(new Error(`BibTeX worker error: ${event.message}`))
      }, { once: true })
    })
    try {
      const ready = await waitFor((message) => !message.cmd && message.result === 'ok')
      const inputs = [
        ['main.aux', String.raw`\citation{papersage-bibtex-fixture}
\bibdata{refs}
\bibstyle{local}
`],
        ['refs.bib', '@article{papersage-bibtex-fixture, author={Ada Lovelace}, title={Analytical Engines}, year={1843}}'],
        ['local.bst', String.raw`ENTRY { author title } {} {}
FUNCTION {begin.bib} { "\begin{thebibliography}{1}" write$ newline$ }
FUNCTION {end.bib} { "\end{thebibliography}" write$ newline$ }
FUNCTION {article} { "\bibitem{" cite$ * "}" * write$ newline$ title write$ newline$ }
FUNCTION {default.type} { article }
READ
EXECUTE {begin.bib}
ITERATE {call.type$}
EXECUTE {end.bib}
`],
      ]
      let write: Record<string, unknown> = {}
      for (const [url, src] of inputs) {
        worker.postMessage({ cmd: 'writefile', url, src })
        write = await waitFor((message) => message.cmd === 'writefile')
        if (write.result !== 'ok') break
      }
      worker.postMessage({ cmd: 'compilebibtex', url: 'main' })
      const compile = await waitFor((message) => message.cmd === 'compile')
      worker.postMessage({ cmd: 'readfile', url: 'main.bbl' })
      const read = await waitFor((message) => message.cmd === 'readfile')
      return { ready, write, compile, read }
    } finally {
      worker.terminate()
    }
  })

  expect(result.ready).toMatchObject({ result: 'ok' })
  expect(result.write).toMatchObject({ result: 'ok' })
  expect(result.compile, JSON.stringify(result.compile)).toMatchObject({ result: 'ok' })
  expect(result.read).toMatchObject({ result: 'ok' })
  expect(result.read.data).toContain('\\bibitem{papersage-bibtex-fixture}')
  expect(result.read.data).toContain('Analytical Engines')
})

test('SwiftLaTeX worker lists and reads nested generated auxiliary files', async ({ page }) => {
  await page.goto('/')
  const result = await page.evaluate(async () => {
    const worker = new Worker('/latex-engines/papersage-swift-worker.js?engine=pdftex')
    const waitFor = (predicate: (message: Record<string, unknown>) => boolean) => new Promise<Record<string, unknown>>((resolve, reject) => {
      const timeout = window.setTimeout(() => reject(new Error('SwiftLaTeX VFS protocol timed out')), 60_000)
      const listener = (event: MessageEvent<Record<string, unknown>>) => {
        if (!predicate(event.data)) return
        window.clearTimeout(timeout)
        worker.removeEventListener('message', listener)
        resolve(event.data)
      }
      worker.addEventListener('message', listener)
      worker.addEventListener('error', (event) => {
        window.clearTimeout(timeout)
        reject(new Error(`SwiftLaTeX worker error: ${event.message}`))
      }, { once: true })
    })
    try {
      await waitFor((message) => !message.cmd && message.result === 'ok')
      worker.postMessage({ cmd: 'mkdir', url: 'chapters' })
      await waitFor((message) => message.cmd === 'mkdir')
      worker.postMessage({ cmd: 'writefile', url: 'chapters/chapter.aux', src: '\\bibdata{refs}\n' })
      await waitFor((message) => message.cmd === 'writefile')
      worker.postMessage({ cmd: 'listaux' })
      const listed = await waitFor((message) => message.cmd === 'listaux')
      worker.postMessage({ cmd: 'readfile', url: 'chapters/chapter.aux' })
      const read = await waitFor((message) => message.cmd === 'readfile')
      return { listed, read }
    } finally {
      worker.terminate()
    }
  })

  expect(result.listed, JSON.stringify(result.listed)).toMatchObject({ result: 'ok' })
  expect(result.listed.files).toContain('chapters/chapter.aux')
  expect(result.read).toMatchObject({ result: 'ok', data: '\\bibdata{refs}\n' })
})

test('BibTeX runs between browser TeX passes and its BBL resolves citations', async ({ page }) => {
  test.skip(!ENABLED, '设置 RUN_BROWSER_LATEX_E2E=1，并指定可访问的 TeX 资源 API 后运行')

  test.setTimeout(10 * 60_000)
  const apiBaseUrl = process.env.BROWSER_LATEX_RESOURCE_API
  await scopeVercelAccessToOrigin(page, apiBaseUrl)
  await page.goto('/')
  if (apiBaseUrl) {
    await page.evaluate((baseUrl) => {
      localStorage.setItem('papersage_custom_api_base_url', baseUrl)
      localStorage.setItem('papersage_api_target', 'custom')
    }, apiBaseUrl)
  }

  const result = await page.evaluate(async () => {
    const { compileLatexInBrowser } = await import('/src/lib/latex/swiftlatex-browser-compiler.ts')
    const source = String.raw`\documentclass{article}
\begin{document}
Bibliography browser acceptance: \cite{papersage-bibtex-fixture}.
\bibliographystyle{plain}
\bibliography{refs}
\end{document}
`
    const files = [
      { path: 'main.tex', content: source },
      { path: 'refs.bib', content: '@article{papersage-bibtex-fixture, author={Ada Lovelace}, title={Analytical Engines}, journal={Example Journal}, year={1843}}' },
    ]
    const events: string[] = []
    const output = await compileLatexInBrowser(files, 'main.tex', 'pdflatex', {
      onProgress: (event) => events.push(event.message),
    })
    return {
      bytes: output.pdf.byteLength,
      prefix: new TextDecoder().decode(output.pdf.subarray(0, 5)),
      log: output.log,
      events,
    }
  })

  expect(result.prefix).toBe('%PDF-')
  expect(result.bytes).toBeGreaterThan(100)
  expect(result.log).toContain('BibTeX main')
  expect(result.log).toContain('Database file #1: refs.bib')
  expect(result.events.some((event) => event.includes('正在生成参考文献'))).toBe(true)
})
