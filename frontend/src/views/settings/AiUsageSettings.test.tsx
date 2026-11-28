import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n'
import { AiUsageSettings } from './AiUsageSettings'
import { FrontendApiError } from '@/services/errors'
import type { AiUsageBucket, AiUsageReport } from '@/services/ai-usage-api'

const api = vi.hoisted(() => ({ getMyAiUsage: vi.fn() }))

vi.mock('@/services/ai-usage-api', async () => {
  const actual = await vi.importActual<typeof import('@/services/ai-usage-api')>('@/services/ai-usage-api')
  return { ...actual, getMyAiUsage: api.getMyAiUsage }
})

function bucket(overrides: Partial<AiUsageBucket> = {}): AiUsageBucket {
  return {
    calls: 0, success: 0, error: 0, timeout: 0, cancelled: 0, success_rate: 0,
    prompt_tokens: 0, completion_tokens: 0, usage_missing_calls: 0,
    latency_p50_ms: null, latency_p95_ms: null, first_token_p50_ms: null,
    ...overrides,
  }
}

const report: AiUsageReport = {
  from: '2026-10-29T00:00:00Z',
  to: '2026-11-28T00:00:00Z',
  timezone: 'Asia/Shanghai',
  totals: bucket({
    calls: 4, success: 3, error: 1, success_rate: 0.75,
    prompt_tokens: 300, completion_tokens: 120, usage_missing_calls: 1,
    latency_p50_ms: 700, latency_p95_ms: 1500,
  }),
  by_day: [bucket({ key: '2026-11-27', prompt_tokens: 300, completion_tokens: 120 })],
  by_model: [bucket({ key: 'deepseek-chat', calls: 4, prompt_tokens: 300, completion_tokens: 120 })],
  by_feature: [],
  by_error: [],
}

const emptyReport: AiUsageReport = { ...report, totals: bucket(), by_day: [], by_model: [] }

function wrap(ui: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={client}>{ui}</QueryClientProvider>
}

describe('AiUsageSettings', () => {
  beforeEach(() => {
    api.getMyAiUsage.mockReset()
  })

  it('shows personal totals, daily tokens and model rows', async () => {
    api.getMyAiUsage.mockResolvedValue(report)
    render(wrap(<AiUsageSettings />))

    expect(await screen.findByText('调用次数')).toBeInTheDocument()
    expect(screen.getAllByText('420').length).toBeGreaterThan(0)
    expect(screen.getByText('输入 300 / 输出 120')).toBeInTheDocument()
    expect(screen.getByText('deepseek-chat')).toBeInTheDocument()
    expect(screen.getByText(/只统计你自己的调用/)).toBeInTheDocument()
  })

  it('explains the empty state', async () => {
    api.getMyAiUsage.mockResolvedValue(emptyReport)
    render(wrap(<AiUsageSettings />))
    expect(await screen.findByText('近 30 天还没有 AI 调用记录')).toBeInTheDocument()
  })

  it('shows an actionable error and retries', async () => {
    api.getMyAiUsage.mockRejectedValue(new FrontendApiError('boom', { kind: 'network' }))
    render(wrap(<AiUsageSettings />))
    expect(await screen.findByText('AI 用量加载失败，请检查网络后重试。')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /重新加载/ }))
    await waitFor(() => expect(api.getMyAiUsage).toHaveBeenCalledTimes(2))
  })
})
