import { useSettingsStore } from '@/store/settings-store'
import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'

export function EditorSettings() {
  const { t } = useTranslation()
  const { settings, patchCategory } = useSettingsStore()
  const editor = settings?.editor

  if (!editor) return null

  function handleChange(key: string, value: unknown) {
    patchCategory('editor', { [key]: value }).catch(() => {})
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{t('editor.display')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <Label htmlFor="font-size">{t('editor.fontSize')}</Label>
            <Input
              id="font-size"
              type="number"
              min={8}
              max={32}
              value={editor.font_size}
              onChange={(e) => handleChange('font_size', Number(e.target.value))}
              className="w-24"
            />
          </div>

          <div className="flex items-center justify-between">
            <Label htmlFor="tab-size">{t('editor.tabSize')}</Label>
            <Input
              id="tab-size"
              type="number"
              min={1}
              max={8}
              value={editor.tab_size}
              onChange={(e) => handleChange('tab_size', Number(e.target.value))}
              className="w-24"
            />
          </div>

          <div className="flex items-center gap-3">
            <Checkbox
              id="word-wrap"
              checked={editor.word_wrap}
              onCheckedChange={(v) => handleChange('word_wrap', !!v)}
            />
            <Label htmlFor="word-wrap">{t('editor.wordWrap')}</Label>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('editor.autoSave')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-3">
            <Checkbox
              id="auto-save"
              checked={editor.auto_save}
              onCheckedChange={(v) => handleChange('auto_save', !!v)}
            />
            <Label htmlFor="auto-save">{t('editor.enableAutoSave')}</Label>
          </div>

          {editor.auto_save && (
            <div className="flex items-center justify-between">
              <Label htmlFor="auto-save-interval">{t('editor.intervalSeconds')}</Label>
              <Input
                id="auto-save-interval"
                type="number"
                min={5}
                max={300}
                value={editor.auto_save_interval}
                onChange={(e) =>
                  handleChange('auto_save_interval', Number(e.target.value))
                }
                className="w-24"
              />
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
