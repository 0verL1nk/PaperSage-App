import { useSettingsStore } from '@/store/settings-store'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'

const TEMPERATURE_OPTIONS = [
  { value: '0.0', label: 'Precise (0.0)' },
  { value: '0.3', label: 'Conservative (0.3)' },
  { value: '0.7', label: 'Balanced (0.7)' },
  { value: '1.0', label: 'Creative (1.0)' },
  { value: '1.5', label: 'Inventive (1.5)' },
]

const WRITING_STYLES = [
  { value: 'academic', label: 'Academic' },
  { value: 'formal', label: 'Formal' },
  { value: 'casual', label: 'Casual' },
  { value: 'concise', label: 'Concise' },
]

export function AgentSettings() {
  const { settings, patchCategory } = useSettingsStore()
  const agent = settings?.agent

  if (!agent) return null

  function handleChange(key: string, value: unknown) {
    patchCategory('agent', { [key]: value }).catch(() => {})
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Model</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <Label htmlFor="model-name">AI Model</Label>
            <Select
              value={agent.model_name}
              onValueChange={(v) => handleChange('model_name', v)}
            >
              <SelectTrigger id="model-name" className="w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="deepseek">DeepSeek</SelectItem>
                <SelectItem value="openai">OpenAI GPT</SelectItem>
                <SelectItem value="claude">Claude</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="flex items-center justify-between">
            <Label htmlFor="temperature">Temperature</Label>
            <Select
              value={String(agent.temperature)}
              onValueChange={(v) => handleChange('temperature', parseFloat(v))}
            >
              <SelectTrigger id="temperature" className="w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {TEMPERATURE_OPTIONS.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Writing</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <Label htmlFor="writing-style">Writing Style</Label>
            <Select
              value={agent.writing_style}
              onValueChange={(v) => handleChange('writing_style', v)}
            >
              <SelectTrigger id="writing-style" className="w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {WRITING_STYLES.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Memory</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-3">
            <Checkbox
              id="memory-enabled"
              checked={agent.memory_enabled}
              onCheckedChange={(v) => handleChange('memory_enabled', !!v)}
            />
            <Label htmlFor="memory-enabled">
              Enable AI memory (remembers context across sessions)
            </Label>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
