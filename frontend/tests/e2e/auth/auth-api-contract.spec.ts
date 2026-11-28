import { createHash } from 'node:crypto'

import { test, expect } from '@playwright/test'

import { e2eEnv } from '../helpers'

/**
 * Auth API 契约（经页面上下文发起真实 HTTP，覆盖 axios 之外的裸协议行为）：
 * refresh 轮换 + 15s 宽限（多标签页并发不误踢）+ 窗口外重放整族吊销。
 */
test.describe('Auth API contract (real backend)', () => {
  test('refresh rotation survives concurrent tabs and revokes the family on replay', async ({ page }) => {
    test.slow() // 需要真实跨过 15s 宽限窗口
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

    // 2) 第一次 refresh：成功并轮换（JWE 随机 IV，新旧串必然不同）
    const r1 = await refreshOnce(refresh1)
    expect(r1.status).toBe(200)
    const refresh2 = r1.body.data.refreshToken
    expect(refresh2).toBeTruthy()
    expect(refresh2).not.toBe(refresh1)

    // 3) 宽限窗口内复用旧 token（模拟第二个标签页同时刷新）：平滑续签，不能误踢
    const grace = await refreshOnce(refresh1)
    expect(grace.status).toBe(200)
    expect(grace.body.data.refreshToken).toBeTruthy()

    // 4) 同族的新 refresh 仍有效
    const r3 = await refreshOnce(refresh2)
    expect(r3.status).toBe(200)
    const latest = r3.body.data.refreshToken
    expect(latest).toBeTruthy()

    // 5) 超出宽限窗口后重放已轮换的旧 token：401，并吊销整条 family
    await page.waitForTimeout(16_000)
    const replay = await refreshOnce(refresh1)
    expect(replay.status).toBe(401)
    expect(replay.body.info).toBe('Refresh token has been revoked')

    // 6) family 内最新 token 也已被吊销，其他登录会话不受影响由单测覆盖
    const afterReplay = await refreshOnce(latest)
    expect(afterReplay.status).toBe(401)
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
