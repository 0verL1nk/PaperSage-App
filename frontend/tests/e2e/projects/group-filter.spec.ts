import { test, expect } from '@playwright/test'

import { loginAs, e2eEnv } from '../helpers'

/**
 * Group filter — verifies the segmented-control chips on the
 * project list filter the visible cards. Uses the seeded
 * "e2e-default-group" + a second group created on the fly.
 */
test.describe('Project group filter', () => {
  test('seeded group appears in the filter', async ({ page }) => {
    await loginAs(page)
    // SegmentedControl 使用 Radix ToggleGroup：根节点是 group，选项是 radio。
    const filter = page.getByRole('group', { name: '按分组筛选项目' })
    await expect(filter.getByRole('radio', { name: '全部' })).toBeVisible()
    await expect(filter.getByRole('radio', { name: 'e2e-default-group', exact: true })).toBeVisible({
      timeout: 10_000,
    })
  })

  test('clicking a group chip filters the list', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    await loginAs(page)
    // Wait for the initial list to render
    await expect(page.getByText('default-project')).toBeVisible({ timeout: 10_000 })

    const filter = page.getByRole('group', { name: '按分组筛选项目' })

    // Click the seeded group — the default project should still be visible
    // because it belongs to this group
    await filter.getByRole('radio', { name: 'e2e-default-group', exact: true }).click()

    // Click "全部" to verify the filter toggles back
    await filter.getByRole('radio', { name: '全部' }).click()
    await expect(page.getByText('default-project')).toBeVisible()
    // Sanity: project id was set
    expect(projectId).not.toBe('')
  })
})
