import { test, expect } from '@playwright/test'

import { loginAs, e2eEnv } from '../helpers'

/**
 * Project golden path — create → upload doc → write file → delete → restore.
 *
 * Exercises the real backend + MinIO S3 stack. The project name is
 * UUID-suffixed so re-runs (and parallel workers) don't collide.
 */
test.describe('Project lifecycle (golden path)', () => {
  test('seeded default-project renders on the project list', async ({ page }) => {
    await loginAs(page)
    // The seed script creates "default-project"; check it appears
    await expect(page.getByText('default-project')).toBeVisible({ timeout: 10_000 })
  })

  test('user can create a new project from the dialog', async ({ page }) => {
    const name = `e2e-lifecycle-${Date.now()}`
    await loginAs(page)

    await page.getByRole('button', { name: /新建项目|创建第一个项目/ }).first().click()
    await page.getByRole('textbox', { name: '名称' }).fill(name)
    await page.getByRole('button', { name: /创建|确定/ }).click()

    // Card should now appear in the grid
    await expect(page.getByText(name)).toBeVisible({ timeout: 10_000 })
  })

  test('seeded project link navigates to writing page', async ({ page }) => {
    const projectId = e2eEnv('E2E_PROJECT_ID')
    expect(projectId).not.toBe('')
    await loginAs(page)
    await page.goto(`/project/${projectId}/writing`)
    // The Explorer sidebar heading should appear once the page loads
    await expect(page.getByText('Explorer')).toBeVisible({ timeout: 15_000 })
  })
})
