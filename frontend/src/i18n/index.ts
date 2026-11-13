/**
 * i18n：zh-CN 默认，en-US 可选。
 *
 * 语言来源优先级：localStorage（papersage_lang，先于后端避免首屏跳变）
 * 到 后端 settings ui.language（加载完成后 changeLanguage 同步，跨设备权威）。
 * 尚未迁移的模块（writing/uploading 深层文案）保持中文，逐步抽取。
 */
import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

import zhCN from './locales/zh-CN.json'
import enUS from './locales/en-US.json'

export type AppLanguage = 'zh-CN' | 'en-US'
export const LANG_STORAGE_KEY = 'papersage_lang'

export function normalizeLang(v: string | null | undefined): AppLanguage | null {
  return v === 'zh-CN' || v === 'en-US' ? v : null
}

export function initialLang(): AppLanguage {
  return normalizeLang(localStorage.getItem(LANG_STORAGE_KEY)) ?? 'zh-CN'
}

i18n.on('languageChanged', (language) => {
  document.documentElement.lang = language
})

void i18n.use(initReactI18next).init({
  resources: {
    'zh-CN': { translation: zhCN },
    'en-US': { translation: enUS },
  },
  lng: initialLang(),
  fallbackLng: 'zh-CN',
  interpolation: { escapeValue: false },
})

export default i18n
