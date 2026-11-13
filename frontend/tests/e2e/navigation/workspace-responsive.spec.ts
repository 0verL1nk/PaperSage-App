import { test, expect } from '@playwright/test'
import { mockWritingPageAPIs, seedAuthTokens, TEST_PROJECT_ID } from '../helpers'

test.use({ storageState: { cookies: [], origins: [] } })

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
  await page.route('**/api/v1/settings', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: { settings: { ui: {}, editor: {}, agent: { providers: [], active_provider_id: '' } } } }),
    })
  })
  await seedAuthTokens(page)

  await page.goto(`/project/${TEST_PROJECT_ID}`)
  await expect(page).toHaveURL(new RegExp(`/project/${TEST_PROJECT_ID}/prepare$`))
  await page.goto(`/project/${TEST_PROJECT_ID}/logic-chain`)
  await expect(page).toHaveURL(new RegExp(`/project/${TEST_PROJECT_ID}/prepare$`))

  await expect(page.getByText('响应式工作台验收')).toBeVisible()

  const viewports = [
    { name: 'small phone', width: 320, height: 640 },
    { name: 'phone', width: 390, height: 844 },
    { name: 'tablet portrait', width: 768, height: 1024 },
    { name: 'tablet landscape', width: 1024, height: 768 },
    { name: 'desktop', width: 1440, height: 900 },
  ]
  const pages = [
    { path: 'prepare', heading: '和研究顾问一起理清思路' },
    { path: 'review', heading: '提交前，把容易漏的地方过一遍' },
    { path: 'uploading', heading: '参考文献' },
    { path: 'writing', heading: '文件' },
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
        const nav = document.querySelector('nav.lg\\:hidden')?.getBoundingClientRect()
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
      if (route.path === 'prepare' && viewport.width < 1024) {
        const tablist = page.getByRole('tablist', { name: '研究准备视图' })
        await expect(tablist).toBeVisible()
        await expect(page.getByRole('tab', { name: '研究顾问' })).toHaveAttribute('aria-selected', 'true')
        await expect(page.getByRole('region', { name: '研究准备对话' })).toBeVisible()
        await page.getByRole('tab', { name: '研究蓝图' }).click()
        await expect(page.getByRole('region', { name: '研究准备对话' })).toBeHidden()
        await page.getByRole('tab', { name: '研究顾问' }).click()
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
  await expect(page.getByRole('link', { name: '本机游客，登录并导入' })).toBeVisible()
  await expect(page.getByRole('heading', { name: '所有项目' })).toBeVisible()
  // Browser-local guest workspaces never call an API, so the backend selector
  // is intentionally omitted from the guest shell.
  await expect(page.getByRole('button', { name: /切换后端/ })).toHaveCount(0)

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
  await expect(page.getByText('研究顾问需要登录后使用')).toBeVisible()
  const projectId = new URL(page.url()).pathname.split('/')[2]
  await page.evaluate(async (id) => {
    const local = await import('/src/services/guest-local-workspace.ts')
    const state = await local.getGuestWritingState(id)
    state.brief.topic = '本机游客工作区持久化验收'
    await local.saveGuestWritingState(id, {
      brief: state.brief,
      requirements: state.requirements,
      phases: state.phases,
      current_phase: 'prepare',
      revision: state.revision,
    })
  }, projectId)
  await page.reload()
  const briefTab = page.getByRole('tab', { name: '研究蓝图' })
  await expect(briefTab).toBeVisible()
  await briefTab.click()
  await expect(page.getByText('本机游客工作区持久化验收')).toBeVisible()

  for (const viewport of viewports) {
    await page.setViewportSize({ width: viewport.width, height: viewport.height })
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), {
      message: `local guest project workspace must not overflow at ${viewport.name}`,
    }).toBe(true)
    const activeStage = page.getByRole('button', { name: '写作准备' })
    await expect(activeStage).toBeVisible()
    expect(await activeStage.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
    if (viewport.width < 1024) {
      await expect(briefTab).toBeVisible()
      await briefTab.click()
    }
    await expect(page.getByText('本机游客工作区持久化验收')).toBeVisible()
  }
  expect(unmockedApiRequests, 'browser-local guest reads and writes must not call cloud APIs').toEqual([])
})
