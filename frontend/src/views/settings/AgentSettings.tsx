import { useState } from 'react'
import { useSettingsStore } from '@/store/settings-store'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'
import { Plus, Trash2, Pencil, Check, X } from 'lucide-react'
import type { ProviderConfig, ProviderType } from '@/services/settings-api'

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

function generateId(): string {
  return `provider_${Date.now().toString(36)}`
}

function maskKey(key: string): string {
  if (!key || key.length <= 8) return key ? '****' : ''
  return `${key.slice(0, 4)}${'*'.repeat(key.length - 8)}${key.slice(-4)}`
}

export function AgentSettings() {
  const { settings, patchCategory } = useSettingsStore()
  const agent = settings?.agent
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState<ProviderConfig | null>(null)

  if (!agent) return null
  const a = agent

  function handleChange(key: string, value: unknown) {
    patchCategory('agent', { [key]: value }).catch(() => {})
  }

  function handleAddProvider() {
    const newProvider: ProviderConfig = {
      id: generateId(),
      name: 'New Provider',
      type: 'openai_compatible',
      base_url: '',
      api_key: '',
      model_name: '',
    }
    // Only add to local state, don't persist yet
    setEditingId(newProvider.id)
    setEditDraft(newProvider)
  }

  function handleDeleteProvider(id: string) {
    const updated = a.providers.filter((p) => p.id !== id)
    const newActiveId = a.active_provider_id === id
      ? (updated[0]?.id ?? '')
      : a.active_provider_id
    patchCategory('agent', { providers: updated, active_provider_id: newActiveId }).catch(() => {})
  }

  function handleSaveEdit() {
    if (!editDraft) return
    const exists = a.providers.some((p) => p.id === editDraft.id)
    const updated = exists
      ? a.providers.map((p) => (p.id === editDraft.id ? editDraft : p))
      : [...a.providers, editDraft]
    const newActiveId = a.active_provider_id || editDraft.id
    patchCategory('agent', { providers: updated, active_provider_id: newActiveId }).catch(() => {})
    setEditingId(null)
    setEditDraft(null)
  }

  function handleCancelEdit() {
    setEditingId(null)
    setEditDraft(null)
  }

  const activeProvider = a.providers.find((p) => p.id === a.active_provider_id)

  return (
    <div className="space-y-4">
      {/* Provider List */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-base">Model Providers</CardTitle>
          <Button variant="ghost" size="icon-sm" onClick={handleAddProvider}>
            <Plus className="size-4" />
          </Button>
        </CardHeader>
        <CardContent className="space-y-3">
          {agent.providers.length === 0 && (
            <p className="text-sm text-muted-foreground py-2">
              No providers configured. Click + to add one.
            </p>
          )}

          {agent.providers.map((provider) => (
            <ProviderCard
              key={provider.id}
              provider={provider}
              isActive={provider.id === agent.active_provider_id}
              isEditing={editingId === provider.id}
              editDraft={editingId === provider.id ? editDraft : null}
              onActivate={() => handleChange('active_provider_id', provider.id)}
              onEdit={() => { setEditingId(provider.id); setEditDraft({ ...provider }) }}
              onDelete={() => handleDeleteProvider(provider.id)}
              onDraftChange={setEditDraft}
              onSave={handleSaveEdit}
              onCancel={handleCancelEdit}
            />
          ))}

          {/* Show editing form for new provider (not yet in list) */}
          {editingId && !agent.providers.some((p) => p.id === editingId) && editDraft && (
            <ProviderCard
              provider={editDraft}
              isActive={false}
              isEditing={true}
              editDraft={editDraft}
              onActivate={() => {}}
              onEdit={() => {}}
              onDelete={handleCancelEdit}
              onDraftChange={setEditDraft}
              onSave={handleSaveEdit}
              onCancel={handleCancelEdit}
            />
          )}
        </CardContent>
      </Card>

      {/* Active Model Info */}
      {activeProvider && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Active Model</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-sm space-y-1">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Provider</span>
                <span>{activeProvider.name}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Model</span>
                <span>{activeProvider.model_name || 'Not set'}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Endpoint</span>
                <span className="truncate ml-4 max-w-[280px]">{activeProvider.base_url || 'Not set'}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">API Key</span>
                <span>{activeProvider.api_key ? maskKey(activeProvider.api_key) : 'Not set'}</span>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Generation Settings */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Generation</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
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

      {/* Memory */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Memory</CardTitle>
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

// --- Sub-components ---

interface ProviderCardProps {
  provider: ProviderConfig
  isActive: boolean
  isEditing: boolean
  editDraft: ProviderConfig | null
  onActivate: () => void
  onEdit: () => void
  onDelete: () => void
  onDraftChange: (draft: ProviderConfig) => void
  onSave: () => void
  onCancel: () => void
}

function ProviderCard({
  provider,
  isActive,
  isEditing,
  editDraft,
  onActivate,
  onEdit,
  onDelete,
  onDraftChange,
  onSave,
  onCancel,
}: ProviderCardProps) {
  if (isEditing && editDraft) {
    return (
      <div className="border rounded-lg p-3 space-y-3 bg-muted/30">
        <div className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-2 items-center">
          <Label className="text-xs text-muted-foreground">Name</Label>
          <Input
            value={editDraft.name}
            onChange={(e) => onDraftChange({ ...editDraft, name: e.target.value })}
            className="h-8 text-sm"
          />
          <Label className="text-xs text-muted-foreground">Type</Label>
          <Select
            value={editDraft.type}
            onValueChange={(v) => onDraftChange({ ...editDraft, type: v as ProviderType })}
          >
            <SelectTrigger className="h-8 text-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="openai_compatible">OpenAI Compatible</SelectItem>
              <SelectItem value="anthropic">Anthropic</SelectItem>
            </SelectContent>
          </Select>
          <Label className="text-xs text-muted-foreground">Base URL</Label>
          <Input
            value={editDraft.base_url}
            onChange={(e) => onDraftChange({ ...editDraft, base_url: e.target.value })}
            placeholder="https://api.openai.com/v1"
            className="h-8 text-sm"
          />
          <Label className="text-xs text-muted-foreground">API Key</Label>
          <Input
            type="password"
            value={editDraft.api_key}
            onChange={(e) => onDraftChange({ ...editDraft, api_key: e.target.value })}
            placeholder="sk-..."
            className="h-8 text-sm"
          />
          <Label className="text-xs text-muted-foreground">Model</Label>
          <Input
            value={editDraft.model_name}
            onChange={(e) => onDraftChange({ ...editDraft, model_name: e.target.value })}
            placeholder="gpt-4o / deepseek-chat"
            className="h-8 text-sm"
          />
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" size="sm" onClick={onCancel}>
            <X className="size-3 mr-1" /> Cancel
          </Button>
          <Button variant="default" size="sm" onClick={onSave}>
            <Check className="size-3 mr-1" /> Save
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div
      className={`border rounded-lg p-3 flex items-center gap-3 cursor-pointer transition-colors ${
        isActive ? 'border-primary bg-primary/5' : 'hover:bg-muted/50'
      }`}
      onClick={onActivate}
    >
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium">{provider.name}</span>
          {isActive && (
            <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-primary text-primary-foreground font-medium">
              Active
            </span>
          )}
        </div>
        <div className="text-xs text-muted-foreground truncate">
          {provider.model_name} · {provider.type === 'anthropic' ? 'Anthropic' : 'OpenAI'} · {provider.base_url || 'No URL'}
        </div>
      </div>
      <div className="flex items-center gap-1 shrink-0">
        <Button variant="ghost" size="icon-xs" onClick={(e) => { e.stopPropagation(); onEdit() }}>
          <Pencil className="size-3" />
        </Button>
        <Button variant="ghost" size="icon-xs" onClick={(e) => { e.stopPropagation(); onDelete() }}>
          <Trash2 className="size-3 text-destructive" />
        </Button>
      </div>
    </div>
  )
}
