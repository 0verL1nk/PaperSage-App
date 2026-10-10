import { readFile, writeFile, mkdir } from 'node:fs/promises'
import { createHash } from 'node:crypto'
import path from 'node:path'
import JSZip from 'jszip'
import { expect, test } from '@playwright/test'
import { scopeVercelAccessToOrigin } from './helpers/vercel-origin-auth'

interface TemplateCase {
  name: string
  sourceUrl?: string
  zipPath: string
  sha256: string
  mainFile: string
  engine: 'pdflatex' | 'xelatex'
  expectedText?: string
}

const enabled = process.env.RUN_BROWSER_LATEX_E2E === '1'
const apiBaseUrl = process.env.BROWSER_LATEX_RESOURCE_API
const cases: TemplateCase[] = JSON.parse(process.env.BROWSER_LATEX_TEMPLATE_CASES ?? '[]')

test.skip(!enabled, '设置 RUN_BROWSER_LATEX_E2E=1 后运行真实浏览器模板编译')
test.skip(cases.length === 0, '通过 BROWSER_LATEX_TEMPLATE_CASES 提供 ZIP 路径、主文件和引擎')

for (const template of cases) {
  test(`${template.name} template compiles in the browser`, async ({ page }) => {
    test.setTimeout(15 * 60_000)
    expect(template.sha256, `${template.name}: sha256 must be a 64-character hexadecimal ZIP digest`)
      .toMatch(/^[a-f\d]{64}$/i)
    await scopeVercelAccessToOrigin(page, apiBaseUrl)

    const workerLogs: string[] = []
    const networkLogs: string[] = []
    page.on('worker', (worker) => worker.on('console', (message) => workerLogs.push(message.text())))
    page.on('request', (request) => {
      if (request.url().includes('/texlive/') || request.url().includes('/latex-engines/stet/')) {
        networkLogs.push(`${request.method()} ${request.url()}`)
      }
    })
    page.on('response', (response) => {
      if (response.url().includes('/texlive/') || response.url().includes('/latex-engines/stet/')) {
        networkLogs.push(`HTTP ${response.status()} ${response.url()}`)
      }
    })
    page.on('requestfailed', (request) => {
      if (request.url().includes('/texlive/') || request.url().includes('/latex-engines/stet/')) {
        networkLogs.push(`FAILED ${request.failure()?.errorText ?? 'unknown'} ${request.url()}`)
      }
    })
    await page.goto('/')
    if (apiBaseUrl) {
      await page.evaluate((url) => {
        localStorage.setItem('papersage_custom_api_base_url', url)
        localStorage.setItem('papersage_api_target', 'custom')
      }, apiBaseUrl)
    }

    const archiveBytes = await readFile(template.zipPath)
    const archiveSha256 = createHash('sha256').update(archiveBytes).digest('hex')
    expect(archiveSha256, `${template.name}: ZIP SHA-256 mismatch for ${template.zipPath}`).toBe(template.sha256.toLowerCase())
    const archive = await JSZip.loadAsync(archiveBytes)
    const projectRoot = template.mainFile.includes('/') ? template.mainFile.slice(0, template.mainFile.lastIndexOf('/') + 1) : ''
    const rootFile = template.mainFile.slice(projectRoot.length)
    const files = await Promise.all(Object.values(archive.files)
      .filter((entry) => !entry.dir && !entry.name.split('/').some((part) => part === '__MACOSX' || part.startsWith('.')))
      .filter((entry) => entry.name.startsWith(projectRoot))
      .filter((entry) => /\.(tex|bib|bst|cls|sty|def|cfg|ist|eps|ps|pdf|png|jpe?g|svg)$/i.test(entry.name))
      .map(async (entry) => ({ path: entry.name.slice(projectRoot.length), base64: (await entry.async('base64')) })))
    expect(files.some((file) => file.path === rootFile), `主文件 ${template.mainFile} 不在 ${template.zipPath} 中`).toBe(true)

    const result = await page.evaluate(async ({ files: projectFiles, rootFile, compiler }) => {
      const binaryFiles = projectFiles.map(({ path: filePath, base64 }) => {
        const binary = atob(base64)
        const bytes = new Uint8Array(binary.length)
        for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
        return { path: filePath, content: bytes }
      })
      try {
        const { compileLatexInBrowser } = await import('/src/lib/latex/swiftlatex-browser-compiler.ts')
        const compiled = await compileLatexInBrowser(binaryFiles, rootFile, compiler)
        let binary = ''
        for (let i = 0; i < compiled.pdf.length; i += 0x8000) {
          binary += String.fromCharCode(...compiled.pdf.subarray(i, i + 0x8000))
        }
        return { pdfBase64: btoa(binary), log: compiled.log }
      } catch (error) {
        return { error: error instanceof Error ? `${error.message}\n${error.stack ?? ''}` : String(error) }
      }
    }, { files, rootFile, compiler: template.engine })

    expect(result.error, `编译失败\n${result.error ?? ''}\nTeX 资源请求:\n${networkLogs.slice(-60).join('\n')}\nWorker 日志:\n${workerLogs.slice(-60).join('\n')}`).toBeUndefined()
    const pdfBytes = Buffer.from(result.pdfBase64 ?? '', 'base64')
    expect(pdfBytes.subarray(0, 5).toString('ascii'), `${template.name}: output does not start with %PDF-`).toBe('%PDF-')

    const pdfValidation = await page.evaluate(async ({ base64, expectedText }) => {
      const { PDFJS } = await import('/src/lib/pdf/pdf-js.ts')
      // Match the worker URL used by WritingPage so the acceptance parser uses
      // the same PDF.js version and browser worker as the product preview.
      PDFJS.GlobalWorkerOptions.workerSrc = `https://unpkg.com/pdfjs-dist@${PDFJS.version}/build/pdf.worker.min.mjs`
      const binary = atob(base64)
      const data = new Uint8Array(binary.length)
      for (let index = 0; index < binary.length; index++) data[index] = binary.charCodeAt(index)

      const task = PDFJS.getDocument({ data, isEvalSupported: false, enableXfa: false, ...((await import('/src/lib/pdf/pdf-js.ts')).pdfDocumentOptions) })
      try {
        const document = await task.promise
        const numPages = document.numPages
        if (!Number.isSafeInteger(numPages) || numPages < 1) {
          throw new Error(`PDF.js reported an invalid page count: ${numPages}`)
        }

        const firstPage = await document.getPage(1)
        const pageSizes: Array<{ width: number; height: number }> = []
        const extractedParts: string[] = []
        for (let pageNumber = 1; pageNumber <= numPages; pageNumber++) {
          const page = pageNumber === 1 ? firstPage : await document.getPage(pageNumber)
          const viewport = page.getViewport({ scale: 1 })
          pageSizes.push({ width: viewport.width, height: viewport.height })
          if (expectedText) {
            const content = await page.getTextContent()
            extractedParts.push(...content.items.flatMap((item) => 'str' in item ? [item.str] : []))
          }
        }

        const normalize = (value: string) => value.normalize('NFKC').replace(/\s+/g, ' ').trim().toLowerCase()
        const extractedText = extractedParts.join(' ')
        const expectedTextFound = expectedText
          ? normalize(extractedText).includes(normalize(expectedText))
          : undefined
        return {
          numPages,
          pageSizes,
          expectedTextFound,
          extractedTextPreview: expectedText ? extractedText.slice(0, 1200) : undefined,
        }
      } finally {
        await task.destroy()
      }
    }, { base64: pdfBytes.toString('base64'), expectedText: template.expectedText })

    expect(pdfValidation.numPages, `${template.name}: PDF.js did not find a usable page tree`).toBeGreaterThan(0)
    expect(pdfValidation.pageSizes[0]?.width, `${template.name}: first page has no width`).toBeGreaterThan(0)
    expect(pdfValidation.pageSizes[0]?.height, `${template.name}: first page has no height`).toBeGreaterThan(0)
    if (template.expectedText) {
      expect(
        pdfValidation.expectedTextFound,
        `${template.name}: expected text was not found in the PDF: ${template.expectedText}\nExtracted preview: ${pdfValidation.extractedTextPreview ?? ''}`,
      ).toBe(true)
    }
    console.log(
      `${template.name}: PDF.js parsed ${pdfValidation.numPages} page(s) (${pdfBytes.byteLength} bytes); ` +
      `ZIP SHA-256 ${archiveSha256}${template.sourceUrl ? `; source ${template.sourceUrl}` : ''}`,
    )

    const outputDirectory = process.env.BROWSER_LATEX_PDF_DIR
    if (outputDirectory && result.pdfBase64) {
      await mkdir(outputDirectory, { recursive: true })
      const outputName = `${template.name.replace(/[^\p{L}\p{N}._-]+/gu, '-')}.pdf`
      await writeFile(path.join(outputDirectory, outputName), Buffer.from(result.pdfBase64, 'base64'))
    }
  })
}
