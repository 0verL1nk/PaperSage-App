import { useEffect, useRef, useState, type ReactNode } from 'react'

import { cn } from '@/lib/utils'

/**
 * 平滑高度容器：内容高度变化时（tab 切换、条件渲染、表单校验信息
 * 展开等）容器高度以 CSS transition 过渡，而非瞬间跳变。
 *
 * 原理：ResizeObserver 观察「内层自然高度元素」（外层高度被钉死后
 * 自身盒子不再变化，观察外层永不触发），把测得高度写入外层显式
 * height 交给 transition 插值。
 *
 * 测量必须「解钉」后读：overflow-hidden 滚动容器的 scrollHeight 恒 ≥
 * 自身 clientHeight（被钉住的旧高度），直接读会把高度焊死在历史最大值、
 * 只涨不缩；而内层块盒的 offset/scrollHeight 又不含 inline-level 子元素
 * （如 inline-flex 按钮）挂在行框基线下的溢出。因此测量时临时把外层
 * height 置 auto——此时 scrollHeight = max(auto 高度, 行内溢出) 才是
 * 双向都正确的真实内容高度（同步改回，浏览器绘制前不会呈现中间态）。
 *
 * 复测兜底：RO 可能在内容入场动画/布局半场时触发，读到偏小值，
 * 因此每次触发后 400ms 再复测一次。
 */
export function SmoothHeight({
  children,
  className,
  duration = 300,
}: {
  children: ReactNode
  className?: string
  duration?: number
}) {
  const outerRef = useRef<HTMLDivElement>(null)
  const innerRef = useRef<HTMLDivElement>(null)
  const [height, setHeight] = useState<number | undefined>(undefined)

  useEffect(() => {
    const outer = outerRef.current
    const inner = innerRef.current
    if (!outer || !inner) return
    const measure = () => {
      const prev = outer.style.height
      outer.style.height = 'auto'
      const content = Math.max(outer.offsetHeight, outer.scrollHeight)
      outer.style.height = prev
      setHeight(content)
    }
    measure()
    // 环境无 ResizeObserver（如部分测试运行时）时降级：仅初始高度
    if (typeof ResizeObserver === 'undefined') return

    let remeasureTimer: ReturnType<typeof setTimeout> | undefined
    const ro = new ResizeObserver(() => {
      measure()
      clearTimeout(remeasureTimer)
      remeasureTimer = setTimeout(measure, 400)
    })
    ro.observe(inner)
    return () => {
      ro.disconnect()
      clearTimeout(remeasureTimer)
    }
  }, [])

  return (
    <div
      ref={outerRef}
      style={
        height === undefined
          ? undefined
          : { height, transitionDuration: `${duration}ms` }
      }
      className={cn('overflow-hidden transition-[height] ease-out', className)}
      data-testid="smooth-height"
    >
      <div ref={innerRef}>{children}</div>
    </div>
  )
}
