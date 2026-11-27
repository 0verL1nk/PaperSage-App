import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

import { logout } from '../helpers'

/**
 * 邮箱验证码登录（真实后端 + fakesmtp 收信）：
 * 发码 → 从 fakesmtp 取回 6 位码 → 登录并自动注册 → 一次性核销；
 * 再走「设置密码 → 退出 → 密码登录」闭环。
 */
const API_BASE = process.env.VITE_API_BASE_URL ?? 'http://localhost:5478'
const FAKE_SMTP = process.env.E2E_FAKE_SMTP_URL ?? 'http://localhost:2526'

async function waitForLoginCode(request: APIRequestContext, email: string): Promise<string> {
  let code = ''
  await expect
    .poll(async () => {
      const resp = await request.get(`${FAKE_SMTP}/messages?to=${encodeURIComponent(email)}`)
      if (!resp.ok()) return ''
      const body = (await resp.json()) as { messages?: Array<{ html?: string }> }
      const match = body.messages?.[0]?.html?.match(/>(\d{6})</)
      code = match?.[1] ?? ''
      return code
    }, { timeout: 15_000, message: 'fakesmtp 应收到验证码邮件' })
    .not.toBe('')
  return code
}

async function loginWithCode(page: Page, request: APIRequestContext, email: string): Promise<void> {
  await page.goto('/auth')
  await expect(page.getByRole('button', { name: '验证码登录' })).toHaveAttribute('aria-pressed', 'true')
  await page.getByLabel('邮箱', { exact: true }).fill(email)
  await page.getByRole('button', { name: '获取验证码' }).click()
  await expect(page.getByRole('button', { name: /重新发送（\d+s）/ })).toBeVisible()

  const code = await waitForLoginCode(request, email)
  await page.getByLabel('验证码', { exact: true }).fill(code)
  await page.getByRole('button', { name: '登录 / 自动注册' }).click()
  await expect(page).toHaveURL(/\/app\/projects/, { timeout: 15_000 })
}

test.describe('Email code login (real backend)', () => {
  test.use({ storageState: { cookies: [], origins: [] } })

  test('new email logs in, auto-registers and the code cannot be replayed', async ({ page, request }) => {
    const email = `e2e_code_${Date.now()}@example.edu.cn`
    await loginWithCode(page, request, email)

    // 一次性核销：同一验证码重放必须 401。
    const messages = await request.get(`${FAKE_SMTP}/messages?to=${encodeURIComponent(email)}`)
    const body = (await messages.json()) as { messages?: Array<{ html?: string }> }
    const code = body.messages?.[0]?.html?.match(/>(\d{6})</)?.[1]
    expect(code).toBeTruthy()
    const replay = await request.post(`${API_BASE}/api/v1/auth/email-code/verify`, {
      data: { email, code },
    })
    expect(replay.status()).toBe(401)
  })

  // B1 回归：错码必须留在当前页、保留邮箱、给出中文错误（不能整页刷新）。
  test('wrong code keeps the form and shows a Chinese error', async ({ page }) => {
    const email = `e2e_wrong_${Date.now()}@example.edu.cn`
    await page.goto('/auth')
    await page.getByLabel('邮箱', { exact: true }).fill(email)
    await page.getByRole('button', { name: '获取验证码' }).click()
    await expect(page.getByRole('button', { name: /重新发送（\d+s）/ })).toBeVisible()

    await page.getByLabel('验证码', { exact: true }).fill('000000')
    await page.getByRole('button', { name: '登录 / 自动注册' }).click()

    await expect(page.getByText(/验证码不正确/)).toBeVisible({ timeout: 10_000 })
    await expect(page).toHaveURL(/\/auth/)
    await expect(page.getByLabel('邮箱', { exact: true })).toHaveValue(email)
  })

  test('code-registered user can set a password and then sign in with it', async ({ page, request }) => {
    const email = `e2e_setpw_${Date.now()}@example.edu.cn`
    const password = 'NewPass123'
    await loginWithCode(page, request, email)

    await page.goto('/app/settings')
    await page.getByRole('tab', { name: '账号与安全' }).click()
    await expect(page.getByText('设置登录密码')).toBeVisible()
    await page.getByLabel('新密码', { exact: true }).fill(password)
    await page.getByLabel('确认新密码', { exact: true }).fill(password)
    await page.getByRole('button', { name: '设置密码' }).click()
    await expect(page.getByText('密码已保存')).toBeVisible({ timeout: 10_000 })

    await logout(page)
    await expect(page).toHaveURL(/\/auth/)
    await page.getByRole('button', { name: '密码登录' }).click()
    await page.getByPlaceholder('请输入您的账号').fill(email)
    await page.getByPlaceholder('••••••••').fill(password)
    await page.getByRole('button', { name: '立即登录' }).click()
    await expect(page).toHaveURL(/\/app\/projects/, { timeout: 15_000 })
  })
})
