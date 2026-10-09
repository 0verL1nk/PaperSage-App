import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSettingsStore } from '@/store/settings-store'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'
import { Plus, Trash2, Pencil, Check, X } from 'lucide-react'
import type { ProviderConfig, ProviderType } from '@/services/settings-api'
import { ModelSelector } from './model-selector'

const TEMPERATURE_OPTIONS = [
  { value: '0.0', labelKey: 'agent.precise' },
  { value: '0.3', labelKey: 'agent.conservative' },
  { value: '0.7', labelKey: 'agent.balanced' },
  { value: '1.0', labelKey: 'agent.creative' },
  { value: '1.5', labelKey: 'agent.inventive' },
]

const WRITING_STYLES = [
  { value: 'academic', labelKey: 'agent.academic' },
  { value: 'formal', labelKey: 'agent.formal' },
  { value: 'casual', labelKey: 'agent.casual' },
  { value: 'concise', labelKey: 'agent.concise' },
]

const TEMP_ID = '__new__'

export function AgentSettings() {
  const { t } = useTranslation()
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
      id: TEMP_ID,
      name: '',
      type: 'openai_compatible',
      base_url: '',
      api_key: '',
      models: [],
      active_model: '',
    }
    setEditingId(TEMP_ID)
    setEditDraft(newProvider)
  }

  function handleDeleteProvider(id: string) {
    const updated = a.providers.filter((p) => p.id !== id)
    const newActiveId = a.active_provider_id === id
      ? (updated[0]?.id ?? '')
      : a.active_provider_id
    patchCategory('agent', { providers: updated, active_provider_id: newActiveId }).catch(() => {})
  }

  function handleSaveEdit(flushedDraft?: ProviderConfig) {
    const source = flushedDraft ?? editDraft
    if (!source) return
    const toSave = source.id === TEMP_ID ? { ...source, id: '' } : source
    const exists = a.providers.some((p) => p.id === source.id)
    const updated = exists
      ? a.providers.map((p) => (p.id === source.id ? toSave : p))
      : [...a.providers, toSave]
    const newActiveId = a.active_provider_id || ''
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
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-base">{t('agent.providers')}</CardTitle>
          <Button variant="ghost" size="icon-sm" aria-label={t('agent.addProvider')} onClick={handleAddProvider}>
            <Plus className="size-4" />
          </Button>
        </CardHeader>
        <CardContent className="space-y-3">
          {a.providers.length === 0 && !editingId && (
            <p className="text-sm text-muted-foreground py-2">
              {t('agent.noProviders')}
            </p>
          )}

          {a.providers.map((provider) => (
            <ProviderCard
              key={provider.id}
              provider={provider}
              isActive={provider.id === a.active_provider_id}
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

          {editingId && !a.providers.some((p) => p.id === editingId) && editDraft && (
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

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">{t('agent.defaultModel')}</CardTitle>
        </CardHeader>
        <CardContent>
          <ModelSelector
            value={{
              providerId: a.active_provider_id,
              model: activeProvider?.active_model ?? '',
            }}
            onChange={({ providerId, model }) => {
              const updated = a.providers.map((p) =>
                p.id === providerId ? { ...p, active_model: model } : p,
              )
              patchCategory('agent', {
                active_provider_id: providerId,
                providers: updated,
              }).catch(() => {})
            }}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">{t('agent.generation')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <Label htmlFor="temperature">{t('agent.temperature')}</Label>
            <Select
              value={String(a.temperature)}
              onValueChange={(v) => handleChange('temperature', parseFloat(v))}
            >
              <SelectTrigger id="temperature" className="w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {TEMPERATURE_OPTIONS.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {t(opt.labelKey)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex items-center justify-between">
            <Label htmlFor="writing-style">{t('agent.writingStyle')}</Label>
            <Select
              value={a.writing_style}
              onValueChange={(v) => handleChange('writing_style', v)}
            >
              <SelectTrigger id="writing-style" className="w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {WRITING_STYLES.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {t(opt.labelKey)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">{t('agent.memory')}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-3">
            <Checkbox
              id="memory-enabled"
              checked={a.memory_enabled}
              onCheckedChange={(v) => handleChange('memory_enabled', !!v)}
            />
            <Label htmlFor="memory-enabled">
              {t('agent.enableMemory')}
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
  onSave: (flushedDraft?: ProviderConfig) => void
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
  const { t } = useTranslation()
  const [newModel, setNewModel] = useState('')

  if (isEditing && editDraft) {
    const d = editDraft

    function flushPendingModel(): ProviderConfig {
      const trimmed = newModel.trim()
      if (!trimmed || d.models.includes(trimmed)) return d
      const next = { ...d, models: [...d.models, trimmed], active_model: d.active_model || trimmed }
      onDraftChange(next)
      setNewModel('')
      return next
    }

    function addModel() {
      const trimmed = newModel.trim()
      if (!trimmed || d.models.includes(trimmed)) return
      onDraftChange({ ...d, models: [...d.models, trimmed], active_model: d.active_model || trimmed })
      setNewModel('')
    }

    function removeModel(idx: number) {
      const updated = d.models.filter((_, i) => i !== idx)
      const newActive = d.active_model === d.models[idx]
        ? (updated[0] ?? '')
        : d.active_model
      onDraftChange({ ...d, models: updated, active_model: newActive })
    }

    return (
      <div className="border rounded-lg p-3 space-y-3 bg-muted/30">
        <div className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-2 items-center">
          <Label className="text-xs text-muted-foreground">{t('agent.name')}</Label>
          <Input
            value={d.name}
            onChange={(e) => onDraftChange({ ...editDraft, name: e.target.value })}
            className="h-8 text-sm"
          />
          <Label className="text-xs text-muted-foreground">{t('agent.type')}</Label>
          <Select
            value={d.type}
            onValueChange={(v) => onDraftChange({ ...editDraft, type: v as ProviderType })}
          >
            <SelectTrigger className="h-8 text-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="openai_compatible">{t('agent.compatible')}</SelectItem>
              <SelectItem value="anthropic">Anthropic</SelectItem>
            </SelectContent>
          </Select>
          <Label className="text-xs text-muted-foreground">{t('agent.baseUrl')}</Label>
          <Input
            value={d.base_url}
            onChange={(e) => onDraftChange({ ...editDraft, base_url: e.target.value })}
            placeholder="https://api.openai.com/v1"
            className="h-8 text-sm"
          />
          <Label className="text-xs text-muted-foreground">{t('agent.apiKey')}</Label>
          <Input
            type="password"
            value={d.api_key}
            onChange={(e) => onDraftChange({ ...editDraft, api_key: e.target.value })}
            placeholder="sk-..."
            className="h-8 text-sm"
          />
        </div>

        {/* Models list */}
        <div className="space-y-2">
          <Label className="text-xs text-muted-foreground">{t('agent.models')}</Label>
          {d.models.length > 0 && (
            <div className="space-y-1">
              {d.models.map((m, idx) => (
                <div key={m} className="flex items-center gap-2">
                  <Button
                    variant={d.active_model === m ? 'default' : 'outline'}
                    size="sm"
                    className="flex-1 justify-start h-7 text-xs"
                    onClick={() => onDraftChange({ ...editDraft, active_model: m })}
                  >
                    {m}
                    {d.active_model === m && <span className="ml-auto text-[10px] opacity-70">{t('agent.active')}</span>}
                  </Button>
                  <Button variant="ghost" size="icon-xs" onClick={() => removeModel(idx)}>
                    <X className="size-3" />
                  </Button>
                </div>
              ))}
            </div>
          )}
          <div className="flex gap-2">
            <Input
              value={newModel}
              onChange={(e) => setNewModel(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') addModel() }}
              placeholder="gpt-4o / deepseek-chat"
              className="h-8 text-sm"
            />
            <Button variant="outline" size="sm" onClick={addModel} disabled={!newModel.trim()}>
              <Plus className="size-3" />
            </Button>
          </div>
        </div>

        <div className="flex justify-end gap-2">
          <Button variant="ghost" size="sm" onClick={onCancel}>
            <X className="size-3 mr-1" /> {t('agent.cancel')}
          </Button>
          <Button variant="default" size="sm" onClick={() => onSave(flushPendingModel())}>
            <Check className="size-3 mr-1" /> {t('agent.save')}
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
              {t('agent.active')}
            </span>
          )}
        </div>
        <div className="text-xs text-muted-foreground truncate">
          {provider.active_model || t('agent.noModel')} · {provider.type === 'anthropic' ? 'Anthropic' : 'OpenAI'} · {provider.base_url || t('agent.noUrl')}
        </div>
        {provider.models.length > 0 && (
          <div className="flex flex-wrap gap-1 mt-1">
            {provider.models.map((m) => (
              <span
                key={m}
                className={`text-[10px] px-1.5 py-0.5 rounded ${
                  provider.active_model === m
                    ? 'bg-primary/10 text-primary font-medium'
                    : 'bg-muted text-muted-foreground'
                }`}
              >
                {m}
              </span>
            ))}
          </div>
        )}
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
