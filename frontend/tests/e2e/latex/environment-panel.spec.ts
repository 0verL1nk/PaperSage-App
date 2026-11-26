import { expect, test } from '@playwright/test'

const ENABLED = process.env.RUN_BROWSER_LATEX_E2E === '1'

test.skip(!ENABLED, '设置 RUN_BROWSER_LATEX_E2E=1，并指定 BROWSER_LATEX_RESOURCE_API 后运行')

test('浏览器编译环境面板完成预热、计数更新与一键清理', async ({ page }) => {
  test.setTimeout(35 * 60_000)
  const apiBaseUrl = process.env.BROWSER_LATEX_RESOURCE_API
  if (!apiBaseUrl) throw new Error('面板验收需要配置 BROWSER_LATEX_RESOURCE_API')
  const wailsRequests: string[] = []
  page.on('request', (request) => {
    if (request.url().includes('/wails/runtime')) wailsRequests.push(request.url())
  })

  const cacheCounts = () => page.evaluate(async () => {
    const open = (name: string, version: number) => new Promise<IDBDatabase>((resolve, reject) => {
      const request = indexedDB.open(name, version)
      request.onsuccess = () => resolve(request.result)
      request.onerror = () => reject(request.error)
    })
    const count = (db: IDBDatabase, store: string) => new Promise<number>((resolve, reject) => {
      const request = db.transaction(store, 'readonly').objectStore(store).count()
      request.onsuccess = () => resolve(request.result)
      request.onerror = () => reject(request.error)
    })
    const workerDb = await open('papersage-swiftlatex-worker-cache-v2', 1)
    const worker = await count(workerDb, 'resources')
    workerDb.close()
    const appDb = await open('papersage-latex-resources', 3)
    const formats = await count(appDb, 'formats')
    appDb.close()
    return { worker, formats }
  })

  await page.goto('/guest')
  await page.waitForURL(/\/app\/projects/, { timeout: 60_000 })
  const projectId = await page.evaluate(async () => {
    const { createGuestProject } = await import('/src/services/guest-local-workspace.ts')
    const project = await createGuestProject({ name: '缓存面板验收' })
    return project.id
  })
  await page.evaluate((url) => {
    localStorage.setItem('papersage_custom_api_base_url', url)
    localStorage.setItem('papersage_api_target', 'custom')
  }, apiBaseUrl)

  await page.goto(`/project/${projectId}/writing`)
  await page.getByRole('button', { name: '初始化浏览器 LaTeX 环境' }).click()
  const panel = page.getByRole('dialog').filter({ hasText: 'TeX Live 宏包与字体' })
  await expect(panel).toBeVisible()

  expect(await cacheCounts()).toEqual({ worker: 0, formats: 0 })

  await panel.getByRole('button', { name: /预热中文论文环境/ }).click()
  await expect(panel.getByRole('button', { name: /取消预热/ })).toBeVisible()
  await expect(panel.getByText(/已加载 \d+ 个资源 · 已用 [1-9]\d* 秒/)).toBeVisible({ timeout: 15_000 })

  await expect(panel.getByText(/预热完成/)).toBeVisible({ timeout: 25 * 60_000 })
  const summary = panel.getByText(/\d+ 个文件 · /).first()
  await expect(summary).toBeVisible()
  await expect(summary).not.toHaveText(/^0 个文件/)
  const afterWarmup = await cacheCounts()
  expect(afterWarmup.worker).toBeGreaterThan(50)
  expect(afterWarmup.formats).toBeGreaterThan(0)

  await panel.getByRole('button', { name: '清理全部缓存' }).click()
  const confirm = page.getByRole('dialog').filter({ hasText: '清理浏览器 LaTeX 缓存？' })
  await confirm.getByRole('button', { name: '确认清理全部' }).click()
  await expect(panel.getByText(/浏览器 LaTeX 缓存已清理/)).toBeVisible({ timeout: 60_000 })
  await expect.poll(cacheCounts, { timeout: 60_000 }).toEqual({ worker: 0, formats: 0 })
  expect(wailsRequests).toEqual([])

  console.log(`[hele39-panel] warmupWorkerEntries=${afterWarmup.worker} formats=${afterWarmup.formats} wailsRequests=0`)
})
