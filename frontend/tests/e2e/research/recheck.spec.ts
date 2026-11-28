import { expect, test } from '@playwright/test'
import { TEST_PROJECT_ID, mockWritingPageAPIs, seedAuthTokens } from '../helpers'

/**
 * 资料变更后的待复查：准备页展示受影响目标并能逐条处理。
 * 全部走 mock，验证前端状态机与接口契约，不代表真实后端验收。
 */

const MARK = {
  id: 'm1', projectId: TEST_PROJECT_ID,
  targetType: 'artifact_revision', targetId: 'r1', artifactId: 'a1', revision: 1,
  targetTitle: '章节计划（1 节）', targetDetail: '修订 v1', targetMissing: false,
  triggerKind: 'source_deleted', triggerLabel: '资料已删除',
  reason: '资料《论文.pdf》已删除', createdAt: '2026-11-26T00:00:00Z',
}

const PLAN = {
  id: 'a1', projectId: TEST_PROJECT_ID, kind: 'chapter_plan', state: 'active',
  currentRevision: 1, adoptedRevision: null,
  createdAt: '', updatedAt: '',
  needsRecheck: true, recheckReasons: ['资料《论文.pdf》已删除'],
  current: {
    id: 'r1', revision: 1, status: 'candidate', origin: 'agent', createdAt: '',
    needsRecheck: true, recheckReasons: ['资料《论文.pdf》已删除'],
    payload: { sections: [{ id: 's1', title: '引言', responsibility: '提出研究问题', order: 1 }] },
  },
}

test('删除资料后准备页展示待复查，处理后提醒消失', async ({ page }) => {
  await page.goto('/')
  await seedAuthTokens(page)
  await mockWritingPageAPIs(page)

  let resolved = false
  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/research-artifacts**`, (route) =>
    route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: [PLAN], info: '' }),
    }))
  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/recheck-marks?status=open`, (route) =>
    route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ code: 200, data: resolved ? [] : [MARK], info: '' }),
    }))
  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/recheck-marks?status=resolved`, (route) =>
    route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        code: 200, info: '',
        data: resolved ? [{ ...MARK, resolution: 'rechecked', resolvedAt: '2026-11-26T01:00:00Z' }] : [],
      }),
    }))
  await page.route(`**/api/v1/projects/${TEST_PROJECT_ID}/recheck-marks/*/resolve`, (route) => {
    resolved = true
    return route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ code: 200, info: '', data: { ...MARK, resolution: 'rechecked' } }),
    })
  })

  await page.goto(`/project/${TEST_PROJECT_ID}/prepare`)

  // 受影响目标、原因与「不自动改写」说明都要可见。
  await expect(page.getByText('资料《论文.pdf》已删除').first()).toBeVisible()
  await expect(page.getByText(/系统不会自动改写正文/)).toBeVisible()
  await expect(page.getByRole('button', { name: '已复查' })).toBeVisible()
  await expect(page.getByRole('button', { name: '已重建' })).toBeVisible()
  await expect(page.getByRole('button', { name: '忽略' })).toBeVisible()
  // 产物面板同步显示待复查徽标与原因。
  await expect(page.getByText('章节计划（1 节）').first()).toBeVisible()

  await page.getByRole('button', { name: '已复查' }).click()
  await expect(page.getByText('资料《论文.pdf》已删除')).toHaveCount(0)

  // 已处理标签仍能查到处理记录。
  await page.getByRole('tab', { name: '已处理' }).click()
  await expect(page.getByText('已复查').first()).toBeVisible()
})
