import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import JSZip from 'jszip'
import { expect, test } from '@playwright/test'

import { mockWritingPageAPIs, seedAuthTokens, TEST_PROJECT_ID, WRITING_URL } from '../helpers'

const samplePdfPath = fileURLToPath(new URL('../fixtures/sample.pdf', import.meta.url))

test.describe('Browser compilation recovery', () => {
  test('cancelling while project files load prevents the browser job from starting', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    let releaseFileRequest: (() => void) | undefined
    let fileRequestSeen: (() => void) | undefined
    const waitingForFileRequest = new Promise<void>((resolve) => { fileRequestSeen = resolve })
    let browserJobRequests = 0

    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex-files/refs.bib`, async (route) => {
      fileRequestSeen?.()
      await new Promise<void>((resolve) => { releaseFileRequest = resolve })
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { path: 'refs.bib', content: '' } }),
      })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser`, async (route) => {
      browserJobRequests++
      await route.fulfill({ status: 202, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { job_id: 'should-not-start', status: 'running' } }) })
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('compile-button').click()
    await waitingForFileRequest
    await page.getByTestId('cancel-browser-compile').click()
    releaseFileRequest?.()

    await expect(page.getByTestId('cancel-browser-compile')).toHaveCount(0)
    await expect.poll(() => browserJobRequests).toBe(0)
    await expect(page.getByText('编译错误')).toHaveCount(0)
  })

  test('terminates an active browser pass and marks its persisted job cancelled', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    const compilerModule = `
      export async function compileLatexInBrowser(_files, _rootFile, _compiler, options = {}) {
        options.onProgress?.({ stage: 'compile', message: 'fixture engine running', pass: 1, totalPasses: 1 });
        return await new Promise((_resolve, reject) => {
          options.signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true });
        });
      }
    `
    let cancelRequests = 0

    await page.route('**/src/lib/latex/swiftlatex-browser-compiler.ts*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/javascript', body: compilerModule })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser`, async (route) => {
      await route.fulfill({ status: 202, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { job_id: 'browser-job-cancel', status: 'running' } }) })
    })
    await page.route((url) => url.pathname.endsWith(`/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/`)
      && url.searchParams.get('job_id') === 'browser-job-cancel', async (route) => {
      if (route.request().method() !== 'DELETE') {
        await route.fallback()
        return
      }
      cancelRequests++
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { cancelled: true, job_id: 'browser-job-cancel' } }) })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser-job-cancel/status/`, async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { job_id: 'browser-job-cancel', status: 'cancelled' } }) })
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('compile-button').click()
    await expect(page.getByText('fixture engine running')).toBeVisible({ timeout: 15_000 })
    await page.getByTestId('cancel-browser-compile').click()
    await expect(page.getByTestId('cancel-browser-compile')).toHaveCount(0)
    await expect.poll(() => cancelRequests).toBe(1)
    await expect(page.getByText('编译错误')).toHaveCount(0)
  })

  test('shows the worker failure details and retries compilation in a fresh browser job', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    const pdfBase64 = (await readFile(samplePdfPath)).toString('base64')
    const compilerModule = `
      let attempts = 0;
      export async function compileLatexInBrowser(_files, _rootFile, _compiler, options = {}) {
        attempts++;
        if (attempts === 1) throw new Error('SwiftLaTeX worker stopped unexpectedly');
        options.onProgress?.({ stage: 'compile', message: 'retry worker compile', pass: 1, totalPasses: 1 });
        return { pdf: Uint8Array.from(atob(${JSON.stringify(pdfBase64)}), char => char.charCodeAt(0)), log: 'retry fixture log' };
      }
    `
    let compileJobs = 0
    let uploads = 0

    await page.route('**/src/lib/latex/swiftlatex-browser-compiler.ts*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/javascript', body: compilerModule })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser`, async (route) => {
      compileJobs++
      await route.fulfill({ status: 202, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { job_id: `browser-retry-${compileJobs}`, status: 'running' } }) })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser-retry-1/browser-failure`, async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { job_id: 'browser-retry-1', status: 'failed' } }) })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser-retry-1/status/`, async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { job_id: 'browser-retry-1', status: 'failed' } }) })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser-retry-2/browser-result`, async (route) => {
      uploads++
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { job_id: 'browser-retry-2', build_id: 'retry-build', status: 'success' } }) })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser-retry-2/status/`, async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { job_id: 'browser-retry-2', build_id: 'retry-build', status: 'success' } }) })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/output/**`, async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/pdf', body: await readFile(samplePdfPath) })
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('compile-button').click()
    await expect(page.getByText('SwiftLaTeX worker stopped unexpectedly')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('retry-browser-compile')).toBeVisible()
    await page.getByTestId('retry-browser-compile').click()
    await expect.poll(() => uploads).toBe(1)
    expect(compileJobs).toBe(2)
  })

  test('opens a previously persisted PDF from compile history', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    const samplePdf = await readFile(samplePdfPath)
    const outputZip = new JSZip()
    outputZip.file('output.pdf', samplePdf)
    const outputZipBytes = await outputZip.generateAsync({ type: 'nodebuffer' })
    let outputRequests = 0
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/history/`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: { total: 1, items: [{
          job_id: 'historical-job-1', status: 'success', root_file: 'main.tex', compiler: 'xelatex',
          build_id: 'historical-build-1', error: null, created_at: '2026-10-09T10:00:00Z', updated_at: '2026-10-09T10:01:00Z',
        }] } }),
      })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/output/**`, async (route) => {
      outputRequests++
      await route.fulfill({ status: 200, contentType: 'application/pdf', body: samplePdf })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/output/historical-build-1/output.zip`, async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/zip', body: outputZipBytes })
    })

    await page.goto(WRITING_URL)
    await page.getByRole('button', { name: '编译历史' }).click()
    await page.getByText('main.tex · xelatex').click()
    await expect.poll(() => outputRequests).toBe(1)
    const downloadPromise = page.waitForEvent('download')
    await page.getByRole('button', { name: '下载 PDF' }).click()
    const download = await downloadPromise
    expect(download.suggestedFilename()).toBe('output.pdf')
  })

  test('retries a failed artifact upload without running the compiler again', async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)

    const pdfBase64 = (await readFile(samplePdfPath)).toString('base64')
    const compilerModule = `
      export async function compileLatexInBrowser(_files, _rootFile, _compiler, options = {}) {
        options.onProgress?.({ stage: 'compile', message: 'fixture compile', pass: 1, totalPasses: 1 });
        return { pdf: Uint8Array.from(atob(${JSON.stringify(pdfBase64)}), char => char.charCodeAt(0)), log: 'fixture log' };
      }
    `
    let compileJobs = 0
    let uploadAttempts = 0

    await page.route('**/src/lib/latex/swiftlatex-browser-compiler.ts*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/javascript', body: compilerModule })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser`, async (route) => {
      compileJobs++
      await route.fulfill({ status: 202, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { job_id: 'browser-job-1', status: 'running' } }) })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser-job-1/browser-result`, async (route) => {
      uploadAttempts++
      if (uploadAttempts === 1) {
        await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 503, info: 'temporary object storage failure' }) })
        return
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { job_id: 'browser-job-1', build_id: 'browser-build-1', status: 'success' } }) })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/compile/browser-job-1/status/`, async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { job_id: 'browser-job-1', build_id: 'browser-build-1', status: 'success' } }) })
    })
    await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/latex/output/**`, async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/pdf', body: await readFile(samplePdfPath) })
    })

    await page.goto(WRITING_URL)
    await page.getByTestId('compile-button').click()
    await expect(page.getByTestId('retry-browser-upload')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText(/临时对象存储失败|temporary object storage failure/)).toBeVisible()
    await page.getByTestId('retry-browser-upload').click()
    await expect(page.getByTestId('retry-browser-upload')).toHaveCount(0)
    await expect.poll(() => uploadAttempts).toBe(2)
    expect(compileJobs).toBe(1)
  })
})
