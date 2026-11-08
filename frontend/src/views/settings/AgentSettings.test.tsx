import { beforeEach, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { toast } from 'sonner'
import i18n from '@/i18n'
import { AgentSettings } from './AgentSettings'
import { useSettingsStore } from '@/store/settings-store'
import { updateCategory } from '@/services/settings-api'
import type { AllSettings } from '@/services/settings-api'

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))
vi.mock('@/services/settings-api', () => ({ getSettings: vi.fn(), updateCategory: vi.fn(), resetCategory: vi.fn(), testAgentConnection: vi.fn() }))
vi.mock('./model-selector', () => ({
  ModelSelector: ({ value, onChange, disabled }: { value: { providerId: string; model: string }; onChange: (s: { providerId: string; model: string }) => void; disabled?: boolean }) => <button disabled={disabled} onClick={() => onChange({ providerId: 'p', model: 'B' })}>{value.model || 'none'}</button>,
}))

const settings: AllSettings = {
  ui: { theme: 'system', language: 'zh-CN', sidebar_collapsed: false },
  editor: { font_size: 14, tab_size: 2, auto_save: true, auto_save_interval: 30, word_wrap: true },
  agent: { active_provider_id: 'p', temperature: 0.7, writing_style: 'academic', memory_enabled: true, providers: [
    { id: 'p', name: 'Provider', type: 'openai_compatible', base_url: 'https://example.invalid', api_key: 'secret', models: ['A', 'B'], active_model: 'A' },
  ] },
}

const originalPatch = useSettingsStore.getState().patchCategory

beforeEach(() => {
  void i18n.changeLanguage('zh-CN')
  useSettingsStore.getState().clear()
  useSettingsStore.setState({ settings: structuredClone(settings), error: null, patchCategory: originalPatch })
  vi.mocked(updateCategory).mockReset()
  vi.mocked(toast.error).mockClear()
})

it('shows an error rather than an empty provider editor when settings could not load', () => {
  useSettingsStore.setState({ settings: null, error: 'private detail' })
  render(<AgentSettings />)
  expect(screen.getByRole('alert')).toHaveTextContent('设置加载失败')
  expect(screen.queryByText(/private detail/)).not.toBeInTheDocument()
})

it('saves only the selected provider and model', async () => {
  const saved = structuredClone(settings.agent)
  saved.providers[0].active_model = 'B'
  vi.mocked(updateCategory).mockResolvedValue({ category: 'agent', settings: { ...saved } })
  render(<AgentSettings />)
  fireEvent.click(screen.getByRole('button', { name: 'A' }))
  await waitFor(() => expect(screen.getByRole('button', { name: 'B' })).toBeInTheDocument())
  expect(updateCategory).toHaveBeenCalledWith('agent', { model_selection: { provider_id: 'p', model: 'B' } })
})

it('reports a settings failure in English', async () => {
  await i18n.changeLanguage('en-US')
  vi.mocked(updateCategory).mockRejectedValue(new Error('save failed'))
  render(<AgentSettings />)
  fireEvent.click(screen.getByRole('button', { name: 'A' }))
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Failed to save settings. Please try again.'))
})

it('reports a model save failure even after settings unmounts', async () => {
  let rejectSave!: (error: Error) => void
  vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((_, reject) => { rejectSave = reject }))
  const view = render(<AgentSettings />)
  fireEvent.click(screen.getByRole('button', { name: 'A' }))
  view.unmount()
  rejectSave(new Error('private detail'))
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('设置保存失败，请重试。'))
})

it('reports provider activation failure after settings unmounts', async () => {
  let rejectSave!: (error: Error) => void
  vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((_, reject) => { rejectSave = reject }))
  const view = render(<AgentSettings />)
  fireEvent.click(screen.getByText('Provider'))
  view.unmount()
  rejectSave(new Error('private detail'))
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('设置保存失败，请重试。'))
})

it('does not report a previous account save after clear', async () => {
  let rejectSave!: (error: Error) => void
  vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((_, reject) => { rejectSave = reject }))
  const patch = vi.spyOn(useSettingsStore.getState(), 'patchCategory')
  const view = render(<AgentSettings />)
  fireEvent.click(screen.getByRole('button', { name: 'A' }))
  const result = expect(patch.mock.results[0].value).rejects.toMatchObject({ name: 'AbortError' })
  view.unmount()
  useSettingsStore.getState().clear()
  await act(async () => { rejectSave(new Error('old account')); await result })
  expect(toast.error).not.toHaveBeenCalled()
  patch.mockRestore()
})

it('reports a non-model save failure after settings unmounts', async () => {
  let rejectSave!: (error: Error) => void
  vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((_, reject) => { rejectSave = reject }))
  const view = render(<AgentSettings />)
  fireEvent.click(screen.getByRole('checkbox'))
  view.unmount()
  rejectSave(new Error('private detail'))
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('设置保存失败，请重试。'))
  expect(toast.error).toHaveBeenCalledTimes(1)
})

it('does not report a cleared non-model save failure', async () => {
  let rejectSave!: (error: Error) => void
  vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((_, reject) => { rejectSave = reject }))
  const patch = vi.spyOn(useSettingsStore.getState(), 'patchCategory')
  const view = render(<AgentSettings />)
  fireEvent.click(screen.getByRole('checkbox'))
  const result = expect(patch.mock.results[0].value).rejects.toMatchObject({ name: 'AbortError' })
  view.unmount()
  useSettingsStore.getState().clear()
  await act(async () => { rejectSave(new Error('old account')); await result })
  expect(toast.error).not.toHaveBeenCalled()
  patch.mockRestore()
})

it('keeps saving feedback until both queued saves complete', async () => {
  let resolveFirst!: (value: { category: string; settings: Record<string, unknown> }) => void
  let resolveSecond!: (value: { category: string; settings: Record<string, unknown> }) => void
  vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve }))
    .mockImplementationOnce(() => new Promise((resolve) => { resolveSecond = resolve }))
  render(<AgentSettings />)
  fireEvent.click(screen.getByText('Provider'))
  fireEvent.click(screen.getByRole('checkbox'))
  expect(updateCategory).toHaveBeenCalledTimes(1)
  expect(screen.getByRole('button', { name: 'A' })).toBeDisabled()
  await act(async () => {
    resolveFirst({ category: 'agent', settings: { ...settings.agent } })
    await vi.waitFor(() => expect(updateCategory).toHaveBeenCalledTimes(2))
  })
  expect(screen.getByText(/正在保存设置/)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'A' })).toBeDisabled()
  await act(async () => { resolveSecond({ category: 'agent', settings: { ...settings.agent, memory_enabled: false } }) })
  await waitFor(() => expect(screen.getByRole('button', { name: 'A' })).toBeEnabled())
  expect(vi.mocked(updateCategory).mock.calls.map(([, partial]) => partial)).toEqual([{ active_provider_id: 'p' }, { memory_enabled: false }])
})

it('keeps saving feedback after a queued save fails', async () => {
  let rejectFirst!: (error: Error) => void
  let resolveSecond!: (value: { category: string; settings: Record<string, unknown> }) => void
  vi.mocked(updateCategory).mockImplementationOnce(() => new Promise((_, reject) => { rejectFirst = reject }))
    .mockImplementationOnce(() => new Promise((resolve) => { resolveSecond = resolve }))
  render(<AgentSettings />)
  fireEvent.click(screen.getByText('Provider'))
  fireEvent.click(screen.getByRole('checkbox'))
  await act(async () => {
    rejectFirst(new Error('failed'))
    await vi.waitFor(() => expect(updateCategory).toHaveBeenCalledTimes(2))
  })
  expect(toast.error).toHaveBeenCalledTimes(1)
  expect(screen.getByRole('button', { name: 'A' })).toBeDisabled()
  await act(async () => { resolveSecond({ category: 'agent', settings: { ...settings.agent, memory_enabled: false } }) })
  await waitFor(() => expect(screen.getByRole('button', { name: 'A' })).toBeEnabled())
})

it('reports a failed selection save and restores the previous model', async () => {
  vi.mocked(updateCategory).mockRejectedValue(new Error('secret error'))
  render(<AgentSettings />)
  fireEvent.click(screen.getByRole('button', { name: 'A' }))
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('设置保存失败，请重试。'))
  await waitFor(() => expect(screen.getByRole('button', { name: 'A' })).toBeInTheDocument())
  expect(screen.queryByText(/secret error/)).not.toBeInTheDocument()
})
