import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { AiUsagePage } from './AiUsagePage'
import { FrontendApiError } from '@/services/errors'
import type { AiUsageBucket, AiUsageReport } from '@/services/ai-usage-api'

const api = vi.hoisted(() => ({
  getAdminAiUsage: vi.fn(),
  downloadAdminAiUsage: vi.fn(),
}))

vi.mock('@/services/ai-usage-api', async () => {
  const actual = await vi.importActual<typeof import('@/services/ai-usage-api')>('@/services/ai-usage-api')
  return {
    ...actual,
    getAdminAiUsage: api.getAdminAiUsage,
    downloadAdminAiUsage: api.downloadAdminAiUsage,
  }
})

function bucket(overrides: Partial<AiUsageBucket> = {}): AiUsageBucket {
  return {
    calls: 0,
    success: 0,
    error: 0,
    timeout: 0,
    cancelled: 0,
    success_rate: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    usage_missing_calls: 0,
    latency_p50_ms: null,
    latency_p95_ms: null,
    first_token_p50_ms: null,
    ...overrides,
  }
}

const report: AiUsageReport = {
  from: '2026-11-21T00:00:00Z',
  to: '2026-11-28T00:00:00Z',
  timezone: 'Asia/Shanghai',
  totals: bucket({
    calls: 8, success: 7, error: 1, success_rate: 0.875,
    prompt_tokens: 1200, completion_tokens: 800, usage_missing_calls: 1,
    latency_p50_ms: 900, latency_p95_ms: 2100,
  }),
  by_day: [bucket({ key: '2026-11-27', calls: 8, success: 7, error: 1, prompt_tokens: 1200, completion_tokens: 800, latency_p50_ms: 900, latency_p95_ms: 2100 })],
  by_model: [bucket({ key: 'deepseek-chat', calls: 8, success: 7, success_rate: 0.875, prompt_tokens: 1200, completion_tokens: 800, latency_p50_ms: 900, latency_p95_ms: 2100 })],
  by_feature: [bucket({ key: 'cross_chapter_review', calls: 2 })],
  by_error: [bucket({ key: 'auth', calls: 1 })],
}

const emptyReport: AiUsageReport = { ...report, totals: bucket(), by_day: [], by_model: [], by_feature: [], by_error: [] }

function wrap(ui: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={client}>{ui}</QueryClientProvider>
}

describe('AiUsagePage', () => {
  beforeEach(() => {
    api.getAdminAiUsage.mockReset()
    api.downloadAdminAiUsage.mockReset()
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:x') })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
  })

  it('renders KPI, trends and detail tables', async () => {
    api.getAdminAiUsage.mockResolvedValue(report)
    render(wrap(<AiUsagePage />))

    expect(await screen.findByText('调用次数')).toBeInTheDocument()
    expect(screen.getAllByText('87.5%').length).toBeGreaterThan(0)
    expect(screen.getAllByText('900 ms').length).toBeGreaterThan(0)
    expect(screen.getAllByText('1,200 / 800').length).toBeGreaterThan(0)
    expect(screen.getByText('deepseek-chat')).toBeInTheDocument()
    expect(screen.getAllByText('跨章节检查').length).toBeGreaterThan(0)
    expect(screen.getByText('鉴权失败')).toBeInTheDocument()
    expect(screen.getByText('1 次调用未返回用量')).toBeInTheDocument()
  })

  it('shows the empty state instead of zeroed charts', async () => {
    api.getAdminAiUsage.mockResolvedValue(emptyReport)
    render(wrap(<AiUsagePage />))
    expect(await screen.findByText('还没有模型调用记录')).toBeInTheDocument()
  })

  it('shows the 403 state for non-superusers', async () => {
    api.getAdminAiUsage.mockRejectedValue(new FrontendApiError('forbidden', { kind: 'http', status: 403 }))
    render(wrap(<AiUsagePage />))
    expect(await screen.findByText('需要管理员权限')).toBeInTheDocument()
  })

  it('shows an actionable error and retries', async () => {
    api.getAdminAiUsage.mockRejectedValue(new FrontendApiError('boom', { kind: 'network' }))
    render(wrap(<AiUsagePage />))
    expect(await screen.findByText('无法加载 AI 调用统计')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /重试/ }))
    await waitFor(() => expect(api.getAdminAiUsage).toHaveBeenCalledTimes(2))
  })

  it('exports the current filter as CSV', async () => {
    api.getAdminAiUsage.mockResolvedValue(report)
    api.downloadAdminAiUsage.mockResolvedValue(new Blob(['day,provider']))
    render(wrap(<AiUsagePage />))
    await screen.findByText('调用次数')

    fireEvent.click(screen.getByRole('button', { name: /导出 CSV/ }))
    await waitFor(() => expect(api.downloadAdminAiUsage).toHaveBeenCalledWith('csv', {}))
  })
})
