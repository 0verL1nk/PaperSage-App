import { test, expect } from '@playwright/test'

/**
 * SmoothHeight 裁剪回归（2026-09 真实事故）：
 * 提交按钮是 inline-flex，挂在行框基线下沿会溢出行框 ~31px；内层块盒的
 * offset/scrollHeight 都不含这部分，测得的显式 height 偏小，overflow-hidden
 * 把按钮腰斩。toBeVisible 查不出这种裁剪（元素在 DOM 里"可见"），必须
 * 像素级比较按钮底边与容器底边。
 */
test('register submit button not clipped by smooth height container', async ({ page }) => {
  // 669px：事故报告的视口宽度
  await page.setViewportSize({ width: 669, height: 900 })
  await page.goto('/auth')
  await page.getByRole('tab', { name: '注册新账号' }).waitFor({ timeout: 15_000 })

  // 快速来回切换，制造"动画中途测量"的最坏时序
  for (let i = 0; i < 2; i++) {
    await page.getByRole('tab', { name: '注册新账号' }).click()
    await page.waitForTimeout(100)
    await page.getByRole('tab', { name: '账号登录' }).click()
    await page.waitForTimeout(100)
  }
  await page.getByRole('tab', { name: '注册新账号' }).click()
  await page.waitForTimeout(1_000)

  const clipPx = await page.evaluate(() => {
    const smooth = document.querySelector('[data-testid="smooth-height"]') as HTMLDivElement | null
    const btn = [...document.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('创建我的账号'),
    )
    if (!smooth || !btn) return null
    return Math.round(btn.getBoundingClientRect().bottom - smooth.getBoundingClientRect().bottom)
  })
  expect(clipPx).not.toBeNull()
  expect(clipPx as number).toBeLessThanOrEqual(0, '按钮底边不得低于容器底边（overflow-hidden 会裁掉）')
})

test('container height shrinks back when switching to the shorter tab', async ({ page }) => {
  await page.setViewportSize({ width: 669, height: 900 })
  await page.goto('/auth')
  await page.getByRole('tab', { name: '账号登录' }).waitFor({ timeout: 15_000 })
  await page.waitForTimeout(800)
  const H = () =>
    page.evaluate(() => {
      const s = document.querySelector('[data-testid="smooth-height"]') as HTMLDivElement | null
      return s ? parseFloat(s.style.height) : -1
    })

  const login = await H()
  await page.getByRole('tab', { name: '注册新账号' }).click()
  await page.waitForTimeout(900)
  const reg = await H()
  await page.getByRole('tab', { name: '账号登录' }).click()
  await page.waitForTimeout(900)
  const back = await H()

  // scrollHeight 恒 ≥ 被钉住的 clientHeight：不「解钉测量」会把高度焊死在
  // 历史最大值（只涨不缩事故）
  expect(reg).toBeGreaterThan(login)
  expect(Math.abs(back - login)).toBeLessThanOrEqual(1, '切回登录 tab 应缩回原高度')
})
