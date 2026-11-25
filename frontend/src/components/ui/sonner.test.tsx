import { act, render, waitFor } from '@testing-library/react'
import { toast } from 'sonner'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Toaster } from './sonner'

/** 桩 matchMedia，返回可切换视口的触发器（模拟窗口跨断点）。 */
function stubViewport(isMobile: boolean) {
  const listeners = new Set<() => void>()
  let matches = isMobile
  vi.stubGlobal(
    'matchMedia',
    vi.fn((query: string) => ({
      get matches() {
        return matches
      },
      media: query,
      onchange: null,
      addEventListener: (_: string, listener: () => void) => listeners.add(listener),
      removeEventListener: (_: string, listener: () => void) => listeners.delete(listener),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  )
  return (next: boolean) => {
    matches = next
    act(() => listeners.forEach((listener) => listener()))
  }
}

function queryToaster(container: HTMLElement): HTMLElement {
  const toaster = container.querySelector('[data-sonner-toaster]')
  expect(toaster).not.toBeNull()
  return toaster as HTMLElement
}

async function renderToaster(): Promise<HTMLElement> {
  const view = render(<Toaster />)
  act(() => {
    toast.success('保存成功', { id: 'toast-position-test' })
  })
  // sonner 在 setTimeout 里落地 toast 状态，等它渲染出 toaster 容器
  return waitFor(() => queryToaster(view.container))
}

afterEach(() => {
  act(() => toast.dismiss())
  vi.unstubAllGlobals()
})

describe('Toaster 全局定位', () => {
  it('桌面端显示在右下角，四边留白 16px', async () => {
    stubViewport(false)
    const toaster = await renderToaster()

    expect(toaster).toHaveAttribute('data-y-position', 'bottom')
    expect(toaster).toHaveAttribute('data-x-position', 'right')
    expect(toaster.style.getPropertyValue('--offset-bottom')).toBe('16px')
    expect(toaster.style.getPropertyValue('--offset-right')).toBe('16px')
  })

  it('窄屏显示在顶部居中，移动端留白 16px', async () => {
    stubViewport(true)
    const toaster = await renderToaster()

    expect(toaster).toHaveAttribute('data-y-position', 'top')
    expect(toaster).toHaveAttribute('data-x-position', 'center')
    expect(toaster.style.getPropertyValue('--mobile-offset-top')).toBe('16px')
    expect(toaster.style.getPropertyValue('--mobile-offset-left')).toBe('16px')
  })

  it('运行环境没有 matchMedia 时按桌面端渲染', async () => {
    vi.stubGlobal('matchMedia', undefined)
    const toaster = await renderToaster()

    expect(toaster).toHaveAttribute('data-y-position', 'bottom')
    expect(toaster).toHaveAttribute('data-x-position', 'right')
  })

  it('视口跨过 640px 断点时切换定位', async () => {
    const setViewport = stubViewport(false)
    const view = render(<Toaster />)
    act(() => {
      toast.success('保存成功', { id: 'toast-position-test' })
    })
    await waitFor(() => {
      expect(queryToaster(view.container)).toHaveAttribute('data-x-position', 'right')
    })

    setViewport(true)

    await waitFor(() => {
      expect(queryToaster(view.container)).toHaveAttribute('data-x-position', 'center')
    })
  })
})
