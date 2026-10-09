import { useTranslation } from 'react-i18next'
import { useSettingsStore } from '@/store/settings-store'
import { useTheme, type Theme } from '@/app/theme-context'
import { LANG_STORAGE_KEY } from '@/i18n'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'

export function GeneralSettings() {
  const { settings, patchCategory } = useSettingsStore()
  const { setTheme } = useTheme()
  const { t, i18n } = useTranslation()
  const ui = settings?.ui

  if (!ui) return null

  // 后端 ui.category 是跨设备权威值：加载后同步到本地（若本地从未
  // 设置过；本地已有值时以本地为准，避免登录瞬间跳变）
  // （初值应用已由 ThemeProvider/i18n 的 localStorage 路径完成）

  function handleThemeChange(value: Theme) {
    setTheme(value)                       // 即时生效 + localStorage
    patchCategory('ui', { theme: value }).catch(() => {})   // 后端持久化
  }

  function handleLanguageChange(value: string) {
    localStorage.setItem(LANG_STORAGE_KEY, value)
    void i18n.changeLanguage(value)       // 即时生效
    patchCategory('ui', { language: value }).catch(() => {})
  }

  function handleChange(key: string, value: unknown) {
    patchCategory('ui', { [key]: value }).catch(() => {})
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{t('settings.appearanceTitle')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <Label htmlFor="theme">{t('settings.theme')}</Label>
            <Select
              value={ui.theme}
              onValueChange={(v) => handleThemeChange(v as Theme)}
            >
              <SelectTrigger id="theme" className="w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="system">{t('settings.themeSystem')}</SelectItem>
                <SelectItem value="light">{t('settings.themeLight')}</SelectItem>
                <SelectItem value="dark">{t('settings.themeDark')}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="flex items-center justify-between">
            <Label htmlFor="language">{t('settings.language')}</Label>
            <Select
              value={ui.language}
              onValueChange={handleLanguageChange}
            >
              <SelectTrigger id="language" className="w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="zh-CN">{t('settings.langZh')}</SelectItem>
                <SelectItem value="en-US">{t('settings.langEn')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('settings.navigationTitle')}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-3">
            <Checkbox
              id="sidebar-collapsed"
              checked={ui.sidebar_collapsed}
              onCheckedChange={(v) => handleChange('sidebar_collapsed', !!v)}
            />
            <Label htmlFor="sidebar-collapsed">{t('settings.collapseSidebar')}</Label>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
