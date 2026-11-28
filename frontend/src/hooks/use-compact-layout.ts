import { useSyncExternalStore } from 'react'

/** 全站唯一移动断点：<1024px 走紧凑布局。 */
export const COMPACT_LAYOUT_MAX_WIDTH = 1024

function subscribe(onChange: () => void) {
  window.addEventListener('resize', onChange)
  return () => window.removeEventListener('resize', onChange)
}

function getSnapshot() {
  return window.innerWidth < COMPACT_LAYOUT_MAX_WIDTH
}

/**
 * 移动端布局判定。用 useSyncExternalStore 读取首帧真实宽度，
 * 避免「先渲染桌面布局再跳成移动布局」的闪烁。
 */
export function useCompactLayout(): boolean {
  return useSyncExternalStore(subscribe, getSnapshot, () => false)
}
