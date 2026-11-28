import { test, expect, type Page } from '@playwright/test'
import { mockWritingPageAPIs, seedAuthTokens, WRITING_URL } from '../helpers'

/** 捕获自动保存的 PUT 内容；其余请求交回 mockWritingPageAPIs。 */
async function captureSaves(page: Page): Promise<() => string> {
  let saved = ''
  await page.route('**/api/v1/projects/*/latex-files/**', async (route) => {
    if (route.request().method() === 'PUT') {
      saved = (route.request().postDataJSON() as { content?: string })?.content ?? ''
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, data: null, info: '' }),
      })
      return
    }
    await route.fallback()
  })
  return () => saved
}

async function openExperimentDialog(page: Page): Promise<void> {
  await expect(page.getByTestId('file-node').filter({ hasText: 'main.tex' })).toBeVisible({ timeout: 15_000 })
  await page.getByTestId('file-node').filter({ hasText: 'main.tex' }).click()
  const button = page.getByRole('button', { name: '实验数据表格与分析' })
  await expect(button).toBeEnabled({ timeout: 10_000 })
  await button.click()
  await expect(page.getByRole('dialog')).toBeVisible()
}

async function pasteAndParse(page: Page): Promise<void> {
  await page.getByPlaceholder(/方法,准确率,F1/).fill('方法\t准确率\tF1\nBaseline\t88.1\t0.842\nOurs\t92.5\t0.881')
  await page.getByRole('button', { name: '解析数据' }).click()
  await expect(page.getByRole('cell', { name: 'Ours' })).toBeVisible()
}

test.describe('实验数据表格与分析', () => {
  test.beforeEach(async ({ page }) => {
    await seedAuthTokens(page)
    await mockWritingPageAPIs(page)
  })

  test('解析粘贴的 TSV，生成 booktabs 并插入自动保存', async ({ page }) => {
    const saved = await captureSaves(page)
    await page.goto(WRITING_URL)
    await openExperimentDialog(page)
    await pasteAndParse(page)

    await expect(page.getByText('LaTeX 预览')).toBeVisible()
    await page.getByRole('button', { name: '插入到编辑器' }).click()

    await expect.poll(() => saved(), { timeout: 8_000 }).toContain('\\toprule')
    expect(saved()).toContain('Baseline & 88.1 \\\\')
    expect(saved()).toContain('\\textbf{92.5}')
  })

  test('有据分析展示来源 chip 并可编辑后插入', async ({ page }) => {
    // 覆盖默认的「未配置模型」设置，让分析面板可用。
    await page.route(/\/api\/v1\/settings\/?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          data: {
            settings: {
              ui: {}, editor: {},
              agent: {
                active_provider_id: 'p',
                providers: [{
                  id: 'p', name: 'Provider', type: 'openai_compatible',
                  base_url: 'https://example.invalid', api_key: 'secret', models: ['m'], active_model: 'm',
                }],
              },
            },
          },
        }),
      })
    })
    await page.route('**/api/v1/projects/*/agent/experiment-analysis', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          data: {
            paragraphs: [{
              text: '本文方法准确率达到 92.5%，较基线提升明显。',
              citations: [{ row: 1, column: 1, value: '92.5', rowLabel: 'Ours', columnLabel: '准确率' }],
            }],
            normRefs: [],
            context: { normsCount: 0, planStatus: 'missing', warnings: ['项目还没有可用的章节计划，分析未对齐章节职责'] },
            model: { name: 'Provider', source: 'user-settings' },
          },
          info: '',
        }),
      })
    })
    const saved = await captureSaves(page)

    await page.goto(WRITING_URL)
    await openExperimentDialog(page)
    await pasteAndParse(page)

    await page.getByRole('tab', { name: '结果分析' }).click()
    await page.getByRole('button', { name: /基于这张表生成分析草稿/ }).click()

    await expect(page.getByText('Ours · 准确率 = 92.5')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('项目还没有可用的章节计划，分析未对齐章节职责')).toBeVisible()

    await page.getByLabel('第 1 段（可编辑）').fill('本文方法准确率达到 92.5%，提升明显。')
    await page.getByRole('button', { name: '插入到编辑器' }).click()

    await expect.poll(() => saved(), { timeout: 8_000 }).toContain('92.5\\%')
  })

  test('未配置模型时分析显式提示，表格仍可插入', async ({ page }) => {
    await page.goto(WRITING_URL)
    await openExperimentDialog(page)
    await pasteAndParse(page)

    await page.getByRole('tab', { name: '结果分析' }).click()
    await expect(page.getByText('尚未配置 AI 模型')).toBeVisible()
    await expect(page.getByRole('button', { name: '去设置' })).toBeVisible()

    await page.getByRole('tab', { name: '表格生成' }).click()
    await expect(page.getByText('LaTeX 预览')).toBeVisible()
  })
})
