import { createHash } from 'node:crypto'

import { test, expect } from '@playwright/test'

import { e2eEnv } from '../helpers'

/**
 * Auth API 契约（经页面上下文发起真实 HTTP，覆盖 axios 之外的裸协议行为）：
 * refresh 轮换语义——新 token 对签发后旧 refresh 立即失效。
 */
test.describe('Auth API contract (real backend)', () => {
  test('refresh rotation revokes the old token', async ({ page }) => {
    const user = e2eEnv('E2E_USER', 'e2euser')
    const pass = e2eEnv('E2E_PASS', 'e2epass1')

    // 1) 登录拿 token 对（先导航拿到 vite 同源 origin，/api 走代理）
    await page.goto('/')
    const base = new URL(page.url()).origin
    const sha = createHash('sha256').update(pass).digest('hex')
    const login = await page.evaluate(
      async ({ base, user, sha }) => {
        const resp = await fetch(`${base}/api/v1/auth/login`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body: new URLSearchParams({ account: user, password: sha }),
        })
        return { status: resp.status, body: await resp.json() }
      },
      { base, user, sha },
    )
    expect(login.status).toBe(200)
    const refresh1 = login.body.data.refreshToken
    expect(refresh1).toBeTruthy()

    const refreshOnce = async (token: string) =>
      page.evaluate(
        async ({ base, token }) => {
          const resp = await fetch(`${base}/api/v1/auth/refresh`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ refresh_token: token }),
          })
          return { status: resp.status, body: await resp.json() }
        },
        { base, token },
      )

    // 2) 第一次 refresh：成功，拿到新对。
    // JWT exp 秒级 + HS256 确定性：同一秒内签发的 refresh 字符串相同，
    // 轮换会退化成 no-op（撤销后同名 key 重新写入）。跨过秒边界再刷新，
    // 新旧 token 必然不同，轮换语义才可判定。
    await page.waitForTimeout(1100)
    const r1 = await refreshOnce(refresh1)
    expect(r1.status).toBe(200)
    const refresh2 = r1.body.data.refreshToken
    expect(refresh2).toBeTruthy()
    // 注：exp 秒级 + HS256 确定性，同一秒内 refresh 可能返回相同串
    //（Python 实现同样如此），不比较字符串不等

    // 3) 旧 refresh 再用：应被拒（真轮换语义，旧 token 用后即废）
    const r2 = await refreshOnce(refresh1)
    expect(r2.status).toBe(401)

    // 4) 新 refresh 仍有效
    const r3 = await refreshOnce(refresh2)
    expect(r3.status).toBe(200)
  })

  test('wrong password returns 401 envelope without redirect', async ({ page }) => {
    await page.goto('/')
    const base = new URL(page.url()).origin
    const user = e2eEnv('E2E_USER', 'e2euser')
    const sha = createHash('sha256').update('definitely-wrong-password').digest('hex')
    const result = await page.evaluate(
      async ({ base, user, sha }) => {
        const resp = await fetch(`${base}/api/v1/auth/login`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body: new URLSearchParams({ account: user, password: sha }),
        })
        return { status: resp.status, body: await resp.json() }
      },
      { base, user, sha },
    )
    expect(result.status).toBe(401)
    expect(result.body.info).toBe('Invalid credentials')
    expect(result.body.code).toBe(401)
  })
})
