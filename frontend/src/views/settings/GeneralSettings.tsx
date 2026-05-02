import { useSettingsStore } from '@/store/settings-store'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'

export function GeneralSettings() {
  const { settings, patchCategory } = useSettingsStore()
  const ui = settings?.ui

  if (!ui) return null

  function handleChange(key: string, value: unknown) {
    patchCategory('ui', { [key]: value }).catch(() => {})
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Appearance</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <Label htmlFor="theme">Theme</Label>
            <Select
              value={ui.theme}
              onValueChange={(v) => handleChange('theme', v)}
            >
              <SelectTrigger id="theme" className="w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="system">System</SelectItem>
                <SelectItem value="light">Light</SelectItem>
                <SelectItem value="dark">Dark</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="flex items-center justify-between">
            <Label htmlFor="language">Language</Label>
            <Select
              value={ui.language}
              onValueChange={(v) => handleChange('language', v)}
            >
              <SelectTrigger id="language" className="w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="zh-CN">Chinese</SelectItem>
                <SelectItem value="en-US">English</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Navigation</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-3">
            <Checkbox
              id="sidebar-collapsed"
              checked={ui.sidebar_collapsed}
              onCheckedChange={(v) => handleChange('sidebar_collapsed', !!v)}
            />
            <Label htmlFor="sidebar-collapsed">Collapse sidebar by default</Label>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
