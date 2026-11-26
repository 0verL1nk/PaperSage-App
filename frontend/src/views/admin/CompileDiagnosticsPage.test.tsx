import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { CompileDiagnosticsPage } from './CompileDiagnosticsPage'
import { FrontendApiError } from '@/services/errors'
import type { CompileDiagnosticDetail, CompileDiagnosticItem, CompileDiagnosticPage } from '@/services/admin-api'

const api = vi.hoisted(() => ({
  listCompileDiagnostics: vi.fn(),
  getCompileDiagnostic: vi.fn(),
}))

vi.mock('@/services/admin-api', () => ({
  listCompileDiagnostics: api.listCompileDiagnostics,
  getCompileDiagnostic: api.getCompileDiagnostic,
}))

// 页面里的 Link 只用于「返回项目列表」入口，独立渲染时不需要真实路由。
vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children }: { to: string; children: ReactNode }) => <a href={to}>{children}</a>,
}))

const item: CompileDiagnosticItem = {
  id: '01a21861-38fc-72c8-bbc1-15f044c684c2',
  created_at: '2026-11-27T01:02:03+08:00',
  user_id: '44444444-4444-4444-4444-444444444444',
  username: 'tester',
  project_id: '55555555-5555-4555-8555-555555555555',
  project_name: '毕业论文',
  is_guest: false,
  client_kind: 'web',
  engine: 'xelatex',
  engine_version: 'papersage-swiftlatex-2026-10-2',
  resource_profile: 'texlive-2026-r80534',
  failure_kind: 'latex',
  failure_stage: 'compile',
  duration_ms: 8421,
  http_status: null,
  error_message: '! Undefined control sequence.',
  failed_resources: [],
}

const detail: CompileDiagnosticDetail = {
  ...item,
  compile_job_id: null,
  log_excerpt: '! Undefined control sequence.\nl.42',
  client_env: { platform: 'Linux', online: true },
}

const page: CompileDiagnosticPage = { items: [item], total: 1, page: 1, page_size: 20 }
const emptyPage: CompileDiagnosticPage = { items: [], total: 0, page: 1, page_size: 20 }

function wrap(ui: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={client}>{ui}</QueryClientProvider>
}

describe('CompileDiagnosticsPage', () => {
  beforeEach(() => {
    api.listCompileDiagnostics.mockReset()
    api.getCompileDiagnostic.mockReset()
  })

  it('shows a loading state before the first page arrives', () => {
    api.listCompileDiagnostics.mockReturnValue(new Promise(() => {}))
    render(wrap(<CompileDiagnosticsPage />))
    expect(screen.getByText('正在加载编译错误日志')).toBeTruthy()
  })

  it('shows the empty state with a hint when no records exist', async () => {
    api.listCompileDiagnostics.mockResolvedValue(emptyPage)
    render(wrap(<CompileDiagnosticsPage />))
    expect(await screen.findByText('暂无编译错误记录')).toBeTruthy()
    expect(screen.getByText(/用户编译失败后，脱敏诊断会自动出现在这里/)).toBeTruthy()
  })

  it('shows an actionable error state with retry', async () => {
    api.listCompileDiagnostics.mockRejectedValue(new FrontendApiError('网络连接失败', { kind: 'network' }))
    render(wrap(<CompileDiagnosticsPage />))
    expect(await screen.findByText('无法加载编译错误日志')).toBeTruthy()
    expect(screen.getByRole('button', { name: /重试/ })).toBeTruthy()
  })

  it('renders a permission notice instead of redirecting on 403', async () => {
    api.listCompileDiagnostics.mockRejectedValue(
      new FrontendApiError('需要管理员权限', { kind: 'backend', status: 403 }),
    )
    render(wrap(<CompileDiagnosticsPage />))
    expect(await screen.findByText('需要管理员权限')).toBeTruthy()
    expect(screen.getByText('返回项目列表').closest('a')?.getAttribute('href')).toBe('/app/projects')
  })

  it('lists records and opens the detail sheet with the redacted log', async () => {
    api.listCompileDiagnostics.mockResolvedValue(page)
    api.getCompileDiagnostic.mockResolvedValue(detail)
    render(wrap(<CompileDiagnosticsPage />))

    // 桌面表格与窄屏卡片都会渲染同一行数据（CSS 控制显隐），故用 getAllByText。
    expect((await screen.findAllByText('! Undefined control sequence.')).length).toBeGreaterThan(0)
    expect(screen.getAllByText('毕业论文').length).toBeGreaterThan(0)
    expect(screen.getAllByText('LaTeX 编译').length).toBeGreaterThan(0)

    fireEvent.click(screen.getAllByText('查看详情')[0])

    await waitFor(() => expect(screen.getByText('编译错误详情')).toBeTruthy())
    expect(await screen.findByText(/l\.42/)).toBeTruthy()
    expect(screen.getByRole('button', { name: /复制 JSON/ })).toBeTruthy()
    expect(screen.getByRole('button', { name: /导出 JSON/ })).toBeTruthy()
    expect(api.getCompileDiagnostic).toHaveBeenCalledWith(item.id)
  })
})
