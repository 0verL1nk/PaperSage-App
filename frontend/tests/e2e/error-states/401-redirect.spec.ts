import { test, expect } from '@playwright/test'

import { ACCESS_TOKEN_KEY, REFRESH_TOKEN_KEY, e2eEnv } from '../helpers'

/**
 * 401 redirect — verify the axios interceptor bounces the user
 * back to /auth when the API returns 401. We seed a deliberately
 * invalid access token so the very first authenticated call fails.
 */
test.describe('401 redirect', () => {
  test('invalid token causes the user to be sent back to /auth', async ({ page }) => {
    // Seed garbage tokens before navigating so the first protected
    // call already has them in place.
    await page.goto('/')
    await page.evaluate(
      ([access, refresh]) => {
        window.localStorage.setItem(access, 'definitely-not-a-real-jwt')
        window.localStorage.setItem(refresh, 'definitely-not-a-real-jwt-either')
      },
      [ACCESS_TOKEN_KEY, REFRESH_TOKEN_KEY],
    )

    // Navigate to a protected route — the axios interceptor should
    // catch the 401 from /auth/me and redirect to /auth.
    await page.goto('/app/projects')
    await expect(page).toHaveURL(/\/auth/, { timeout: 10_000 })

    // Sanity: env helper reads
    expect(e2eEnv('E2E_USER', 'fallback')).toBeTruthy()
  })
})
