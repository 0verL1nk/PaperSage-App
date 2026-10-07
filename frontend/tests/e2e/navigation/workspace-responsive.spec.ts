import { test, expect } from '@playwright/test'
import { mockWritingPageAPIs, seedAuthTokens, TEST_PROJECT_ID } from '../helpers'

const writingState = {
  project_id: TEST_PROJECT_ID,
  scenario: 'undergraduate_thesis',
  brief: {
    topic: '响应式工作台验收', background: '', questions: [], goals: [], scope_out: [],
    objects_and_data: '', methods: '', key_chapters: [], milestones: [], constraints: [],
    known_facts: [], assumptions: [], unverified: [],
  },
  requirements: [],
  phases: { prepare: 'in_progress', write: 'not_started', review: 'not_started' },
  current_phase: 'review',
  revision: 1,
}
let persistedWritingState = { ...writingState }

test('project workspace stages, writing editor, and reference library fit phone/tablet/desktop viewports', async ({ page }) => {
  test.setTimeout(120_000)
  const unmockedApiRequests: string[] = []
  await page.route('**/api/**', async (route) => {
    unmockedApiRequests.push(route.request().url())
    await route.abort()
  })
  await mockWritingPageAPIs(page)
  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/writing-state`, async (route) => {
    const isPut = route.request().method() === 'PUT'
    if (isPut) {
      const update = route.request().postDataJSON()
      persistedWritingState = { ...persistedWritingState, ...update, revision: persistedWritingState.revision + 1 }
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: persistedWritingState }),
    })
  })
  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/documents**`, async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: [] }) })
  })
  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/folders`, async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: [] }) })
  })
  await seedAuthTokens(page)

  await page.goto(`/project/${TEST_PROJECT_ID}`)
  await expect(page).toHaveURL(new RegExp(`/project/${TEST_PROJECT_ID}/prepare$`))
  await page.goto(`/project/${TEST_PROJECT_ID}/logic-chain`)
  await expect(page).toHaveURL(new RegExp(`/project/${TEST_PROJECT_ID}/prepare$`))

  await page.getByPlaceholder('用一句话概括研究对象与问题').fill('已通过接口持久化的研究主题')
  await expect(page.getByText('所有更改已保存')).toBeVisible({ timeout: 5000 })
  await page.reload()
  await expect(page.getByPlaceholder('用一句话概括研究对象与问题')).toHaveValue('已通过接口持久化的研究主题')

  const viewports = [
    { name: 'small phone', width: 320, height: 640 },
    { name: 'phone', width: 390, height: 844 },
    { name: 'tablet portrait', width: 768, height: 1024 },
    { name: 'tablet landscape', width: 1024, height: 768 },
    { name: 'desktop', width: 1440, height: 900 },
  ]
  const pages = [
    { path: 'prepare', heading: '先把研究说清楚' },
    { path: 'review', heading: '完成情况由你确认' },
    { path: 'uploading', heading: '参考文献' },
    { path: 'writing', heading: 'Explorer' },
  ]

  for (const route of pages) {
    await page.goto(`/project/${TEST_PROJECT_ID}/${route.path}`)
    if (route.path === 'writing') await expect(page.getByText(route.heading, { exact: true })).toBeVisible({ timeout: 15_000 })
    else await expect(page.getByRole(route.path === 'uploading' ? 'heading' : 'heading', { name: route.heading }).first()).toBeVisible({ timeout: 10_000 })

    for (const viewport of viewports) {
      await page.setViewportSize({ width: viewport.width, height: viewport.height })
      await expect.poll(async () => page.evaluate(() => document.fonts.ready.then(() => document.documentElement.scrollWidth <= window.innerWidth + 1)), {
        message: `${route.path} must not overflow horizontally at ${viewport.name} (${viewport.width}px)`,
      }).toBe(true)

      const geometry = await page.evaluate(() => {
        const header = document.querySelector('header')?.getBoundingClientRect()
        const nav = document.querySelector('nav.md\\:hidden')?.getBoundingClientRect()
        const main = document.querySelector('main')?.getBoundingClientRect()
        const editor = document.querySelector('.monaco-editor')?.getBoundingClientRect()
        return {
          headerBottom: header?.bottom ?? 0,
          navBottom: nav?.bottom ?? 0,
          mainTop: main?.top ?? 0,
          editorRight: editor?.right ?? 0,
          editorExists: Boolean(editor),
        }
      })
      expect(geometry.mainTop, `${route.path} main should start below fixed workspace chrome at ${viewport.name}`).toBeGreaterThanOrEqual(Math.max(geometry.headerBottom, geometry.navBottom))
      if (route.path === 'writing' && geometry.editorExists) {
        expect(geometry.editorRight, `editor should stay within viewport at ${viewport.name}`).toBeLessThanOrEqual(viewport.width + 1)
      }
      if (viewport.width < 768) {
        const activeTab = page.locator('nav').getByRole('button', { name: route.path === 'prepare' ? '写作准备' : route.path === 'review' ? '检查完善' : route.path === 'uploading' ? '上传资料' : '论文撰写' })
        await expect(activeTab).toBeVisible()
        await expect(activeTab).toHaveAttribute('aria-current', 'page')
        expect(await activeTab.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
        if (route.path === 'prepare' && viewport.width === 320) {
          await activeTab.focus()
          await page.keyboard.press('Enter')
          await expect(activeTab).toHaveAttribute('aria-current', 'page')
          await page.keyboard.press('Tab')
          expect(await page.evaluate(() => document.activeElement?.tagName)).toBe('BUTTON')
        }
      }
    }
  }
  expect(unmockedApiRequests, 'every API request must use an explicit test mock').toEqual([])
})

test('browser-local guest can create and save a real project across mobile, tablet, and desktop', async ({ page }) => {
  test.setTimeout(120_000)
  const unmockedApiRequests: string[] = []
  await page.route('**/api/**', async (route) => {
    unmockedApiRequests.push(route.request().url())
    await route.abort()
  })

  const viewports = [
    { name: 'small phone', width: 320, height: 640 },
    { name: 'phone', width: 390, height: 844 },
    { name: 'tablet portrait', width: 768, height: 1024 },
    { name: 'tablet landscape', width: 1024, height: 768 },
    { name: 'desktop', width: 1440, height: 900 },
  ]
  await page.setViewportSize({ width: 320, height: 640 })
  await page.goto('/guest')
  await expect(page).toHaveURL(/\/app\/projects$/)
  await expect(page.getByText('本机游客 · 未同步')).toBeVisible()
  await expect(page.getByRole('heading', { name: '所有项目' })).toBeVisible()
  const backendToggle = page.getByRole('button', { name: /切换后端，当前：在线/ })
  await expect(backendToggle).toBeVisible()
  expect(await backendToggle.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)

  await page.getByRole('button', { name: '创建第一个项目' }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  const nameInput = page.getByLabel('项目名称')
  await expect(nameInput).toBeVisible()
  await expect.poll(() => nameInput.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
  const createButton = dialog.getByRole('button', { name: '创建项目' })
  await expect.poll(() => createButton.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
  const dialogGeometry = await dialog.evaluate((element) => ({ width: element.clientWidth, scrollWidth: element.scrollWidth, height: element.clientHeight, scrollHeight: element.scrollHeight }))
  expect(dialogGeometry.scrollWidth).toBeLessThanOrEqual(dialogGeometry.width + 1)
  await nameInput.fill('Local Guest Research')
  await createButton.click()
  const projectCard = page.getByRole('button', { name: /Local Guest Research/ })
  await expect(projectCard).toBeVisible()
  await projectCard.click()
  await expect(page).toHaveURL(/\/project\/.+\/prepare$/)
  await expect(page.getByPlaceholder('用一句话概括研究对象与问题')).toBeVisible()

  const topicInput = page.getByPlaceholder('用一句话概括研究对象与问题')
  await topicInput.fill('本机游客工作区持久化验收')
  await expect(page.getByText('已保存在此设备')).toBeVisible({ timeout: 10_000 })
  await page.reload()
  await expect(topicInput).toHaveValue('本机游客工作区持久化验收')

  for (const viewport of viewports) {
    await page.setViewportSize({ width: viewport.width, height: viewport.height })
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), {
      message: `local guest project workspace must not overflow at ${viewport.name}`,
    }).toBe(true)
    const activeStage = page.getByRole('button', { name: '写作准备' })
    await expect(activeStage).toBeVisible()
    expect(await activeStage.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
    await expect(page.getByText('已保存在此设备')).toBeVisible()
  }
  expect(unmockedApiRequests, 'browser-local guest reads and writes must not call cloud APIs').toEqual([])
})
