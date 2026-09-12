import { useUiStore } from '@/store'

/**
 * 全局加载指示（业界通行形态，参考 nprogress/GitHub/YouTube）：
 * - 顶部 2px 细进度条，pointer-events-none，不遮挡、不打断任何交互
 * - 延迟显示由纯 CSS 实现（animation-delay:250ms + fill-mode:backwards）：
 *   250ms 内完成的请求从未被绘制，天然防闪烁；组件零状态
 * - 只跟随 GET 类页面数据加载；写操作（POST/PUT/…）默认静默，
 *   由按钮 spinner / 行内状态提供就地反馈
 * - store 侧是并发安全的计数器（loadingCount），非布尔
 */
export function TopProgressBar() {
  const loadingCount = useUiStore((state) => state.loadingCount)

  if (loadingCount === 0) {
    return null
  }

  return (
    <div
      role="status"
      aria-label="页面加载中"
      className="pointer-events-none fixed inset-x-0 top-0 z-[60] h-0.5 overflow-hidden progress-appear"
    >
      <div className="h-full w-1/3 rounded-full bg-primary/80 progress-slide" />
    </div>
  )
}
