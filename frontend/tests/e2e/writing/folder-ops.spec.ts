import { test as base, expect } from '@playwright/test'

import { apiCreateLatexFile, apiDeleteLatexFolder, loginAs, e2eEnv } from '../helpers'

/**
 * Folder operations — verifies the file-tree's folder create /
 * rename / recursive-delete flow against a real backend.
 *
 * 隔离策略（e2e 最佳实践）：
 * - 夹具经真实 API 创建、唯一命名（时间戳），重试/重跑/并行 worker 互不碰撞
 * - 清理放在自定义 fixture 的 teardown：断言失败也会执行，残留会把虚拟化
 *   树撑长，新节点落到折叠线以下导致后续断言 element(s) not found
 * - 交互后的状态变化用 waitForResponse 等待真实响应，不做盲等
 */

// 每个用例一个专属夹具目录：setup 建、teardown 删（失败也执行）。
// track() 用于登记“用例中被改名”的夹具新名字。
const test = base.extend<{
  fxFolder: { create: (tag: string) => Promise<string>; track: (name: string) => void }
}>({
  fxFolder: async ({ page }, provide) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    const created: string[] = []
    // 参数名避开 use：react-hooks/rules-of-hooks 会把 use 误判为 React hook
    provide({
      async create(tag: string) {
        const name = `fx-${tag}-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`
        await apiCreateLatexFile(page, projectId, `${name}/keep.tex`, 'x')
        created.push(name)
        return name
      },
      track(name: string) {
        created.push(name)
      },
    })
    for (const name of created) {
      await apiDeleteLatexFolder(page, projectId, name).catch(() => {})
    }
  },
})

test.describe('Folder operations (real backend)', () => {
  // Radix 菜单/树都有入场动画：CI 慢机上菜单项会长时间处于 unstable，
  // 等待稳定期间元素被重建导致 click 超时。灭动画让交互可判定。
  test.beforeEach(async ({ page }) => {
    await page.emulateMedia({ reducedMotion: 'reduce' })
  })

  test('renders the seeded file tree', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    expect(projectId).not.toBe('')
    await loginAs(page)
    await page.goto(`/project/${projectId}/writing`)
    // Seed files should appear
    await expect(page.getByText('main.tex').first()).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText('refs.bib').first()).toBeVisible({ timeout: 15_000 })
  })

  test('tree search filters the file list', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    await loginAs(page)
    await page.goto(`/project/${projectId}/writing`)
    const searchInput = page.getByTestId('file-tree-search-input')
    await expect(searchInput).toBeVisible({ timeout: 15_000 })
    await searchInput.fill('refs')
    // refs.bib should remain; main.tex should be hidden
    await expect(page.getByText('refs.bib').first()).toBeVisible()
  })

  test('clicking a file makes it the active one', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    await loginAs(page)
    await page.goto(`/project/${projectId}/writing`)
    const mainFile = page
      .getByTestId('file-node')
      .filter({ hasText: 'main.tex' })
      .first()
    await expect(mainFile).toBeVisible({ timeout: 15_000 })
    await mainFile.click()
    await expect(mainFile).toHaveAttribute('data-active', 'true', { timeout: 5_000 })
  })

  test('creates a new folder via header button', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    await loginAs(page)
    await page.goto(`/project/${projectId}/writing`)
    await expect(page.getByTestId('file-node').first()).toBeVisible({ timeout: 15_000 })

    // 提交即触发 POST（创建 folder/.gitkeep 占位文件）——等待真实响应
    const createResponse = page.waitForResponse(
      (r) => r.url().includes('/latex-files/') && r.request().method() === 'POST',
    )

    // Click "New Folder" button
    await page.getByTitle('新建文件夹').click()

    // The tree opens its inline creation form with a folder name field.
    const folderNameInput = page.getByLabel('文件夹名称')
    await expect(folderNameInput).toBeVisible({ timeout: 8_000 })

    const folderName = await newFixtureName('create')
    await folderNameInput.fill(folderName)
    await page.getByRole('button', { name: '确认创建' }).click()

    const resp = await createResponse
    expect(resp.status(), `创建响应体: ${await resp.text()}`).toBe(200)

    // The folder should appear in the tree（创建成功后 query 失效重取）
    await expect(
      page.getByTestId('folder-node').filter({ hasText: folderName }).first(),
    ).toBeVisible({ timeout: 10_000 })

    await apiDeleteLatexFolder(page, projectId, folderName)
  })

  test('renames a folder via context menu', async ({ page, fxFolder }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    await loginAs(page)

    // 夹具先于导航创建：页面首次拉取即包含它，树一次成型无二次重渲染
    //（重渲染会把右键弹出的菜单顶掉）
    const fxRename = await fxFolder.create('rename')

    await page.goto(`/project/${projectId}/writing`)
    await expect(page.getByTestId('file-node').first()).toBeVisible({ timeout: 15_000 })
    const folder = page.getByTestId('folder-node').filter({ hasText: fxRename }).first()
    await expect(folder).toBeVisible({ timeout: 10_000 })

    await openFolderMenu(page, folder, '重命名')

    const renameInput = page.getByTestId('inline-rename-input')
    await expect(renameInput).toBeVisible({ timeout: 8_000 })
    const fxRenamed = await newFixtureName('renamed')
    fxFolder.track(fxRenamed)
    await renameInput.fill(fxRenamed)
    // 响应等待在「触发请求的动作」前武装即可——提前武装会被
    // openFolderMenu 内部的菜单重试（右键+3s×N）耗尽 10s 窗口
    const renameResponse = page.waitForResponse(
      (r) => r.url().includes('/folder/rename') && r.request().method() === 'POST',
    )
    await renameInput.press('Enter')

    const resp = await renameResponse
    expect(resp.status(), `重命名响应体: ${await resp.text()}`).toBe(200)

    await expect(
      page.getByTestId('folder-node').filter({ hasText: fxRenamed }).first(),
    ).toBeVisible({ timeout: 10_000 })
  })

  test('deletes a folder via context menu + confirm dialog', async ({ page, fxFolder }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    await loginAs(page)

    const fxDelete = await fxFolder.create('delete')

    await page.goto(`/project/${projectId}/writing`)
    await expect(page.getByTestId('file-node').first()).toBeVisible({ timeout: 15_000 })
    const folder = page.getByTestId('folder-node').filter({ hasText: fxDelete }).first()
    await expect(folder).toBeVisible({ timeout: 10_000 })

    await openFolderMenu(page, folder, '删除')

    // Confirm dialog；等待器在确认点击前武装（同理：菜单重试不吃窗口）
    const confirmButton = page.getByRole('button', { name: '删除' })
    await expect(confirmButton).toBeVisible({ timeout: 3_000 })
    const deleteResponse = page.waitForResponse(
      (r) => r.url().includes('/latex-files/folder') && r.request().method() === 'DELETE',
    )
    await confirmButton.click()

    const resp = await deleteResponse
    expect(resp.status(), `删除响应体: ${await resp.text()}`).toBe(200)

    // The folder should be removed from the tree
    await expect(folder).not.toBeVisible({ timeout: 10_000 })
  })
})

async function newFixtureName(tag: string): Promise<string> {
  return `e2e-${tag}-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`
}

/**
 * 右键文件夹并等待指定菜单项出现。Radix 菜单偶发被随后的树重渲染关掉
 * （CI 慢机上可复现：菜单项 not stable → detached → 永不回来），
 * 整体重试右键而非等待，重开后菜单稳定。
 */
/**
 * 右键打开文件夹菜单并点击指定项（原子操作）。
 * 点击包含在重试环内：慢 CI 上菜单可能在「可见」与「点击」之间
 * 被树重渲染顶掉——单独返回 locator 再点会吃到 stale 菜单超时。
 */
async function openFolderMenu(page: import('@playwright/test').Page, folder: import('@playwright/test').Locator, itemLabel: string) {
  let lastError: unknown
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      await folder.click({ button: 'right' })
      const item = page.getByRole('menuitem', { name: itemLabel })
      await item.waitFor({ state: 'visible', timeout: 3_000 })
      await item.click()
      return
    } catch (e) {
      lastError = e
      // 菜单可能没弹出/被关掉：按 Escape 确保干净后重试
      await page.keyboard.press('Escape')
    }
  }
  throw lastError
}
