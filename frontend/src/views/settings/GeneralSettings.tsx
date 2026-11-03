import { useTranslation } from 'react-i18next'
import { useSettingsStore } from '@/store/settings-store'
import type { Theme } from '@/app/theme-context'
import { toast } from 'sonner'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'

export function GeneralSettings() {
  const { settings, patchCategory } = useSettingsStore()
  const { t } = useTranslation()
  const ui = settings?.ui

  if (!ui) return null

  function handleChange(key: string, value: unknown) {
    patchCategory('ui', { [key]: value }).catch((error: unknown) => {
      if (!(error instanceof DOMException && error.name === 'AbortError')) toast.error(t('settings.saveFailed'))
    })
  }

  function handleThemeChange(value: Theme) {
    handleChange('theme', value)
  }

  function handleLanguageChange(value: string) {
    handleChange('language', value)
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
