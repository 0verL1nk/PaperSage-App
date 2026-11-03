import { fireEvent, render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import { ModelSelector } from './model-selector'
import { useSettingsStore } from '@/store/settings-store'

vi.mock('@/components/ui/select', () => ({
  Select: ({ value, onValueChange, children }: { value: string; onValueChange: (value: string) => void; children: ReactNode }) => <select aria-label="model" value={value} onChange={(e) => onValueChange(e.target.value)}>{children}</select>,
  SelectContent: ({ children }: { children: ReactNode }) => <>{children}</>,
  SelectGroup: ({ children }: { children: ReactNode }) => <>{children}</>,
  SelectItem: ({ value, children }: { value: string; children: ReactNode }) => <option value={value}>{children}</option>,
  SelectLabel: () => null,
  SelectTrigger: () => null,
  SelectValue: () => null,
}))

it('preserves complete provider and model IDs containing delimiters', () => {
  useSettingsStore.setState({ settings: { agent: { providers: [{ id: 'provider::x', name: 'P', models: ['model::B'] }] } } } as never)
  const onChange = vi.fn()
  render(<ModelSelector value={{ providerId: 'provider::x', model: 'model::B' }} onChange={onChange} />)
  fireEvent.change(screen.getByLabelText('model'), { target: { value: (screen.getByRole('option', { name: 'model::B' }) as HTMLOptionElement).value } })
  expect(onChange).toHaveBeenCalledWith({ providerId: 'provider::x', model: 'model::B' })
})
