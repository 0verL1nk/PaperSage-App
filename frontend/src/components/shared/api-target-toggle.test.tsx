import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiTargetToggle } from '@/components/shared/api-target-toggle'

const KEY = 'papersage_api_target'
const reload = vi.fn()

beforeEach(() => {
  localStorage.clear()
  reload.mockClear()
  // jsdom 的 location.reload 会抛 "Not implemented"，替换掉才能断言它被调用
  Object.defineProperty(window, 'location', {
    configurable: true,
    writable: true,
    value: { ...window.location, reload },
  })
})

describe('ApiTargetToggle', () => {
  it('默认在线，切到本地后写入选择并刷新页面', async () => {
    const user = userEvent.setup()
    render(<ApiTargetToggle />)

    expect(screen.getByTitle(/在线/)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /切换接口环境/ }))
    await user.click(await screen.findByText('本地'))

    expect(localStorage.getItem(KEY)).toBe('local')
    // 必须刷新：访问令牌与 React Query 缓存都绑在原来的后端上
    expect(reload).toHaveBeenCalledTimes(1)
  })

  it('已经选中本地时不再重复切换，避免无谓刷新', async () => {
    localStorage.setItem(KEY, 'local')
    const user = userEvent.setup()
    render(<ApiTargetToggle />)

    expect(screen.getByTitle(/本地/)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /切换接口环境/ }))
    await user.click(await screen.findByText('本地'))

    expect(reload).not.toHaveBeenCalled()
  })
})
