import { expect, test } from '@playwright/test'
import { TEST_PROJECT_ID, mockWritingPageAPIs, seedAuthTokens } from '../helpers'

/**
 * 科研组织产物：准备页采用章节计划 → 撰写侧栏读取已采用版本。
 * 全部走 mock，验证前端状态机与接口契约，不代表真实模型验收。
 */

const PLAN = {
  id: 'a1', projectId: TEST_PROJECT_ID, kind: 'chapter_plan', state: 'active',
  currentRevision: 1, adoptedRevision: null as number | null, createdAt: '', updatedAt: '',
  current: {
    id: 'r1', revision: 1, status: 'candidate', origin: 'agent', createdAt: '',
    payload: { sections: [{ id: 's1', title: '引言', responsibility: '提出研究问题', order: 1 }] },
  },
}

test('采用章节计划后，撰写侧栏只显示已采用版本', async ({ page }) => {
  await page.goto('/')
  await seedAuthTokens(page)
  await mockWritingPageAPIs(page)

  let adopted = false

  await page.route('**/api/v1/projects/*/research-artifacts**', (route) => {
    const artifact = adopted
      ? {
        ...PLAN, adoptedRevision: 1,
        current: { ...PLAN.current, status: 'adopted' },
        adopted: { ...PLAN.current, status: 'adopted' },
      }
      : PLAN
    return route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: [artifact], info: '' }),
    })
  })

  await page.route('**/api/v1/research-artifacts/*/adopt', (route) => {
    adopted = true
    return route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        code: 200, info: '',
        data: { ...PLAN, adoptedRevision: 1, current: { ...PLAN.current, status: 'adopted' } },
      }),
    })
  })

  await page.goto(`/project/${TEST_PROJECT_ID}/prepare`)
  await page.getByText('章节计划（1 节）').click()
  await page.getByRole('button', { name: '采用' }).click()
  await expect(page.getByText(/上次更新：/)).toBeVisible()
  await expect(page.getByText('已采用', { exact: true })).toBeVisible()

  await page.goto(`/project/${TEST_PROJECT_ID}/writing`)
  await page.getByTestId('writing-tab-ai').click()
  await expect(page.getByText('已采用的写作计划')).toBeVisible()
  await expect(page.getByText(/引言/)).toBeVisible()
})
