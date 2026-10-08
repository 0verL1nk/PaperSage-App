import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'
import { clearSessionTokens, resetGlobalLoading } from '@/store'

afterEach(() => {
  cleanup()
  clearSessionTokens()
  resetGlobalLoading()
  window.localStorage.clear()
})

// jsdom 不实现 ResizeObserver；SmoothHeight 等组件依赖它。
// stub：observe 时异步回调一次（带目标元素），内容不变则不再触发。
class ResizeObserverStub implements ResizeObserver {
  private targets = new Set<Element>()
  private callback: ResizeObserverCallback
  constructor(cb: ResizeObserverCallback) { this.callback = cb }
  observe(target: Element) {
    this.targets.add(target)
    queueMicrotask(() =>
      this.callback(
        [{ target } as ResizeObserverEntry],
        this as unknown as ResizeObserver,
      ),
    )
  }
  unobserve(target: Element) { this.targets.delete(target) }
  disconnect() { this.targets.clear() }
}
globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver

// Radix Select uses pointer capture for mouse selection; jsdom does not implement it.
if (!Element.prototype.hasPointerCapture) {
  Element.prototype.hasPointerCapture = () => false
  Element.prototype.setPointerCapture = () => undefined
  Element.prototype.releasePointerCapture = () => undefined
}
if (!Element.prototype.scrollIntoView) Element.prototype.scrollIntoView = () => undefined
