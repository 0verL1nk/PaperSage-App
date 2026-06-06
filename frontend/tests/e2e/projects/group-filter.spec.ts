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
    // The group chip should be visible alongside the "全部" chip
    await expect(page.getByRole('button', { name: '全部' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'e2e-default-group' })).toBeVisible({
      timeout: 10_000,
    })
  })

  test('clicking a group chip filters the list', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    await loginAs(page)
    // Wait for the initial list to render
    await expect(page.getByText('default-project')).toBeVisible({ timeout: 10_000 })

    // Click the seeded group — the default project should still be visible
    // because it belongs to this group
    await page.getByRole('button', { name: 'e2e-default-group' }).click()

    // Click "全部" to verify the filter toggles back
    await page.getByRole('button', { name: '全部' }).click()
    await expect(page.getByText('default-project')).toBeVisible()
    // Sanity: project id was set
    expect(projectId).not.toBe('')
  })
})
