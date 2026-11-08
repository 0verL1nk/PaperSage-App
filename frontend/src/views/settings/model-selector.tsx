import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronsUpDown } from 'lucide-react'
import { useSettingsStore } from '@/store/settings-store'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export interface ModelSelection {
  providerId: string
  model: string
}

interface ModelSelectorProps {
  variant?: 'full' | 'compact'
  value: ModelSelection
  disabled?: boolean
  onChange: (selection: ModelSelection) => void
}

export function ModelSelector({ variant = 'full', value, onChange, disabled = false }: ModelSelectorProps) {
  const { t } = useTranslation()
  const { settings, fetchSettings } = useSettingsStore()
  const providers = settings?.agent.providers ?? []

  useEffect(() => {
    if (!settings) {
      fetchSettings().catch(() => {})
    }
  }, [settings, fetchSettings])

  if (providers.length === 0) {
    return variant === 'full' ? (
      <p className="text-sm text-muted-foreground">{t('modelSelector.noneConfigured')}</p>
    ) : (
      <span className="text-xs text-muted-foreground">{t('modelSelector.none')}</span>
    )
  }

  if (variant === 'compact') {
    return <CompactSelector providers={providers} value={value} onChange={onChange} t={t} disabled={disabled} />
  }

  return <FullSelector providers={providers} value={value} onChange={onChange} t={t} disabled={disabled} />
}

type ProviderItem = {
  id: string
  name: string
  models: string[]
}

function FullSelector({
  providers,
  value,
  onChange,
  t,
  disabled,
}: {
  providers: ProviderItem[]
  value: ModelSelection
  onChange: (s: ModelSelection) => void
  t: (key: string) => string
  disabled: boolean
}) {
  const selectValue = JSON.stringify([value.providerId, value.model])

  return (
    <Select
      disabled={disabled}
      value={selectValue}
      onValueChange={(v) => {
        const [providerId, model] = JSON.parse(v) as [string, string]
        onChange({ providerId, model })
      }}
    >
      <SelectTrigger className="w-full">
        <SelectValue placeholder={t('modelSelector.selectModel')} />
      </SelectTrigger>
      <SelectContent>
        {providers.map((p) => (
          <SelectGroup key={p.id}>
            <SelectLabel>{p.name}</SelectLabel>
            {p.models.map((m) => (
              <SelectItem key={JSON.stringify([p.id, m])} value={JSON.stringify([p.id, m])}>
                {m}
              </SelectItem>
            ))}
          </SelectGroup>
        ))}
      </SelectContent>
    </Select>
  )
}

function CompactSelector({
  providers,
  value,
  onChange,
  t,
  disabled,
}: {
  providers: ProviderItem[]
  value: ModelSelection
  onChange: (s: ModelSelection) => void
  t: (key: string) => string
  disabled: boolean
}) {
  const activeProvider = providers.find((p) => p.id === value.providerId)
  const label = activeProvider
    ? `${activeProvider.name} / ${value.model || t('modelSelector.select')}`
    : t('modelSelector.selectModel')

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          disabled={disabled}
          variant="ghost"
          size="sm"
          title={label}
          className="h-7 min-w-0 max-w-[150px] gap-1 text-xs text-muted-foreground lg:max-w-none"
        >
          <span className="min-w-0 truncate lg:hidden">{value.model || label}</span>
          <span className="hidden min-w-0 truncate lg:inline">{label}</span>
          <ChevronsUpDown className="size-3 shrink-0" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        <DropdownMenuRadioGroup
          value={JSON.stringify([value.providerId, value.model])}
          onValueChange={(v) => {
            const [providerId, model] = JSON.parse(v) as [string, string]
            onChange({ providerId, model })
          }}
        >
          {providers.map((p, i) => (
            <DropdownMenuGroup key={p.id}>
              {i > 0 && <DropdownMenuSeparator />}
              <DropdownMenuLabel>{p.name}</DropdownMenuLabel>
              {p.models.length === 0 && (
                <DropdownMenuItem disabled>{t('modelSelector.none')}</DropdownMenuItem>
              )}
              {p.models.map((m) => (
                <DropdownMenuRadioItem key={JSON.stringify([p.id, m])} value={JSON.stringify([p.id, m])}>
                  {m}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuGroup>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
