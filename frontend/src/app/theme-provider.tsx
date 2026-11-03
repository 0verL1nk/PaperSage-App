/**
 * 主题 Provider：light / dark / system 三态。
 *
 * - localStorage（papersage_theme）即时生效，先于后端设置加载，
 *   避免首屏闪色
 * - system 态监听 prefers-color-scheme 变化实时跟随
 * - 后端 ui.theme 为跨设备权威值：settings 加载完成后调 setTheme 同步
 */
import { useCallback, useEffect, useLayoutEffect, useMemo, useState } from 'react'
import { useSettingsStore } from '@/store/settings-store'
import { useSessionStore } from '@/store/session-store'
import i18n, { LANG_STORAGE_KEY } from '@/i18n'

import { ThemeContext, applyTheme, type Theme, THEME_STORAGE_KEY } from './theme-context'

const SYSTEM_DARK = '(prefers-color-scheme: dark)'

function readStoredTheme(): Theme {
  const saved = localStorage.getItem(THEME_STORAGE_KEY)
  return saved === 'light' || saved === 'dark' || saved === 'system' ? saved : 'system'
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(() => useSettingsStore.getState().settings?.ui.theme ?? readStoredTheme())

  // 渲染前同步应用一次（useMemo 在首渲染提交前执行），避免首帧错色
  useMemo(() => applyTheme(theme), [theme])

  useEffect(() => {
    if (theme !== 'system') return
    const mq = window.matchMedia(SYSTEM_DARK)
    const onChange = () => applyTheme('system')
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [theme])

  const setTheme = useCallback((t: Theme) => {
    localStorage.setItem(THEME_STORAGE_KEY, t)
    setThemeState(t)
  }, [])

  useEffect(() => useSessionStore.subscribe((session, previous) => {
    if (Boolean(session.accessToken || session.refreshToken) !== Boolean(previous.accessToken || previous.refreshToken) || session.isGuest !== previous.isGuest) {
      useSettingsStore.getState().clear()
    }
  }), [])

  useLayoutEffect(() => {
    const sync = (settings: ReturnType<typeof useSettingsStore.getState>['settings']) => {
      const ui = settings?.ui
      if (!ui) return
      if (ui.theme === 'light' || ui.theme === 'dark' || ui.theme === 'system') setTheme(ui.theme)
      if (ui.language === 'zh-CN' || ui.language === 'en-US') {
        localStorage.setItem(LANG_STORAGE_KEY, ui.language)
        if (i18n.language !== ui.language) void i18n.changeLanguage(ui.language)
      }
    }
    const unsubscribe = useSettingsStore.subscribe((state) => sync(state.settings))
    sync(useSettingsStore.getState().settings)
    return unsubscribe
  }, [setTheme])

  const value = useMemo(() => ({ theme, setTheme }), [theme, setTheme])
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}
