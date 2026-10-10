import type { Page } from '@playwright/test'

/** Add short-lived Vercel access credentials only to one explicitly configured origin. */
export async function scopeVercelAccessToOrigin(page: Page, targetUrl: string | undefined): Promise<void> {
  const headers = {
    ...(process.env.VERCEL_OIDC_TOKEN
      ? { 'x-vercel-trusted-oidc-idp-token': process.env.VERCEL_OIDC_TOKEN }
      : {}),
    ...(process.env.VERCEL_AUTOMATION_BYPASS_SECRET
      ? { 'x-vercel-protection-bypass': process.env.VERCEL_AUTOMATION_BYPASS_SECRET }
      : {}),
  }
  if (!targetUrl || Object.keys(headers).length === 0) return
  const trustedOrigin = new URL(targetUrl).origin
  await page.context().route('**/*', async (route) => {
    if (new URL(route.request().url()).origin !== trustedOrigin) {
      await route.continue()
      return
    }
    await route.continue({ headers: { ...route.request().headers(), ...headers } })
  })
}
