/** 主题上下文类型与 hook（与 provider 分文件以满足 fast-refresh 约束）。 */
import { createContext, useContext } from 'react'

export type Theme = 'light' | 'dark' | 'system'

export const THEME_STORAGE_KEY = 'papersage_theme'

interface ThemeCtx {
  theme: Theme
  setTheme: (t: Theme) => void
}

export const ThemeContext = createContext<ThemeCtx>({ theme: 'system', setTheme: () => {} })

export const useTheme = () => useContext(ThemeContext)

const MEDIA = '(prefers-color-scheme: dark)'

/** 应用主题到 <html>：class 切换 + 原生控件配色。 */
export function applyTheme(theme: Theme) {
  // matchMedia 在 jsdom 等环境缺失：system 态降级为浅色
  const systemDark =
    typeof window.matchMedia === 'function' && window.matchMedia(MEDIA).matches
  const dark = theme === 'dark' || (theme === 'system' && systemDark)
  document.documentElement.classList.toggle('dark', dark)
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
}
