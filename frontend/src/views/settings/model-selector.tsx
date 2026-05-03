import { useEffect } from 'react'
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
  onChange: (selection: ModelSelection) => void
}

export function ModelSelector({ variant = 'full', value, onChange }: ModelSelectorProps) {
  const { settings, fetchSettings } = useSettingsStore()
  const providers = settings?.agent.providers ?? []

  useEffect(() => {
    if (!settings) {
      fetchSettings().catch(() => {})
    }
  }, [settings, fetchSettings])

  if (providers.length === 0) {
    return variant === 'full' ? (
      <p className="text-sm text-muted-foreground">No models configured. Add a provider above.</p>
    ) : (
      <span className="text-xs text-muted-foreground">No models</span>
    )
  }

  if (variant === 'compact') {
    return <CompactSelector providers={providers} value={value} onChange={onChange} />
  }

  return <FullSelector providers={providers} value={value} onChange={onChange} />
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
}: {
  providers: ProviderItem[]
  value: ModelSelection
  onChange: (s: ModelSelection) => void
}) {
  const selectValue = `${value.providerId}::${value.model}`

  return (
    <Select
      value={selectValue}
      onValueChange={(v) => {
        const [providerId, model] = v.split('::')
        onChange({ providerId, model })
      }}
    >
      <SelectTrigger className="w-full">
        <SelectValue placeholder="Select a model" />
      </SelectTrigger>
      <SelectContent>
        {providers.map((p) => (
          <SelectGroup key={p.id}>
            <SelectLabel>{p.name}</SelectLabel>
            {p.models.map((m) => (
              <SelectItem key={`${p.id}::${m}`} value={`${p.id}::${m}`}>
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
}: {
  providers: ProviderItem[]
  value: ModelSelection
  onChange: (s: ModelSelection) => void
}) {
  const activeProvider = providers.find((p) => p.id === value.providerId)
  const label = activeProvider
    ? `${activeProvider.name} / ${value.model || 'Select'}`
    : 'Select model'

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs text-muted-foreground">
          {label}
          <ChevronsUpDown className="size-3" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        <DropdownMenuRadioGroup
          value={`${value.providerId}::${value.model}`}
          onValueChange={(v) => {
            const [providerId, model] = v.split('::')
            onChange({ providerId, model })
          }}
        >
          {providers.map((p, i) => (
            <DropdownMenuGroup key={p.id}>
              {i > 0 && <DropdownMenuSeparator />}
              <DropdownMenuLabel>{p.name}</DropdownMenuLabel>
              {p.models.length === 0 && (
                <DropdownMenuItem disabled>No models</DropdownMenuItem>
              )}
              {p.models.map((m) => (
                <DropdownMenuRadioItem key={`${p.id}::${m}`} value={`${p.id}::${m}`}>
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
